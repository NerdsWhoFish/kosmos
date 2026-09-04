package operations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/mail"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
	platformmodules "github.com/NerdsWhoFish/kosmos/backend/internal/platform/modules"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
	"golang.org/x/oauth2"
)

const maxUploadSize = 10 << 20

var GoogleScopes = []string{
	"https://www.googleapis.com/auth/gmail.compose",
	"https://www.googleapis.com/auth/gmail.metadata",
	"https://www.googleapis.com/auth/gmail.settings.basic",
	"https://www.googleapis.com/auth/spreadsheets.readonly",
}

type IdentityFunc func(*http.Request) (string, Identity, error)

type Workspace interface {
	ListAccounts(context.Context, string) ([]workspace.Account, error)
	GetAccount(context.Context, string, string) (workspace.Account, error)
	CreateAccountWithContact(context.Context, string, workspace.Account, workspace.Contact) (workspace.Account, workspace.Contact, error)
	LinkWebsiteRenewal(context.Context, string, string, workspace.Website, []workspace.Reminder) (workspace.Account, []workspace.Reminder, error)
	ListContacts(context.Context, string) ([]workspace.Contact, error)
	CreateContact(context.Context, string, workspace.Contact) (workspace.Contact, error)
	ListCosts(context.Context, string) ([]workspace.Cost, error)
}

type Module struct {
	store       Store
	blobs       BlobStore
	workspace   Workspace
	identity    IdentityFunc
	publicScope string
	key         []byte
	google      GoogleProvider
	cloudflare  CloudflareProvider
	jobs        JobQueue
	limiter     *ipLimiter
}

func NewModule(store Store, blobs BlobStore, workspaceStore Workspace, identity IdentityFunc, publicScope string, key []byte, google GoogleProvider, options ...ModuleOption) *Module {
	module := &Module{store: store, blobs: blobs, workspace: workspaceStore, identity: identity, publicScope: publicScope, key: append([]byte(nil), key...), google: google, cloudflare: NewLiveCloudflareProvider(nil), limiter: newIPLimiter()}
	for _, option := range options {
		option(module)
	}
	return module
}

func (m *Module) MigrateGoogleConnectionSecrets(ctx context.Context, legacyKey []byte) (int, error) {
	if len(m.key) == 0 || len(legacyKey) == 0 || bytes.Equal(m.key, legacyKey) {
		return 0, nil
	}
	var connections []GoogleConnection
	if err := m.store.List(ctx, m.publicScope, "googleConnections", &connections); err != nil {
		return 0, err
	}
	migrated := 0
	skipped := 0
	for _, connection := range connections {
		if _, err := decrypt(m.key, connection.EncryptedToken); err == nil {
			continue
		}
		plaintext, err := decrypt(legacyKey, connection.EncryptedToken)
		if err != nil {
			skipped++
			continue
		}
		connection.EncryptedToken, err = encrypt(m.key, plaintext)
		if err != nil {
			return migrated, fmt.Errorf("encrypt Google connection %s with current key: %w", connection.ID, err)
		}
		connection.UpdatedAt = time.Now().UTC()
		if err := m.store.Put(ctx, m.publicScope, "googleConnections", connection.ID, connection); err != nil {
			return migrated, err
		}
		migrated++
	}
	if skipped > 0 {
		return migrated, fmt.Errorf("%d Google connection secrets require reconnection", skipped)
	}
	return migrated, nil
}

func (*Module) Name() string { return "operations" }

func (*Module) Manifest() platformmodules.Manifest {
	return platformmodules.Manifest{Name: "operations", Navigation: []platformmodules.Navigation{{Path: "/communications", Label: "Inbox", Icon: "inbox"}, {Path: "/operations", Label: "Operations", Icon: "operations"}, {Path: "/settings", Label: "Settings", Icon: "settings"}}, Permissions: []string{"communications.send", "integrations.manage", "members.manage", "records.export"}, Resources: []string{"members", "pipelineStages", "notifications", "emailTemplates", "mailMetadata", "transactions", "attachments", "audit", "cloudflareConnections", "sendAsMappings", "tillerWebhookConnections", "tillerProductMappings"}, EventTypes: []string{"lead.created", "email.received", "email.sent", "transaction.imported", "cloudflare.domain_linked", "tiller.purchase_imported"}, BackgroundJobs: []string{"gmail.sync", "tiller.sync"}, SearchProviders: []string{"mail", "transactions"}, DocumentLinkTargets: []string{"attachment"}}
}

func (m *Module) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/intake/contact", m.intakeContact)
	mux.HandleFunc("GET /api/v1/members", m.members)
	mux.HandleFunc("PATCH /api/v1/members/{id}", m.updateMember)
	mux.HandleFunc("GET /api/v1/email/send-as", m.sendAsMappings)
	mux.HandleFunc("PUT /api/v1/members/{id}/send-as", m.configureSendAs)
	mux.HandleFunc("GET /api/v1/pipeline-stages", m.pipelineStages)
	mux.HandleFunc("POST /api/v1/pipeline-stages", m.createPipelineStage)
	mux.HandleFunc("GET /api/v1/notifications", m.notifications)
	mux.HandleFunc("PATCH /api/v1/notifications/{id}", m.readNotification)
	mux.HandleFunc("POST /api/v1/events", m.publishEvent)
	mux.HandleFunc("GET /api/v1/email/templates", m.emailTemplates)
	mux.HandleFunc("POST /api/v1/email/templates", m.createEmailTemplate)
	mux.HandleFunc("GET /api/v1/integrations/google", m.googleStatus)
	mux.HandleFunc("POST /api/v1/email/send", m.sendEmail)
	mux.HandleFunc("POST /api/v1/email/sync", m.syncEmail)
	mux.HandleFunc("GET /api/v1/email/messages", m.mailMessages)
	mux.HandleFunc("PUT /api/v1/integrations/tiller", m.configureTiller)
	mux.HandleFunc("POST /api/v1/integrations/tiller/sync", m.syncTiller)
	mux.HandleFunc("GET /api/v1/integrations/tiller/webhook", m.tillerWebhookStatus)
	mux.HandleFunc("PUT /api/v1/integrations/tiller/webhook", m.configureTillerWebhook)
	mux.HandleFunc("DELETE /api/v1/integrations/tiller/webhook", m.disconnectTillerWebhook)
	mux.HandleFunc("GET /api/v1/integrations/tiller/product-mappings", m.tillerProductMappings)
	mux.HandleFunc("PUT /api/v1/integrations/tiller/product-mappings/{id}", m.configureTillerProductMapping)
	mux.HandleFunc("DELETE /api/v1/integrations/tiller/product-mappings/{id}", m.deleteTillerProductMapping)
	mux.HandleFunc("POST /api/v1/webhooks/tiller", m.receiveTillerWebhook)
	mux.HandleFunc("GET /api/v1/integrations/cloudflare", m.cloudflareStatus)
	mux.HandleFunc("PUT /api/v1/integrations/cloudflare", m.configureCloudflare)
	mux.HandleFunc("DELETE /api/v1/integrations/cloudflare", m.disconnectCloudflare)
	mux.HandleFunc("GET /api/v1/integrations/cloudflare/domains", m.cloudflareDomains)
	mux.HandleFunc("POST /api/v1/integrations/cloudflare/link", m.linkCloudflareDomain)
	mux.HandleFunc("GET /api/v1/transactions", m.transactions)
	mux.HandleFunc("PATCH /api/v1/transactions/{id}", m.updateTransaction)
	mux.HandleFunc("POST /api/v1/attachments", m.uploadAttachment)
	mux.HandleFunc("GET /api/v1/attachments", m.attachments)
	mux.HandleFunc("GET /api/v1/attachments/{id}/download", m.downloadAttachment)
	mux.HandleFunc("GET /api/v1/audit", m.auditEntries)
	mux.HandleFunc("GET /api/v1/exports/{kind}", m.exportRecords)
	mux.HandleFunc("GET /api/v1/voice/link", m.voiceLink)
}

