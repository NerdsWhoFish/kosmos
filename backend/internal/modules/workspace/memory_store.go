package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
)

type memoryWorkspace struct {
	accounts          []Account
	contacts          []Contact
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

func (s *MemoryStore) ListOpportunities(_ context.Context, scope string) ([]Opportunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Opportunity(nil), s.workspace(scope).opportunities...), nil
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

func (s *MemoryStore) ListCosts(_ context.Context, scope string) ([]Cost, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Cost(nil), s.workspace(scope).costs...), nil
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

func applyContactPatch(item *Contact, patch ContactPatch) {
	if patch.AccountID != nil {
		item.AccountID = *patch.AccountID
	}
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.Company != nil {
		item.Company = *patch.Company
	}
	if patch.Email != nil {
		item.Email = *patch.Email
	}
	if patch.Phone != nil {
		item.Phone = *patch.Phone
	}
	if patch.Status != nil {
		item.Status = *patch.Status
	}
	if patch.Source != nil {
		item.Source = *patch.Source
	}
}

func applyOpportunityPatch(item *Opportunity, patch OpportunityPatch) {
	if patch.Name != nil {
		item.Name = *patch.Name
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
