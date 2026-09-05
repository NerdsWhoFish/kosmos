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

func parallelSigningDraft(t *testing.T, mux http.Handler) SigningRequest {
	t.Helper()
	item := uploadSigningFixture(t, mux, signingPDFTestDocument("", "", 1))
	signers := []SigningSignerInput{{ID: "staff", Name: "Staff Intended", Email: "staff@example.com"}, {ID: "prospect", Name: "Prospect Intended", Email: "prospect@example.com"}}
	var fields []SigningField
	for index, signer := range signers {
		for row, kind := range []string{"signature", "name", "date"} {
			fields = append(fields, SigningField{ID: signer.ID + "-" + kind, SignerID: signer.ID, Type: kind, Label: kind, Page: 1, X: .1 + .45*float64(index), Y: .1 + .2*float64(row), Width: .35, Height: .1, Required: true})
		}
	}
	return decodeSigningResponse(t, signingCall(t, mux, "PUT", "/api/v1/signing-requests/"+item.ID, "", map[string]any{"revision": item.Revision, "fields": fields, "signers": signers}), 200)
}

func parallelSigningFixture(t *testing.T, mux http.Handler) (SigningRequest, map[string]string) {
	t.Helper()
	draft := parallelSigningDraft(t, mux)
	w := signingCall(t, mux, "POST", "/api/v1/signing-requests/"+draft.ID+"/link", "", map[string]any{"revision": draft.Revision, "expiresDays": 7})
	if w.Code != 200 {
		t.Fatalf("parallel link: %d %s", w.Code, w.Body.String())
	}
	var result struct {
		Request SigningRequest `json:"request"`
		Links   []struct {
			SignerID string `json:"signerId"`
			URL      string `json:"signingUrl"`
		} `json:"signingLinks"`
		Legacy string `json:"signingUrl"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Legacy != "" || len(result.Links) != 2 {
		t.Fatal("parallel link response incorrect", err)
	}
	tokens := make(map[string]string)
	for _, link := range result.Links {
		tokens[link.SignerID] = strings.TrimPrefix(link.URL, "/sign#"+draft.ID+".")
	}
	if tokens["staff"] == tokens["prospect"] {
		t.Fatal("participants received the same token")
	}
	return result.Request, tokens
}

func parallelSigningBody(id string) map[string]any {
	return map[string]any{"signerName": id + " Actual", "consent": true, "values": map[string]string{id + "-signature": id + " Signature", id + "-date": "1900-01-01", id + "-name": "Forged auto-name"}}
}

func TestParallelSigningIndependentAccessAndImmutableEvidence(t *testing.T) {
	m, mux, _ := newTestModule(t)
	item, tokens := parallelSigningFixture(t, mux)
	public := "/api/v1/signing/" + item.ID
	for id, token := range tokens {
		view := decodeSigningResponse(t, signingCall(t, mux, "GET", public, token, nil), 200)
		if view.CurrentSignerID != id || len(view.Fields) != 3 || len(view.Signers) != 2 || view.AccessExpiresAt == nil || !view.AccessExpiresAt.Equal(*item.ExpiresAt) {
			t.Fatal("recipient view missing own fields or signing deadline")
		}
		for _, field := range view.Fields {
			if field.SignerID != id {
				t.Fatal("other recipient fields exposed")
			}
		}
		for _, signer := range view.Signers {
			if signer.ID != id && (signer.Email != "" || signer.Session != nil) {
				t.Fatal("other recipient personal metadata exposed")
			}
		}
	}
	forged := parallelSigningBody("staff")
	forged["values"].(map[string]string)["prospect-signature"] = "Forged other signature"
	if w := signingCall(t, mux, "POST", public+"/complete", tokens["staff"], forged); w.Code != 400 {
		t.Fatal("cross-recipient signature accepted")
	}
	partial := decodeSigningResponse(t, signingCall(t, mux, "POST", public+"/complete", tokens["staff"], parallelSigningBody("staff")), 200)
	if partial.Status != "pending" || partial.CompletedAt != nil || partial.Signers[0].SignedAt == nil || partial.Signers[1].SignedAt != nil || partial.AccessExpiresAt == nil || !partial.AccessExpiresAt.Equal(partial.Signers[0].SignedAt.Add(15*time.Minute)) {
		t.Fatal("first signature did not create partial completion and own deadline")
	}
	partialPDF := signingCall(t, mux, "GET", public+"/pdf?completed=true", tokens["staff"], nil)
	if partialPDF.Code != 200 || !strings.Contains(partialPDF.Header().Get("Content-Disposition"), "partially-signed-document.pdf") || signingHash(partialPDF.Body.Bytes()) != partial.SignedSHA256 {
		t.Fatal("partial signed copy unavailable or mislabeled")
	}
	text := strings.Join(signingPDFTestRenderedText(t, partialPDF.Body.Bytes()), " ")
	if !strings.Contains(text, "PARTIALLY SIGNED") || !strings.Contains(text, "staff Signature") || strings.Contains(text, "Forged auto-name") || strings.Contains(text, "1900-01-01") {
		t.Fatal("partial PDF missing signature or trusted automatic values")
	}
	if w := signingCall(t, mux, "GET", public+"/pdf?completed=true", tokens["prospect"], nil); w.Code != 403 {
		t.Fatal("unsigned recipient downloaded signed copy")
	}
	if w := signingCall(t, mux, "POST", "/api/v1/signing-requests/"+item.ID+"/download-link", "", map[string]any{"revision": partial.Revision}); w.Code != 409 {
		t.Fatal("partial request created final download link")
	}
	before := loadStoredSigning(t, m, item.ID)
	retryBody := parallelSigningBody("staff")
	retryBody["signerName"] = "Replacement Identity"
	retry := decodeSigningResponse(t, signingCall(t, mux, "POST", public+"/complete", tokens["staff"], retryBody), 200)
	if retry.Revision != partial.Revision || !reflect.DeepEqual(loadStoredSigning(t, m, item.ID).Signers[0], before.Signers[0]) {
		t.Fatal("retry changed first signature")
	}
	final := decodeSigningResponse(t, signingCall(t, mux, "POST", public+"/complete", tokens["prospect"], parallelSigningBody("prospect")), 200)
	after := loadStoredSigning(t, m, item.ID)
	if final.Status != "completed" || final.CompletedAt == nil || final.Signers[1].SignedAt == nil || !reflect.DeepEqual(after.Signers[0], before.Signers[0]) || final.SignedSHA256 == partial.SignedSHA256 {
		t.Fatal("final completion replaced previous evidence or did not merge")
	}
	for id, token := range tokens {
		view := decodeSigningResponse(t, signingCall(t, mux, "GET", public, token, nil), 200)
		for _, signer := range view.Signers {
			if signer.ID != id && (signer.Session != nil || signer.Email != "" || signer.CompletedSignerName != "" || signer.Consent != "") {
				t.Fatal("other signer session leaked after completion")
			}
		}
		pdf := signingCall(t, mux, "GET", public+"/pdf?completed=true", token, nil)
		if pdf.Code != 200 || signingHash(pdf.Body.Bytes()) != final.SignedSHA256 {
			t.Fatal("recipient cannot download latest final PDF")
		}
	}
	finalPDF := signingCall(t, mux, "GET", public+"/pdf?completed=true", tokens["prospect"], nil)
	text = strings.Join(signingPDFTestRenderedText(t, finalPDF.Body.Bytes()), " ")
	if !strings.Contains(text, "staff Signature") || !strings.Contains(text, "prospect Signature") || strings.Contains(text, "PARTIALLY SIGNED") {
		t.Fatal("final PDF did not merge both signatures")
	}
	download, downloadToken := downloadSigningFixture(t, mux, final, nil)
	view := decodeSigningResponse(t, signingCall(t, mux, "GET", public, downloadToken, nil), 200)
	if view.CurrentSignerID != "" || len(view.Fields) != 0 || view.AccessExpiresAt == nil || !view.AccessExpiresAt.Equal(download.ExpiresAt) {
		t.Fatal("final download view contained writable signer context")
	}
	for _, signer := range view.Signers {
		if signer.Session != nil || signer.Email != "" {
			t.Fatal("download metadata exposed personal participant details")
		}
	}
	if w := signingCall(t, mux, "DELETE", "/api/v1/signing-requests/"+item.ID, "", map[string]any{"revision": download.Request.Revision, "confirmed": true}); w.Code != 204 {
		t.Fatal("completed multi request could not be deleted")
	}
	var cleanup SigningCleanup
	if err := m.store.Get(context.Background(), m.publicScope, "signingCleanup", item.ID, &cleanup); err != nil || len(cleanup.Objects) != 3 {
		t.Fatal("deletion omitted immutable intermediate snapshot", err)
	}
}

func TestParallelSigningGraceNeverExtendsAnotherSigner(t *testing.T) {
	m, mux, _ := newTestModule(t)
	item, tokens := parallelSigningFixture(t, mux)
	public := "/api/v1/signing/" + item.ID
	decodeSigningResponse(t, signingCall(t, mux, "POST", public+"/complete", tokens["staff"], parallelSigningBody("staff")), 200)
	stored := loadStoredSigning(t, m, item.ID)
	old := time.Now().Add(-16 * time.Minute)
	stored.Signers[0].SignedAt = &old
	putStoredSigning(t, m, stored)
	if w := signingCall(t, mux, "GET", public, tokens["staff"], nil); w.Code != 410 {
		t.Fatal("first signer grace did not expire")
	}
	decodeSigningResponse(t, signingCall(t, mux, "GET", public, tokens["prospect"], nil), 200)
	final := decodeSigningResponse(t, signingCall(t, mux, "POST", public+"/complete", tokens["prospect"], parallelSigningBody("prospect")), 200)
	if w := signingCall(t, mux, "GET", public, tokens["staff"], nil); w.Code != 410 {
		t.Fatal("final completion revived earlier signer link")
	}
	stored = loadStoredSigning(t, m, item.ID)
	stored.ExpiresAt = &old
	putStoredSigning(t, m, stored)
	decodeSigningResponse(t, signingCall(t, mux, "GET", public, tokens["prospect"], nil), 200)
	_, downloadToken := downloadSigningFixture(t, mux, final, nil)
	decodeSigningResponse(t, signingCall(t, mux, "GET", public, downloadToken, nil), 200)
}

func TestParallelSigningRejectsInvalidAssignmentsAndParticipants(t *testing.T) {
	_, mux, _ := newTestModule(t)
	draft := parallelSigningDraft(t, mux)
	path := "/api/v1/signing-requests/" + draft.ID
	for _, kind := range []string{"unassigned", "unknown signer", "duplicate field", "missing signature"} {
		fields := append([]SigningField(nil), draft.Fields...)
		switch kind {
		case "unassigned":
			fields[0].SignerID = ""
		case "unknown signer":
			fields[0].SignerID = "outsider"
		case "duplicate field":
			fields[3].ID = fields[0].ID
		case "missing signature":
			fields[3].Required = false
		}
		w := signingCall(t, mux, "PUT", path, "", map[string]any{"revision": draft.Revision, "fields": fields})
		if kind == "missing signature" {
			draft = decodeSigningResponse(t, w, 200)
			w = signingCall(t, mux, "POST", path+"/link", "", map[string]any{"revision": draft.Revision, "expiresDays": 7})
		}
		if w.Code != 400 {
			t.Fatalf("invalid %s accepted: %d", kind, w.Code)
		}
	}
	for _, signers := range [][]SigningSignerInput{{}, {{ID: "same", Name: "A", Email: "a@example.com"}, {ID: "same", Name: "B", Email: "b@example.com"}}, {{ID: "bad|id", Name: "A", Email: "a@example.com"}}, {{ID: "a", Name: "A", Email: "not-email"}}, make([]SigningSignerInput, 11)} {
		if w := signingCall(t, mux, "PUT", path, "", map[string]any{"revision": draft.Revision, "fields": draft.Fields, "signers": signers}); w.Code != 400 {
			t.Fatal("invalid participants accepted")
		}
	}
	if w := signingCall(t, mux, "PUT", path, "", map[string]any{"revision": draft.Revision, "fields": draft.Fields, "signers": []map[string]any{{"id": "staff", "name": "Staff", "email": "staff@example.com", "signedAt": time.Now()}}}); w.Code != 400 {
		t.Fatal("client submitted completion metadata")
	}
}

func TestParallelSigningTokensBindRecipientBeforeDatastore(t *testing.T) {
	m, mux, _ := newTestModule(t)
	guard := &signingReadGuard{Store: m.store}
	m.store = guard
	valid := m.signingRecipientToken(m.publicScope, "request", "staff")
	for _, token := range []string{strings.Replace(valid, "s1_staff_", "s1_prospect_", 1), m.signingRecipientToken("other-org", "request", "staff"), m.signingRecipientToken(m.publicScope, "other-request", "staff"), m.signingRecipientToken(m.publicScope, "request", strings.Repeat("a", 65)), valid[:len(valid)-1] + "!"} {
		if w := signingCall(t, mux, "GET", "/api/v1/signing/request", token, nil); w.Code != 404 || guard.reads != 0 {
			t.Fatal("invalid recipient token reached datastore")
		}
	}
}

type parallelSigningBarrierStore struct {
	Store
	signingStore
	mu      sync.Mutex
	calls   int
	ready   chan struct{}
	release chan struct{}
}

func (s *parallelSigningBarrierStore) ReplaceSigningRequest(ctx context.Context, scope string, revision int, state string, next SigningRequest) error {
	if state == "pending" && len(next.Signers) > 0 {
		s.mu.Lock()
		s.calls++
		call := s.calls
		if call == 2 {
			close(s.ready)
		}
		s.mu.Unlock()
		if call <= 2 {
			<-s.release
		}
	}
	return s.signingStore.ReplaceSigningRequest(ctx, scope, revision, state, next)
}

func parallelSigningStore(t *testing.T, m *Module, kind string) Store {
	t.Helper()
	if kind == "memory" {
		return m.store
	}
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST is not configured")
	}
	client, err := firestore.NewClient(context.Background(), "kosmos-signing-tests")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	scope := fmt.Sprintf("multi-%d", time.Now().UnixNano())
	m.publicScope = scope
	m.identity = func(*http.Request) (string, Identity, error) {
		return scope, Identity{Email: "owner@example.com", Name: "Owner"}, nil
	}
	m.store = NewFirestoreStore(client)
	return m.store
}

func TestParallelSigningConcurrentParticipantsAndDuplicateSubmissions(t *testing.T) {
	for _, kind := range []string{"memory", "firestore"} {
		for _, duplicate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/duplicate=%v", kind, duplicate), func(t *testing.T) {
				m, mux, _ := newTestModule(t)
				base := parallelSigningStore(t, m, kind)
				item, tokens := parallelSigningFixture(t, mux)
				barrier := &parallelSigningBarrierStore{Store: base, signingStore: base.(signingStore), ready: make(chan struct{}), release: make(chan struct{})}
				m.store = barrier
				ids := []string{"staff", "prospect"}
				if duplicate {
					ids[1] = "staff"
				}
				results := make(chan *httptest.ResponseRecorder, 2)
				for index, id := range ids {
					go func(index int, id string) {
						body := parallelSigningBody(id)
						body["signerName"] = fmt.Sprintf("Signer %d", index)
						results <- signingCall(t, mux, "POST", "/api/v1/signing/"+item.ID+"/complete", tokens[id], body)
					}(index, id)
				}
				select {
				case <-barrier.ready:
				case <-time.After(10 * time.Second):
					close(barrier.release)
					t.Fatal("concurrent completion did not reach CAS")
				}
				close(barrier.release)
				first := decodeSigningResponse(t, <-results, 200)
				second := decodeSigningResponse(t, <-results, 200)
				stored := loadStoredSigning(t, m, item.ID)
				if duplicate {
					if stored.Status != "pending" || stored.Signers[1].SignedAt != nil || first.Revision != second.Revision || first.Signers[0].CompletedSignerName != second.Signers[0].CompletedSignerName {
						t.Fatal("duplicate submission changed winner")
					}
				} else {
					if stored.Status != "completed" || stored.Signers[0].SignedAt == nil || stored.Signers[1].SignedAt == nil || stored.Signers[0].SignedObject == stored.Signers[1].SignedObject {
						t.Fatal("concurrent signatures lost evidence")
					}
					pdf := signingCall(t, mux, "GET", "/api/v1/signing/"+item.ID+"/pdf?completed=true", tokens["staff"], nil)
					text := strings.Join(signingPDFTestRenderedText(t, pdf.Body.Bytes()), " ")
					if !strings.Contains(text, "staff Signature") || !strings.Contains(text, "prospect Signature") {
						t.Fatal("concurrent final PDF lost a signature")
					}
				}
				m.store = base
				var cleanups []SigningCleanup
				if err := base.List(context.Background(), m.publicScope, "signingCleanup", &cleanups); err != nil || len(cleanups) != 1 {
					t.Fatal("losing CAS artifact missing durable cleanup", err)
				}
				for _, cleanup := range cleanups {
					if cleanup.RequestID != item.ID || cleanup.NextAttemptAt.Before(time.Now().Add(59*time.Minute)) {
						t.Fatal("orphan cleanup lacks late-commit delay")
					}
					if err := m.runSigningCleanup(context.Background(), Job{Scope: m.publicScope, OutboxID: cleanup.ID}, time.Now().Add(2*time.Hour)); err != nil {
						t.Fatal(err)
					}
				}
				for _, signer := range stored.Signers {
					if signer.SignedAt != nil {
						reader, err := m.blobs.Open(context.Background(), signer.SignedObject)
						if err != nil {
							t.Fatal("winner snapshot removed", err)
						}
						_ = reader.Close()
					}
				}
			})
		}
	}
}

func TestParallelSigningStoreRejectsExpiredSignatureAndEvidenceMutation(t *testing.T) {
	for _, kind := range []string{"memory", "firestore"} {
		t.Run(kind, func(t *testing.T) {
			m, mux, _ := newTestModule(t)
			store := parallelSigningStore(t, m, kind)
			item, tokens := parallelSigningFixture(t, mux)
			decodeSigningResponse(t, signingCall(t, mux, "POST", "/api/v1/signing/"+item.ID+"/complete", tokens["staff"], parallelSigningBody("staff")), 200)
			stored := loadStoredSigning(t, m, item.ID)
			for _, change := range []string{"values", "session", "time", "field"} {
				next := cloneSigningRequest(stored)
				switch change {
				case "values":
					next.Signers[0].Values["staff-signature"] = "Tampered"
				case "session":
					next.Signers[0].Session.IPAddress = "203.0.113.10"
				case "time":
					*next.Signers[0].SignedAt = time.Now()
				case "field":
					next.Fields[0].SignerID = "prospect"
				}
				next = itemWithNextRevision(next)
				if err := store.(signingStore).ReplaceSigningRequest(context.Background(), m.publicScope, stored.Revision, "pending", next); !errors.Is(err, errSigningConflict) {
					t.Fatalf("changed %s evidence accepted: %v", change, err)
				}
			}
			unchanged := loadStoredSigning(t, m, item.ID)
			if !reflect.DeepEqual(unchanged.Signers, stored.Signers) {
				t.Fatal("cloned evidence aliased memory storage")
			}
			past := time.Now().Add(-time.Minute)
			stored.ExpiresAt = &past
			putStoredSigning(t, m, stored)
			next := cloneSigningRequest(stored)
			now := time.Now()
			next.Signers[1].SignedAt = &now
			next = itemWithNextRevision(next)
			if err := store.(signingStore).ReplaceSigningRequest(context.Background(), m.publicScope, stored.Revision, "pending", next); !errors.Is(err, errSigningConflict) {
				t.Fatal("expired partial signature committed")
			}
		})
	}
}