func (m *Module) publishEvent(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	var request struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
		Kind    string `json:"kind"`
		Href    string `json:"href"`
	}
	if !decodeJSON(w, r, &request, 32<<10) {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	request.Title, request.Summary, request.Kind, request.Href = strings.TrimSpace(request.Title), strings.TrimSpace(request.Summary), strings.TrimSpace(request.Kind), strings.TrimSpace(request.Href)
	if key == "" || request.Title == "" || request.Kind == "" || !strings.HasPrefix(request.Href, "/") {
		writeError(w, http.StatusBadRequest, "invalid_event", "Event title, kind, workspace link, and Idempotency-Key are required")
		return
	}
	if err := m.notify(r.Context(), scope, request.Title, request.Summary, request.Kind, request.Href, "event:"+key); err != nil {
		writeError(w, http.StatusInternalServerError, "event_publish_failed", "Could not publish event")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "event.published", "event", deterministicID(key), request.Title)
	writeJSON(w, http.StatusAccepted, map[string]string{"id": deterministicID("event:" + key), "status": "accepted"})
}

func (m *Module) SaveGoogleGrant(ctx context.Context, user Identity, token *oauth2.Token) error {
	payload, err := tokenJSON(token)
	if err != nil {
		return err
	}
	sealed, err := encrypt(m.key, payload)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	connection := GoogleConnection{ID: memberID(user.Email), UserEmail: strings.ToLower(user.Email), GoogleEmail: strings.ToLower(user.Email), EncryptedToken: sealed, CreatedAt: now, UpdatedAt: now}
	var existing GoogleConnection
	if err := m.store.Get(ctx, m.publicScope, "googleConnections", connection.ID, &existing); err == nil {
		connection.CreatedAt = existing.CreatedAt
		connection.Tiller = existing.Tiller
		connection.LastMailSyncAt = existing.LastMailSyncAt
	}
	if err := m.store.Put(ctx, m.publicScope, "googleConnections", connection.ID, connection); err != nil {
		return err
	}
	return m.audit(ctx, m.publicScope, user.Email, "google.connected", "integration", connection.ID, "Connected Google Workspace")
}

func (m *Module) CheckAccess(ctx context.Context, scope string, actor Identity, mutation bool) error {
	member, err := m.ensureMember(ctx, scope, actor)
	if err != nil {
		return err
	}
	if member.Status != "active" {
		return errors.New("membership is disabled")
	}
	if mutation && member.Role == "viewer" {
		return errors.New("viewer access is read only")
	}
	return nil
}

func (m *Module) CheckRole(ctx context.Context, scope string, actor Identity, roles ...string) error {
	member, err := m.ensureMember(ctx, scope, actor)
	if err != nil {
		return err
	}
	if member.Status != "active" || !oneOf(member.Role, roles...) {
		return errors.New("role is not allowed")
	}
	return nil
}

func (m *Module) members(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	writeStorePage[Member](w, r, m.store, scope, "members", "members", "members_load_failed", "Could not load team members", pagination.Spec{Key: "operations.members", OrderBy: "name", Direction: pagination.Ascending, ValueKind: pagination.StringValue})
}

func (m *Module) updateMember(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	var request struct{ Role, Status string }
	if !decodeJSON(w, r, &request, 8<<10) {
		return
	}
	request.Role = strings.ToLower(strings.TrimSpace(request.Role))
	request.Status = strings.ToLower(strings.TrimSpace(request.Status))
	if !oneOf(request.Role, "owner", "admin", "member", "viewer") || !oneOf(request.Status, "active", "disabled") {
		writeError(w, http.StatusBadRequest, "invalid_member", "Choose a valid role and status")
		return
	}
	var member Member
	if err := m.store.Get(r.Context(), scope, "members", r.PathValue("id"), &member); err != nil {
		writeError(w, http.StatusNotFound, "member_not_found", "Team member was not found")
		return
	}
	if member.Role == "owner" && member.ID == memberID(actor.Email) && (request.Role != "owner" || request.Status != "active") {
		writeError(w, http.StatusConflict, "owner_required", "You cannot remove or disable your own owner access")
		return
	}
	if member.Role == "owner" && (request.Role != "owner" || request.Status != "active") {
		var members []Member
		if err := m.store.List(r.Context(), scope, "members", &members); err != nil {
			writeError(w, http.StatusInternalServerError, "member_update_failed", "Could not verify organization ownership")
			return
		}
		activeOwners := 0
		for _, candidate := range members {
			if candidate.Role == "owner" && candidate.Status == "active" && candidate.ID != member.ID {
				activeOwners++
			}
		}
		if activeOwners == 0 {
			writeError(w, http.StatusConflict, "owner_required", "Assign another active owner before changing this owner")
			return
		}
	}
	member.Role, member.Status, member.UpdatedAt = request.Role, request.Status, time.Now().UTC()
	if err := m.store.Put(r.Context(), scope, "members", member.ID, member); err != nil {
		writeError(w, http.StatusInternalServerError, "member_update_failed", "Could not update team member")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "member.updated", "member", member.ID, "Changed "+member.Email+" to "+member.Role)
	writeJSON(w, http.StatusOK, member)
}

