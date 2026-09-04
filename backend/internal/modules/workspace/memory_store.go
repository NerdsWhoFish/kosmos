package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
)

type memoryWorkspace struct {
	accounts          []Account
	accountEvents     []AccountEvent
	contacts          []Contact
	contactSources    []ContactSource
	opportunities     []Opportunity
	activities        []Activity
	reminders         []Reminder
	documents         []Document
	documentRevisions []DocumentRevision
	costs             []Cost
}

func (s *MemoryStore) ListAccounts(_ context.Context, scope string) ([]Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Account(nil), s.workspace(scope).accounts...), nil
}

func (s *MemoryStore) GetAccount(_ context.Context, scope, id string) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.workspace(scope).accounts {
		if item.ID == id {
			return item, nil
		}
	}
	return Account{}, errNotFound
}

func (s *MemoryStore) CreateAccount(_ context.Context, scope string, item Account) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	id, err := newID()
	if err != nil {
		return Account{}, err
	}
	item.ID, item.CreatedAt, item.UpdatedAt = id, now, now
	workspace := s.workspace(scope)
	workspace.accounts = append([]Account{item}, workspace.accounts...)
	return item, nil
}

func (s *MemoryStore) CreateAccountWithContact(_ context.Context, scope string, account Account, contact Contact) (Account, Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	accountID, err := newID()
	if err != nil {
		return Account{}, Contact{}, err
	}
	contactID, err := newID()
	if err != nil {
		return Account{}, Contact{}, err
	}
	account.ID, account.CreatedAt, account.UpdatedAt = accountID, now, now
	contact.ID, contact.AccountID, contact.CreatedAt, contact.UpdatedAt = contactID, accountID, now, now
	workspace := s.workspace(scope)
	workspace.accounts = append([]Account{account}, workspace.accounts...)
	workspace.contacts = append([]Contact{contact}, workspace.contacts...)
	return account, contact, nil
}

func (s *MemoryStore) UpdateAccount(_ context.Context, scope, id string, patch AccountPatch) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	for index := range workspace.accounts {
		if workspace.accounts[index].ID != id {
			continue
		}
		if patch.Websites != nil {
			websites := preserveManagedWebsiteMetadata(accountWebsites(workspace.accounts[index]), *patch.Websites)
			patch.Websites = &websites
		}
		applyAccountPatch(&workspace.accounts[index], patch)
		workspace.accounts[index].UpdatedAt = time.Now().UTC()
		return workspace.accounts[index], nil
	}
	return Account{}, errNotFound
}

func (s *MemoryStore) DeleteAccount(_ context.Context, scope, id string) ([]Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	accountIndex := -1
	for index := range workspace.accounts {
		if workspace.accounts[index].ID == id {
			accountIndex = index
			break
		}
	}
	if accountIndex < 0 {
		return nil, errNotFound
	}

	deletedContacts := make([]Contact, 0)
	contactIDs := make(map[string]struct{})
	keptContacts := workspace.contacts[:0]
	for _, contact := range workspace.contacts {
		if contact.AccountID == id {
			deletedContacts = append(deletedContacts, contact)
			contactIDs[contact.ID] = struct{}{}
			continue
		}
		keptContacts = append(keptContacts, contact)
	}
	workspace.contacts = keptContacts

	opportunityIDs := make(map[string]struct{})
	keptOpportunities := workspace.opportunities[:0]
	for _, opportunity := range workspace.opportunities {
		if opportunity.AccountID == id {
			opportunityIDs[opportunity.ID] = struct{}{}
			continue
		}
		keptOpportunities = append(keptOpportunities, opportunity)
	}
	workspace.opportunities = keptOpportunities

	keptActivities := workspace.activities[:0]
	for _, activity := range workspace.activities {
		_, contactDeleted := contactIDs[activity.ContactID]
		_, opportunityDeleted := opportunityIDs[activity.OpportunityID]
		if !contactDeleted && !opportunityDeleted {
			keptActivities = append(keptActivities, activity)
		}
	}
	workspace.activities = keptActivities

	keptReminders := workspace.reminders[:0]
	for _, reminder := range workspace.reminders {
		_, contactDeleted := contactIDs[reminder.ContactID]
		if reminder.AccountID != id && !contactDeleted {
			keptReminders = append(keptReminders, reminder)
		}
	}
	workspace.reminders = keptReminders

	deletedDocuments := make(map[string]struct{})
	keptDocuments := workspace.documents[:0]
	for _, document := range workspace.documents {
		links := remainingAccountLinks(document.Links, id, contactIDs, opportunityIDs)
		if len(links) == 0 && len(links) != len(document.Links) {
			deletedDocuments[document.ID] = struct{}{}
			continue
		}
		document.Links = links
		keptDocuments = append(keptDocuments, document)
	}
	workspace.documents = keptDocuments
	keptRevisions := workspace.documentRevisions[:0]
	for _, revision := range workspace.documentRevisions {
		if _, deleted := deletedDocuments[revision.DocumentID]; !deleted {
			keptRevisions = append(keptRevisions, revision)
		}
	}
	workspace.documentRevisions = keptRevisions
	keptEvents := workspace.accountEvents[:0]
	for _, event := range workspace.accountEvents {
		if event.AccountID != id {
			keptEvents = append(keptEvents, event)
		}
	}
	workspace.accountEvents = keptEvents
	workspace.accounts = append(workspace.accounts[:accountIndex], workspace.accounts[accountIndex+1:]...)
	return deletedContacts, nil
}

