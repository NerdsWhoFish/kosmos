package operations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
	"golang.org/x/oauth2"
)

const voiceContactsConnectionID = "shared"

func (m *Module) SaveVoiceContactsGrant(ctx context.Context, actor, connected Identity, token *oauth2.Token) error {
	if token == nil {
		return errors.New("Google Contacts token is required")
	}
	var existing VoiceContactsConnection
	existingErr := m.store.Get(ctx, m.publicScope, "voiceContactsConnections", voiceContactsConnectionID, &existing)
	if existingErr != nil && !errors.Is(existingErr, errNotFound) {
		return existingErr
	}
	if token.RefreshToken == "" && existing.EncryptedToken != "" {
		payload, err := decrypt(m.key, existing.EncryptedToken)
		if err != nil {
			return err
		}
		previous, err := parseToken(payload)
		if err != nil {
			return err
		}
		token.RefreshToken = previous.RefreshToken
	}
	if token.RefreshToken == "" {
		return errors.New("Google did not return offline access")
	}
	payload, err := tokenJSON(token)
	if err != nil {
		return err
	}
	sealed, err := encrypt(m.key, payload)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	connection := VoiceContactsConnection{
		ID:             voiceContactsConnectionID,
		GoogleEmail:    strings.ToLower(strings.TrimSpace(connected.Email)),
		GoogleSubject:  connected.Subject,
		EncryptedToken: sealed,
		CreatedBy:      strings.ToLower(strings.TrimSpace(actor.Email)),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if existingErr == nil {
		connection.CreatedAt = existing.CreatedAt
	}
	if err := m.store.Put(ctx, m.publicScope, "voiceContactsConnections", voiceContactsConnectionID, connection); err != nil {
		return err
	}
	if err := m.audit(ctx, m.publicScope, actor.Email, "google_contacts.connected", "integration", connection.ID, "Connected shared Google Contacts account"); err != nil {
		return err
	}
	contacts, err := m.workspace.ListContacts(ctx, m.publicScope)
	if err != nil {
		slog.ErrorContext(ctx, "initial Google Contacts list failed")
		return nil
	}
	for _, contact := range contacts {
		if err := m.enqueueGoogleContactMutation(ctx, m.publicScope, contact, "upsert", actor.Email, connection.UpdatedAt); err != nil {
			slog.ErrorContext(ctx, "initial Google contact sync enqueue failed", "contact.id", contact.ID)
		}
	}
	return nil
}

func (m *Module) AuthorizeVoiceContacts(ctx context.Context, actor Identity) error {
	return m.CheckRole(ctx, m.publicScope, actor, "owner", "admin")
}

func (m *Module) EnqueueGoogleContactMutation(ctx context.Context, scope string, contact workspace.Contact, action, actor string) error {
	return m.enqueueGoogleContactMutation(ctx, scope, contact, action, actor, contact.UpdatedAt)
}

func (m *Module) enqueueGoogleContactMutation(ctx context.Context, scope string, contact workspace.Contact, action, actor string, versionTime time.Time) error {
	var connection VoiceContactsConnection
	if err := m.store.Get(ctx, scope, "voiceContactsConnections", voiceContactsConnectionID, &connection); errors.Is(err, errNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if contact.ID == "" || !oneOf(action, "upsert", "delete") {
		return errors.New("Google contact mutation is invalid")
	}
	mapping := m.googleContactMapping(ctx, scope, contact.ID)
	mapping.Status = "pending"
	mapping.LastError = ""
	mapping.UpdatedAt = time.Now().UTC()
	if err := m.store.Put(ctx, scope, "googleContactMappings", mapping.ID, mapping); err != nil {
		return err
	}
	version := versionTime.UTC().Format(time.RFC3339Nano)
	if version == "0001-01-01T00:00:00Z" {
		version = time.Now().UTC().Format(time.RFC3339Nano)
	}
	job := Job{
		ID:           deterministicID("google-contact|" + action + "|" + contact.ID + "|" + version),
		Type:         JobTypeGoogleContactSync,
		Scope:        scope,
		ConnectionID: connection.ID,
		ContactID:    contact.ID,
		Action:       action,
		Actor:        actor,
	}
	if err := m.enqueueJob(ctx, job); err != nil {
		mapping := m.googleContactMapping(ctx, scope, contact.ID)
		mapping.Status = "failed"
		mapping.LastError = "Could not queue Google Contacts sync"
		mapping.UpdatedAt = time.Now().UTC()
		_ = m.store.Put(ctx, scope, "googleContactMappings", mapping.ID, mapping)
		return err
	}
	return nil
}

func (m *Module) voiceContactsStatus(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	var connection VoiceContactsConnection
	err := m.store.Get(r.Context(), scope, "voiceContactsConnections", voiceContactsConnectionID, &connection)
	if err != nil && !errors.Is(err, errNotFound) {
		writeError(w, http.StatusInternalServerError, "google_contacts_load_failed", "Could not load the shared Google Contacts connection")
		return
	}
	var mappings []GoogleContactMapping
	if err := m.store.List(r.Context(), scope, "googleContactMappings", &mappings); err != nil {
		writeError(w, http.StatusInternalServerError, "google_contacts_load_failed", "Could not load Google Contacts sync status")
		return
	}
	pending, failed, synced := 0, 0, 0
	for _, mapping := range mappings {
		switch mapping.Status {
		case "pending":
			pending++
		case "failed":
			failed++
		case "synced":
			synced++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connected":   err == nil,
		"googleEmail": connection.GoogleEmail,
		"connectUrl":  "/auth/connect/voice-contacts",
		"pending":     pending,
		"failed":      failed,
		"synced":      synced,
	})
}

func (m *Module) disconnectVoiceContacts(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	if err := m.store.Delete(r.Context(), scope, "voiceContactsConnections", voiceContactsConnectionID); err != nil && !errors.Is(err, errNotFound) {
		writeError(w, http.StatusInternalServerError, "google_contacts_disconnect_failed", "Could not disconnect the shared Google Contacts account")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "google_contacts.disconnected", "integration", voiceContactsConnectionID, "Disconnected shared Google Contacts account")
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) syncVoiceContacts(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	var connection VoiceContactsConnection
	if err := m.store.Get(r.Context(), scope, "voiceContactsConnections", voiceContactsConnectionID, &connection); err != nil {
		writeError(w, http.StatusConflict, "google_contacts_not_connected", "Connect the shared Google Contacts account first")
		return
	}
	contacts, err := m.workspace.ListContacts(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "google_contacts_sync_failed", "Could not load contacts for synchronization")
		return
	}
	version := time.Now().UTC()
	for _, contact := range contacts {
		if err := m.enqueueGoogleContactMutation(r.Context(), scope, contact, "upsert", actor.Email, version); err != nil {
			writeError(w, http.StatusServiceUnavailable, "google_contacts_sync_failed", "Could not queue Google Contacts synchronization")
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "queued": len(contacts)})
}

func (m *Module) voiceContactsToken(ctx context.Context, scope string) (*oauth2.Token, VoiceContactsConnection, error) {
	var connection VoiceContactsConnection
	if err := m.store.Get(ctx, scope, "voiceContactsConnections", voiceContactsConnectionID, &connection); err != nil {
		return nil, connection, err
	}
	payload, err := decrypt(m.key, connection.EncryptedToken)
	if err != nil {
		return nil, connection, err
	}
	token, err := parseToken(payload)
	return token, connection, err
}

func (m *Module) syncGoogleContact(ctx context.Context, job Job) error {
	token, _, err := m.voiceContactsToken(ctx, job.Scope)
	if err != nil {
		return err
	}
	mapping := m.googleContactMapping(ctx, job.Scope, job.ContactID)
	if job.Action == "delete" {
		if err := m.google.DeleteContact(ctx, token, job.ContactID, mapping.ResourceName); err != nil {
			mapping.Status = "failed"
			mapping.LastError = "Google rejected the contact deletion"
			mapping.UpdatedAt = time.Now().UTC()
			_ = m.store.Put(ctx, job.Scope, "googleContactMappings", mapping.ID, mapping)
			return err
		}
		if err := m.store.Delete(ctx, job.Scope, "googleContactMappings", mapping.ID); err != nil && !errors.Is(err, errNotFound) {
			return err
		}
		return m.auditJob(ctx, job.Scope, job.Actor, "google_contact.deleted", "contact", job.ContactID, "Deleted contact from shared Google Contacts", job.ID)
	}
	contact, err := m.workspace.GetContact(ctx, job.Scope, job.ContactID)
	if err != nil {
		return err
	}
	organization := ""
	if contact.AccountID != "" {
		if account, accountErr := m.workspace.GetAccount(ctx, job.Scope, contact.AccountID); accountErr == nil {
			organization = account.Name
		}
	}
	reference, err := m.google.UpsertContact(ctx, token, GoogleContact{
		ID:           contact.ID,
		Name:         contact.Name,
		Email:        contact.Email,
		Phone:        contact.Phone,
		Organization: organization,
		LinkedInURL:  contact.LinkedInURL,
	}, mapping.ResourceName)
	if err != nil {
		mapping.Status = "failed"
		mapping.LastError = "Google rejected the contact synchronization"
		mapping.UpdatedAt = time.Now().UTC()
		_ = m.store.Put(ctx, job.Scope, "googleContactMappings", mapping.ID, mapping)
		return err
	}
	now := time.Now().UTC()
	mapping.ResourceName = reference.ResourceName
	mapping.ETag = reference.ETag
	mapping.Status = "synced"
	mapping.LastError = ""
	mapping.LastSyncedAt = &now
	mapping.UpdatedAt = now
	if err := m.store.Put(ctx, job.Scope, "googleContactMappings", mapping.ID, mapping); err != nil {
		return err
	}
	return m.auditJob(ctx, job.Scope, job.Actor, "google_contact.synced", "contact", contact.ID, fmt.Sprintf("Synchronized %s with shared Google Contacts", contact.Name), job.ID)
}

func (m *Module) googleContactMapping(ctx context.Context, scope, contactID string) GoogleContactMapping {
	var mapping GoogleContactMapping
	if err := m.store.Get(ctx, scope, "googleContactMappings", contactID, &mapping); err == nil {
		return mapping
	}
	now := time.Now().UTC()
	return GoogleContactMapping{ID: contactID, ContactID: contactID, Status: "pending", CreatedAt: now, UpdatedAt: now}
}
