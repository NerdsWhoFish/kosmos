package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

type signingDownloadResponse struct {
	Request     SigningRequest `json:"request"`
	DownloadURL string         `json:"downloadUrl"`
	ExpiresAt   time.Time      `json:"expiresAt"`
}

func completedSigningFixture(t *testing.T, mux http.Handler) (SigningRequest, string) {
	t.Helper()
	item, token := issueSigningFixture(t, mux, createSigningFixture(t, mux))
	return decodeSigningResponse(t, signingCall(t, mux, "POST", "/api/v1/signing/"+item.ID+"/complete", token, completeSigningBody()), 200), token
}

func downloadSigningFixture(t *testing.T, mux http.Handler, item SigningRequest, minutes *int) (signingDownloadResponse, string) {
	t.Helper()
	body := map[string]any{"revision": item.Revision}
	if minutes != nil {
		body["expiresMinutes"] = *minutes
	}
	w := signingCall(t, mux, "POST", "/api/v1/signing-requests/"+item.ID+"/download-link", "", body)
	if w.Code != 200 {
		t.Fatalf("download link: %d %s", w.Code, w.Body.String())
	}
	var result signingDownloadResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	prefix := "/sign#" + item.ID + "."
	if !strings.HasPrefix(result.DownloadURL, prefix) {
		t.Fatal("download link did not use fragment transport")
	}
	return result, strings.TrimPrefix(result.DownloadURL, prefix)
}

func loadStoredSigning(t *testing.T, m *Module, id string) SigningRequest {
	t.Helper()
	var item SigningRequest
	if err := m.store.Get(context.Background(), m.publicScope, "signingRequests", id, &item); err != nil {
		t.Fatal(err)
	}
	return item
}

func putStoredSigning(t *testing.T, m *Module, item SigningRequest) {
	t.Helper()
	if err := m.store.Put(context.Background(), m.publicScope, "signingRequests", item.ID, item); err != nil {
		t.Fatal(err)
	}
}

func TestSigningPostCompletionDeadlineOverridesSigningDeadline(t *testing.T) {
	m, mux, _ := newTestModule(t)
	completed, token := completedSigningFixture(t, mux)
	deadline := completed.CompletedAt.Add(15 * time.Minute)
	if completed.PostSignExpiresAt == nil || !completed.PostSignExpiresAt.Equal(deadline) || completed.AccessExpiresAt == nil || !completed.AccessExpiresAt.Equal(deadline) {
		t.Fatal("completion response omitted post-signing access deadline")
	}
	stored := loadStoredSigning(t, m, completed.ID)
	past := time.Now().Add(-time.Second)
	stored.ExpiresAt = &past
	stored.PostSignExpiresAt = nil
	putStoredSigning(t, m, stored)
	public := "/api/v1/signing/" + stored.ID
	view := decodeSigningResponse(t, signingCall(t, mux, "GET", public, token, nil), 200)
	if view.AccessExpiresAt == nil || !view.AccessExpiresAt.Equal(deadline) || view.PostSignExpiresAt == nil || !view.PostSignExpiresAt.Equal(deadline) {
		t.Fatal("legacy completion did not derive its access deadline")
	}
	if w := signingCall(t, mux, "GET", public+"/pdf?completed=true", token, nil); w.Code != 200 {
		t.Fatalf("signing just before deadline lost completed download: %d", w.Code)
	}
	retry := decodeSigningResponse(t, signingCall(t, mux, "POST", public+"/complete", token, completeSigningBody()), 200)
	if retry.AccessExpiresAt == nil || !retry.AccessExpiresAt.Equal(deadline) || retry.Revision != completed.Revision {
		t.Fatal("completion retry changed deadline or revision")
	}
	for _, malformed := range []bool{false, true} {
		old := time.Now().Add(-16 * time.Minute)
		future := time.Now().Add(24 * time.Hour)
		stored.CompletedAt, stored.ExpiresAt, stored.PostSignExpiresAt = &old, &future, &future
		if malformed {
			stored.CompletedAt = nil
		}
		putStoredSigning(t, m, stored)
		for _, suffix := range []string{"", "/pdf?completed=true"} {
			if w := signingCall(t, mux, "GET", public+suffix, token, nil); w.Code != 410 {
				t.Fatalf("expired or malformed completed link accessible: %d", w.Code)
			}
		}
		if w := signingCall(t, mux, "POST", public+"/complete", token, completeSigningBody()); w.Code != 410 {
			t.Fatalf("expired completion retry revived access: %d", w.Code)
		}
	}
}

