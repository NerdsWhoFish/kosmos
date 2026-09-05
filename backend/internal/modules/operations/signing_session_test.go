package operations

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var signingSessionTestKey = []byte("0123456789abcdef0123456789abcdef")
var signingSessionTestTime = time.Unix(1788600000, 0).UTC()

func signingSessionTestEnvelope() signingSessionEnvelope {
	return signingSessionEnvelope{Version: 1, IPAddress: "2001:db8::1", UserAgent: "Browser/1.0 (Test OS)", City: "São Paulo", Region: "Île-de-France", Country: "BR", Timestamp: signingSessionTestTime.Unix()}
}

func signingSessionTestRequest(t *testing.T, envelope signingSessionEnvelope) *http.Request {
	t.Helper()
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return signingSessionTestPayload(payload)
}

func signingSessionTestPayload(payload []byte) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "https://kosmos.example/api/v1/signing/test_request-1/complete?token=not-collected", nil)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	r.Header.Set("X-Kosmos-Signer-Evidence", encoded)
	mac := hmac.New(sha256.New, signingSessionTestKey)
	_, _ = mac.Write([]byte("kosmos-signing-evidence-v1\nPOST\n" + r.URL.Path + "\n" + encoded))
	r.Header.Set("X-Kosmos-Signer-Signature", hex.EncodeToString(mac.Sum(nil)))
	return r
}