func (s *MemoryStore) CreateAccountEvent(_ context.Context, scope string, item AccountEvent) (AccountEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	if item.ID != "" {
		for _, existing := range workspace.accountEvents {
			if existing.ID == item.ID && existing.AccountID == item.AccountID {
				return existing, nil
			}
		}
	} else {
		id, err := newID()
		if err != nil {
			return AccountEvent{}, err
		}
		item.ID = id
	}
	now := time.Now().UTC()
	if item.OccurredAt.IsZero() {
		item.OccurredAt = now
	}
	item.CreatedAt = now
	workspace.accountEvents = append(workspace.accountEvents, item)
	return item, nil
}

func (s *MemoryStore) ListAccountEventsPage(_ context.Context, scope, accountID string, request pagination.Request, kind string) ([]AccountEvent, pagination.Metadata, error) {
	s.mu.Lock()
	items := append([]AccountEvent(nil), s.workspace(scope).accountEvents...)
	s.mu.Unlock()
	spec := pagination.Spec{Key: "workspace.account-events:" + accountID + ":" + kind, OrderBy: "occurredAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue, Filters: []pagination.Filter{{Field: "accountId", Value: accountID}}}
	if kind != "" {
		spec.Filters = append(spec.Filters, pagination.Filter{Field: "kind", Value: kind})
	}
	metadata, err := pagination.Apply(&items, request, spec)
	return items, metadata, err
}

func remainingAccountLinks(links []RecordLink, accountID string, contactIDs, opportunityIDs map[string]struct{}) []RecordLink {
	kept := make([]RecordLink, 0, len(links))
	for _, link := range links {
		_, deletedContact := contactIDs[link.ID]
		_, deletedOpportunity := opportunityIDs[link.ID]
		if (link.Type == "account" && link.ID == accountID) || (link.Type == "contact" && deletedContact) || (link.Type == "opportunity" && deletedOpportunity) {
			continue
		}
		kept = append(kept, link)
	}
	return kept
}

func (s *MemoryStore) LinkWebsiteRenewal(_ context.Context, scope, id string, website Website, reminders []Reminder) (Account, []Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	accountIndex := -1
	for index := range workspace.accounts {
		if workspace.accounts[index].ID == id {
			accountIndex = index
			break
		}
	}
	if accountIndex < 0 {
		return Account{}, nil, errNotFound
	}
	account := &workspace.accounts[accountIndex]
	account.Websites = mergeWebsite(accountWebsites(*account), website)
	account.Website = account.Websites[0].URL
	account.UpdatedAt = time.Now().UTC()
	prefix := "cloudflare:" + website.Domain + ":"
	desired := make(map[string]struct{}, len(reminders))
	for _, reminder := range reminders {
		desired[reminder.ID] = struct{}{}
	}
	kept := workspace.reminders[:0]
	for _, reminder := range workspace.reminders {
		_, current := desired[reminder.ID]
		if reminder.AccountID == id && strings.HasPrefix(reminder.SourceKey, prefix) && !current {
			continue
		}
		kept = append(kept, reminder)
	}
	workspace.reminders = kept
	created := make([]Reminder, 0, len(reminders))
	for _, reminder := range reminders {
		found := false
		for _, existing := range workspace.reminders {
			if existing.ID == reminder.ID {
				created = append(created, existing)
				found = true
				break
			}
		}
		if found {
			continue
		}
		now := time.Now().UTC()
		reminder.CreatedAt, reminder.UpdatedAt = now, now
		workspace.reminders = append(workspace.reminders, reminder)
		created = append(created, reminder)
	}
	return *account, created, nil
}

type MemoryStore struct {
	mu         sync.Mutex
	workspaces map[string]*memoryWorkspace
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{workspaces: make(map[string]*memoryWorkspace)}
}

func (s *MemoryStore) workspace(scope string) *memoryWorkspace {
	workspace, ok := s.workspaces[scope]
	if !ok {
		workspace = &memoryWorkspace{}
		s.workspaces[scope] = workspace
	}
	return workspace
}

func (s *MemoryStore) ListPage(ctx context.Context, scope, collection string, request pagination.Request, spec pagination.Spec, target any) (pagination.Metadata, error) {
	var source any
	var err error
	switch collection {
	case "accounts":
		source, err = s.ListAccounts(ctx, scope)
	case "contacts":
		source, err = s.ListContacts(ctx, scope)
	case "contactSources":
		source, err = s.ListContactSources(ctx, scope)
	case "opportunities":
		source, err = s.ListOpportunities(ctx, scope)
	case "activities":
		source, err = s.ListActivities(ctx, scope)
	case "reminders":
		source, err = s.ListReminders(ctx, scope)
	case "documents":
		source, err = s.ListDocuments(ctx, scope)
	case "documentRevisions":
		s.mu.Lock()
		source = append([]DocumentRevision(nil), s.workspace(scope).documentRevisions...)
		s.mu.Unlock()
	case "costs":
		source, err = s.ListCosts(ctx, scope)
	default:
		return pagination.Metadata{}, errors.New("unknown workspace collection")
	}
	if err != nil {
		return pagination.Metadata{}, err
	}
	targetValue := reflect.ValueOf(target)
	sourceValue := reflect.ValueOf(source)
	if targetValue.Kind() != reflect.Pointer || targetValue.Elem().Type() != sourceValue.Type() {
		return pagination.Metadata{}, errors.New("pagination target does not match collection")
	}
	targetValue.Elem().Set(sourceValue)
	return pagination.Apply(target, request, spec)
}

func (s *MemoryStore) ListContacts(_ context.Context, scope string) ([]Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Contact(nil), s.workspace(scope).contacts...), nil
}

