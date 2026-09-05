package operations

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type SigningSession struct {
	IPAddress  string    `json:"ipAddress" firestore:"ipAddress"`
	UserAgent  string    `json:"userAgent" firestore:"userAgent"`
	City       string    `json:"city" firestore:"city"`
	Region     string    `json:"region" firestore:"region"`
	Country    string    `json:"country" firestore:"country"`
	CapturedAt time.Time `json:"capturedAt" firestore:"capturedAt"`
	Source     string    `json:"source" firestore:"source"`
}

type signingSessionEnvelope struct {
	Version   int    `json:"version"`
	IPAddress string `json:"ipAddress"`
	UserAgent string `json:"userAgent"`
	City      string `json:"city"`
	Region    string `json:"region"`
	Country   string `json:"country"`
	Timestamp int64  `json:"timestamp"`
}

var signingCompletionPath = regexp.MustCompile(`^/api/v1/signing/[A-Za-z0-9_-]{1,64}/complete$`)
var errSigningSession = errors.New("signing session evidence could not be verified")

func captureSigningSession(r *http.Request, key []byte, requireSigned bool, now time.Time) (SigningSession, error) {
	if r.Method != http.MethodPost || !signingCompletionPath.MatchString(r.URL.Path) || r.URL.EscapedPath() != r.URL.Path {
		return SigningSession{}, errSigningSession
	}
	if !requireSigned {
		address, err := clientIP(r)
		if err != nil {
			return SigningSession{}, errSigningSession
		}
		return SigningSession{IPAddress: address, UserAgent: normalizeSigningSessionText(r.UserAgent(), 512), CapturedAt: now.UTC(), Source: "direct"}, nil
	}
	if len(key) < 32 {
		return SigningSession{}, errSigningSession
	}
	encoded, ok := singleSigningSessionHeader(r.Header, "X-Kosmos-Signer-Evidence")
	if !ok || len(encoded) == 0 || len(encoded) > 4096 {
		return SigningSession{}, errSigningSession
	}
	signature, ok := singleSigningSessionHeader(r.Header, "X-Kosmos-Signer-Signature")
	if !ok || len(signature) != sha256.Size*2 {
		return SigningSession{}, errSigningSession
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return SigningSession{}, errSigningSession
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("kosmos-signing-evidence-v1\nPOST\n" + r.URL.Path + "\n" + encoded))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return SigningSession{}, errSigningSession
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || !utf8.Valid(payload) || base64.RawURLEncoding.EncodeToString(payload) != encoded {
		return SigningSession{}, errSigningSession
	}
	var envelope signingSessionEnvelope
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return SigningSession{}, errSigningSession
	}
	if decoder.Decode(new(any)) != io.EOF || !uniqueSigningSessionFields(payload) {
		return SigningSession{}, errSigningSession
	}
	if envelope.Version != 1 || envelope.Timestamp < now.Unix()-60 || envelope.Timestamp > now.Unix()+60 {
		return SigningSession{}, errSigningSession
	}
	address, err := canonicalClientIP(envelope.IPAddress)
	if err != nil || address != envelope.IPAddress {
		return SigningSession{}, errSigningSession
	}
	for _, field := range []struct {
		value string
		limit int
	}{{envelope.UserAgent, 512}, {envelope.City, 128}, {envelope.Region, 128}} {
		if !utf8.ValidString(field.value) || field.value != normalizeSigningSessionText(field.value, field.limit) {
			return SigningSession{}, errSigningSession
		}
	}
	if envelope.Country != "" && (len(envelope.Country) != 2 || envelope.Country[0] < 'A' || envelope.Country[0] > 'Z' || envelope.Country[1] < 'A' || envelope.Country[1] > 'Z') {
		return SigningSession{}, errSigningSession
	}
	return SigningSession{IPAddress: address, UserAgent: envelope.UserAgent, City: envelope.City, Region: envelope.Region, Country: envelope.Country, CapturedAt: time.Unix(envelope.Timestamp, 0).UTC(), Source: "cloudflare"}, nil
}

func singleSigningSessionHeader(headers http.Header, name string) (string, bool) {
	var values []string
	for key, entries := range headers {
		if strings.EqualFold(key, name) {
			values = append(values, entries...)
		}
	}
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}

func uniqueSigningSessionFields(payload []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok || seen[name] {
			return false
		}
		switch name {
		case "version", "ipAddress", "userAgent", "city", "region", "country", "timestamp":
		default:
			return false
		}
		seen[name] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil || bytes.Equal(value, []byte("null")) {
			return false
		}
	}
	return len(seen) == 7
}

func normalizeSigningSessionText(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' || r == utf8.RuneError {
			return ' '
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if len(value) > limit {
		value = value[:limit]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	return strings.TrimSpace(value)
}