func (m *Module) sendAsMappings(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	var mappings []SendAsMapping
	if err := m.store.List(r.Context(), scope, "sendAsMappings", &mappings); err != nil {
		writeError(w, http.StatusInternalServerError, "send_as_load_failed", "Could not load Gmail sender mappings")
		return
	}
	sort.Slice(mappings, func(i, j int) bool { return mappings[i].MemberEmail < mappings[j].MemberEmail })
	writeJSON(w, http.StatusOK, map[string]any{"mappings": mappings})
}

func (m *Module) configureSendAs(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	var request struct{ Email string }
	if !decodeJSON(w, r, &request, 8<<10) {
		return
	}
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	address, err := mail.ParseAddress(request.Email)
	if err != nil || address.Address != request.Email {
		writeError(w, http.StatusBadRequest, "invalid_send_as", "Enter a valid Gmail send-as address")
		return
	}
	var member Member
	if err := m.store.Get(r.Context(), scope, "members", r.PathValue("id"), &member); err != nil {
		writeError(w, http.StatusNotFound, "member_not_found", "Team member was not found")
		return
	}
	token, connection, err := m.connectionTokenByID(r.Context(), scope, member.ID)
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusConflict, "google_not_connected", "This member must connect Google Workspace before assigning a sender address")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "google_connection_failed", "Could not open this member's Google connection")
		return
	}
	verified := strings.EqualFold(request.Email, connection.GoogleEmail)
	if !verified {
		aliases, aliasErr := m.google.SendAsAliases(r.Context(), token)
		if aliasErr != nil {
			writeError(w, http.StatusBadGateway, "send_as_check_failed", "Google could not verify this sender address. Reconnect Google Workspace and try again")
			return
		}
		for _, alias := range aliases {
			if strings.EqualFold(alias, request.Email) {
				verified = true
				break
			}
		}
	}
	if !verified {
		writeError(w, http.StatusBadRequest, "send_as_not_verified", "That address is not a verified Gmail send-as alias for this member")
		return
	}
	now := time.Now().UTC()
	mapping := SendAsMapping{ID: member.ID, MemberID: member.ID, MemberEmail: member.Email, Email: request.Email, UpdatedBy: actor.Email, CreatedAt: now, UpdatedAt: now}
	var existing SendAsMapping
	if m.store.Get(r.Context(), scope, "sendAsMappings", member.ID, &existing) == nil {
		mapping.CreatedAt = existing.CreatedAt
	}
	if err := m.store.Put(r.Context(), scope, "sendAsMappings", member.ID, mapping); err != nil {
		writeError(w, http.StatusInternalServerError, "send_as_save_failed", "Could not save Gmail sender mapping")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "gmail.send_as_mapped", "member", member.ID, "Mapped "+member.Email+" to "+request.Email)
	writeJSON(w, http.StatusOK, mapping)
}

func (m *Module) pipelineStages(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	if err := m.ensureStages(r.Context(), scope); err != nil {
		writeError(w, http.StatusInternalServerError, "stages_load_failed", "Could not load pipeline stages")
		return
	}
	writeStorePage[PipelineStage](w, r, m.store, scope, "pipelineStages", "stages", "stages_load_failed", "Could not load pipeline stages", pagination.Spec{Key: "operations.pipeline-stages", OrderBy: "position", Direction: pagination.Ascending, ValueKind: pagination.IntegerValue})
}

func (m *Module) createPipelineStage(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	var stage PipelineStage
	if !decodeJSON(w, r, &stage, 16<<10) {
		return
	}
	stage.Name = strings.TrimSpace(stage.Name)
	if stage.Name == "" || len(stage.Name) > 80 || stage.Probability < 0 || stage.Probability > 100 {
		writeError(w, http.StatusBadRequest, "invalid_stage", "Stage needs a name and a probability from 0 to 100")
		return
	}
	now := time.Now().UTC()
	stage.ID, stage.CreatedAt, stage.UpdatedAt = slug(stage.Name), now, now
	if err := m.store.Create(r.Context(), scope, "pipelineStages", stage.ID, stage); errors.Is(err, errAlreadyExists) {
		writeError(w, http.StatusConflict, "stage_exists", "A pipeline stage with this name already exists")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "stage_create_failed", "Could not save pipeline stage")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "pipeline_stage.created", "pipeline_stage", stage.ID, stage.Name)
	writeJSON(w, http.StatusCreated, stage)
}

