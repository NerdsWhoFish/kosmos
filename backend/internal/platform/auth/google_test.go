package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMeRequiresASession(t *testing.T) {
	server := NewGoogle()
	record := httptest.NewRecorder()
	server.me(record, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))

	if record.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", record.Code, http.StatusUnauthorized)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	server := &Google{sessionKey: []byte("01234567890123456789012345678901")}
	want := session{User: User{Subject: "google-subject", Email: "owner@example.com", Name: "Owner"}, ExpiresAt: time.Now().Add(time.Hour)}
	value, err := server.signSession(want)
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}
	var got session
	if err := server.verifySession(value, &got); err != nil {
		t.Fatalf("verify session: %v", err)
	}
	if got.User != want.User || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Fatalf("session = %#v, want %#v", got, want)
	}
}

func TestSessionRejectsWeakSigningKey(t *testing.T) {
	server := &Google{sessionKey: []byte("too-short")}
	value := session{User: User{Subject: "google-subject"}, ExpiresAt: time.Now().Add(time.Hour)}
	if _, err := server.signSession(value); err == nil {
		t.Fatal("signSession accepted a weak key")
	}
	if err := server.verifySession("payload.signature", &session{}); err == nil {
		t.Fatal("verifySession accepted a weak key")
	}
}

func TestAllowsOnlyConfiguredGoogleDomains(t *testing.T) {
	server := &Google{
		production:     true,
		allowedDomains: parseDomains("nerdswhofish.com, theoutdoorprogrammer.com,apollorion.com"),
	}

	for _, email := range []string{"joey@nerdswhofish.com", "JOEY@TheOutdoorProgrammer.com", "admin@apollorion.com"} {
		if !server.allowsEmail(email) {
			t.Fatalf("expected %q to be allowed", email)
		}
	}
	for _, email := range []string{"joey@gmail.com", "attacker@nerdswhofish.com.example", "invalid"} {
		if server.allowsEmail(email) {
			t.Fatalf("expected %q to be rejected", email)
		}
	}
}

func TestCurrentUserRechecksDomainPolicy(t *testing.T) {
	server := &Google{
		sessionKey:     []byte("01234567890123456789012345678901"),
		production:     true,
		allowedDomains: parseDomains("nerdswhofish.com"),
	}
	value, err := server.signSession(session{
		User:      User{Subject: "google-subject", Email: "former@example.com", Name: "Former User"},
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: value})

	if _, err := server.CurrentUser(request); err == nil {
		t.Fatal("CurrentUser accepted a session outside the configured domains")
	}
}

func TestDevelopmentAllowsVerifiedGoogleIdentityWithoutDomainConfig(t *testing.T) {
	server := &Google{}
	if !server.allowsEmail("developer@example.com") {
		t.Fatal("development should allow a Google identity when no domains are configured")
	}
}
