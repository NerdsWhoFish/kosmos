package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type callbackTokenTransport struct{ response string }

func (t callbackTokenTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(t.response)), Request: r}, nil
}

func TestGoogleCallbackRequiresGrantedIntegrationScopes(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := json.Marshal(map[string]any{
		"iss": "https://accounts.google.com", "aud": "client-id", "sub": "owner-subject",
		"email": "owner@example.com", "email_verified": true, "exp": time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	idToken := unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	const contacts = "https://www.googleapis.com/auth/contacts"
	const mail = "https://www.googleapis.com/auth/gmail.compose"
	for _, test := range []struct {
		name     string
		purpose  string
		required []string
		scope    any
		allowed  bool
	}{
		{name: "identity only", purpose: "voice-contacts", required: []string{contacts}, scope: "openid email profile"},
		{name: "missing scope", purpose: "voice-contacts", required: []string{contacts}},
		{name: "malformed scope", purpose: "voice-contacts", required: []string{contacts}, scope: []string{contacts}},
		{name: "read only is insufficient", purpose: "voice-contacts", required: []string{contacts}, scope: contacts + ".readonly"},
		{name: "callback query cannot grant access", purpose: "voice-contacts", required: []string{contacts}, scope: "openid"},
		{name: "full contacts consent", purpose: "voice-contacts", required: []string{contacts}, scope: "openid  " + contacts + " email profile", allowed: true},
		{name: "partial workspace consent", purpose: "workspace", required: []string{contacts, mail}, scope: contacts},
		{name: "full workspace consent", purpose: "workspace", required: []string{contacts, mail}, scope: mail + " " + contacts, allowed: true},
		{name: "ordinary login", purpose: "login", allowed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			saved := "existing connection"
			server := &Google{
				clientID: "client-id", clientSecret: "client-secret", publicURL: "https://kosmos.example",
				sessionKey: []byte("01234567890123456789012345678901"), allowedDomains: parseDomains("example.com"),
				grants: make(map[string]grant), provider: &oidc.Provider{},
				verifier: oidc.NewVerifier("https://accounts.google.com", &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidc.Config{ClientID: "client-id"}),
			}
			handler := func(_ context.Context, _ User, token *oauth2.Token) error {
				saved = token.AccessToken
				return nil
			}
			if test.purpose == "voice-contacts" {
				server.RegisterDelegatedGrant(test.purpose, test.required, func(context.Context, User) error { return nil }, func(ctx context.Context, actor, connected User, token *oauth2.Token) error {
					return handler(ctx, connected, token)
				})
			} else if test.purpose != "login" {
				server.RegisterGrant(test.purpose, test.required, handler)
			}
			response := map[string]any{"access_token": "new-access", "refresh_token": "new-refresh", "token_type": "Bearer", "id_token": idToken}
			if test.scope != nil {
				response["scope"] = test.scope
			}
			body, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: callbackTokenTransport{response: string(body)}})
			state := test.purpose + ":owner-subject:nonce"
			request := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=code&scope="+contacts, nil).WithContext(ctx)
			request.AddCookie(&http.Cookie{Name: stateCookie, Value: state})
			sessionValue, err := server.signSession(session{User: User{Subject: "owner-subject", Email: "owner@example.com"}, ExpiresAt: time.Now().Add(time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
			request.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionValue})
			record := httptest.NewRecorder()
			server.callback(record, request)
			if test.allowed {
				if record.Code != http.StatusFound {
					t.Fatalf("callback failed: %d %s", record.Code, record.Body.String())
				}
				if test.purpose != "login" && (saved != "new-access" || record.Header().Get("Location") != "/settings?connected="+test.purpose) {
					t.Fatal("complete consent did not save the grant and return to Settings")
				}
				return
			}
			if record.Code != http.StatusForbidden || !strings.Contains(record.Body.String(), "select every requested permission") {
				t.Fatalf("missing consent was not explained: %d %s", record.Code, record.Body.String())
			}
			if saved != "existing connection" || record.Header().Get("Location") != "" || len(record.Result().Cookies()) != 0 {
				t.Fatal("incomplete consent changed the connection or reported success")
			}
			for _, secret := range []string{"new-access", "new-refresh", idToken} {
				if strings.Contains(record.Body.String(), secret) {
					t.Fatal("callback disclosed a token")
				}
			}
		})
	}
}