func (m *Module) notifications(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	writeStorePage[Notification](w, r, m.store, scope, "notifications", "notifications", "notifications_load_failed", "Could not load notifications", pagination.Spec{Key: "operations.notifications", OrderBy: "createdAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue})
}

func (m *Module) readNotification(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	var item Notification
	if err := m.store.Get(r.Context(), scope, "notifications", r.PathValue("id"), &item); err != nil {
		writeError(w, http.StatusNotFound, "notification_not_found", "Notification was not found")
		return
	}
	now := time.Now().UTC()
	item.ReadAt = &now
	if err := m.store.Put(r.Context(), scope, "notifications", item.ID, item); err != nil {
		writeError(w, http.StatusInternalServerError, "notification_update_failed", "Could not update notification")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (m *Module) emailTemplates(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	writeStorePage[EmailTemplate](w, r, m.store, scope, "emailTemplates", "templates", "templates_load_failed", "Could not load email templates", pagination.Spec{Key: "operations.email-templates", OrderBy: "name", Direction: pagination.Ascending, ValueKind: pagination.StringValue})
}

func (m *Module) createEmailTemplate(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	var item EmailTemplate
	if !decodeJSON(w, r, &item, 128<<10) {
		return
	}
	item.Name, item.Subject = strings.TrimSpace(item.Name), strings.TrimSpace(item.Subject)
	if item.Name == "" || item.Subject == "" || strings.TrimSpace(item.Body) == "" || len(item.Body) > 100000 {
		writeError(w, http.StatusBadRequest, "invalid_template", "Template name, subject, and a body under 100,000 characters are required")
		return
	}
	now := time.Now().UTC()
	id, err := newID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "template_create_failed", "Could not save email template")
		return
	}
	item.ID, item.CreatedAt, item.UpdatedAt = id, now, now
	if err := m.store.Put(r.Context(), scope, "emailTemplates", item.ID, item); err != nil {
		writeError(w, http.StatusInternalServerError, "template_create_failed", "Could not save email template")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "email_template.created", "email_template", item.ID, item.Name)
	writeJSON(w, http.StatusCreated, item)
}

func (m *Module) googleStatus(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	var connection GoogleConnection
	err := m.store.Get(r.Context(), scope, "googleConnections", memberID(actor.Email), &connection)
	writeJSON(w, http.StatusOK, map[string]any{"connected": err == nil, "connection": nullableConnection(connection, err), "connectUrl": "/auth/connect/workspace"})
}

func (m *Module) sendEmail(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	var request struct{ To, Subject, Body string }
	if !decodeJSON(w, r, &request, 256<<10) {
		return
	}
	request.To, request.Subject = strings.TrimSpace(request.To), strings.TrimSpace(request.Subject)
	address, err := mail.ParseAddress(request.To)
	if err != nil || address.Address != request.To || request.Subject == "" || strings.ContainsAny(request.Subject, "\r\n") || request.Body == "" {
		writeError(w, http.StatusBadRequest, "invalid_email", "Recipient, subject, and message are required")
		return
	}
	token, connection, err := m.connectionToken(r.Context(), scope, actor.Email)
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusConflict, "google_not_connected", "Connect Google Workspace in Settings before sending email")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "google_connection_failed", "Could not open Google connection")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 200 {
		writeError(w, http.StatusBadRequest, "idempotency_key_required", "A valid Idempotency-Key header is required")
		return
	}
	deliveryID := deterministicID(strings.ToLower(actor.Email) + "|email|" + idempotencyKey)
	var previous EmailDelivery
	if err := m.store.Get(r.Context(), scope, "emailDeliveries", deliveryID, &previous); err == nil {
		if previous.Status == "sent" {
			writeJSON(w, http.StatusOK, map[string]string{"id": previous.MessageID, "status": "sent"})
			return
		}
		writeError(w, http.StatusConflict, "email_send_in_progress", "This email is already being sent")
		return
	}
	delivery := EmailDelivery{ID: deliveryID, Status: "sending", CreatedAt: time.Now().UTC()}
	if err := m.store.Create(r.Context(), scope, "emailDeliveries", deliveryID, delivery); errors.Is(err, errAlreadyExists) {
		writeError(w, http.StatusConflict, "email_send_in_progress", "This email is already being sent")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "email_send_failed", "Could not reserve this email delivery")
		return
	}
	from := connection.GoogleEmail
	if from == "" {
		from = strings.ToLower(actor.Email)
	}
	var mapping SendAsMapping
	if m.store.Get(r.Context(), scope, "sendAsMappings", memberID(actor.Email), &mapping) == nil {
		from = mapping.Email
	}
	messageID, err := m.google.Send(r.Context(), token, from, request.To, request.Subject, request.Body)
	if err != nil {
		_ = m.store.Delete(r.Context(), scope, "emailDeliveries", deliveryID)
		writeError(w, http.StatusBadGateway, "email_send_failed", "Google could not send this email")
		return
	}
	delivery.MessageID, delivery.Status = messageID, "sent"
	if err := m.store.Put(r.Context(), scope, "emailDeliveries", deliveryID, delivery); err != nil {
		writeError(w, http.StatusInternalServerError, "email_send_record_failed", "Email was sent, but Kosmos could not save its delivery record")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "email.sent", "gmail_message", messageID, "Sent email to "+request.To)
	_ = m.notify(r.Context(), scope, "Email sent", request.Subject, "email", "/communications", "sent:"+messageID)
	writeJSON(w, http.StatusCreated, map[string]string{"id": messageID, "status": "sent"})
}

