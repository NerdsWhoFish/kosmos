package operations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const apiCredentialTokenPrefix = "kosmos_api_"

var apiCredentialIDPattern = regexp.MustCompile(`^[a-f0-9]{24}$`)

func (m *Module) apiCredentials(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	writeStorePage[APICredential](w, r, m.store, scope, "apiCredentials", "credentials", "api_credentials_load_failed", "Could not load API credentials", pagination.Spec{Key: "operations.api-credentials", OrderBy: "createdAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue})
}

func (m *Module) createAPICredential(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	var request struct {
		Name   string `json:"name"`
		Access string `json:"access"`
	}
	if !decodeJSON(w, r, &request, 8<<10) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Access = strings.ToLower(strings.TrimSpace(request.Access))
	if request.Name == "" || len(request.Name) > 80 || !oneOf(request.Access, "read", "write") {
		writeError(w, http.StatusBadRequest, "invalid_api_credential", "Add a name and choose read-only or read-and-write access")
		return
	}
	id, err := newID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_credential_create_failed", "Could not create API credential")
		return
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		writeError(w, http.StatusInternalServerError, "api_credential_create_failed", "Could not create API credential")
		return
	}
	token := apiCredentialTokenPrefix + id + "_" + base64.RawURLEncoding.EncodeToString(secret)
	hash := sha256.Sum256([]byte(token))
	now := time.Now().UTC()
	credential := APICredential{ID: id, Name: request.Name, Access: request.Access, TokenPrefix: apiCredentialTokenPrefix + id[:8] + "…", SecretHash: hex.EncodeToString(hash[:]), CreatedBy: actor.Email, CreatedAt: now, UpdatedAt: now}
	if err := m.store.Create(r.Context(), scope, "apiCredentials", credential.ID, credential); err != nil {
		writeError(w, http.StatusInternalServerError, "api_credential_create_failed", "Could not create API credential")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "api_credential.created", "api_credential", credential.ID, credential.Name+" created with "+credential.Access+" access")
	writeJSON(w, http.StatusCreated, map[string]any{"credential": credential, "token": token})
}

func (m *Module) revokeAPICredential(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	var credential APICredential
	if err := m.store.Get(r.Context(), scope, "apiCredentials", r.PathValue("id"), &credential); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "api_credential_not_found", "API credential was not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "api_credential_revoke_failed", "Could not revoke API credential")
		return
	}
	if credential.RevokedAt == nil {
		now := time.Now().UTC()
		credential.RevokedAt = &now
		credential.UpdatedAt = now
		if err := m.store.Put(r.Context(), scope, "apiCredentials", credential.ID, credential); err != nil {
			writeError(w, http.StatusInternalServerError, "api_credential_revoke_failed", "Could not revoke API credential")
			return
		}
		_ = m.audit(r.Context(), scope, actor.Email, "api_credential.revoked", "api_credential", credential.ID, credential.Name+" revoked")
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) AuthenticateAPICredential(ctx context.Context, token string) (Identity, error) {
	ctx, span := otel.Tracer("github.com/NerdsWhoFish/kosmos/backend/operations").Start(ctx, "api_credential.authenticate")
	defer span.End()
	fail := func() (Identity, error) {
		span.SetAttributes(attribute.String("auth.outcome", "rejected"))
		span.SetStatus(codes.Error, "credential rejected")
		return Identity{}, errors.New("invalid API credential")
	}
	value := strings.TrimSpace(token)
	if !strings.HasPrefix(value, apiCredentialTokenPrefix) {
		return fail()
	}
	id, secret, ok := strings.Cut(strings.TrimPrefix(value, apiCredentialTokenPrefix), "_")
	if !ok || !apiCredentialIDPattern.MatchString(id) || secret == "" {
		return fail()
	}
	var credential APICredential
	if err := m.store.Get(ctx, m.publicScope, "apiCredentials", id, &credential); err != nil || credential.RevokedAt != nil || !oneOf(credential.Access, "read", "write") {
		return fail()
	}
	want, err := hex.DecodeString(credential.SecretHash)
	if err != nil {
		return fail()
	}
	got := sha256.Sum256([]byte(value))
	if subtle.ConstantTimeCompare(got[:], want) != 1 {
		return fail()
	}
	span.SetAttributes(attribute.String("auth.outcome", "accepted"), attribute.String("api_credential.access", credential.Access))
	return Identity{Subject: "api-credential:" + credential.ID, Email: "api-credential:" + credential.ID, Name: credential.Name, Kind: "api", Access: credential.Access}, nil
}

func apiCredentialRouteAllowed(r *http.Request) bool {
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/v1/api-credentials") || strings.HasPrefix(path, "/api/v1/members") || strings.HasPrefix(path, "/api/v1/integrations/") || strings.HasPrefix(path, "/api/v1/email/") || strings.HasPrefix(path, "/api/v1/audit") || strings.HasPrefix(path, "/api/v1/voice/link") {
		return false
	}
	if path == "/api/v1/pipeline-stages" && r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	return true
}