func TestSigningDownloadLinkRotatesWithoutChangingEvidence(t *testing.T) {
	m, mux, _ := newTestModule(t)
	completed, signingToken := completedSigningFixture(t, mux)
	stored := loadStoredSigning(t, m, completed.ID)
	old := time.Now().Add(-time.Hour)
	stored.CompletedAt, stored.ExpiresAt = &old, &old
	putStoredSigning(t, m, stored)
	before := loadStoredSigning(t, m, completed.ID)
	first, firstToken := downloadSigningFixture(t, mux, completed, nil)
	if len(firstToken) != 69 || !strings.HasPrefix(firstToken, "d1_") || time.Until(first.ExpiresAt) < 59*time.Minute || time.Until(first.ExpiresAt) > time.Hour {
		t.Fatal("invalid download token or default deadline")
	}
	public := "/api/v1/signing/" + completed.ID
	view := decodeSigningResponse(t, signingCall(t, mux, "GET", public, firstToken, nil), 200)
	if view.AccessExpiresAt == nil || !view.AccessExpiresAt.Equal(first.ExpiresAt) || view.DownloadExpiresAt == nil || !view.DownloadExpiresAt.Equal(first.ExpiresAt) {
		t.Fatal("download token deadline not reflected in response")
	}
	if first.Request.AccessExpiresAt != nil {
		t.Fatal("authenticated response retained public token access deadline")
	}
	if w := signingCall(t, mux, "GET", public+"/pdf?completed=true", firstToken, nil); w.Code != 200 || signingHash(w.Body.Bytes()) != completed.SignedSHA256 {
		t.Fatal("download link cannot retrieve immutable completed PDF")
	}
	for _, suffix := range []string{"/pdf", "/pdf?completed=false", "/pdf?uploaded=true", "/pdf?uploaded=true&completed=true"} {
		if w := signingCall(t, mux, "GET", public+suffix, firstToken, nil); w.Code != 400 {
			t.Fatalf("download-only source access returned %d", w.Code)
		}
	}
	if w := signingCall(t, mux, "POST", public+"/complete", firstToken, completeSigningBody()); w.Code != 403 {
		t.Fatal("download token permitted completion POST")
	}
	minutes := 10080
	second, secondToken := downloadSigningFixture(t, mux, first.Request, &minutes)
	if firstToken == secondToken || time.Until(second.ExpiresAt) < 7*24*time.Hour-time.Minute {
		t.Fatal("rotation did not create independent token and selected deadline")
	}
	if w := signingCall(t, mux, "GET", public, firstToken, nil); w.Code != 404 {
		t.Fatal("rotated download token still valid")
	}
	if w := signingCall(t, mux, "GET", public, signingToken, nil); w.Code != 410 {
		t.Fatal("download rotation revived expired original signing token")
	}
	if w := signingCall(t, mux, "POST", "/api/v1/signing-requests/"+completed.ID+"/download-link", "", map[string]any{"revision": first.Request.Revision}); w.Code != 409 {
		t.Fatal("stale mint request rotated the winning token")
	}
	after := loadStoredSigning(t, m, completed.ID)
	if after.TokenHash != before.TokenHash || after.SignedObject != before.SignedObject || after.SignedSHA256 != before.SignedSHA256 || !reflect.DeepEqual(after.Session, before.Session) || !reflect.DeepEqual(after.Events, before.Events) || !after.CompletedAt.Equal(*before.CompletedAt) || after.DownloadTokenHash != signingHash([]byte(secondToken)) {
		t.Fatal("download rotation changed signed evidence or original token")
	}
	encoded, err := json.Marshal(second.Request)
	if err != nil || strings.Contains(string(encoded), "TokenHash") || strings.Contains(string(encoded), "tokenHash") || strings.Contains(string(encoded), secondToken) || strings.Contains(string(encoded), "Object") {
		t.Fatal("private token or object details escaped into response")
	}
	after.DownloadExpiresAt = &old
	putStoredSigning(t, m, after)
	if w := signingCall(t, mux, "GET", public, secondToken, nil); w.Code != 410 {
		t.Fatal("expired download token accepted")
	}
}