func (m *Module) syncEmail(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	connectionID := memberID(actor.Email)
	var connection GoogleConnection
	if err := m.store.Get(r.Context(), scope, "googleConnections", connectionID, &connection); err != nil {
		writeError(w, http.StatusConflict, "google_not_connected", "Connect Google Workspace in Settings before syncing mail")
		return
	}
	job := Job{ID: m.manualJobID(r, scope, connectionID, JobTypeGmailSync), Type: JobTypeGmailSync, Scope: scope, ConnectionID: connectionID, Actor: actor.Email}
	if err := m.enqueueJob(r.Context(), job); err != nil {
		writeError(w, http.StatusServiceUnavailable, "email_sync_unavailable", "Could not queue mail sync")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": job.ID, "status": "accepted"})
}

func (m *Module) mailMessages(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	writeStorePage[MailMetadata](w, r, m.store, scope, "mailMetadata", "messages", "mail_load_failed", "Could not load mail notifications", pagination.Spec{Key: "operations.mail", OrderBy: "receivedAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue})
}

func (m *Module) configureTiller(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	var settings TillerSettings
	if !decodeJSON(w, r, &settings, 16<<10) {
		return
	}
	settings.SpreadsheetID, settings.Range = strings.TrimSpace(settings.SpreadsheetID), strings.TrimSpace(settings.Range)
	if settings.SpreadsheetID == "" {
		writeError(w, http.StatusBadRequest, "invalid_tiller_settings", "Tiller spreadsheet ID is required")
		return
	}
	if settings.Range == "" {
		settings.Range = "Transactions!A:Z"
	}
	_, connection, err := m.connectionToken(r.Context(), scope, actor.Email)
	if err != nil {
		writeError(w, http.StatusConflict, "google_not_connected", "Connect Google Workspace before configuring Tiller")
		return
	}
	connection.Tiller, connection.UpdatedAt = &settings, time.Now().UTC()
	if err := m.store.Put(r.Context(), scope, "googleConnections", connection.ID, connection); err != nil {
		writeError(w, http.StatusInternalServerError, "tiller_config_failed", "Could not save Tiller settings")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "tiller.configured", "integration", connection.ID, "Configured Tiller spreadsheet")
	writeJSON(w, http.StatusOK, connection)
}

func (m *Module) syncTiller(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	connectionID := memberID(actor.Email)
	var connection GoogleConnection
	if err := m.store.Get(r.Context(), scope, "googleConnections", connectionID, &connection); err != nil || connection.Tiller == nil {
		writeError(w, http.StatusConflict, "tiller_not_configured", "Connect Google and configure a Tiller spreadsheet first")
		return
	}
	job := Job{ID: m.manualJobID(r, scope, connectionID, JobTypeTillerSync), Type: JobTypeTillerSync, Scope: scope, ConnectionID: connectionID, Actor: actor.Email}
	if err := m.enqueueJob(r.Context(), job); err != nil {
		writeError(w, http.StatusServiceUnavailable, "tiller_sync_unavailable", "Could not queue Tiller sync")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": job.ID, "status": "accepted"})
}

func (m *Module) transactions(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	writeStorePage[Transaction](w, r, m.store, scope, "transactions", "transactions", "transactions_load_failed", "Could not load transactions", pagination.Spec{Key: "operations.transactions", OrderBy: "date", Direction: pagination.Descending, ValueKind: pagination.StringValue})
}

func (m *Module) updateTransaction(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	var request struct{ ContactID, CostID, MatchStatus string }
	if !decodeJSON(w, r, &request, 16<<10) {
		return
	}
	if !oneOf(request.MatchStatus, "matched", "review", "ignored") {
		writeError(w, http.StatusBadRequest, "invalid_transaction", "Choose matched, review, or ignored")
		return
	}
	var item Transaction
	if err := m.store.Get(r.Context(), scope, "transactions", r.PathValue("id"), &item); err != nil {
		writeError(w, http.StatusNotFound, "transaction_not_found", "Transaction was not found")
		return
	}
	item.ContactID, item.CostID, item.MatchStatus, item.UpdatedAt = request.ContactID, request.CostID, request.MatchStatus, time.Now().UTC()
	if err := m.store.Put(r.Context(), scope, "transactions", item.ID, item); err != nil {
		writeError(w, http.StatusInternalServerError, "transaction_update_failed", "Could not update transaction")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "transaction.reviewed", "transaction", item.ID, item.Description)
	writeJSON(w, http.StatusOK, item)
}

func (m *Module) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+1<<20)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_attachment", "Choose a file smaller than 10 MB")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_attachment", "A file is required")
		return
	}
	defer file.Close()
	contentType := detectedContentType(file, header)
	if !oneOf(contentType, "image/jpeg", "image/png", "image/webp", "application/pdf", "text/plain") {
		writeError(w, http.StatusBadRequest, "unsupported_attachment", "Upload a PDF, text file, JPEG, PNG, or WebP image")
		return
	}
	kind := normalizedOr(r.FormValue("kind"), "attachment")
	recordType := strings.TrimSpace(r.FormValue("recordType"))
	if !oneOf(kind, "attachment", "receipt") || recordType == "" || strings.TrimSpace(r.FormValue("recordId")) == "" {
		writeError(w, http.StatusBadRequest, "invalid_attachment", "Choose a supported file kind and record")
		return
	}
	if kind == "receipt" && recordType != "cost" {
		writeError(w, http.StatusBadRequest, "invalid_receipt", "Receipts must be linked to a cost")
		return
	}
	id, err := newID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "attachment_upload_failed", "Could not prepare attachment storage")
		return
	}
	objectName := scope + "/" + id + filepath.Ext(filepath.Base(header.Filename))
	if err := m.blobs.Put(r.Context(), objectName, contentType, file); err != nil {
		writeError(w, http.StatusInternalServerError, "attachment_upload_failed", "Could not store attachment")
		return
	}
	item := Attachment{ID: id, FileName: filepath.Base(header.Filename), ContentType: contentType, Size: header.Size, Kind: kind, RecordType: recordType, RecordID: strings.TrimSpace(r.FormValue("recordId")), ObjectName: objectName, CreatedBy: actor.Email, CreatedAt: time.Now().UTC()}
	if err := m.store.Put(r.Context(), scope, "attachments", item.ID, item); err != nil {
		writeError(w, http.StatusInternalServerError, "attachment_upload_failed", "Could not save attachment metadata")
		return
	}
	item.DownloadURL = m.downloadURL(scope, item.ID, time.Now().Add(15*time.Minute))
	_ = m.audit(r.Context(), scope, actor.Email, "attachment.uploaded", "attachment", item.ID, item.FileName)
	writeJSON(w, http.StatusCreated, item)
}

func (m *Module) attachments(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	recordType, recordID := r.URL.Query().Get("recordType"), r.URL.Query().Get("recordId")
	spec := pagination.Spec{Key: "operations.attachments:" + recordType + ":" + recordID, OrderBy: "createdAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue}
	if recordType != "" {
		spec.Filters = append(spec.Filters, pagination.Filter{Field: "recordType", Value: recordType})
	}
	if recordID != "" {
		spec.Filters = append(spec.Filters, pagination.Filter{Field: "recordId", Value: recordID})
	}
	items, metadata, ok := loadStorePage[Attachment](w, r, m.store, scope, "attachments", "attachments_load_failed", "Could not load attachments", spec)
	if !ok {
		return
	}
	for index := range items {
		items[index].DownloadURL = m.downloadURL(scope, items[index].ID, time.Now().Add(15*time.Minute))
	}
	writeJSON(w, http.StatusOK, map[string]any{"attachments": items, "page": metadata})
}

func (m *Module) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok || !m.validDownload(scope, r.PathValue("id"), r.URL.Query().Get("expires"), r.URL.Query().Get("signature")) {
		if ok {
			writeError(w, http.StatusForbidden, "invalid_download", "Download link is invalid or expired")
		}
		return
	}
	var item Attachment
	if err := m.store.Get(r.Context(), scope, "attachments", r.PathValue("id"), &item); err != nil {
		writeError(w, http.StatusNotFound, "attachment_not_found", "Attachment was not found")
		return
	}
	reader, err := m.blobs.Open(r.Context(), item.ObjectName)
	if err != nil {
		writeError(w, http.StatusNotFound, "attachment_not_found", "Attachment was not found")
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", item.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", item.FileName))
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(w, reader)
}

func (m *Module) auditEntries(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	writeStorePage[AuditEntry](w, r, m.store, scope, "audit", "entries", "audit_load_failed", "Could not load audit history", pagination.Spec{Key: "operations.audit", OrderBy: "createdAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue})
}

func (m *Module) exportRecords(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	switch r.PathValue("kind") {
	case "contacts":
		items, err := m.workspace.ListContacts(r.Context(), scope)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "export_failed", "Could not export contacts")
			return
		}
		_ = writer.Write([]string{"name", "account_id", "email", "phone", "source", "created_at"})
		for _, item := range items {
			_ = writer.Write([]string{item.Name, item.AccountID, item.Email, item.Phone, item.Source, item.CreatedAt.Format(time.RFC3339)})
		}
	case "costs":
		items, err := m.workspace.ListCosts(r.Context(), scope)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "export_failed", "Could not export costs")
			return
		}
		_ = writer.Write([]string{"date", "vendor", "description", "amount", "category", "recurring", "tax_deductible"})
		for _, item := range items {
			_ = writer.Write([]string{item.IncurredOn, item.Vendor, item.Description, fmt.Sprintf("%.2f", float64(item.AmountCents)/100), item.Category, strconv.FormatBool(item.Recurring), strconv.FormatBool(item.TaxDeductible)})
		}
	default:
		writeError(w, http.StatusNotFound, "export_not_found", "Choose contacts or costs")
		return
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		writeError(w, http.StatusInternalServerError, "export_failed", "Could not build export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=kosmos-"+r.PathValue("kind")+".csv")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output.Bytes())
	_ = m.audit(r.Context(), scope, actor.Email, "records.exported", "export", r.PathValue("kind"), "Exported "+r.PathValue("kind"))
}

