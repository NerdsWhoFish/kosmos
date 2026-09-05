package operations

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/firestorepage"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errNotFound = errors.New("record not found")
var errAlreadyExists = errors.New("record already exists")

type Store interface {
	AdvanceMailCheckpoint(context.Context, string, string, time.Time) error
	List(context.Context, string, string, any) error
	ListPage(context.Context, string, string, pagination.Request, pagination.Spec, any) (pagination.Metadata, error)
	Get(context.Context, string, string, string, any) error
	Put(context.Context, string, string, string, any) error
	Create(context.Context, string, string, string, any) error
	Delete(context.Context, string, string, string) error
	UpdateMemberName(context.Context, string, string, string, time.Time) error
	AllowRateLimit(context.Context, string, string, int, time.Duration, time.Time) (bool, time.Duration, error)
}

type MemoryStore struct {
	mu      sync.Mutex
	records map[string]map[string]map[string]any
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: make(map[string]map[string]map[string]any)}
}

func (s *MemoryStore) List(_ context.Context, scope, collection string, target any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Pointer || targetValue.Elem().Kind() != reflect.Slice {
		return errors.New("list target must be a slice pointer")
	}
	items := reflect.MakeSlice(targetValue.Elem().Type(), 0, len(s.collection(scope, collection)))
	for _, value := range s.collection(scope, collection) {
		itemValue := reflect.ValueOf(value)
		if !itemValue.Type().AssignableTo(items.Type().Elem()) {
			return errors.New("stored record type does not match list target")
		}
		items = reflect.Append(items, itemValue)
	}
	targetValue.Elem().Set(items)
	return nil
}

func (s *MemoryStore) ListPage(ctx context.Context, scope, collection string, request pagination.Request, spec pagination.Spec, target any) (pagination.Metadata, error) {
	if err := s.List(ctx, scope, collection, target); err != nil {
		return pagination.Metadata{}, err
	}
	return pagination.Apply(target, request, spec)
}

func (s *MemoryStore) Get(_ context.Context, scope, collection, id string, target any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.collection(scope, collection)[id]
	if !ok {
		return errNotFound
	}
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Pointer || !reflect.ValueOf(value).Type().AssignableTo(targetValue.Elem().Type()) {
		return errors.New("stored record type does not match target")
	}
	targetValue.Elem().Set(reflect.ValueOf(value))
	return nil
}

func (s *MemoryStore) Put(_ context.Context, scope, collection, id string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collection(scope, collection)[id] = value
	return nil
}

func (s *MemoryStore) Create(_ context.Context, scope, collection, id string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.collection(scope, collection)[id]; ok {
		return errAlreadyExists
	}
	s.collection(scope, collection)[id] = value
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, scope, collection, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.collection(scope, collection)[id]; !ok {
		return errNotFound
	}
	delete(s.collection(scope, collection), id)
	return nil
}

func (s *MemoryStore) collection(scope, name string) map[string]any {
	if s.records[scope] == nil {
		s.records[scope] = make(map[string]map[string]any)
	}
	if s.records[scope][name] == nil {
		s.records[scope][name] = make(map[string]any)
	}
	return s.records[scope][name]
}

type FirestoreStore struct{ client *firestore.Client }

func NewFirestoreStore(client *firestore.Client) *FirestoreStore {
	return &FirestoreStore{client: client}
}

func (s *FirestoreStore) collection(scope, name string) *firestore.CollectionRef {
	return s.client.Collection("organizations").Doc(scope).Collection(name)
}

func (s *FirestoreStore) List(ctx context.Context, scope, collection string, target any) error {
	iter := s.collection(scope, collection).Documents(ctx)
	defer iter.Stop()
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Pointer || targetValue.Elem().Kind() != reflect.Slice {
		return errors.New("list target must be a slice pointer")
	}
	values := reflect.MakeSlice(targetValue.Elem().Type(), 0, 0)
	for {
		document, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return err
		}
		value := reflect.New(values.Type().Elem())
		if err := document.DataTo(value.Interface()); err != nil {
			return err
		}
		values = reflect.Append(values, value.Elem())
	}
	targetValue.Elem().Set(values)
	return nil
}

func (s *FirestoreStore) ListPage(ctx context.Context, scope, collection string, request pagination.Request, spec pagination.Spec, target any) (pagination.Metadata, error) {
	return firestorepage.List(ctx, s.collection(scope, collection), request, spec, target)
}

func (s *FirestoreStore) Get(ctx context.Context, scope, collection, id string, target any) error {
	document, err := s.collection(scope, collection).Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return errNotFound
	}
	if err != nil {
		return err
	}
	return document.DataTo(target)
}

func (s *FirestoreStore) Put(ctx context.Context, scope, collection, id string, value any) error {
	_, err := s.collection(scope, collection).Doc(id).Set(ctx, value)
	return err
}

func (s *FirestoreStore) Create(ctx context.Context, scope, collection, id string, value any) error {
	_, err := s.collection(scope, collection).Doc(id).Create(ctx, value)
	if status.Code(err) == codes.AlreadyExists {
		return errAlreadyExists
	}
	return err
}

func (s *FirestoreStore) Delete(ctx context.Context, scope, collection, id string) error {
	_, err := s.collection(scope, collection).Doc(id).Delete(ctx, firestore.Exists)
	if status.Code(err) == codes.NotFound || status.Code(err) == codes.FailedPrecondition {
		return errNotFound
	}
	return err
}
