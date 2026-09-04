package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	stateCookie   = "kosmos_oauth_state"
	sessionCookie = "kosmos_session"
)

type Google struct {
	clientID       string
	clientSecret   string
	publicURL      string
	sessionKey     []byte
	production     bool
	allowedDomains map[string]struct{}
	grants         map[string]grant

	mu       sync.Mutex
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
}

type grant struct {
	scopes  []string
	handler func(context.Context, User, *oauth2.Token) error
}

type User struct {
	Subject string `json:"subject"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture,omitempty"`
}

type session struct {
	User      User      `json:"user"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func NewGoogle() *Google {
	return &Google{
		clientID:       os.Getenv("GOOGLE_CLIENT_ID"),
		clientSecret:   os.Getenv("GOOGLE_CLIENT_SECRET"),
		publicURL:      strings.TrimRight(os.Getenv("KOSMOS_PUBLIC_URL"), "/"),
		sessionKey:     []byte(os.Getenv("KOSMOS_SESSION_SECRET")),
		production:     os.Getenv("KOSMOS_ENV") == "production",
		allowedDomains: parseDomains(os.Getenv("KOSMOS_ALLOWED_GOOGLE_DOMAINS")),
		grants:         make(map[string]grant),
	}
}

func (g *Google) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", g.login)
	mux.HandleFunc("GET /auth/connect/{provider}", g.connect)
	mux.HandleFunc("GET /auth/callback", g.callback)
	mux.HandleFunc("POST /auth/logout", g.logout)
	mux.HandleFunc("GET /api/v1/me", g.me)
}

func (g *Google) RegisterGrant(name string, scopes []string, handler func(context.Context, User, *oauth2.Token) error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.grants[name] = grant{scopes: append([]string(nil), scopes...), handler: handler}
}

func (g *Google) login(w http.ResponseWriter, r *http.Request) {
	config, err := g.oauthConfig(r.Context(), r, nil)
	if err != nil {
		http.Error(w, "Google login is not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := randomString(32)
	if err != nil {
		http.Error(w, "could not create login state", http.StatusInternalServerError)
		return
	}
	state = "login:" + state
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: state, Path: "/", MaxAge: 600, HttpOnly: true, Secure: g.secureCookies(), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, config.AuthCodeURL(state), http.StatusFound)
}

func (g *Google) connect(w http.ResponseWriter, r *http.Request) {
	current, err := g.CurrentUser(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	name := r.PathValue("provider")
	g.mu.Lock()
	registered, ok := g.grants[name]
	g.mu.Unlock()
	if !ok {
		http.Error(w, "unknown Google connection", http.StatusNotFound)
		return
	}
	config, err := g.oauthConfig(r.Context(), r, registered.scopes)
	if err != nil {
		http.Error(w, "Google connection is not configured", http.StatusServiceUnavailable)
		return
	}
	nonce, err := randomString(32)
	if err != nil {
		http.Error(w, "could not create connection state", http.StatusInternalServerError)
		return
	}
	state := name + ":" + current.Subject + ":" + nonce
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: state, Path: "/", MaxAge: 600, HttpOnly: true, Secure: g.secureCookies(), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce), http.StatusFound)
}

func (g *Google) callback(w http.ResponseWriter, r *http.Request) {
	stateCookieValue, err := r.Cookie(stateCookie)
	if err != nil || stateCookieValue.Value == "" || !hmac.Equal([]byte(stateCookieValue.Value), []byte(r.URL.Query().Get("state"))) {
		http.Error(w, "invalid login state", http.StatusBadRequest)
		return
	}
	purpose, _, ok := strings.Cut(r.URL.Query().Get("state"), ":")
	if !ok {
		http.Error(w, "invalid login state", http.StatusBadRequest)
		return
	}
	var registered grant
	if purpose != "login" {
		g.mu.Lock()
		registered, ok = g.grants[purpose]
		g.mu.Unlock()
		if !ok {
			http.Error(w, "unknown Google connection", http.StatusBadRequest)
			return
		}
	}
	config, err := g.oauthConfig(r.Context(), r, registered.scopes)
	if err != nil {
		http.Error(w, "Google login is not configured", http.StatusServiceUnavailable)
		return
	}
	token, err := config.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "Google authorization failed", http.StatusUnauthorized)
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "Google did not return an identity token", http.StatusUnauthorized)
		return
	}
	idToken, err := g.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "Google identity verification failed", http.StatusUnauthorized)
		return
	}
	var claims struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Subject == "" || claims.Email == "" || !claims.EmailVerified {
		http.Error(w, "Google identity was incomplete", http.StatusUnauthorized)
		return
	}
	if !g.allowsEmail(claims.Email) {
		http.Error(w, "This Google account is not allowed to use Kosmos", http.StatusForbidden)
		return
	}
	if purpose != "login" {
		current, sessionErr := g.CurrentUser(r)
		if sessionErr != nil || current.Subject != claims.Subject {
			http.Error(w, "Google connection must use your signed-in account", http.StatusForbidden)
			return
		}
		if err := registered.handler(r.Context(), current, token); err != nil {
			http.Error(w, "could not save Google connection", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: g.secureCookies(), SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, "/settings?connected=1", http.StatusFound)
		return
	}
	if len(g.sessionKey) < 32 {
		http.Error(w, "session signing is not configured", http.StatusServiceUnavailable)
		return
	}
	value, err := g.signSession(session{User: User{Subject: claims.Subject, Email: claims.Email, Name: claims.Name, Picture: claims.Picture}, ExpiresAt: time.Now().Add(7 * 24 * time.Hour)})
	if err != nil {
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: value, Path: "/", MaxAge: 7 * 24 * 60 * 60, HttpOnly: true, Secure: g.secureCookies(), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (g *Google) logout(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Kosmos-CSRF") != "1" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: g.secureCookies(), SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (g *Google) me(w http.ResponseWriter, r *http.Request) {
	current, err := g.CurrentUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	writeJSON(w, http.StatusOK, current)
}

func (g *Google) CurrentUser(r *http.Request) (User, error) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-Kosmos-CSRF") != "1" {
		return User{}, errors.New("invalid mutation request")
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return User{}, errors.New("missing session")
	}
	var current session
	if err := g.verifySession(cookie.Value, &current); err != nil || time.Now().After(current.ExpiresAt) {
		return User{}, errors.New("invalid session")
	}
	if !g.allowsEmail(current.User.Email) {
		return User{}, errors.New("session user is no longer allowed")
	}
	return current.User, nil
}

func (g *Google) oauthConfig(ctx context.Context, r *http.Request, extraScopes ...[]string) (*oauth2.Config, error) {
	if g.clientID == "" || g.clientSecret == "" || len(g.sessionKey) < 32 || (g.production && len(g.allowedDomains) == 0) {
		return nil, errors.New("Google OAuth environment is incomplete")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.provider == nil {
		provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
		if err != nil {
			return nil, err
		}
		g.provider = provider
		g.verifier = provider.Verifier(&oidc.Config{ClientID: g.clientID})
	}
	redirectURL := g.publicURL + "/auth/callback"
	if g.publicURL == "" {
		scheme := "https"
		if r.Header.Get("X-Forwarded-Proto") != "" {
			scheme = r.Header.Get("X-Forwarded-Proto")
		}
		redirectURL = scheme + "://" + r.Host + "/auth/callback"
	}
	scopes := []string{oidc.ScopeOpenID, "email", "profile"}
	if len(extraScopes) != 0 {
		scopes = append(scopes, extraScopes[0]...)
	}
	return &oauth2.Config{ClientID: g.clientID, ClientSecret: g.clientSecret, Endpoint: google.Endpoint, RedirectURL: redirectURL, Scopes: scopes}, nil
}

func (g *Google) signSession(value session) (string, error) {
	if len(g.sessionKey) < 32 {
		return "", errors.New("session signing key is too short")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + signature(g.sessionKey, encoded), nil
}

func (g *Google) verifySession(value string, target *session) error {
	if len(g.sessionKey) < 32 {
		return errors.New("session signing key is too short")
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(signature(g.sessionKey, parts[0]))) {
		return errors.New("invalid session signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, target)
}

func (g *Google) secureCookies() bool { return g.production }

func (g *Google) allowsEmail(email string) bool {
	if len(g.allowedDomains) == 0 {
		return !g.production
	}
	normalized := strings.ToLower(strings.TrimSpace(email))
	local, domain, ok := strings.Cut(normalized, "@")
	if !ok || local == "" || domain == "" || strings.Contains(domain, "@") {
		return false
	}
	_, ok = g.allowedDomains[domain]
	return ok
}

func parseDomains(value string) map[string]struct{} {
	domains := make(map[string]struct{})
	for _, domain := range strings.Split(value, ",") {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain != "" && !strings.Contains(domain, "@") {
			domains[domain] = struct{}{}
		}
	}
	return domains
}

func signature(key []byte, value string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randomString(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