func (m *Module) voiceLink(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := m.authorize(w, r); !ok {
		return
	}
	phone := strings.TrimSpace(r.URL.Query().Get("phone"))
	writeJSON(w, http.StatusOK, map[string]string{"googleVoiceUrl": "https://voice.google.com/u/0/messages", "callUrl": "tel:" + phone, "smsUrl": "sms:" + phone})
}

func (m *Module) intakeContact(w http.ResponseWriter, r *http.Request) {
	if !m.limiter.Allow(clientIP(r), time.Now()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Please wait before trying again")
		return
	}
	var request struct{ Name, Company, Email, Phone, Message, Source, Website string }
	if !decodeJSON(w, r, &request, 32<<10) {
		return
	}
	if request.Website != "" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	request.Name, request.Email = strings.TrimSpace(request.Name), strings.ToLower(strings.TrimSpace(request.Email))
	if request.Name == "" || request.Email == "" || len(request.Message) > 5000 {
		writeError(w, http.StatusBadRequest, "invalid_lead", "Name and email are required")
		return
	}
	address, err := mail.ParseAddress(request.Email)
	if err != nil || address.Address != request.Email {
		writeError(w, http.StatusBadRequest, "invalid_lead", "Enter a valid email address")
		return
	}
	contacts, err := m.workspace.ListContacts(r.Context(), m.publicScope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lead_create_failed", "Could not save this request")
		return
	}
	for _, contact := range contacts {
		if strings.EqualFold(contact.Email, request.Email) {
			w.WriteHeader(http.StatusAccepted)
			return
		}
	}
	source := normalizedOr(request.Source, "contact-form")
	contactInput := workspace.Contact{Name: request.Name, Email: request.Email, Phone: strings.TrimSpace(request.Phone), Source: source}
	var contact workspace.Contact
	if company := strings.TrimSpace(request.Company); company != "" {
		accounts, listErr := m.workspace.ListAccounts(r.Context(), m.publicScope)
		if listErr != nil {
			writeError(w, http.StatusInternalServerError, "lead_create_failed", "Could not save this request")
			return
		}
		for _, account := range accounts {
			if strings.EqualFold(account.Name, company) {
				contactInput.AccountID = account.ID
				break
			}
		}
		if contactInput.AccountID != "" {
			contact, err = m.workspace.CreateContact(r.Context(), m.publicScope, contactInput)
		} else {
			_, contact, err = m.workspace.CreateAccountWithContact(r.Context(), m.publicScope, workspace.Account{Name: company, Status: "prospect"}, contactInput)
		}
	} else {
		contact, err = m.workspace.CreateContact(r.Context(), m.publicScope, contactInput)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lead_create_failed", "Could not save this request")
		return
	}
	_ = m.notify(r.Context(), m.publicScope, "New lead", request.Name+" via "+source, "lead", "/contacts", "lead:"+contact.ID)
	_ = m.audit(r.Context(), m.publicScope, "public-contact-form", "lead.created", "contact", contact.ID, request.Name+" via "+source)
	writeJSON(w, http.StatusCreated, map[string]string{"id": contact.ID, "status": "received"})
}

func (m *Module) authorize(w http.ResponseWriter, r *http.Request) (string, Identity, bool) {
	scope, actor, err := m.identity(r)
	if err != nil || scope == "" {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return "", Identity{}, false
	}
	member, err := m.ensureMember(r.Context(), scope, actor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "membership_failed", "Could not load organization membership")
		return "", Identity{}, false
	}
	if member.Status != "active" {
		writeError(w, http.StatusForbidden, "membership_disabled", "Your organization access is disabled")
		return "", Identity{}, false
	}
	mutation := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
	if mutation && member.Role == "viewer" {
		writeError(w, http.StatusForbidden, "permission_denied", "Your role has read-only access")
		return "", Identity{}, false
	}
	return scope, actor, true
}

func (m *Module) ensureMember(ctx context.Context, scope string, actor Identity) (Member, error) {
	id := memberID(actor.Email)
	var member Member
	if err := m.store.Get(ctx, scope, "members", id, &member); err == nil {
		if member.Name != actor.Name {
			member.Name, member.UpdatedAt = actor.Name, time.Now().UTC()
			_ = m.store.Put(ctx, scope, "members", id, member)
		}
		return member, nil
	}
	now := time.Now().UTC()
	var bootstrap struct {
		Email string `json:"email" firestore:"email"`
	}
	err := m.store.Get(ctx, scope, "settings", "bootstrapOwner", &bootstrap)
	if errors.Is(err, errNotFound) {
		candidate := struct {
			Email string `json:"email" firestore:"email"`
		}{Email: strings.ToLower(actor.Email)}
		if createErr := m.store.Create(ctx, scope, "settings", "bootstrapOwner", candidate); createErr == nil {
			bootstrap = candidate
		} else if errors.Is(createErr, errAlreadyExists) {
			err = m.store.Get(ctx, scope, "settings", "bootstrapOwner", &bootstrap)
		} else {
			return Member{}, createErr
		}
	}
	if err != nil && !errors.Is(err, errNotFound) {
		return Member{}, err
	}
	role := "member"
	if strings.EqualFold(bootstrap.Email, actor.Email) {
		role = "owner"
	}
	member = Member{ID: id, Email: strings.ToLower(actor.Email), Name: actor.Name, Role: role, Status: "active", CreatedAt: now, UpdatedAt: now}
	return member, m.store.Put(ctx, scope, "members", id, member)
}

func (m *Module) requireRole(w http.ResponseWriter, ctx context.Context, scope string, actor Identity, roles ...string) bool {
	var member Member
	if err := m.store.Get(ctx, scope, "members", memberID(actor.Email), &member); err != nil || !oneOf(member.Role, roles...) {
		writeError(w, http.StatusForbidden, "permission_denied", "You do not have permission to manage team access")
		return false
	}
	return true
}

