package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
)

var errNotFound = errors.New("record not found")

func (m Module) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/summary", m.summary)
	mux.HandleFunc("GET /api/v1/search", m.search)
	mux.HandleFunc("GET /api/v1/accounts", m.listAccounts)
	mux.HandleFunc("POST /api/v1/accounts", m.createAccount)
	mux.HandleFunc("GET /api/v1/accounts/{id}", m.getAccount)
	mux.HandleFunc("PATCH /api/v1/accounts/{id}", m.updateAccount)
	mux.HandleFunc("DELETE /api/v1/accounts/{id}", m.deleteAccount)
	mux.HandleFunc("GET /api/v1/leads", m.listLeads)
	mux.HandleFunc("GET /api/v1/contacts", m.listContacts)
	mux.HandleFunc("POST /api/v1/contacts", m.createContact)
	mux.HandleFunc("GET /api/v1/contacts/{id}", m.getContact)
	mux.HandleFunc("PATCH /api/v1/contacts/{id}", m.updateContact)
	mux.HandleFunc("DELETE /api/v1/contacts/{id}", m.deleteContact)
	mux.HandleFunc("GET /api/v1/contact-sources", m.listContactSources)
	mux.HandleFunc("POST /api/v1/contact-sources", m.createContactSource)
	mux.HandleFunc("GET /api/v1/opportunities", m.listOpportunities)
	mux.HandleFunc("POST /api/v1/opportunities", m.createOpportunity)
	mux.HandleFunc("PATCH /api/v1/opportunities/{id}", m.updateOpportunity)
	mux.HandleFunc("DELETE /api/v1/opportunities/{id}", m.deleteOpportunity)
	mux.HandleFunc("GET /api/v1/activities", m.listActivities)
	mux.HandleFunc("POST /api/v1/activities", m.createActivity)
	mux.HandleFunc("GET /api/v1/reminders", m.listReminders)
	mux.HandleFunc("POST /api/v1/reminders", m.createReminder)
	mux.HandleFunc("PATCH /api/v1/reminders/{id}", m.updateReminder)
	mux.HandleFunc("GET /api/v1/documents", m.listDocuments)
	mux.HandleFunc("POST /api/v1/documents", m.createDocument)
	mux.HandleFunc("PATCH /api/v1/documents/{id}", m.updateDocument)
	mux.HandleFunc("DELETE /api/v1/documents/{id}", m.deleteDocument)
	mux.HandleFunc("GET /api/v1/documents/{id}/revisions", m.documentRevisions)
	mux.HandleFunc("GET /api/v1/costs", m.listCosts)
	mux.HandleFunc("POST /api/v1/costs", m.createCost)
	mux.HandleFunc("PATCH /api/v1/costs/{id}", m.updateCost)
	mux.HandleFunc("DELETE /api/v1/costs/{id}", m.deleteCost)
}