func (s *MemoryStore) GetContact(_ context.Context, scope, id string) (Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.workspace(scope).contacts {
		if item.ID == id {
			return item, nil
		}
	}
	return Contact{}, errNotFound
}

func (s *MemoryStore) ListContactSources(_ context.Context, scope string) ([]ContactSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ContactSource(nil), s.workspace(scope).contactSources...), nil
}

func (s *MemoryStore) CreateContactSource(_ context.Context, scope string, item ContactSource) (ContactSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := newID()
	if err != nil {
		return ContactSource{}, err
	}
	now := time.Now().UTC()
	item.ID, item.CreatedAt, item.UpdatedAt = id, now, now
	workspace := s.workspace(scope)
	workspace.contactSources = append(workspace.contactSources, item)
	return item, nil
}

func (s *MemoryStore) CreateContact(_ context.Context, scope string, item Contact) (Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	id, err := newID()
	if err != nil {
		return Contact{}, err
	}
	item.ID = id
	item.CreatedAt = now
	item.UpdatedAt = now
	workspace := s.workspace(scope)
	workspace.contacts = append([]Contact{item}, workspace.contacts...)
	return item, nil
}

func (s *MemoryStore) UpdateContact(_ context.Context, scope, id string, patch ContactPatch) (Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	for index := range workspace.contacts {
		if workspace.contacts[index].ID != id {
			continue
		}
		applyContactPatch(&workspace.contacts[index], patch)
		workspace.contacts[index].UpdatedAt = time.Now().UTC()
		return workspace.contacts[index], nil
	}
	return Contact{}, errNotFound
}

func (s *MemoryStore) DeleteContact(_ context.Context, scope, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	for index := range workspace.contacts {
		if workspace.contacts[index].ID != id {
			continue
		}
		workspace.contacts = append(workspace.contacts[:index], workspace.contacts[index+1:]...)
		return nil
	}
	return errNotFound
}

func (s *MemoryStore) ListOpportunities(_ context.Context, scope string) ([]Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Opportunity(nil), s.workspace(scope).opportunities...), nil
}

func (s *MemoryStore) GetOpportunity(_ context.Context, scope, id string) (Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.workspace(scope).opportunities {
		if item.ID == id {
			return item, nil
		}
	}
	return Opportunity{}, errNotFound
}

func (s *MemoryStore) CreateOpportunity(_ context.Context, scope string, item Opportunity) (Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	id, err := newID()
	if err != nil {
		return Opportunity{}, err
	}
	item.ID = id
	item.CreatedAt = now
	item.UpdatedAt = now
	workspace := s.workspace(scope)
	workspace.opportunities = append([]Opportunity{item}, workspace.opportunities...)
	return item, nil
}