func (m *Module) ensureStages(ctx context.Context, scope string) error {
	var stages []PipelineStage
	_, err := m.store.ListPage(ctx, scope, "pipelineStages", pagination.Request{Limit: 1}, pagination.Spec{Key: "operations.pipeline-stages", OrderBy: "position", Direction: pagination.Ascending, ValueKind: pagination.IntegerValue}, &stages)
	if err != nil {
		return err
	}
	if len(stages) == 0 {
		now := time.Now().UTC()
		for position, value := range []struct {
			id, name    string
			probability int
			closed, won bool
		}{{"new", "New", 10, false, false}, {"qualified", "Qualified", 30, false, false}, {"proposal", "Proposal", 60, false, false}, {"won", "Won", 100, true, true}, {"lost", "Lost", 0, true, false}} {
			stage := PipelineStage{ID: value.id, Name: value.name, Position: position, Probability: value.probability, Closed: value.closed, Won: value.won, CreatedAt: now, UpdatedAt: now}
			if err := m.store.Put(ctx, scope, "pipelineStages", stage.ID, stage); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Module) connectionToken(ctx context.Context, scope, email string) (*oauth2.Token, GoogleConnection, error) {
	return m.connectionTokenByID(ctx, scope, memberID(email))
}

func (m *Module) connectionTokenByID(ctx context.Context, scope, connectionID string) (*oauth2.Token, GoogleConnection, error) {
	var connection GoogleConnection
	if err := m.store.Get(ctx, scope, "googleConnections", connectionID, &connection); err != nil {
		return nil, connection, err
	}
	payload, err := decrypt(m.key, connection.EncryptedToken)
	if err != nil {
		return nil, connection, err
	}
	token, err := parseToken(payload)
	return token, connection, err
}

func (m *Module) manualJobID(r *http.Request, scope, connectionID, jobType string) string {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		key = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return deterministicID("manual|" + scope + "|" + connectionID + "|" + jobType + "|" + key)
}

func (m *Module) syncEmailConnection(ctx context.Context, scope, connectionID, actor, jobID string) (int, error) {
	token, connection, err := m.connectionTokenByID(ctx, scope, connectionID)
	if err != nil {
		return 0, err
	}
	since := time.Now().UTC().Add(-30 * 24 * time.Hour)
	if connection.LastMailSyncAt != nil {
		since = connection.LastMailSyncAt.Add(-time.Minute)
	}
	messages, err := m.google.RecentMail(ctx, token, since)
	if err != nil {
		return 0, err
	}
	contacts, err := m.workspace.ListContacts(ctx, scope)
	if err != nil {
		return 0, err
	}
	createdIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		message.ContactID = matchSender(message.From, contacts)
		if message.ContactID == "" {
			continue
		}
		if err := m.store.Create(ctx, scope, "mailMetadata", message.ID, message); errors.Is(err, errAlreadyExists) {
			continue
		} else if err != nil {
			return len(createdIDs), err
		}
		if err := m.notify(ctx, scope, "New prospect email", message.Subject, "email", "/communications", "gmail:"+message.ID); err != nil {
			return len(createdIDs), err
		}
		createdIDs = append(createdIDs, message.ID)
	}
	now := time.Now().UTC()
	connection.LastMailSyncAt, connection.UpdatedAt = &now, now
	if err := m.store.Put(ctx, scope, "googleConnections", connection.ID, connection); err != nil {
		return len(createdIDs), err
	}
	if len(createdIDs) > 0 {
		sort.Strings(createdIDs)
		if err := m.auditJob(ctx, scope, actor, "email.synced", "integration", connection.ID, fmt.Sprintf("Synced %d relevant messages", len(createdIDs)), "gmail:"+strings.Join(createdIDs, "|")); err != nil {
			return len(createdIDs), err
		}
	}
	return len(createdIDs), nil
}

func (m *Module) syncTillerConnection(ctx context.Context, scope, connectionID, actor, jobID string) (int, int, error) {
	token, connection, err := m.connectionTokenByID(ctx, scope, connectionID)
	if err != nil {
		return 0, 0, err
	}
	if connection.Tiller == nil {
		return 0, 0, errNotFound
	}
	rows, err := m.google.TillerRows(ctx, token, *connection.Tiller)
	if err != nil {
		return 0, 0, err
	}
	contacts, err := m.workspace.ListContacts(ctx, scope)
	if err != nil {
		return 0, 0, err
	}
	accounts, err := m.workspace.ListAccounts(ctx, scope)
	if err != nil {
		return 0, 0, err
	}
	transactions := parseTillerRows(rows, contacts, accounts)
	createdIDs := make([]string, 0, len(transactions))
	for _, item := range transactions {
		if err := m.store.Create(ctx, scope, "transactions", item.ID, item); errors.Is(err, errAlreadyExists) {
			continue
		} else if err != nil {
			return len(createdIDs), len(transactions), err
		}
		createdIDs = append(createdIDs, item.ID)
	}
	if len(createdIDs) > 0 {
		sort.Strings(createdIDs)
		effectKey := "tiller:" + strings.Join(createdIDs, "|")
		if err := m.notify(ctx, scope, "Tiller import ready", fmt.Sprintf("%d new transactions imported", len(createdIDs)), "transaction", "/operations", effectKey); err != nil {
			return len(createdIDs), len(transactions), err
		}
		if err := m.auditJob(ctx, scope, actor, "tiller.synced", "integration", connection.ID, fmt.Sprintf("Imported %d transactions", len(createdIDs)), effectKey); err != nil {
			return len(createdIDs), len(transactions), err
		}
	}
	return len(createdIDs), len(transactions), nil
}

func (m *Module) auditJob(ctx context.Context, scope, actor, action, entityType, entityID, summary, jobID string) error {
	if jobID == "" {
		return m.audit(ctx, scope, actor, action, entityType, entityID, summary)
	}
	entry := AuditEntry{ID: deterministicID("job-audit|" + jobID), Actor: actor, Action: action, EntityType: entityType, EntityID: entityID, Summary: summary, CreatedAt: time.Now().UTC()}
	if err := m.store.Create(ctx, scope, "audit", entry.ID, entry); errors.Is(err, errAlreadyExists) {
		return nil
	} else {
		return err
	}
}

func (m *Module) notify(ctx context.Context, scope, title, summary, kind, href, key string) error {
	id := deterministicID(key)
	err := m.store.Create(ctx, scope, "notifications", id, Notification{ID: id, Title: title, Summary: summary, Kind: kind, Href: href, IdempotencyKey: key, CreatedAt: time.Now().UTC()})
	if errors.Is(err, errAlreadyExists) {
		return nil
	}
	return err
}

func (m *Module) audit(ctx context.Context, scope, actor, action, entityType, entityID, summary string) error {
	id, err := newID()
	if err != nil {
		return err
	}
	entry := AuditEntry{ID: id, Actor: actor, Action: action, EntityType: entityType, EntityID: entityID, Summary: summary, CreatedAt: time.Now().UTC()}
	return m.store.Put(ctx, scope, "audit", entry.ID, entry)
}

func (m *Module) downloadURL(scope, id string, expires time.Time) string {
	value := strconv.FormatInt(expires.Unix(), 10)
	signature := sign(m.key, scope+"|"+id+"|"+value)
	return "/api/v1/attachments/" + id + "/download?expires=" + value + "&signature=" + signature
}

func (m *Module) validDownload(scope, id, expires, signature string) bool {
	unix, err := strconv.ParseInt(expires, 10, 64)
	return err == nil && time.Now().Before(time.Unix(unix, 0)) && hmac.Equal([]byte(signature), []byte(sign(m.key, scope+"|"+id+"|"+expires)))
}

func parseTillerRows(rows [][]any, contacts []workspace.Contact, accounts []workspace.Account) []Transaction {
	if len(rows) < 2 {
		return []Transaction{}
	}
	headers := make(map[string]int)
	for index, value := range rows[0] {
		headers[strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))] = index
	}
	value := func(row []any, names ...string) string {
		for _, name := range names {
			if index, ok := headers[name]; ok && index < len(row) {
				return strings.TrimSpace(fmt.Sprint(row[index]))
			}
		}
		return ""
	}
	items := make([]Transaction, 0, len(rows)-1)
	for _, row := range rows[1:] {
		date, description := value(row, "date"), value(row, "description", "full description")
		amount, err := strconv.ParseFloat(strings.ReplaceAll(strings.ReplaceAll(value(row, "amount"), "$", ""), ",", ""), 64)
		if date == "" || description == "" || err != nil {
			continue
		}
		external := value(row, "transaction id", "id")
		if external == "" {
			external = date + "|" + description + "|" + fmt.Sprintf("%.2f", amount)
		}
		item := Transaction{ID: deterministicID("tiller:" + external), ExternalID: external, Date: date, Description: description, Merchant: value(row, "merchant"), AmountCents: int64(amount * 100), Source: "tiller", MatchStatus: "review", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		value := item.Merchant + " " + item.Description
		accountMatches := matchAccounts(value, accounts)
		if len(accountMatches) == 1 {
			item.AccountID, item.MatchStatus = accountMatches[0], "matched"
			for _, contact := range contacts {
				if contact.AccountID == item.AccountID {
					item.ContactID = contact.ID
					break
				}
			}
		} else if matches := matchContacts(value, contacts); len(matches) == 1 {
			item.ContactID, item.MatchStatus = matches[0], "matched"
		}
		items = append(items, item)
	}
	return items
}

