package operations

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *MemoryStore) AdvanceMailCheckpoint(_ context.Context, scope, id string, startedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.collection(scope, "googleConnections")[id]
	if !exists {
		return errNotFound
	}
	connection, ok := record.(GoogleConnection)
	if !ok {
		return errNotFound
	}
	if connection.LastMailSyncAt == nil || connection.LastMailSyncAt.Before(startedAt) {
		connection.LastMailSyncAt = &startedAt
		connection.UpdatedAt = time.Now().UTC()
		s.collection(scope, "googleConnections")[id] = connection
	}
	return nil
}

func (s *FirestoreStore) AdvanceMailCheckpoint(ctx context.Context, scope, id string, startedAt time.Time) error {
	reference := s.collection(scope, "googleConnections").Doc(id)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snapshot, err := tx.Get(reference)
		if status.Code(err) == codes.NotFound {
			return errNotFound
		}
		if err != nil {
			return err
		}
		var connection GoogleConnection
		if err := snapshot.DataTo(&connection); err != nil {
			return err
		}
		if connection.LastMailSyncAt != nil && !connection.LastMailSyncAt.Before(startedAt) {
			return nil
		}
		return tx.Update(reference, []firestore.Update{{Path: "lastMailSyncAt", Value: startedAt}, {Path: "updatedAt", Value: time.Now().UTC()}})
	})
}