func (s *MemoryStore) UpdateOpportunity(_ context.Context, scope, id string, patch OpportunityPatch) (Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	for index := range workspace.opportunities {
		if workspace.opportunities[index].ID != id {
			continue
		}
		applyOpportunityPatch(&workspace.opportunities[index], patch)
		workspace.opportunities[index].UpdatedAt = time.Now().UTC()
		return workspace.opportunities[index], nil
	}
	return Opportunity{}, errNotFound
}

func (s *MemoryStore) DeleteOpportunity(_ context.Context, scope, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	for index := range workspace.opportunities {
		if workspace.opportunities[index].ID != id {
			continue
		}
		workspace.opportunities = append(workspace.opportunities[:index], workspace.opportunities[index+1:]...)
		return nil
	}
	return errNotFound
}

func (s *MemoryStore) ListActivities(_ context.Context, scope string) ([]Activity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := append([]Activity(nil), s.workspace(scope).activities...)
	sortActivities(items)
	return items, nil
}

func (s *MemoryStore) CreateActivity(_ context.Context, scope string, item Activity) (Activity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := newID()
	if err != nil {
		return Activity{}, err
	}
	item.ID = id
	item.CreatedAt = time.Now().UTC()
	workspace := s.workspace(scope)
	workspace.activities = append([]Activity{item}, workspace.activities...)
	return item, nil
}

func (s *MemoryStore) ListReminders(_ context.Context, scope string) ([]Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Reminder(nil), s.workspace(scope).reminders...), nil
}

func (s *MemoryStore) CreateReminder(_ context.Context, scope string, item Reminder) (Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	id, err := newID()
	if err != nil {
		return Reminder{}, err
	}
	item.ID = id
	item.CreatedAt = now
	item.UpdatedAt = now
	workspace := s.workspace(scope)
	workspace.reminders = append(workspace.reminders, item)
	return item, nil
}

func (s *MemoryStore) UpdateReminder(_ context.Context, scope, id string, patch ReminderPatch) (Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	for index := range workspace.reminders {
		if workspace.reminders[index].ID != id {
			continue
		}
		if patch.Completed != nil {
			workspace.reminders[index].Completed = *patch.Completed
		}
		if patch.OwnerEmail != nil {
			workspace.reminders[index].OwnerEmail = *patch.OwnerEmail
		}
		workspace.reminders[index].UpdatedAt = time.Now().UTC()
		return workspace.reminders[index], nil
	}
	return Reminder{}, errNotFound
}

func (s *MemoryStore) ListDocuments(_ context.Context, scope string) ([]Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Document(nil), s.workspace(scope).documents...), nil
}

func (s *MemoryStore) CreateDocument(_ context.Context, scope string, item Document) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	id, err := newID()
	if err != nil {
		return Document{}, err
	}
	item.ID = id
	item.Revision = 1
	item.CreatedAt = now
	item.UpdatedAt = now
	workspace := s.workspace(scope)
	workspace.documents = append([]Document{item}, workspace.documents...)
	return item, nil
}

func (s *MemoryStore) SyncManagedDocument(_ context.Context, scope, sourceKey string, item Document) (Document, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.workspace(scope)
	id := managedDocumentID(sourceKey)
	now := time.Now().UTC()
	for index := range current.documents {
		if current.documents[index].ID != id {
			continue
		}
		existing := current.documents[index]
		if existing.Title == item.Title && existing.Body == item.Body && reflect.DeepEqual(existing.Links, item.Links) {
			return existing, false, nil
		}
		revisionID, err := newID()
		if err != nil {
			return Document{}, false, err
		}
		current.documentRevisions = append([]DocumentRevision{{ID: revisionID, DocumentID: existing.ID, Title: existing.Title, Body: existing.Body, Links: existing.Links, Revision: existing.Revision, CreatedAt: now}}, current.documentRevisions...)
		existing.SourceKey = sourceKey
		existing.Title = item.Title
		existing.Body = item.Body
		existing.Links = item.Links
		existing.Revision++
		existing.UpdatedAt = now
		current.documents[index] = existing
		return existing, false, nil
	}
	item.ID = id
	item.SourceKey = sourceKey
	item.Revision = 1
	item.CreatedAt = now
	item.UpdatedAt = now
	current.documents = append([]Document{item}, current.documents...)
	return item, true, nil
}

