package landing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

type Button struct {
	ID          string    `json:"id" firestore:"-"`
	Label       string    `json:"label" firestore:"label"`
	Description string    `json:"description" firestore:"description"`
	Href        string    `json:"href" firestore:"href"`
	Icon        string    `json:"icon" firestore:"icon"`
	CreatedAt   time.Time `json:"-" firestore:"createdAt"`
}

type Store interface {
	ListButtons(context.Context, string) ([]Button, error)
	CreateButton(context.Context, string, Button) (Button, error)
}

type MemoryStore struct {
	mu      sync.Mutex
	buttons map[string][]Button
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{buttons: make(map[string][]Button)}
}

func (s *MemoryStore) ListButtons(_ context.Context, owner string) ([]Button, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.buttons[owner]; !ok {
		s.buttons[owner] = defaultButtons()
	}
	return append([]Button(nil), s.buttons[owner]...), nil
}

func (s *MemoryStore) CreateButton(_ context.Context, owner string, button Button) (Button, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.buttons[owner]; !ok {
		s.buttons[owner] = defaultButtons()
	}
	id, err := newButtonID()
	if err != nil {
		return Button{}, err
	}
	button.ID = id
	button.CreatedAt = time.Now().UTC()
	s.buttons[owner] = append(s.buttons[owner], button)
	return button, nil
}

type FirestoreStore struct {
	client *firestore.Client
}

func NewFirestoreStore(client *firestore.Client) *FirestoreStore {
	return &FirestoreStore{client: client}
}

func (s *FirestoreStore) ListButtons(ctx context.Context, owner string) ([]Button, error) {
	collection := s.collection(owner)
	iter := collection.OrderBy("createdAt", firestore.Asc).Documents(ctx)
	defer iter.Stop()

	var buttons []Button
	for {
		document, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var button Button
		if err := document.DataTo(&button); err != nil {
			return nil, err
		}
		button.ID = document.Ref.ID
		buttons = append(buttons, button)
	}
	if len(buttons) != 0 {
		return buttons, nil
	}

	buttons = defaultButtons()
	batch := s.client.Batch()
	for _, button := range buttons {
		batch.Set(collection.Doc(button.ID), button)
	}
	if _, err := batch.Commit(ctx); err != nil {
		return nil, err
	}
	return buttons, nil
}

func (s *FirestoreStore) CreateButton(ctx context.Context, owner string, button Button) (Button, error) {
	document := s.collection(owner).NewDoc()
	button.ID = document.ID
	button.CreatedAt = time.Now().UTC()
	if _, err := document.Set(ctx, button); err != nil {
		return Button{}, err
	}
	return button, nil
}

func (s *FirestoreStore) collection(owner string) *firestore.CollectionRef {
	return s.client.Collection("users").Doc(owner).Collection("landingButtons")
}

func defaultButtons() []Button {
	createdAt := time.Unix(0, 0).UTC()
	return []Button{
		{ID: "website", Label: "Open website", Description: "Jump straight to the public business site.", Href: "https://www.nerdswhofish.com", Icon: "globe", CreatedAt: createdAt},
		{ID: "bookings", Label: "Bookings", Description: "Manage meetings and availability.", Href: "https://book.nerdswhofish.com", Icon: "calendar", CreatedAt: createdAt.Add(time.Second)},
		{ID: "contacts", Label: "Contacts", Description: "Keep every relationship in one place.", Href: "/contacts", Icon: "users", CreatedAt: createdAt.Add(2 * time.Second)},
	}
}

func newButtonID() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