func (m Module) listAccounts(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	respondStoreList[Account](w, r, m.store, scope, "accounts", "accounts", pagination.Spec{Key: "workspace.accounts", OrderBy: "updatedAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue})
}

func (m Module) createAccount(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var request struct {
		Account
		PrimaryContact *Contact `json:"primaryContact"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if err := normalizeAccount(&request.Account); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_account", err.Error())
		return
	}
	if message := validateAccount(request.Account); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_account", message)
		return
	}
	if request.PrimaryContact == nil {
		created, err := m.store.CreateAccount(r.Context(), scope, request.Account)
		respondCreated(w, map[string]any{"account": created, "contact": nil}, err, "account_save_failed", "Could not save account")
		return
	}
	normalizeContact(request.PrimaryContact)
	if message := validateContact(*request.PrimaryContact); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_contact", message)
		return
	}
	if err := m.ensureContactSource(r.Context(), scope, request.PrimaryContact.Source); err != nil {
		writeError(w, http.StatusInternalServerError, "contact_source_save_failed", "Could not save contact source")
		return
	}
	account, contact, err := m.store.CreateAccountWithContact(r.Context(), scope, request.Account, *request.PrimaryContact)
	if err == nil {
		m.publishContactMutation(r.Context(), scope, contact, "upsert")
	}
	respondCreated(w, map[string]any{"account": account, "contact": contact}, err, "account_save_failed", "Could not save account and contact")
}

func (m Module) getAccount(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	selected, err := m.store.GetAccount(r.Context(), scope, r.PathValue("id"))
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "account_not_found", "Account not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "account_load_failed", "Could not load account")
		return
	}
	contacts, err := m.store.ListContacts(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "account_load_failed", "Could not load account")
		return
	}
	linkedContacts := make([]Contact, 0)
	contactIDs := make(map[string]struct{})
	for _, contact := range contacts {
		if contact.AccountID == selected.ID {
			linkedContacts = append(linkedContacts, contact)
			contactIDs[contact.ID] = struct{}{}
		}
	}
	opportunities, err := m.store.ListOpportunities(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "account_load_failed", "Could not load account")
		return
	}
	linkedOpportunities := make([]Opportunity, 0)
	for _, opportunity := range opportunities {
		_, legacyContactLink := contactIDs[opportunity.ContactID]
		if opportunity.AccountID == selected.ID || (opportunity.AccountID == "" && legacyContactLink) {
			linkedOpportunities = append(linkedOpportunities, opportunity)
		}
	}
	documents, err := m.store.ListDocuments(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "account_load_failed", "Could not load account")
		return
	}
	linkedDocuments := make([]Document, 0)
	for _, document := range documents {
		for _, link := range document.Links {
			if link.Type == "account" && link.ID == selected.ID {
				linkedDocuments = append(linkedDocuments, document)
				break
			}
		}
	}
	sort.Slice(linkedDocuments, func(i, j int) bool { return linkedDocuments[i].CreatedAt.After(linkedDocuments[j].CreatedAt) })
	writeJSON(w, http.StatusOK, map[string]any{"account": selected, "contacts": linkedContacts, "opportunities": linkedOpportunities, "documents": linkedDocuments})
}

func (m Module) updateAccount(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var patch AccountPatch
	if !decodeJSON(w, r, &patch) {
		return
	}
	if err := normalizeAccountPatch(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_account", err.Error())
		return
	}
	if message := validateAccountPatch(patch); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_account", message)
		return
	}
	updated, err := m.store.UpdateAccount(r.Context(), scope, r.PathValue("id"), patch)
	respondUpdated(w, updated, err, "account_not_found", "Account not found", "account_save_failed", "Could not save account")
}

func (m Module) deleteAccount(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	contacts, err := m.store.DeleteAccount(r.Context(), scope, r.PathValue("id"))
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "account_not_found", "Account not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "account_delete_failed", "Could not delete account")
		return
	}
	for _, contact := range contacts {
		m.publishContactMutation(r.Context(), scope, contact, "delete")
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m Module) listLeads(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	respondStoreList[Contact](w, r, m.store, scope, "contacts", "leads", pagination.Spec{Key: "workspace.leads", OrderBy: "updatedAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue, Filters: []pagination.Filter{{Field: "accountId", Value: ""}}})
}

func (m Module) listContacts(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	respondStoreList[Contact](w, r, m.store, scope, "contacts", "contacts", pagination.Spec{Key: "workspace.contacts", OrderBy: "updatedAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue})
}

func (m Module) getContact(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	contact, err := m.store.GetContact(r.Context(), scope, r.PathValue("id"))
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "contact_not_found", "Contact not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "contact_load_failed", "Could not load contact")
		return
	}
	writeJSON(w, http.StatusOK, contact)
}

func (m Module) createContact(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var contact Contact
	if !decodeJSON(w, r, &contact) {
		return
	}
	normalizeContact(&contact)
	if message := validateContact(contact); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_contact", message)
		return
	}
	if contact.AccountID != "" {
		if _, err := m.store.GetAccount(r.Context(), scope, contact.AccountID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_contact", "Choose an existing account")
			return
		}
	}
	if err := m.ensureContactSource(r.Context(), scope, contact.Source); err != nil {
		writeError(w, http.StatusInternalServerError, "contact_source_save_failed", "Could not save contact source")
		return
	}
	created, err := m.store.CreateContact(r.Context(), scope, contact)
	if err == nil {
		m.publishContactMutation(r.Context(), scope, created, "upsert")
	}
	respondCreated(w, created, err, "contact_save_failed", "Could not save contact")
}

func (m Module) updateContact(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var patch ContactPatch
	if !decodeJSON(w, r, &patch) {
		return
	}
	normalizeContactPatch(&patch)
	if message := validateContactPatch(patch); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_contact", message)
		return
	}
	if patch.AccountID != nil && *patch.AccountID != "" {
		if _, err := m.store.GetAccount(r.Context(), scope, *patch.AccountID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_contact", "Choose an existing account")
			return
		}
	}
	if patch.Source != nil {
		if err := m.ensureContactSource(r.Context(), scope, *patch.Source); err != nil {
			writeError(w, http.StatusInternalServerError, "contact_source_save_failed", "Could not save contact source")
			return
		}
	}
	updated, err := m.store.UpdateContact(r.Context(), scope, r.PathValue("id"), patch)
	if err == nil {
		m.publishContactMutation(r.Context(), scope, updated, "upsert")
	}
	respondUpdated(w, updated, err, "contact_not_found", "Contact not found", "contact_save_failed", "Could not save contact")
}

func (m Module) deleteContact(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	contact, err := m.store.GetContact(r.Context(), scope, r.PathValue("id"))
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "contact_not_found", "Contact not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "contact_delete_failed", "Could not delete contact")
		return
	}
	if err := m.store.DeleteContact(r.Context(), scope, contact.ID); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "contact_not_found", "Contact not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "contact_delete_failed", "Could not delete contact")
		return
	}
	m.publishContactMutation(r.Context(), scope, contact, "delete")
	w.WriteHeader(http.StatusNoContent)
}

func (m Module) publishContactMutation(ctx context.Context, scope string, contact Contact, action string) {
	if m.contactMutation == nil {
		return
	}
	if err := m.contactMutation(ctx, scope, contact, action); err != nil {
		slog.ErrorContext(ctx, "Google contact mutation enqueue failed", "contact.id", contact.ID, "contact.action", action)
	}
}

func (m Module) listContactSources(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	items, err := m.store.ListContactSources(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "contact_sources_load_failed", "Could not load contact sources")
		return
	}
	items = mergeDefaultContactSources(items)
	page, err := pagination.Parse(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_page", err.Error())
		return
	}
	metadata, err := pagination.Apply(&items, page, pagination.Spec{Key: "workspace.contact-sources", OrderBy: "name", Direction: pagination.Ascending, ValueKind: pagination.StringValue})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_page", "Pagination cursor is invalid")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": items, "page": metadata})
}

func (m Module) createContactSource(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var item ContactSource
	if !decodeJSON(w, r, &item) {
		return
	}
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" || len(item.Name) > 80 {
		writeError(w, http.StatusBadRequest, "invalid_contact_source", "Source name must be between 1 and 80 characters")
		return
	}
	existing, err := m.store.ListContactSources(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "contact_source_save_failed", "Could not save contact source")
		return
	}
	for _, source := range mergeDefaultContactSources(existing) {
		if strings.EqualFold(source.Name, item.Name) {
			writeJSON(w, http.StatusOK, source)
			return
		}
	}
	created, err := m.store.CreateContactSource(r.Context(), scope, item)
	respondCreated(w, created, err, "contact_source_save_failed", "Could not save contact source")
}

func mergeDefaultContactSources(custom []ContactSource) []ContactSource {
	createdAt := time.Unix(0, 0).UTC()
	items := []ContactSource{
		{ID: "event", Name: "Event", CreatedAt: createdAt, UpdatedAt: createdAt},
		{ID: "outbound", Name: "Outbound", CreatedAt: createdAt, UpdatedAt: createdAt},
		{ID: "referral", Name: "Referral", CreatedAt: createdAt, UpdatedAt: createdAt},
		{ID: "social-media", Name: "Social media", CreatedAt: createdAt, UpdatedAt: createdAt},
		{ID: "website", Name: "Website", CreatedAt: createdAt, UpdatedAt: createdAt},
	}
	for _, source := range custom {
		duplicate := false
		for _, current := range items {
			if strings.EqualFold(current.Name, source.Name) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			items = append(items, source)
		}
	}
	return items
}

func (m Module) ensureContactSource(ctx context.Context, scope, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	items, err := m.store.ListContactSources(ctx, scope)
	if err != nil {
		return err
	}
	for _, source := range mergeDefaultContactSources(items) {
		if strings.EqualFold(source.Name, name) {
			return nil
		}
	}
	_, err = m.store.CreateContactSource(ctx, scope, ContactSource{Name: name})
	return err
}

func (m Module) listOpportunities(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	respondStoreList[Opportunity](w, r, m.store, scope, "opportunities", "opportunities", pagination.Spec{Key: "workspace.opportunities", OrderBy: "updatedAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue})
}

func (m Module) createOpportunity(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var item Opportunity
	if !decodeJSON(w, r, &item) {
		return
	}
	normalizeOpportunity(&item)
	if message := validateOpportunity(item); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_opportunity", message)
		return
	}
	if message := m.resolveOpportunityLinks(r.Context(), scope, &item); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_opportunity", message)
		return
	}
	if item.AccountID == "" {
		writeError(w, http.StatusBadRequest, "invalid_opportunity", "Choose an account for this opportunity")
		return
	}
	created, err := m.store.CreateOpportunity(r.Context(), scope, item)
	respondCreated(w, created, err, "opportunity_save_failed", "Could not save opportunity")
}

func (m Module) updateOpportunity(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var patch OpportunityPatch
	if !decodeJSON(w, r, &patch) {
		return
	}
	normalizeOpportunityPatch(&patch)
	if message := validateOpportunityPatch(patch); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_opportunity", message)
		return
	}
	existing, err := m.store.GetOpportunity(r.Context(), scope, r.PathValue("id"))
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "opportunity_not_found", "Opportunity not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "opportunity_save_failed", "Could not save opportunity")
		return
	}
	candidate := existing
	applyOpportunityPatch(&candidate, patch)
	if message := m.resolveOpportunityLinks(r.Context(), scope, &candidate); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_opportunity", message)
		return
	}
	if candidate.AccountID == "" {
		writeError(w, http.StatusBadRequest, "invalid_opportunity", "Choose an account for this opportunity")
		return
	}
	if patch.AccountID != nil {
		patch.AccountID = &candidate.AccountID
	} else if candidate.AccountID != existing.AccountID {
		patch.AccountID = &candidate.AccountID
	}
	updated, err := m.store.UpdateOpportunity(r.Context(), scope, r.PathValue("id"), patch)
	respondUpdated(w, updated, err, "opportunity_not_found", "Opportunity not found", "opportunity_save_failed", "Could not save opportunity")
}

func (m Module) deleteOpportunity(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	if err := m.store.DeleteOpportunity(r.Context(), scope, r.PathValue("id")); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "opportunity_not_found", "Opportunity not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "opportunity_delete_failed", "Could not delete opportunity")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m Module) resolveOpportunityLinks(ctx context.Context, scope string, item *Opportunity) string {
	if item.AccountID != "" {
		if _, err := m.store.GetAccount(ctx, scope, item.AccountID); err != nil {
			return "Choose an existing account"
		}
	}
	if item.ContactID == "" {
		return ""
	}
	contact, err := m.store.GetContact(ctx, scope, item.ContactID)
	if err != nil {
		return "Choose an existing contact"
	}
	if item.AccountID == "" {
		item.AccountID = contact.AccountID
		return ""
	}
	if contact.AccountID != item.AccountID {
		return "Choose a contact from the selected account"
	}
	return ""
}

func (m Module) listActivities(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	contactID := r.URL.Query().Get("contactId")
	spec := pagination.Spec{Key: "workspace.activities:" + contactID, OrderBy: "occurredAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue}
	if contactID != "" {
		spec.Filters = []pagination.Filter{{Field: "contactId", Value: contactID}}
	}
	respondStoreList[Activity](w, r, m.store, scope, "activities", "activities", spec)
}

func (m Module) createActivity(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var item Activity
	if !decodeJSON(w, r, &item) {
		return
	}
	item.Body = strings.TrimSpace(item.Body)
	item.Kind = strings.ToLower(strings.TrimSpace(item.Kind))
	if item.Kind == "" {
		item.Kind = "note"
	}
	if item.OccurredAt.IsZero() {
		item.OccurredAt = time.Now().UTC()
	}
	if item.Body == "" || len(item.Body) > 4000 || !contains([]string{"note", "call", "email", "meeting"}, item.Kind) {
		writeError(w, http.StatusBadRequest, "invalid_activity", "Add a note of 4,000 characters or fewer")
		return
	}
	created, err := m.store.CreateActivity(r.Context(), scope, item)
	respondCreated(w, created, err, "activity_save_failed", "Could not save activity")
}

func (m Module) listReminders(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	respondStoreList[Reminder](w, r, m.store, scope, "reminders", "reminders", pagination.Spec{Key: "workspace.reminders", OrderBy: "dueAt", Direction: pagination.Ascending, ValueKind: pagination.TimeValue})
}

func (m Module) createReminder(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var item Reminder
	if !decodeJSON(w, r, &item) {
		return
	}
	item.AccountID = strings.TrimSpace(item.AccountID)
	item.ContactID = strings.TrimSpace(item.ContactID)
	item.SourceKey = strings.TrimSpace(item.SourceKey)
	item.Title = strings.TrimSpace(item.Title)
	item.OwnerEmail = strings.ToLower(strings.TrimSpace(item.OwnerEmail))
	if item.Title == "" || len(item.Title) > 160 || item.DueAt.IsZero() {
		writeError(w, http.StatusBadRequest, "invalid_reminder", "A title and due date are required")
		return
	}
	created, err := m.store.CreateReminder(r.Context(), scope, item)
	respondCreated(w, created, err, "reminder_save_failed", "Could not save reminder")
}

func (m Module) updateReminder(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var patch ReminderPatch
	if !decodeJSON(w, r, &patch) {
		return
	}
	trimPointer(patch.OwnerEmail, true)
	if patch.Completed == nil && patch.OwnerEmail == nil {
		writeError(w, http.StatusBadRequest, "invalid_reminder", "Reminder completion state or owner is required")
		return
	}
	updated, err := m.store.UpdateReminder(r.Context(), scope, r.PathValue("id"), patch)
	respondUpdated(w, updated, err, "reminder_not_found", "Reminder not found", "reminder_save_failed", "Could not save reminder")
}

func (m Module) listDocuments(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	respondStoreList[Document](w, r, m.store, scope, "documents", "documents", pagination.Spec{Key: "workspace.documents", OrderBy: "updatedAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue})
}

func (m Module) createDocument(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var item Document
	if !decodeJSON(w, r, &item) {
		return
	}
	normalizeDocument(&item)
	if message := validateDocument(item); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_document", message)
		return
	}
	created, err := m.store.CreateDocument(r.Context(), scope, item)
	respondCreated(w, created, err, "document_save_failed", "Could not save document")
}

func (m Module) updateDocument(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var patch DocumentPatch
	if !decodeJSON(w, r, &patch) {
		return
	}
	if patch.Title != nil {
		value := strings.TrimSpace(*patch.Title)
		patch.Title = &value
	}
	if patch.Title != nil && (*patch.Title == "" || len(*patch.Title) > 160) {
		writeError(w, http.StatusBadRequest, "invalid_document", "Document title must be between 1 and 160 characters")
		return
	}
	if patch.Body != nil && len(*patch.Body) > 100000 {
		writeError(w, http.StatusBadRequest, "invalid_document", "Document body must be 100,000 characters or fewer")
		return
	}
	documents, err := m.store.ListDocuments(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "document_save_failed", "Could not save document")
		return
	}
	for _, current := range documents {
		if current.ID == r.PathValue("id") {
			_, err = m.store.CreateDocumentRevision(r.Context(), scope, DocumentRevision{DocumentID: current.ID, Title: current.Title, Body: current.Body, Links: current.Links, Revision: current.Revision})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "document_save_failed", "Could not save document history")
				return
			}
			break
		}
	}
	updated, err := m.store.UpdateDocument(r.Context(), scope, r.PathValue("id"), patch)
	respondUpdated(w, updated, err, "document_not_found", "Document not found", "document_save_failed", "Could not save document")
}

func (m Module) deleteDocument(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	if err := m.store.DeleteDocument(r.Context(), scope, r.PathValue("id")); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "document_not_found", "Document not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "document_delete_failed", "Could not delete document")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m Module) documentRevisions(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	documentID := r.PathValue("id")
	respondStoreList[DocumentRevision](w, r, m.store, scope, "documentRevisions", "revisions", pagination.Spec{Key: "workspace.document-revisions:" + documentID, OrderBy: "createdAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue, Filters: []pagination.Filter{{Field: "documentId", Value: documentID}}})
}

func (m Module) listCosts(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	respondStoreList[Cost](w, r, m.store, scope, "costs", "costs", pagination.Spec{Key: "workspace.costs", OrderBy: "incurredOn", Direction: pagination.Descending, ValueKind: pagination.StringValue})
}

func (m Module) createCost(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var item Cost
	if !decodeJSON(w, r, &item) {
		return
	}
	normalizeCost(&item)
	if message := validateCost(item); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_cost", message)
		return
	}
	created, err := m.store.CreateCost(r.Context(), scope, item)
	respondCreated(w, created, err, "cost_save_failed", "Could not save cost")
}

func (m Module) updateCost(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	var patch CostPatch
	if !decodeJSON(w, r, &patch) {
		return
	}
	normalizeCostPatch(&patch)
	if message := validateCostPatch(patch); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_cost", message)
		return
	}
	updated, err := m.store.UpdateCost(r.Context(), scope, r.PathValue("id"), patch)
	respondUpdated(w, updated, err, "cost_not_found", "Cost not found", "cost_save_failed", "Could not save cost")
}

func (m Module) deleteCost(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := m.store.GetCost(r.Context(), scope, id); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "cost_not_found", "Cost not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "cost_delete_failed", "Could not delete cost")
		return
	}
	if m.costDeletion != nil {
		if err := m.costDeletion(r.Context(), scope, id); err != nil {
			writeError(w, http.StatusInternalServerError, "cost_delete_failed", "Could not delete cost attachments")
			return
		}
	}
	if err := m.store.DeleteCost(r.Context(), scope, id); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "cost_not_found", "Cost not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "cost_delete_failed", "Could not delete cost")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type summaryResponse struct {
	Contacts              int        `json:"contacts"`
	OpenOpportunities     int        `json:"openOpportunities"`
	PipelineAmountCents   int64      `json:"pipelineAmountCents"`
	WonOpportunities      int        `json:"wonOpportunities"`
	WonAmountCents        int64      `json:"wonAmountCents"`
	LostOpportunities     int        `json:"lostOpportunities"`
	LostAmountCents       int64      `json:"lostAmountCents"`
	FollowUpsDue          int        `json:"followUpsDue"`
	CurrentMonthCostCents int64      `json:"currentMonthCostCents"`
	RecentActivities      []Activity `json:"recentActivities"`
}

func (m Module) summary(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	contacts, err := m.store.ListContacts(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "summary_load_failed", "Could not load workspace summary")
		return
	}
	opportunities, err := m.store.ListOpportunities(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "summary_load_failed", "Could not load workspace summary")
		return
	}
	reminders, err := m.store.ListReminders(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "summary_load_failed", "Could not load workspace summary")
		return
	}
	costs, err := m.store.ListCosts(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "summary_load_failed", "Could not load workspace summary")
		return
	}
	activities, err := m.store.ListActivities(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "summary_load_failed", "Could not load workspace summary")
		return
	}

	now := time.Now().UTC()
	reminderCutoff := now.Add(7 * 24 * time.Hour)
	if activities == nil {
		activities = []Activity{}
	}
	response := summaryResponse{Contacts: len(contacts), RecentActivities: activities}
	if len(response.RecentActivities) > 5 {
		response.RecentActivities = response.RecentActivities[:5]
	}
	for _, opportunity := range opportunities {
		switch opportunity.Stage {
		case "won":
			response.WonOpportunities++
			response.WonAmountCents += opportunity.AmountCents
		case "lost":
			response.LostOpportunities++
			response.LostAmountCents += opportunity.AmountCents
		default:
			response.OpenOpportunities++
			response.PipelineAmountCents += opportunity.AmountCents
		}
	}
	for _, reminder := range reminders {
		if !reminder.Completed && !reminder.DueAt.After(reminderCutoff) {
			response.FollowUpsDue++
		}
	}
	month := now.Format("2006-01")
	for _, cost := range costs {
		if strings.HasPrefix(cost.IncurredOn, month) {
			response.CurrentMonthCostCents += cost.AmountCents
		}
	}
	writeJSON(w, http.StatusOK, response)
}

type searchResult struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Href     string `json:"href"`
}

func (m Module) search(w http.ResponseWriter, r *http.Request) {
	scope, ok := m.requireScope(w, r)
	if !ok {
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if query == "" {
		writeJSON(w, http.StatusOK, map[string][]searchResult{"results": {}})
		return
	}
	results := make([]searchResult, 0)
	accounts, err := m.store.ListAccounts(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed", "Could not search workspace")
		return
	}
	accountNames := make(map[string]string, len(accounts))
	for _, item := range accounts {
		accountNames[item.ID] = item.Name
		websiteValues := make([]string, 0, len(item.Websites)*2+len(item.Links)*2)
		for _, website := range accountWebsites(item) {
			websiteValues = append(websiteValues, website.URL, website.Domain)
		}
		for _, link := range item.Links {
			websiteValues = append(websiteValues, link.Label, link.URL)
		}
		if matches(query, append([]string{item.Name, item.BillingEmail, item.Notes}, websiteValues...)...) {
			results = append(results, searchResult{ID: item.ID, Kind: "account", Title: item.Name, Subtitle: titleCase(item.Status), Href: "/accounts/" + item.ID})
		}
	}
	contacts, err := m.store.ListContacts(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed", "Could not search workspace")
		return
	}
	for _, item := range contacts {
		accountName := accountNames[item.AccountID]
		if matches(query, item.Name, accountName, item.Email, item.Phone) {
			results = append(results, searchResult{ID: item.ID, Kind: "contact", Title: item.Name, Subtitle: firstNonEmpty(accountName, item.Email), Href: "/contacts/" + item.ID})
		}
	}
	opportunities, err := m.store.ListOpportunities(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed", "Could not search workspace")
		return
	}
	for _, item := range opportunities {
		if matches(query, item.Name, item.Stage, item.NextStep) {
			results = append(results, searchResult{ID: item.ID, Kind: "opportunity", Title: item.Name, Subtitle: titleCase(item.Stage), Href: "/opportunities"})
		}
	}
	documents, err := m.store.ListDocuments(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed", "Could not search workspace")
		return
	}
	for _, item := range documents {
		if matches(query, item.Title, item.Body) {
			results = append(results, searchResult{ID: item.ID, Kind: "document", Title: item.Title, Subtitle: "Document", Href: "/documents"})
		}
	}
	costs, err := m.store.ListCosts(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed", "Could not search workspace")
		return
	}
	for _, item := range costs {
		if matches(query, item.Vendor, item.Description, item.Category) {
			results = append(results, searchResult{ID: item.ID, Kind: "cost", Title: item.Description, Subtitle: item.Vendor, Href: "/operations"})
		}
	}
	if len(results) > 25 {
		results = results[:25]
	}
	writeJSON(w, http.StatusOK, map[string][]searchResult{"results": results})
}

func titleCase(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func (m Module) requireScope(w http.ResponseWriter, r *http.Request) (string, bool) {
	scope, err := m.scope(r)
	if err != nil || scope == "" {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return "", false
	}
	return scope, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
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

func respondStoreList[T any](w http.ResponseWriter, r *http.Request, store Store, scope, collection, key string, spec pagination.Spec) {
	page, err := pagination.Parse(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pagination", err.Error())
		return
	}
	items := make([]T, 0)
	metadata, err := store.ListPage(r.Context(), scope, collection, page, spec, &items)
	if errors.Is(err, pagination.ErrInvalidCursor) {
		writeError(w, http.StatusBadRequest, "invalid_pagination", err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, key+"_load_failed", "Could not load "+key)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{key: items, "page": metadata})
}

func respondCreated[T any](w http.ResponseWriter, item T, err error, code, message string) {
	if err != nil {
		writeError(w, http.StatusInternalServerError, code, message)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func respondUpdated[T any](w http.ResponseWriter, item T, err error, notFoundCode, notFoundMessage, failedCode, failedMessage string) {
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, notFoundCode, notFoundMessage)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, failedCode, failedMessage)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func normalizeContact(contact *Contact) {
	contact.AccountID = strings.TrimSpace(contact.AccountID)
	contact.Name = strings.TrimSpace(contact.Name)
	contact.Email = strings.ToLower(strings.TrimSpace(contact.Email))
	contact.Phone = strings.TrimSpace(contact.Phone)
	contact.LinkedInURL = normalizeLinkedInURL(contact.LinkedInURL)
	contact.Source = strings.TrimSpace(contact.Source)
}

func normalizeContactPatch(patch *ContactPatch) {
	trimPointer(patch.AccountID, false)
	trimPointer(patch.Name, false)
	trimPointer(patch.Email, true)
	trimPointer(patch.Phone, false)
	if patch.LinkedInURL != nil {
		*patch.LinkedInURL = normalizeLinkedInURL(*patch.LinkedInURL)
	}
	trimPointer(patch.Source, false)
}

func validateContact(contact Contact) string {
	if contact.Name == "" || len(contact.Name) > 160 {
		return "Contact name must be between 1 and 160 characters"
	}
	if contact.Email != "" {
		address, err := mail.ParseAddress(contact.Email)
		if err != nil || !strings.EqualFold(address.Address, contact.Email) {
			return "Enter a valid email address"
		}
	}
	if contact.LinkedInURL != "" {
		parsed, err := url.Parse(contact.LinkedInURL)
		if err != nil || parsed.Scheme != "https" || (parsed.Hostname() != "linkedin.com" && !strings.HasSuffix(parsed.Hostname(), ".linkedin.com")) || parsed.User != nil || parsed.Path == "" || parsed.Path == "/" {
			return "Enter a valid LinkedIn profile URL"
		}
	}
	return ""
}

func validateContactPatch(patch ContactPatch) string {
	contact := Contact{Name: "Valid"}
	if patch.Name != nil {
		contact.Name = *patch.Name
	}
	if patch.Email != nil {
		contact.Email = *patch.Email
	}
	if patch.LinkedInURL != nil {
		contact.LinkedInURL = *patch.LinkedInURL
	}
	return validateContact(contact)
}

func normalizeLinkedInURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	return value
}

func normalizeOpportunity(item *Opportunity) {
	item.Name = strings.TrimSpace(item.Name)
	item.AccountID = strings.TrimSpace(item.AccountID)
	item.ContactID = strings.TrimSpace(item.ContactID)
	item.Stage = strings.ToLower(strings.TrimSpace(item.Stage))
	item.NextStep = strings.TrimSpace(item.NextStep)
	item.CloseDate = strings.TrimSpace(item.CloseDate)
	item.OwnerEmail = strings.ToLower(strings.TrimSpace(item.OwnerEmail))
	if item.Stage == "" {
		item.Stage = "new"
	}
}

func normalizeOpportunityPatch(patch *OpportunityPatch) {
	trimPointer(patch.Name, false)
	trimPointer(patch.AccountID, false)
	trimPointer(patch.ContactID, false)
	trimPointer(patch.Stage, true)
	trimPointer(patch.NextStep, false)
	trimPointer(patch.CloseDate, false)
	trimPointer(patch.OwnerEmail, true)
}

func validateOpportunity(item Opportunity) string {
	if item.Name == "" || len(item.Name) > 160 {
		return "Opportunity name must be between 1 and 160 characters"
	}
	if item.AmountCents < 0 {
		return "Opportunity amount cannot be negative"
	}
	if item.Stage == "" || len(item.Stage) > 80 {
		return "Choose a valid pipeline stage"
	}
	if item.CloseDate != "" {
		if _, err := time.Parse(time.DateOnly, item.CloseDate); err != nil {
			return "Close date must be a valid date"
		}
	}
	if item.OwnerEmail != "" {
		address, err := mail.ParseAddress(item.OwnerEmail)
		if err != nil || !strings.EqualFold(address.Address, item.OwnerEmail) {
			return "Opportunity owner must be a valid email address"
		}
	}
	return ""
}

func validateOpportunityPatch(patch OpportunityPatch) string {
	item := Opportunity{Name: "Valid", Stage: "new"}
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.AmountCents != nil {
		item.AmountCents = *patch.AmountCents
	}
	if patch.Stage != nil {
		item.Stage = *patch.Stage
	}
	if patch.CloseDate != nil {
		item.CloseDate = *patch.CloseDate
	}
	return validateOpportunity(item)
}

func normalizeDocument(item *Document) {
	item.Title = strings.TrimSpace(item.Title)
}

func validateDocument(item Document) string {
	if item.Title == "" || len(item.Title) > 160 {
		return "Document title must be between 1 and 160 characters"
	}
	if len(item.Body) > 100000 {
		return "Document body must be 100,000 characters or fewer"
	}
	for _, link := range item.Links {
		if !contains([]string{"account", "contact", "opportunity", "cost", "document"}, link.Type) || strings.TrimSpace(link.ID) == "" {
			return "Document links must reference a supported record"
		}
	}
	return ""
}

func normalizeCost(item *Cost) {
	item.Vendor = strings.TrimSpace(item.Vendor)
	item.Description = strings.TrimSpace(item.Description)
	item.Category = strings.TrimSpace(item.Category)
	item.IncurredOn = strings.TrimSpace(item.IncurredOn)
	item.Recurrence = strings.ToLower(strings.TrimSpace(item.Recurrence))
	item.Notes = strings.TrimSpace(item.Notes)
	item.RenewalDate = strings.TrimSpace(item.RenewalDate)
	item.PaymentMethod = strings.TrimSpace(item.PaymentMethod)
	item.ReviewState = strings.ToLower(strings.TrimSpace(item.ReviewState))
	if item.ReviewState == "" {
		item.ReviewState = "ready"
	}
}

func normalizeCostPatch(patch *CostPatch) {
	trimPointer(patch.Vendor, false)
	trimPointer(patch.Description, false)
	trimPointer(patch.Category, false)
	trimPointer(patch.IncurredOn, false)
	trimPointer(patch.Recurrence, true)
	trimPointer(patch.Notes, false)
	trimPointer(patch.RenewalDate, false)
	trimPointer(patch.PaymentMethod, false)
	trimPointer(patch.ReviewState, true)
}

func validateCostPatch(patch CostPatch) string {
	item := Cost{Description: "Valid", IncurredOn: time.Now().Format(time.DateOnly), ReviewState: "ready"}
	if patch.Vendor != nil {
		item.Vendor = *patch.Vendor
	}
	if patch.Description != nil {
		item.Description = *patch.Description
	}
	if patch.AmountCents != nil {
		item.AmountCents = *patch.AmountCents
	}
	if patch.Category != nil {
		item.Category = *patch.Category
	}
	if patch.IncurredOn != nil {
		item.IncurredOn = *patch.IncurredOn
	}
	if patch.Recurring != nil {
		item.Recurring = *patch.Recurring
	}
	if patch.Recurrence != nil {
		item.Recurrence = *patch.Recurrence
	}
	if patch.TaxDeductible != nil {
		item.TaxDeductible = *patch.TaxDeductible
	}
	if patch.Notes != nil {
		item.Notes = *patch.Notes
	}
	if patch.RenewalDate != nil {
		item.RenewalDate = *patch.RenewalDate
	}
	if patch.PaymentMethod != nil {
		item.PaymentMethod = *patch.PaymentMethod
	}
	if patch.ReviewState != nil {
		item.ReviewState = *patch.ReviewState
	}
	if patch.Recurrence != nil && *patch.Recurrence != "" && !contains([]string{"monthly", "quarterly", "yearly"}, *patch.Recurrence) {
		return "Choose monthly, quarterly, or yearly recurrence"
	}
	return validateCost(item)
}

func validateCost(item Cost) string {
	if item.Description == "" || len(item.Description) > 200 {
		return "Cost description must be between 1 and 200 characters"
	}
	if item.AmountCents < 0 {
		return "Cost amount cannot be negative"
	}
	if _, err := time.Parse(time.DateOnly, item.IncurredOn); err != nil {
		return "Cost date must be a valid date"
	}
	if item.Recurring && !contains([]string{"monthly", "quarterly", "yearly"}, item.Recurrence) {
		return "Choose monthly, quarterly, or yearly recurrence"
	}
	if item.RenewalDate != "" {
		if _, err := time.Parse(time.DateOnly, item.RenewalDate); err != nil {
			return "Renewal date must be a valid date"
		}
	}
	if !contains([]string{"ready", "review", "complete"}, item.ReviewState) {
		return "Choose a valid review state"
	}
	return ""
}

func trimPointer(value *string, lower bool) {
	if value == nil {
		return
	}
	trimmed := strings.TrimSpace(*value)
	if lower {
		trimmed = strings.ToLower(trimmed)
	}
	*value = trimmed
}

func filterActivities(items []Activity, contactID string) []Activity {
	filtered := make([]Activity, 0)
	for _, item := range items {
		if item.ContactID == contactID {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func matches(query string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func sortActivities(items []Activity) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].OccurredAt.Equal(items[j].OccurredAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].OccurredAt.After(items[j].OccurredAt)
	})
}

func sortAccounts(items []Account) {
	sort.Slice(items, func(i, j int) bool {
		return newerFirst(items[i].UpdatedAt, items[i].ID, items[j].UpdatedAt, items[j].ID)
	})
}

func sortContacts(items []Contact) {
	sort.Slice(items, func(i, j int) bool {
		return newerFirst(items[i].UpdatedAt, items[i].ID, items[j].UpdatedAt, items[j].ID)
	})
}

func sortOpportunities(items []Opportunity) {
	sort.Slice(items, func(i, j int) bool {
		return newerFirst(items[i].UpdatedAt, items[i].ID, items[j].UpdatedAt, items[j].ID)
	})
}

func sortReminders(items []Reminder) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].DueAt.Equal(items[j].DueAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].DueAt.Before(items[j].DueAt)
	})
}

func sortDocuments(items []Document) {
	sort.Slice(items, func(i, j int) bool {
		return newerFirst(items[i].UpdatedAt, items[i].ID, items[j].UpdatedAt, items[j].ID)
	})
}

func sortDocumentRevisions(items []DocumentRevision) {
	sort.Slice(items, func(i, j int) bool {
		return newerFirst(items[i].CreatedAt, items[i].ID, items[j].CreatedAt, items[j].ID)
	})
}

func sortCosts(items []Cost) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].IncurredOn == items[j].IncurredOn {
			return items[i].ID < items[j].ID
		}
		return items[i].IncurredOn > items[j].IncurredOn
	})
}

func newerFirst(leftTime time.Time, leftID string, rightTime time.Time, rightID string) bool {
	if leftTime.Equal(rightTime) {
		return leftID < rightID
	}
	return leftTime.After(rightTime)
}
