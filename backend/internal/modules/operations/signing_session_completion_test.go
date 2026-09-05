package operations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func signingSessionCompletion(t *testing.T, mux http.Handler, id, token string, envelope *signingSessionEnvelope) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(completeSigningBody())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/v1/signing/"+id+"/complete", bytes.NewReader(body))
	r.Header.Set("X-Kosmos-CSRF", "1")
	r.Header.Set("X-Kosmos-Signing-Token", token)
	r.Header.Set("User-Agent", "Forged browser header")
	r.Header.Set("X-Forwarded-For", "198.51.100.200")
	if envelope != nil {
		payload, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		encoded := base64.RawURLEncoding.EncodeToString(payload)
		mac := hmac.New(sha256.New, signingSessionTestKey)
		_, _ = mac.Write([]byte("kosmos-signing-evidence-v1\nPOST\n" + r.URL.Path + "\n" + encoded))
		r.Header.Set("X-Kosmos-Signer-Evidence", encoded)
		r.Header.Set("X-Kosmos-Signer-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestSigningCompletionPreservesAuthenticatedSession(t *testing.T) {
	m, mux, _ := newTestModule(t)
	item, token := issueSigningFixture(t, mux, createSigningFixture(t, mux))
	m.intakeSigningKey, m.verifySignedIntake = signingSessionTestKey, true
	missing := signingSessionCompletion(t, mux, item.ID, token, nil)
	if missing.Code != 403 || !strings.Contains(missing.Body.String(), "signing_session_unverified") {
		t.Fatalf("unverified completion returned %d: %s", missing.Code, missing.Body)
	}
	var stored SigningRequest
	if err := m.store.Get(context.Background(), m.publicScope, "signingRequests", item.ID, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Session != nil || stored.Status != "pending" || len(m.blobs.(*MemoryBlobStore).blobs) != 1 {
		t.Fatal("rejected completion persisted evidence or a signed PDF")
	}
	envelope := signingSessionTestEnvelope()
	envelope.Timestamp = time.Now().Unix()
	completed := decodeSigningResponse(t, signingSessionCompletion(t, mux, item.ID, token, &envelope), 200)
	want := SigningSession{IPAddress: envelope.IPAddress, UserAgent: envelope.UserAgent, City: envelope.City, Region: envelope.Region, Country: envelope.Country, CapturedAt: time.Unix(envelope.Timestamp, 0).UTC(), Source: "cloudflare"}
	if completed.Session == nil || *completed.Session != want {
		t.Fatalf("completion did not preserve authenticated session: %#v", completed.Session)
	}
	envelope.IPAddress, envelope.UserAgent, envelope.City = "192.0.2.99", "Different Browser", "Different City"
	for _, retry := range []*signingSessionEnvelope{&envelope, nil} {
		again := decodeSigningResponse(t, signingSessionCompletion(t, mux, item.ID, token, retry), 200)
		if !reflect.DeepEqual(again, completed) {
			t.Fatal("completion retry changed the original session or signed PDF")
		}
	}
	if err := m.store.Get(context.Background(), m.publicScope, "signingRequests", item.ID, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Session == nil || *stored.Session != want || stored.SignedSHA256 != completed.SignedSHA256 {
		t.Fatal("saved evidence does not match completed response")
	}
	cloned := cloneSigningRequest(stored)
	cloned.Session.IPAddress = "192.0.2.50"
	if *stored.Session != want {
		t.Fatal("cloned request aliases stored session evidence")
	}
}