func (s *MemoryStore) ListDocumentRevisions(_ context.Context, scope, documentID string) ([]DocumentRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]DocumentRevision, 0)
	for _, item := range s.workspace(scope).documentRevisions {
		if item.DocumentID == documentID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *MemoryStore) CreateDocumentRevision(_ context.Context, scope string, item DocumentRevision) (DocumentRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := newID()
	if err != nil {
		return DocumentRevision{}, err
	}
	item.ID, item.CreatedAt = id, time.Now().UTC()
	workspace := s.workspace(scope)
	workspace.documentRevisions = append([]DocumentRevision{item}, workspace.documentRevisions...)
	return item, nil
}

func (s *MemoryStore) UpdateDocument(_ context.Context, scope, id string, patch DocumentPatch) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	for index := range workspace.documents {
		if workspace.documents[index].ID != id {
			continue
		}
		applyDocumentPatch(&workspace.documents[index], patch)
		workspace.documents[index].UpdatedAt = time.Now().UTC()
		return workspace.documents[index], nil
	}
	return Document{}, errNotFound
}

func (s *MemoryStore) DeleteDocument(_ context.Context, scope, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	found := false
	for index := range workspace.documents {
		if workspace.documents[index].ID != id {
			continue
		}
		workspace.documents = append(workspace.documents[:index], workspace.documents[index+1:]...)
		found = true
		break
	}
	if !found {
		return errNotFound
	}
	kept := workspace.documentRevisions[:0]
	for _, revision := range workspace.documentRevisions {
		if revision.DocumentID != id {
			kept = append(kept, revision)
		}
	}
	workspace.documentRevisions = kept
	return nil
}

func (s *MemoryStore) ListCosts(_ context.Context, scope string) ([]Cost, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Cost(nil), s.workspace(scope).costs...), nil
}

func (s *MemoryStore) GetCost(_ context.Context, scope, id string) (Cost, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.workspace(scope).costs {
		if item.ID == id {
			return item, nil
		}
	}
	return Cost{}, errNotFound
}

func (s *MemoryStore) CreateCost(_ context.Context, scope string, item Cost) (Cost, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	id, err := newID()
	if err != nil {
		return Cost{}, err
	}
	item.ID = id
	item.CreatedAt = now
	item.UpdatedAt = now
	workspace := s.workspace(scope)
	workspace.costs = append([]Cost{item}, workspace.costs...)
	return item, nil
}

func (s *MemoryStore) UpdateCost(_ context.Context, scope, id string, patch CostPatch) (Cost, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	for index := range workspace.costs {
		if workspace.costs[index].ID != id {
			continue
		}
		applyCostPatch(&workspace.costs[index], patch)
		workspace.costs[index].UpdatedAt = time.Now().UTC()
		return workspace.costs[index], nil
	}
	return Cost{}, errNotFound
}

func (s *MemoryStore) DeleteCost(_ context.Context, scope, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace := s.workspace(scope)
	for index := range workspace.costs {
		if workspace.costs[index].ID == id {
			workspace.costs = append(workspace.costs[:index], workspace.costs[index+1:]...)
			return nil
		}
	}
	return errNotFound
}

func applyContactPatch(item *Contact, patch ContactPatch) {
	if patch.AccountID != nil {
		item.AccountID = *patch.AccountID
	}
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.Email != nil {
		item.Email = *patch.Email
	}
	if patch.Phone != nil {
		item.Phone = *patch.Phone
	}
	if patch.LinkedInURL != nil {
		item.LinkedInURL = *patch.LinkedInURL
	}
	if patch.Source != nil {
		item.Source = *patch.Source
	}
}

func applyOpportunityPatch(item *Opportunity, patch OpportunityPatch) {
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.AccountID != nil {
		item.AccountID = *patch.AccountID
	}
	if patch.ContactID != nil {
		item.ContactID = *patch.ContactID
	}
	if patch.AmountCents != nil {
		item.AmountCents = *patch.AmountCents
	}
	if patch.Stage != nil {
		item.Stage = *patch.Stage
	}
	if patch.NextStep != nil {
		item.NextStep = *patch.NextStep
	}
	if patch.CloseDate != nil {
		item.CloseDate = *patch.CloseDate
	}
	if patch.OwnerEmail != nil {
		item.OwnerEmail = *patch.OwnerEmail
	}
}

func applyCostPatch(item *Cost, patch CostPatch) {
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
}

func applyDocumentPatch(item *Document, patch DocumentPatch) {
	if patch.Title != nil {
		item.Title = *patch.Title
	}
	if patch.Body != nil {
		item.Body = *patch.Body
	}
	if patch.Links != nil {
		item.Links = *patch.Links
	}
	item.Revision++
}

func newID() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
