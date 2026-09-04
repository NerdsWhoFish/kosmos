package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type memoryWorkspace struct {
	contacts      []Contact
	opportunities []Opportunity
	activities    []Activity
	reminders     []Reminder
	documents     []Document
	costs         []Cost
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
		workspace.reminders[index].Completed = *patch.Completed
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
	item.CreatedAt = now
	item.UpdatedAt = now
	workspace := s.workspace(scope)
	workspace.documents = append([]Document{item}, workspace.documents...)
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

func applyContactPatch(item *Contact, patch ContactPatch) {
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
}

func applyDocumentPatch(item *Document, patch DocumentPatch) {
	if patch.Title != nil {
		item.Title = *patch.Title
	}
	if patch.Body != nil {
		item.Body = *patch.Body
	}
}

func newID() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
