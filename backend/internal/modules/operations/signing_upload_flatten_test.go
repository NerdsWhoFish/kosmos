package operations

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

type signingUnreadBody struct{ read bool }

func (b *signingUnreadBody) Read([]byte) (int, error) {
	b.read = true
	return 0, io.EOF
}

func (*signingUnreadBody) Close() error { return nil }

func TestSigningBusyRejectsBeforeBufferingUpload(t *testing.T) {
	_, mux, _ := newTestModule(t)
	signingUploadSlot <- struct{}{}
	defer func() { <-signingUploadSlot }()
	body := &signingUnreadBody{}
	r := httptest.NewRequest("POST", "/api/v1/signing-requests", body)
	r.Header.Set("X-Kosmos-CSRF", "1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 429 || w.Header().Get("Retry-After") != "1" || body.read {
		t.Fatalf("busy upload status %d, body read %t", w.Code, body.read)
	}
}

func TestFlattenedSigningUploadRetainsSourceAndSignsPreparedCopy(t *testing.T) {
	m, mux, _ := newTestModule(t)
	source := signingPDFTestDocument("", "/OpenAction << /S /JavaScript /JS (unsafe) >>", 1)
	item := uploadSigningFixture(t, mux, source)
	if !item.Flattened || item.UploadedSHA256 != signingHash(source) || item.OriginalSHA256 == item.UploadedSHA256 {
		t.Fatal("normalization did not record distinct uploaded and prepared hashes")
	}
	private := "/api/v1/signing-requests/" + item.ID
	uploaded := signingCall(t, mux, "GET", private+"/pdf?uploaded=true", "", nil)
	if uploaded.Code != 200 || !bytes.Equal(uploaded.Body.Bytes(), source) {
		t.Fatal("uploaded bytes were not retained unchanged")
	}
	prepared := signingCall(t, mux, "GET", private+"/pdf", "", nil)
	if prepared.Code != 200 || signingHash(prepared.Body.Bytes()) != item.OriginalSHA256 {
		t.Fatal("review copy does not match its hash")
	}
	if _, err := inspectSigningPDF(prepared.Body.Bytes()); err != nil {
		t.Fatalf("prepared PDF retains unsupported content: %v", err)
	}
	fields := []SigningField{{ID: "signature", Type: "signature", Label: "Signature", Page: 1, X: .1, Y: .6, Width: .5, Height: .1, Required: true}}
	item = decodeSigningResponse(t, signingCall(t, mux, "PUT", private, "", map[string]any{"revision": item.Revision, "fields": fields}), 200)
	item, token := issueSigningFixture(t, mux, item)
	public := "/api/v1/signing/" + item.ID
	if w := signingCall(t, mux, "GET", public+"/pdf?uploaded=true", token, nil); w.Code != 400 || bytes.Equal(w.Body.Bytes(), source) {
		t.Fatal("public signer could access active uploaded source")
	}
	if w := signingCall(t, mux, "GET", private+"/pdf?uploaded=true&completed=true", "", nil); w.Code != 400 {
		t.Fatal("ambiguous PDF version accepted")
	}
	completed := decodeSigningResponse(t, signingCall(t, mux, "POST", public+"/complete", token, map[string]any{"signerName": "Ada Example", "consent": true, "values": map[string]string{"signature": "Ada Example"}}), 200)
	if !completed.Flattened || completed.UploadedSHA256 != item.UploadedSHA256 || completed.OriginalSHA256 != item.OriginalSHA256 || completed.SignedSHA256 == "" {
		t.Fatal("completion lost the uploaded-to-reviewed provenance")
	}
	if len(m.blobs.(*MemoryBlobStore).blobs) != 3 {
		t.Fatal("expected uploaded, prepared and signed PDFs")
	}
}

func TestSigningCertificateIdentifiesUploadedAndReviewedDocuments(t *testing.T) {
	certificate := signingPDFTestCertificate()
	certificate.UploadedSHA256 = strings.Repeat("a", 64)
	content, err := signingCertificateContent(certificate)
	if err != nil {
		t.Fatal(err)
	}
	for _, hash := range []string{certificate.OriginalSHA256, certificate.UploadedSHA256} {
		if !bytes.Contains(content, []byte(hex.EncodeToString([]byte(hash)))) {
			t.Fatal("signing record omitted a document hash")
		}
	}
}

type signingUploadCommitStore struct {
	Store
	commit bool
}

func (s signingUploadCommitStore) Create(ctx context.Context, scope, collection, id string, value any) error {
	if collection != "signingRequests" {
		return s.Store.Create(ctx, scope, collection, id, value)
	}
	if s.commit {
		if err := s.Store.Create(ctx, scope, collection, id, value); err != nil {
			return err
		}
	}
	return errors.New("upload metadata unavailable")
}

func TestFlattenedUploadCleanupPreservesCommittedSource(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "before commit", true: "after commit"}[commit], func(t *testing.T) {
			m, mux, _ := newTestModule(t)
			m.store = signingUploadCommitStore{Store: m.store, commit: commit}
			source := signingPDFTestDocument("", "/OpenAction << /S /JavaScript /JS (unsafe) >>", 1)
			response := signingUploadCall(t, mux, source)
			if response.Code != 500 {
				t.Fatalf("status %d, want failed metadata response: %s", response.Code, response.Body)
			}
			want := 0
			if commit {
				want = 2
			}
			if got := len(m.blobs.(*MemoryBlobStore).blobs); got != want {
				t.Fatalf("retained %d objects, want %d", got, want)
			}
		})
	}
}