func matchAccounts(value string, accounts []workspace.Account) []string {
	normalized := strings.ToLower(value)
	matches := make([]string, 0)
	for _, account := range accounts {
		candidates := []string{account.Name}
		for _, website := range account.Websites {
			candidates = append(candidates, website.Domain)
		}
		for _, candidate := range candidates {
			candidate = strings.ToLower(strings.TrimSpace(candidate))
			if len(candidate) >= 4 && strings.Contains(normalized, candidate) {
				matches = append(matches, account.ID)
				break
			}
		}
	}
	return matches
}

func matchContacts(value string, contacts []workspace.Contact) []string {
	normalized := strings.ToLower(value)
	matches := make([]string, 0)
	for _, contact := range contacts {
		for _, candidate := range []string{contact.Name, contact.Email} {
			candidate = strings.ToLower(strings.TrimSpace(candidate))
			if len(candidate) >= 4 && strings.Contains(normalized, candidate) {
				matches = append(matches, contact.ID)
				break
			}
		}
	}
	return matches
}

func matchSender(from string, contacts []workspace.Contact) string {
	address, err := mail.ParseAddress(from)
	if err != nil {
		return ""
	}
	for _, contact := range contacts {
		if strings.EqualFold(contact.Email, address.Address) {
			return contact.ID
		}
	}
	return ""
}

func detectedContentType(file multipart.File, header *multipart.FileHeader) string {
	buffer := make([]byte, 512)
	count, _ := file.Read(buffer)
	_, _ = file.Seek(0, io.SeekStart)
	detected := http.DetectContentType(buffer[:count])
	if strings.HasPrefix(detected, "text/plain") {
		return "text/plain"
	}
	if detected == "application/octet-stream" && strings.EqualFold(filepath.Ext(header.Filename), ".pdf") {
		return "application/pdf"
	}
	return detected
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Request body is invalid")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Request body must contain one JSON object")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func loadStorePage[T any](w http.ResponseWriter, r *http.Request, store Store, scope, collection, errorCode, errorMessage string, spec pagination.Spec) ([]T, pagination.Metadata, bool) {
	page, err := pagination.Parse(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pagination", err.Error())
		return nil, pagination.Metadata{}, false
	}
	items := make([]T, 0)
	metadata, err := store.ListPage(r.Context(), scope, collection, page, spec, &items)
	if errors.Is(err, pagination.ErrInvalidCursor) {
		writeError(w, http.StatusBadRequest, "invalid_pagination", err.Error())
		return nil, pagination.Metadata{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, errorCode, errorMessage)
		return nil, pagination.Metadata{}, false
	}
	return items, metadata, true
}

func writeStorePage[T any](w http.ResponseWriter, r *http.Request, store Store, scope, collection, key, errorCode, errorMessage string, spec pagination.Spec) {
	items, metadata, ok := loadStorePage[T](w, r, store, scope, collection, errorCode, errorMessage, spec)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{key: items, "page": metadata})
}

func newID() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func deterministicID(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:16])
}

func memberID(email string) string { return deterministicID(strings.ToLower(strings.TrimSpace(email))) }

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Trim(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, value), "-")
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func normalizedOr(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func nullableConnection(connection GoogleConnection, err error) any {
	if err != nil {
		return nil
	}
	return connection
}

func clientIP(r *http.Request) string {
	if value := strings.TrimSpace(strings.Split(r.Header.Get("CF-Connecting-IP"), ",")[0]); value != "" {
		return value
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

type ipLimiter struct {
	mu      sync.Mutex
	windows map[string][]time.Time
}

func newIPLimiter() *ipLimiter { return &ipLimiter{windows: make(map[string][]time.Time)} }

func (l *ipLimiter) Allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-time.Hour)
	recent := make([]time.Time, 0, 6)
	for _, seen := range l.windows[ip] {
		if seen.After(cutoff) {
			recent = append(recent, seen)
		}
	}
	if len(recent) >= 5 {
		l.windows[ip] = recent
		return false
	}
	l.windows[ip] = append(recent, now)
	return true
}
