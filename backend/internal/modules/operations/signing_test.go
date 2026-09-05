package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func signingCall(t *testing.T, mux http.Handler, method, route, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		input = bytes.NewReader(data)
	}
	r := httptest.NewRequest(method, route, input)
	r.Header.Set("X-Kosmos-CSRF", "1")
	if token != "" {
		r.Header.Set("X-Kosmos-Signing-Token", token)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func decodeSigningResponse(t *testing.T, response *httptest.ResponseRecorder, status int) SigningRequest {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status %d, want %d: %s", response.Code, status, response.Body.String())
	}
	var request SigningRequest
	if err := json.Unmarshal(response.Body.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	return request
}

func createSigningFixture(t *testing.T, mux http.Handler) SigningRequest {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("title", "Test agreement"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "agreement.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(signingPDFTestDocument("", "", 1)); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/v1/signing-requests", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	item := decodeSigningResponse(t, w, 201)
	if strings.Contains(w.Body.String(), "originalObject") || strings.Contains(w.Body.String(), "createdBy") {
		t.Fatal("private storage metadata exposed")
	}
	fields := []SigningField{
		{ID: "signature", Type: "signature", Label: "Signature", Page: 1, X: 0.1, Y: 0.2, Width: 0.5, Height: 0.1, Required: true},
		{ID: "date", Type: "date", Label: "Date", Page: 1, X: 0.1, Y: 0.4, Width: 0.5, Height: 0.1, Required: true},
		{ID: "name", Type: "name", Label: "Full name", Page: 1, X: 0.1, Y: 0.6, Width: 0.5, Height: 0.1, Required: true},
	}
	return decodeSigningResponse(t, signingCall(t, mux, "PUT", "/api/v1/signing-requests/"+item.ID, "", map[string]any{"revision": item.Revision, "fields": fields}), 200)
}

func issueSigningFixture(t *testing.T, mux http.Handler, item SigningRequest) (SigningRequest, string) {
	t.Helper()
	w := signingCall(t, mux, "POST", "/api/v1/signing-requests/"+item.ID+"/link", "", map[string]any{"revision": item.Revision, "signerName": "Ada Example", "signerEmail": "ada@example.com", "expiresDays": 7})
	if w.Code != 200 {
		t.Fatalf("issue link: %s", w.Body.String())
	}
	var result struct {
		Request    SigningRequest `json:"request"`
		SigningURL string         `json:"signingUrl"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	prefix := "/sign#" + item.ID + "."
	if !strings.HasPrefix(result.SigningURL, prefix) {
		t.Fatalf("unsafe link URL: %s", result.SigningURL)
	}
	return result.Request, strings.TrimPrefix(result.SigningURL, prefix)
}

func completeSigningBody() map[string]any {
	return map[string]any{"signerName": "Ada Example", "consent": true, "values": map[string]string{"signature": "Ada Example", "date": "1900-01-01", "name": "Forged auto-name"}}
}

func TestSigningLifecycleFreezesPDFAndHidesTokens(t *testing.T) {
	m, mux, _ := newTestModule(t)
	draft := createSigningFixture(t, mux)
	pending, token := issueSigningFixture(t, mux, draft)
	public := "/api/v1/signing/" + draft.ID
	private := "/api/v1/signing-requests/" + draft.ID
	for _, candidate := range []string{"", strings.Repeat("f", 64)} {
		if w := signingCall(t, mux, "GET", public, candidate, nil); w.Code != 404 {
			t.Fatalf("invalid token returned %d", w.Code)
		}
	}
	view := signingCall(t, mux, "GET", public, token, nil)
	decodeSigningResponse(t, view, 200)
	if strings.Contains(view.Body.String(), token) || strings.Contains(view.Body.String(), "tokenHash") {
		t.Fatal("token exposed in request")
	}
	if view.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("public request may be cached")
	}
	if w := signingCall(t, mux, "PUT", private, "", map[string]any{"revision": pending.Revision, "fields": draft.Fields}); w.Code != 409 {
		t.Fatal("issued fields remained editable")
	}
	noConsent := completeSigningBody()
	noConsent["consent"] = false
	if w := signingCall(t, mux, "POST", public+"/complete", token, noConsent); w.Code != 400 {
		t.Fatal("signing without consent accepted")
	}
	missing := completeSigningBody()
	missing["values"] = map[string]string{}
	if w := signingCall(t, mux, "POST", public+"/complete", token, missing); w.Code != 400 {
		t.Fatal("missing signature accepted")
	}
	complete := decodeSigningResponse(t, signingCall(t, mux, "POST", public+"/complete", token, completeSigningBody()), 200)
	if complete.Status != "completed" || complete.CompletedAt == nil || complete.Consent != signingConsent || len(complete.SignedSHA256) != 64 || len(complete.Events) != 3 {
		t.Fatalf("missing completion evidence: %#v", complete)
	}
	for _, route := range []string{public + "/pdf?completed=true", private + "/pdf?completed=true"} {
		w := signingCall(t, mux, "GET", route, token, nil)
		if w.Code != 200 || !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-")) || signingHash(w.Body.Bytes()) != complete.SignedSHA256 {
			t.Fatal("signed PDF does not match recorded hash")
		}
	}
	duplicate := completeSigningBody()
	duplicate["signerName"] = "Different Person"
	again := decodeSigningResponse(t, signingCall(t, mux, "POST", public+"/complete", token, duplicate), 200)
	if again.SignedSHA256 != complete.SignedSHA256 || again.Revision != complete.Revision {
		t.Fatal("replay changed signed document")
	}
	if w := signingCall(t, mux, "POST", private+"/revoke", "", map[string]any{"revision": complete.Revision}); w.Code != 409 {
		t.Fatal("completed request was mutable")
	}
	var stored SigningRequest
	if err := m.store.Get(context.Background(), m.publicScope, "signingRequests", draft.ID, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.TokenHash == token || stored.TokenHash != signingHash([]byte(token)) {
		t.Fatal("token not hashed")
	}
	if len(m.blobs.(*MemoryBlobStore).blobs) != 2 {
		t.Fatal("completion leaked a PDF object")
	}
}

func TestSigningExpiryRevocationAndOrganizationIsolation(t *testing.T) {
	for _, state := range []string{"expired", "revoked", "other organization"} {
		t.Run(state, func(t *testing.T) {
			m, mux, _ := newTestModule(t)
			item, token := issueSigningFixture(t, mux, createSigningFixture(t, mux))
			if state == "revoked" {
				decodeSigningResponse(t, signingCall(t, mux, "POST", "/api/v1/signing-requests/"+item.ID+"/revoke", "", map[string]any{"revision": item.Revision}), 200)
			} else if state == "expired" {
				var stored SigningRequest
				if err := m.store.Get(context.Background(), m.publicScope, "signingRequests", item.ID, &stored); err != nil {
					t.Fatal(err)
				}
				past := time.Now().Add(-time.Hour)
				stored.ExpiresAt = &past
				if err := m.store.Put(context.Background(), m.publicScope, "signingRequests", item.ID, stored); err != nil {
					t.Fatal(err)
				}
			} else {
				m.identity = func(*http.Request) (string, Identity, error) {
					return "other-org", Identity{Email: "other@example.com"}, nil
				}
				if w := signingCall(t, mux, "GET", "/api/v1/signing-requests/"+item.ID, "", nil); w.Code != 404 {
					t.Fatal("cross-org read accepted")
				}
				return
			}
			for _, route := range []string{"", "/pdf"} {
				if w := signingCall(t, mux, "GET", "/api/v1/signing/"+item.ID+route, token, nil); w.Code != 410 {
					t.Fatalf("unavailable request returned %d", w.Code)
				}
			}
			if w := signingCall(t, mux, "POST", "/api/v1/signing/"+item.ID+"/complete", token, completeSigningBody()); w.Code != 410 {
				t.Fatal("signed expired or revoked request")
			}
		})
	}
}

type signingFaultStore struct {
	Store
	signingStore
	afterCommit bool
}

func (s signingFaultStore) ReplaceSigningRequest(ctx context.Context, scope string, revision int, state string, item SigningRequest) error {
	if s.afterCommit {
		if err := s.signingStore.ReplaceSigningRequest(ctx, scope, revision, state, item); err != nil {
			return err
		}
	}
	return errors.New("injected write failure")
}

func TestSigningCompletionHandlesAmbiguousCommit(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			m, mux, _ := newTestModule(t)
			item, token := issueSigningFixture(t, mux, createSigningFixture(t, mux))
			m.store = signingFaultStore{Store: m.store, signingStore: m.store.(signingStore), afterCommit: committed}
			w := signingCall(t, mux, "POST", "/api/v1/signing/"+item.ID+"/complete", token, completeSigningBody())
			if committed {
				completed := decodeSigningResponse(t, w, 200)
				pdf := signingCall(t, mux, "GET", "/api/v1/signing/"+item.ID+"/pdf?completed=true", token, nil)
				if pdf.Code != 200 || signingHash(pdf.Body.Bytes()) != completed.SignedSHA256 {
					t.Fatal("winning PDF deleted after ambiguous error")
				}
			} else if w.Code != 500 || len(m.blobs.(*MemoryBlobStore).blobs) != 1 {
				t.Fatal("failed commit exposed completion or leaked object")
			}
		})
	}
}

func TestSigningTransitionHasOneWinner(t *testing.T) {
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
			scope := fmt.Sprintf("signing-%d", time.Now().UnixNano())
			expires := time.Now().Add(time.Hour)
			item := SigningRequest{ID: "request", Status: "pending", Revision: 2, ExpiresAt: &expires}
			if err := store.Create(ctx, scope, "signingRequests", item.ID, item); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			results := make(chan error, 2)
			for _, state := range []string{"completed", "revoked"} {
				wg.Add(1)
				go func(state string) {
					defer wg.Done()
					next := item
					next.Status = state
					next.Revision = 3
					results <- store.(signingStore).ReplaceSigningRequest(ctx, scope, 2, "pending", next)
				}(state)
			}
			wg.Wait()
			close(results)
			won, conflicted := 0, 0
			for err := range results {
				if err == nil {
					won++
				} else if errors.Is(err, errSigningConflict) {
					conflicted++
				} else {
					t.Fatal(err)
				}
			}
			if won != 1 || conflicted != 1 {
				t.Fatalf("winners %d, conflicts %d", won, conflicted)
			}
		})
	}
}

type signingReadGuard struct {
	Store
	reads int
}

func (s *signingReadGuard) Get(ctx context.Context, scope, collection, id string, target any) error {
	s.reads++
	return s.Store.Get(ctx, scope, collection, id, target)
}

func TestInvalidSigningTokenDoesNotReadDatastore(t *testing.T) {
	m, mux, _ := newTestModule(t)
	guard := &signingReadGuard{Store: m.store}
	m.store = guard
	w := signingCall(t, mux, "GET", "/api/v1/signing/unknown-request", strings.Repeat("a", 43), nil)
	if w.Code != 404 || guard.reads != 0 {
		t.Fatalf("invalid token status %d, reads %d", w.Code, guard.reads)
	}
}

type signingAmbiguousBlob struct{ BlobStore }

func (s signingAmbiguousBlob) Put(ctx context.Context, name, contentType string, source io.Reader) error {
	if err := s.BlobStore.Put(ctx, name, contentType, source); err != nil {
		return err
	}
	return errors.New("timeout after object commit")
}

func TestSigningUploadTimeoutCleansUnreferencedPDF(t *testing.T) {
	m, mux, _ := newTestModule(t)
	item, token := issueSigningFixture(t, mux, createSigningFixture(t, mux))
	blobs := m.blobs.(*MemoryBlobStore)
	m.blobs = signingAmbiguousBlob{BlobStore: blobs}
	w := signingCall(t, mux, "POST", "/api/v1/signing/"+item.ID+"/complete", token, completeSigningBody())
	if w.Code != 500 || len(blobs.blobs) != 1 {
		t.Fatalf("status %d, objects %d", w.Code, len(blobs.blobs))
	}
}
