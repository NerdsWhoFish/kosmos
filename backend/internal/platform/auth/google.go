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
	clientID     string
	clientSecret string
	publicURL    string
	sessionKey   []byte

	mu       sync.Mutex
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
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
		clientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		clientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		publicURL:    strings.TrimRight(os.Getenv("KOSMOS_PUBLIC_URL"), "/"),
		sessionKey:   []byte(os.Getenv("KOSMOS_SESSION_SECRET")),
	}
}

func (g *Google) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", g.login)
	mux.HandleFunc("GET /auth/callback", g.callback)
	mux.HandleFunc("POST /auth/logout", g.logout)
	mux.HandleFunc("GET /api/v1/me", g.me)
}

func (g *Google) login(w http.ResponseWriter, r *http.Request) {
	config, err := g.oauthConfig(r.Context(), r)
	if err != nil {
		http.Error(w, "Google login is not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := randomString(32)
	if err != nil {
		http.Error(w, "could not create login state", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: state, Path: "/", MaxAge: 600, HttpOnly: true, Secure: g.secureCookies(), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, config.AuthCodeURL(state, oauth2.AccessTypeOffline), http.StatusFound)
}

func (g *Google) callback(w http.ResponseWriter, r *http.Request) {
	stateCookieValue, err := r.Cookie(stateCookie)
	if err != nil || stateCookieValue.Value == "" || !hmac.Equal([]byte(stateCookieValue.Value), []byte(r.URL.Query().Get("state"))) {
		http.Error(w, "invalid login state", http.StatusBadRequest)
		return
	}
	config, err := g.oauthConfig(r.Context(), r)
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

func (g *Google) logout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: g.secureCookies(), SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (g *Google) me(w http.ResponseWriter, r *http.Request) {
	current, err := g.currentUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	writeJSON(w, http.StatusOK, current)
}

func (g *Google) currentUser(r *http.Request) (User, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return User{}, errors.New("missing session")
	}
	var current session
	if err := g.verifySession(cookie.Value, &current); err != nil || time.Now().After(current.ExpiresAt) {
		return User{}, errors.New("invalid session")
	}
	return current.User, nil
}

func (g *Google) oauthConfig(ctx context.Context, r *http.Request) (*oauth2.Config, error) {
	if g.clientID == "" || g.clientSecret == "" || len(g.sessionKey) < 32 {
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
	return &oauth2.Config{ClientID: g.clientID, ClientSecret: g.clientSecret, Endpoint: google.Endpoint, RedirectURL: redirectURL, Scopes: []string{oidc.ScopeOpenID, "email", "profile"}}, nil
}

func (g *Google) signSession(value session) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + signature(g.sessionKey, encoded), nil
}

func (g *Google) verifySession(value string, target *session) error {
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

func (g *Google) secureCookies() bool { return os.Getenv("KOSMOS_ENV") == "production" }

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
