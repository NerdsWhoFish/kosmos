package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ContactMutation struct {
	ID        string    `json:"id" firestore:"id"`
	ContactID string    `json:"contactId" firestore:"contactId"`
	Version   time.Time `json:"version" firestore:"version"`
	Action    string    `json:"action" firestore:"action"`
	CreatedAt time.Time `json:"createdAt" firestore:"createdAt"`
}

type ContactMutationStore interface {
	ListContactMutations(context.Context, string) ([]ContactMutation, error)
	GetContactMutation(context.Context, string, string) (ContactMutation, bool, error)
	PutContactMutation(context.Context, string, ContactMutation) error
	CompleteContactMutation(context.Context, string, string) error
}

func NewContactMutation(contact Contact, action string) ContactMutation {
	hash := sha256.Sum256([]byte(contact.ID + "|" + action + "|" + contact.UpdatedAt.UTC().Format(time.RFC3339Nano)))
	return ContactMutation{ID: hex.EncodeToString(hash[:16]), ContactID: contact.ID, Version: contact.UpdatedAt, Action: action, CreatedAt: time.Now().UTC()}
}

func (w *memoryWorkspace) recordContactMutation(contact Contact, action string) {
	if w.contactMutations == nil {
		w.contactMutations = make(map[string]ContactMutation)
	}
	mutation := NewContactMutation(contact, action)
	w.contactMutations[mutation.ID] = mutation
}

func (s *MemoryStore) ListContactMutations(_ context.Context, scope string) ([]ContactMutation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]ContactMutation, 0, len(s.workspace(scope).contactMutations))
	for _, item := range s.workspace(scope).contactMutations {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (s *MemoryStore) CompleteContactMutation(_ context.Context, scope, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workspace(scope).contactMutations, id)
	return nil
}

func (s *MemoryStore) GetContactMutation(_ context.Context, scope, id string) (ContactMutation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, found := s.workspace(scope).contactMutations[id]
	return item, found, nil
}

func (s *MemoryStore) PutContactMutation(_ context.Context, scope string, mutation ContactMutation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.workspace(scope)
	if w.contactMutations == nil {
		w.contactMutations = make(map[string]ContactMutation)
	}
	w.contactMutations[mutation.ID] = mutation
	return nil
}

func (s *FirestoreStore) ListContactMutations(ctx context.Context, scope string) ([]ContactMutation, error) {
	iter := s.collection(scope, "contactMutationOutbox").OrderBy(firestore.DocumentID, firestore.Asc).Documents(ctx)
	defer iter.Stop()
	items := make([]ContactMutation, 0)
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return items, nil
		}
		if err != nil {
			return nil, err
		}
		var mutation ContactMutation
		if err := doc.DataTo(&mutation); err != nil {
			return nil, err
		}
		items = append(items, mutation)
	}
}

func (s *FirestoreStore) CompleteContactMutation(ctx context.Context, scope, id string) error {
	_, err := s.collection(scope, "contactMutationOutbox").Doc(id).Delete(ctx)
	return err
}

func (s *FirestoreStore) PutContactMutation(ctx context.Context, scope string, mutation ContactMutation) error {
	_, err := s.collection(scope, "contactMutationOutbox").Doc(mutation.ID).Set(ctx, mutation)
	return err
}

func (s *FirestoreStore) GetContactMutation(ctx context.Context, scope, id string) (ContactMutation, bool, error) {
	doc, err := s.collection(scope, "contactMutationOutbox").Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return ContactMutation{}, false, nil
	}
	if err != nil {
		return ContactMutation{}, false, err
	}
	var mutation ContactMutation
	if err := doc.DataTo(&mutation); err != nil {
		return ContactMutation{}, false, err
	}
	return mutation, true, nil
}

func contactMutationData(contact Contact, action string) (string, ContactMutation) {
	mutation := NewContactMutation(contact, action)
	return mutation.ID, mutation
}
