package operations

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const signingCleanupBatchSize = 50

type signingCleanupStore interface {
	AdvanceSigningCleanup(context.Context, string, string, string, time.Time) error
}

func (m *Module) queueSigningOrphan(ctx context.Context, scope, requestID, object string) error {
	prefix := scope + "/signing/" + requestID + "/"
	name := strings.TrimPrefix(object, prefix)
	validName := name == "original.pdf" || name == "uploaded.pdf"
	if strings.HasPrefix(name, "signed-") && strings.HasSuffix(name, ".pdf") {
		validName = signingIDPattern.MatchString(strings.TrimSuffix(strings.TrimPrefix(name, "signed-"), ".pdf"))
	}
	if scope == "" || !signingIDPattern.MatchString(requestID) || !strings.HasPrefix(object, prefix) || !validName {
		return errors.New("signing orphan object is invalid")
	}
	now := time.Now().UTC()
	item := SigningCleanup{ID: "orphan-" + deterministicID(object), RequestID: requestID, Objects: []string{object}, CreatedAt: now, NextAttemptAt: now.Add(time.Hour)}
	if err := m.store.Create(ctx, scope, "signingCleanup", item.ID, item); err != nil && !errors.Is(err, errAlreadyExists) {
		return errors.New("signing orphan cleanup could not be queued")
	}
	return nil
}

func advanceSigningCleanup(current SigningCleanup, removed string, nextAttempt time.Time) SigningCleanup {
	objects := make([]string, 0, len(current.Objects))
	for _, object := range current.Objects {
		if object != removed {
			objects = append(objects, object)
		}
	}
	current.Objects = objects
	if nextAttempt.After(current.NextAttemptAt) {
		current.NextAttemptAt = nextAttempt
	}
	return current
}

func (s *MemoryStore) AdvanceSigningCleanup(_ context.Context, scope, id, removed string, nextAttempt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	collection := s.collection(scope, "signingCleanup")
	value, found := collection[id]
	if !found {
		return nil
	}
	current := advanceSigningCleanup(value.(SigningCleanup), removed, nextAttempt)
	if len(current.Objects) == 0 {
		delete(collection, id)
	} else {
		collection[id] = current
	}
	return nil
}

func (s *FirestoreStore) AdvanceSigningCleanup(ctx context.Context, scope, id, removed string, nextAttempt time.Time) error {
	ref := s.collection(scope, "signingCleanup").Doc(id)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(ref)
		if status.Code(err) == codes.NotFound {
			return nil
		}
		if err != nil {
			return err
		}
		var current SigningCleanup
		if err := doc.DataTo(&current); err != nil {
			return err
		}
		current = advanceSigningCleanup(current, removed, nextAttempt)
		if len(current.Objects) == 0 {
			return tx.Delete(ref)
		}
		return tx.Set(ref, current)
	})
}

func (m *Module) enqueueSigningCleanup(ctx context.Context, scope string, now time.Time, targets ...string) (int, error) {
	var items []SigningCleanup
	_, err := m.store.ListPage(ctx, scope, "signingCleanup", pagination.Request{Limit: signingCleanupBatchSize}, pagination.Spec{
		Key: "operations.signing-cleanup", OrderBy: "nextAttemptAt", Direction: pagination.Ascending, ValueKind: pagination.TimeValue,
	}, &items)
	if err != nil {
		return 0, errors.New("signing cleanup queue could not be loaded")
	}
	queued := 0
	for _, item := range items {
		if item.NextAttemptAt.After(now) {
			break
		}
		job := Job{
			ID:   deterministicID("signing-cleanup|" + item.ID + "|" + now.UTC().Truncate(time.Hour).Format(time.RFC3339)),
			Type: JobTypeSigningCleanup, Scope: scope, OutboxID: item.ID, Actor: "system",
		}
		if err := m.enqueueJob(ctx, job, targets...); err != nil {
			return queued, err
		}
		queued++
	}
	return queued, nil
}

func (m *Module) runSigningCleanup(ctx context.Context, job Job, now time.Time) error {
	store, ok := m.store.(signingCleanupStore)
	if !ok {
		return errors.New("signing cleanup progress is unavailable")
	}
	var current SigningCleanup
	if err := m.store.Get(ctx, job.Scope, "signingCleanup", job.OutboxID, &current); errors.Is(err, errNotFound) {
		return nil
	} else if err != nil {
		return errors.New("signing cleanup record could not be loaded")
	}
	if current.NextAttemptAt.After(now) {
		return nil
	}
	nextAttempt := now.Add(time.Hour)
	if len(current.Objects) == 0 {
		if err := store.AdvanceSigningCleanup(ctx, job.Scope, current.ID, "", nextAttempt); err != nil {
			return errors.New("signing cleanup progress could not be saved")
		}
		return nil
	}
	deferred := 0
	for _, object := range current.Objects {
		removed := object
		canDelete, err := m.signingCleanupCanDelete(ctx, job.Scope, current.RequestID, object)
		if err != nil {
			removed = ""
			deferred++
		} else if canDelete {
			if err := m.blobs.Delete(ctx, object); err != nil && !errors.Is(err, errNotFound) && !errors.Is(err, storage.ErrObjectNotExist) {
				removed = ""
				deferred++
			}
		}
		if err := store.AdvanceSigningCleanup(ctx, job.Scope, current.ID, removed, nextAttempt); err != nil {
			return errors.New("signing cleanup progress could not be saved")
		}
	}
	if deferred > 0 {
		slog.InfoContext(ctx, "signing PDF cleanup deferred", "job.type", JobTypeSigningCleanup, "job.status", "deferred", "object.count", deferred)
	}
	return nil
}

func (m *Module) signingCleanupCanDelete(ctx context.Context, scope, requestID, object string) (bool, error) {
	if requestID == "" {
		return true, nil
	}
	if !signingIDPattern.MatchString(requestID) {
		return false, errors.New("signing orphan request is invalid")
	}
	var current SigningRequest
	if err := m.store.Get(ctx, scope, "signingRequests", requestID, &current); errors.Is(err, errNotFound) {
		return true, nil
	} else if err != nil {
		return false, errors.New("signing orphan ownership could not be checked")
	}
	return !signingOwnsObject(current, object), nil
}