func TestSigningDownloadLinkValidationAndPermissions(t *testing.T) {
	for _, state := range []string{"draft", "pending", "revoked"} {
		t.Run(state, func(t *testing.T) {
			_, mux, _ := newTestModule(t)
			item := createSigningFixture(t, mux)
			if state == "pending" {
				item, _ = issueSigningFixture(t, mux, item)
			} else if state == "revoked" {
				item = decodeSigningResponse(t, signingCall(t, mux, "POST", "/api/v1/signing-requests/"+item.ID+"/revoke", "", map[string]any{"revision": item.Revision}), 200)
			}
			if w := signingCall(t, mux, "POST", "/api/v1/signing-requests/"+item.ID+"/download-link", "", map[string]any{"revision": item.Revision}); w.Code != 409 {
				t.Fatalf("created download link for %s: %d", state, w.Code)
			}
		})
	}
	m, mux, _ := newTestModule(t)
	item, _ := completedSigningFixture(t, mux)
	path := "/api/v1/signing-requests/" + item.ID
	for _, minutes := range []int{-1, 0, 10081} {
		if w := signingCall(t, mux, "POST", path+"/download-link", "", map[string]any{"revision": item.Revision, "expiresMinutes": minutes}); w.Code != 400 {
			t.Fatalf("invalid expiry %d returned %d", minutes, w.Code)
		}
	}
	for _, role := range []string{"viewer", "unauthenticated", "other organization"} {
		t.Run(role, func(t *testing.T) {
			previous := m.identity
			t.Cleanup(func() { m.identity = previous })
			status := 403
			if role == "viewer" {
				viewer := Member{ID: memberID("viewer@example.com"), Email: "viewer@example.com", Name: "Viewer", Role: "viewer", Status: "active"}
				if err := m.store.Put(context.Background(), m.publicScope, "members", viewer.ID, viewer); err != nil {
					t.Fatal(err)
				}
				m.identity = func(*http.Request) (string, Identity, error) {
					return m.publicScope, Identity{Email: viewer.Email, Name: viewer.Name}, nil
				}
			} else if role == "unauthenticated" {
				m.identity = func(*http.Request) (string, Identity, error) { return "", Identity{}, errors.New("missing session") }
				status = 401
			} else {
				m.identity = func(*http.Request) (string, Identity, error) {
					return "other-organization", Identity{Email: "owner@other.example"}, nil
				}
				status = 404
			}
			for _, action := range []struct{ method, suffix string }{{"POST", "/download-link"}, {"DELETE", ""}} {
				if w := signingCall(t, mux, action.method, path+action.suffix, "", map[string]any{"revision": item.Revision, "confirmed": true}); w.Code != status {
					t.Fatalf("%s %s permission returned %d: %s", role, action.method, w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestSigningDownloadTokensRejectTamperingBeforeDatastore(t *testing.T) {
	m, mux, _ := newTestModule(t)
	guard := &signingReadGuard{Store: m.store}
	m.store = guard
	nonce := strings.Repeat("A", 22)
	valid := m.signingDownloadToken(m.publicScope, "missing", nonce)
	tokens := []string{strings.Repeat("a", 69), valid[:len(valid)-1] + "!", m.signingDownloadToken("other-org", "missing", nonce), m.signingDownloadToken(m.publicScope, "other-id", nonce), m.signingDownloadToken(m.publicScope, "missing", strings.Repeat("A", 21)+"B"), "d2_" + valid[3:], valid + "extra"}
	for _, token := range tokens {
		if w := signingCall(t, mux, "GET", "/api/v1/signing/missing", token, nil); w.Code != 404 || guard.reads != 0 {
			t.Fatalf("invalid token caused datastore lookup: status%d reads%d", w.Code, guard.reads)
		}
	}
	r := httptest.NewRequest("GET", "/api/v1/signing/missing", nil)
	r.Header.Add("X-Kosmos-Signing-Token", valid)
	r.Header.Add("X-Kosmos-Signing-Token", valid)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 404 || guard.reads != 0 {
		t.Fatal("duplicate token header caused datastore lookup")
	}
	if w := signingCall(t, mux, "POST", "/api/v1/signing/missing/complete", valid, completeSigningBody()); w.Code != 403 || guard.reads != 0 {
		t.Fatal("download token mutation reached datastore")
	}
}

func TestSigningDeleteRemovesLiveAccessAndQueuesPrivateObjects(t *testing.T) {
	for _, state := range []string{"draft", "completed", "revoked"} {
		t.Run(state, func(t *testing.T) {
			m, mux, _ := newTestModule(t)
			var item SigningRequest
			var token string
			if state == "completed" {
				item, token = completedSigningFixture(t, mux)
			} else {
				item = createSigningFixture(t, mux)
				if state == "revoked" {
					item, token = issueSigningFixture(t, mux, item)
					item = decodeSigningResponse(t, signingCall(t, mux, "POST", "/api/v1/signing-requests/"+item.ID+"/revoke", "", map[string]any{"revision": item.Revision}), 200)
				}
			}
			stored := loadStoredSigning(t, m, item.ID)
			stored.UploadedObject = m.publicScope + "/signing/" + item.ID + "/uploaded.pdf"
			putStoredSigning(t, m, stored)
			path := "/api/v1/signing-requests/" + item.ID
			for _, body := range []map[string]any{{"revision": item.Revision}, {"revision": 0, "confirmed": true}} {
				if w := signingCall(t, mux, "DELETE", path, "", body); w.Code != 400 {
					t.Fatal("deletion lacked explicit confirmation or revision")
				}
			}
			if w := signingCall(t, mux, "DELETE", path, "", map[string]any{"revision": item.Revision - 1, "confirmed": true}); w.Code != 409 {
				t.Fatal("stale revision deleted request")
			}
			count := len(m.blobs.(*MemoryBlobStore).blobs)
			if w := signingCall(t, mux, "DELETE", path, "", map[string]any{"revision": item.Revision, "confirmed": true}); w.Code != 204 || w.Body.Len() != 0 {
				t.Fatalf("delete failed: %d %s", w.Code, w.Body.String())
			}
			for _, route := range []string{path, path + "/pdf", path + "/pdf?completed=true", "/api/v1/signing/" + item.ID, "/api/v1/signing/" + item.ID + "/pdf?completed=true"} {
				if w := signingCall(t, mux, "GET", route, token, nil); w.Code != 404 {
					t.Fatalf("deleted request accessible at %s: %d", route, w.Code)
				}
			}
			if w := signingCall(t, mux, "GET", "/api/v1/signing-requests", "", nil); w.Code != 200 || strings.Contains(w.Body.String(), item.ID) {
				t.Fatal("deleted request remains listed")
			}
			var cleanup SigningCleanup
			if err := m.store.Get(context.Background(), m.publicScope, "signingCleanup", item.ID, &cleanup); err != nil {
				t.Fatal("deletion lost cleanup outbox", err)
			}
			want := signingCleanupFor(stored, cleanup.CreatedAt)
			if !reflect.DeepEqual(cleanup, want) || len(cleanup.Objects) < 2 || cleanup.CreatedAt.IsZero() || len(m.blobs.(*MemoryBlobStore).blobs) != count {
				t.Fatal("outbox missing source objects or deletion bypassed retention cleanup")
			}
			encoded, err := json.Marshal(cleanup)
			if err != nil || strings.Contains(string(encoded), stored.Title) || strings.Contains(string(encoded), stored.SignerEmail) && stored.SignerEmail != "" || strings.Contains(string(encoded), "token") || strings.Contains(string(encoded), "session") {
				t.Fatal("cleanup outbox retained business record details")
			}
		})
	}
}

func TestSigningPendingDeletionRequiresRevocation(t *testing.T) {
	m, mux, _ := newTestModule(t)
	item, token := issueSigningFixture(t, mux, createSigningFixture(t, mux))
	if w := signingCall(t, mux, "DELETE", "/api/v1/signing-requests/"+item.ID, "", map[string]any{"revision": item.Revision, "confirmed": true}); w.Code != 409 {
		t.Fatal("active signing request deleted without revocation")
	}
	decodeSigningResponse(t, signingCall(t, mux, "GET", "/api/v1/signing/"+item.ID, token, nil), 200)
	var cleanup SigningCleanup
	if err := m.store.Get(context.Background(), m.publicScope, "signingCleanup", item.ID, &cleanup); !errors.Is(err, errNotFound) {
		t.Fatal("rejected deletion queued active document cleanup")
	}
}

func TestSigningLifecycleStoreTransactions(t *testing.T) {
	factories := map[string]func(*testing.T) Store{
		"memory": func(*testing.T) Store { return NewMemoryStore() },
		"firestore": func(t *testing.T) Store {
			if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
				t.Skip("FIRESTORE_EMULATOR_HOST is not configured")
			}
			client, err := firestore.NewClient(context.Background(), "kosmos-signing-tests")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			return NewFirestoreStore(client)
		},
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			store := factory(t)
			ctx := context.Background()
			scope := fmt.Sprintf("lifecycle-%d", time.Now().UnixNano())
			old := time.Now().Add(-time.Hour)
			item := SigningRequest{ID: "request", Status: "completed", Revision: 4, CompletedAt: &old, ExpiresAt: &old, OriginalObject: "original.pdf", UploadedObject: "original.pdf", SignedObject: "signed.pdf"}
			if err := store.Create(ctx, scope, "signingRequests", item.ID, item); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			results := make(chan error, 2)
			wg.Add(2)
			go func() {
				defer wg.Done()
				next := itemWithNextRevision(item)
				next.DownloadTokenHash = "new-download-token-hash"
				results <- store.(signingStore).ReplaceSigningRequest(ctx, scope, item.Revision, "completed", next)
			}()
			go func() {
				defer wg.Done()
				results <- store.(signingStore).DeleteSigningRequest(ctx, scope, item.ID, item.Revision)
			}()
			wg.Wait()
			close(results)
			won, lost := 0, 0
			for err := range results {
				if err == nil {
					won++
				} else if errors.Is(err, errSigningConflict) || errors.Is(err, errNotFound) {
					lost++
				} else {
					t.Fatal(err)
				}
			}
			if won != 1 || lost != 1 {
				t.Fatalf("rotation/delete race: winners%d losers%d", won, lost)
			}
			var current SigningRequest
			if err := store.Get(ctx, scope, "signingRequests", item.ID, &current); err == nil {
				if current.Revision != 5 || current.DownloadTokenHash != "new-download-token-hash" {
					t.Fatal("rotation winner not saved")
				}
				if err := store.(signingStore).DeleteSigningRequest(ctx, scope, item.ID, current.Revision); err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, errNotFound) {
				t.Fatal(err)
			}
			var cleanup SigningCleanup
			if err := store.Get(ctx, scope, "signingCleanup", item.ID, &cleanup); err != nil || !reflect.DeepEqual(cleanup.Objects, []string{"original.pdf", "signed.pdf"}) {
				t.Fatal("atomic deletion missing deduplicated cleanup keys", err)
			}
			if err := store.(signingStore).DeleteSigningRequest(ctx, scope, item.ID, 5); !errors.Is(err, errNotFound) {
				t.Fatal("replayed deletion recreated cleanup record")
			}
			item.ID = "cleanup-conflict"
			if err := store.Create(ctx, scope, "signingRequests", item.ID, item); err != nil {
				t.Fatal(err)
			}
			if err := store.Create(ctx, scope, "signingCleanup", item.ID, signingCleanupFor(item, time.Now().UTC())); err != nil {
				t.Fatal(err)
			}
			if err := store.(signingStore).DeleteSigningRequest(ctx, scope, item.ID, item.Revision); err == nil {
				t.Fatal("conflicting outbox record overwritten")
			}
			if err := store.Get(ctx, scope, "signingRequests", item.ID, &current); err != nil {
				t.Fatal("failed outbox transaction removed live record")
			}
		})
	}
}