func TestCaptureSigningSessionVerifiedEdge(t *testing.T) {
	for _, address := range []string{"192.0.2.1", "2001:db8::1"} {
		envelope := signingSessionTestEnvelope()
		envelope.IPAddress = address
		r := signingSessionTestRequest(t, envelope)
		r.Header.Set("CF-Connecting-IP", "198.51.100.1")
		r.Header.Set("X-Forwarded-For", "198.51.100.2")
		r.Header.Set("CF-IPCity", "Forged city")
		r.Header.Set("User-Agent", "Changed after edge")
		session, err := captureSigningSession(r, signingSessionTestKey, true, signingSessionTestTime.Add(20*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if session.IPAddress != address || session.UserAgent != envelope.UserAgent || session.City != envelope.City || session.Region != envelope.Region || session.Country != "BR" || session.Source != "cloudflare" || !session.CapturedAt.Equal(signingSessionTestTime) {
			t.Fatalf("verified evidence changed: %#v", session)
		}
	}
	envelope := signingSessionTestEnvelope()
	envelope.City, envelope.Region, envelope.Country, envelope.UserAgent = "", "", "", ""
	if _, err := captureSigningSession(signingSessionTestRequest(t, envelope), signingSessionTestKey, true, signingSessionTestTime); err != nil {
		t.Fatalf("unavailable metadata rejected: %v", err)
	}
}

func TestCaptureSigningSessionWorkerProtocolFixture(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "https://kosmos.example/api/v1/signing/test_request-1/complete", nil)
	r.Header.Set("X-Kosmos-Signer-Evidence", "eyJ2ZXJzaW9uIjoxLCJpcEFkZHJlc3MiOiIyMDAxOmRiODo6MSIsInVzZXJBZ2VudCI6IkJyb3dzZXIvMS4wIChUZXN0IE9TKSIsImNpdHkiOiJTw6NvIFBhdWxvIiwicmVnaW9uIjoiw45sZS1kZS1GcmFuY2UiLCJjb3VudHJ5IjoiQlIiLCJ0aW1lc3RhbXAiOjE3ODg2MDAwMDB9")
	r.Header.Set("X-Kosmos-Signer-Signature", "89a19a6450ec1f9107839b14c7c464f8535837675af125232a958b8b66e8a45d")
	session, err := captureSigningSession(r, signingSessionTestKey, true, signingSessionTestTime)
	if err != nil || session.IPAddress != "2001:db8::1" || session.City != "São Paulo" || session.Region != "Île-de-France" {
		t.Fatalf("Worker protocol fixture rejected: %#v, %v", session, err)
	}
}

func TestCaptureSigningSessionRejectsUntrustedEnvelope(t *testing.T) {
	for name, mutate := range map[string]func(*http.Request){
		"missing evidence":  func(r *http.Request) { r.Header.Del("X-Kosmos-Signer-Evidence") },
		"missing signature": func(r *http.Request) { r.Header.Del("X-Kosmos-Signer-Signature") },
		"forged signature":  func(r *http.Request) { r.Header.Set("X-Kosmos-Signer-Signature", strings.Repeat("0", 64)) },
		"invalid signature": func(r *http.Request) { r.Header.Set("X-Kosmos-Signer-Signature", strings.Repeat("z", 64)) },
		"short signature":   func(r *http.Request) { r.Header.Set("X-Kosmos-Signer-Signature", "ab") },
		"duplicate evidence": func(r *http.Request) {
			r.Header.Add("X-Kosmos-Signer-Evidence", r.Header.Get("X-Kosmos-Signer-Evidence"))
		},
		"duplicate signature": func(r *http.Request) {
			r.Header.Add("X-Kosmos-Signer-Signature", r.Header.Get("X-Kosmos-Signer-Signature"))
		},
		"mixed case duplicate": func(r *http.Request) {
			r.Header["x-kosmos-signer-evidence"] = []string{r.Header.Get("X-Kosmos-Signer-Evidence")}
		},
		"comma joined signature": func(r *http.Request) {
			r.Header.Set("X-Kosmos-Signer-Signature", r.Header.Get("X-Kosmos-Signer-Signature")+", forged")
		},
		"oversized payload": func(r *http.Request) { r.Header.Set("X-Kosmos-Signer-Evidence", strings.Repeat("a", 4097)) },
		"changed request":   func(r *http.Request) { r.URL.Path = "/api/v1/signing/different/complete" },
		"changed method":    func(r *http.Request) { r.Method = http.MethodGet },
		"unrelated route":   func(r *http.Request) { r.URL.Path = "/api/v1/intake/contact" },
		"escaped route":     func(r *http.Request) { r.URL.RawPath = "/api/v1/signing/%74est_request-1/complete" },
	} {
		t.Run(name, func(t *testing.T) {
			r := signingSessionTestRequest(t, signingSessionTestEnvelope())
			mutate(r)
			if session, err := captureSigningSession(r, signingSessionTestKey, true, signingSessionTestTime); err == nil || session != (SigningSession{}) {
				t.Fatalf("invalid envelope accepted: %#v, %v", session, err)
			}
		})
	}
	for _, key := range [][]byte{nil, []byte("short"), []byte(strings.Repeat("x", 32))} {
		if _, err := captureSigningSession(signingSessionTestRequest(t, signingSessionTestEnvelope()), key, true, signingSessionTestTime); err == nil {
			t.Fatal("unavailable or wrong key accepted")
		}
	}
}

func TestCaptureSigningSessionValidatesSignedClaims(t *testing.T) {
	for name, mutate := range map[string]func(*signingSessionEnvelope){
		"unknown version":   func(e *signingSessionEnvelope) { e.Version = 2 },
		"expired":           func(e *signingSessionEnvelope) { e.Timestamp -= 61 },
		"future":            func(e *signingSessionEnvelope) { e.Timestamp += 61 },
		"noncanonical IPv6": func(e *signingSessionEnvelope) { e.IPAddress = "2001:0DB8:0:0:0:0:0:1" },
		"mapped IPv6":       func(e *signingSessionEnvelope) { e.IPAddress = "::ffff:192.0.2.1" },
		"scoped IPv6":       func(e *signingSessionEnvelope) { e.IPAddress = "fe80::1%en0" },
		"address list":      func(e *signingSessionEnvelope) { e.IPAddress = "192.0.2.1, 192.0.2.2" },
		"missing address":   func(e *signingSessionEnvelope) { e.IPAddress = "" },
		"long browser":      func(e *signingSessionEnvelope) { e.UserAgent = strings.Repeat("a", 513) },
		"multibyte browser": func(e *signingSessionEnvelope) { e.UserAgent = strings.Repeat("界", 171) },
		"long city":         func(e *signingSessionEnvelope) { e.City = strings.Repeat("é", 65) },
		"long region":       func(e *signingSessionEnvelope) { e.Region = strings.Repeat("a", 129) },
		"controls":          func(e *signingSessionEnvelope) { e.City = "Fake\nCity" },
		"bidi formatting":   func(e *signingSessionEnvelope) { e.UserAgent = "Browser\u202eFake" },
		"line separator":    func(e *signingSessionEnvelope) { e.Region = "One\u2028Two" },
		"untrimmed":         func(e *signingSessionEnvelope) { e.City = " City " },
		"lowercase country": func(e *signingSessionEnvelope) { e.Country = "br" },
		"invalid country":   func(e *signingSessionEnvelope) { e.Country = "1A" },
	} {
		t.Run(name, func(t *testing.T) {
			envelope := signingSessionTestEnvelope()
			mutate(&envelope)
			if _, err := captureSigningSession(signingSessionTestRequest(t, envelope), signingSessionTestKey, true, signingSessionTestTime); err == nil {
				t.Fatal("invalid signed claim accepted")
			}
		})
	}
	for _, delta := range []int64{-60, 60} {
		envelope := signingSessionTestEnvelope()
		envelope.Timestamp += delta
		if _, err := captureSigningSession(signingSessionTestRequest(t, envelope), signingSessionTestKey, true, signingSessionTestTime); err != nil {
			t.Fatalf("inclusive time boundary rejected: %v", err)
		}
	}
}

func TestCaptureSigningSessionStrictJSON(t *testing.T) {
	payload, err := json.Marshal(signingSessionTestEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		strings.Replace(string(payload), "{", "{\"unknown\":true,", 1),
		strings.Replace(string(payload), "{", "{\"version\":1,", 1),
		strings.Replace(string(payload), "\"version\"", "\"Version\"", 1),
		strings.Replace(string(payload), "\"version\":1,", "", 1),
		strings.Replace(string(payload), "\"country\":\"BR\"", "\"country\":null", 1),
		strings.Replace(string(payload), "São Paulo", "\\ud800", 1),
		string(payload) + "{}", "null", "[]", "{", string([]byte{0xff}),
	} {
		if _, err := captureSigningSession(signingSessionTestPayload([]byte(invalid)), signingSessionTestKey, true, signingSessionTestTime); err == nil {
			t.Fatal("invalid JSON envelope accepted")
		}
	}
}

func TestCaptureSigningSessionDirectConnection(t *testing.T) {
	r := signingSessionTestRequest(t, signingSessionTestEnvelope())
	r.RemoteAddr = "[::ffff:192.0.2.8]:1234"
	r.Header.Set("CF-Connecting-IP", "198.51.100.1")
	r.Header.Set("X-Forwarded-For", "198.51.100.2")
	r.Header.Set("CF-IPCity", "Forged city")
	r.Header.Set("User-Agent", "  Browser\nName\u202e "+strings.Repeat("界", 200))
	session, err := captureSigningSession(r, nil, false, signingSessionTestTime)
	if err != nil {
		t.Fatal(err)
	}
	if session.IPAddress != "192.0.2.8" || session.Source != "direct" || session.City != "" || session.Region != "" || session.Country != "" || !session.CapturedAt.Equal(signingSessionTestTime) {
		t.Fatalf("direct request trusted forwarded claims: %#v", session)
	}
	if len(session.UserAgent) > 512 || !utf8.ValidString(session.UserAgent) || !strings.HasPrefix(session.UserAgent, "Browser Name  ") {
		t.Fatalf("browser metadata not normalized: %q", session.UserAgent)
	}
	r.RemoteAddr = "invalid"
	if _, err := captureSigningSession(r, nil, false, signingSessionTestTime); err == nil {
		t.Fatal("invalid direct peer accepted")
	}
}

func TestNormalizeSigningSessionText(t *testing.T) {
	for _, test := range []struct {
		value string
		limit int
		want  string
	}{
		{"  São Paulo  ", 128, "São Paulo"},
		{"東京", 5, "東"},
		{"a\n\t\u0085\u202e\u2028\u2029\ufffdb", 128, "a       b"},
		{" ab cd ", 3, "ab"},
		{strings.Repeat("é", 65), 128, strings.Repeat("é", 64)},
	} {
		if got := normalizeSigningSessionText(test.value, test.limit); got != test.want {
			t.Fatalf("normalization: got %q want %q", got, test.want)
		}
	}
}
