package operations

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
)

func (s *MemoryStore) UpdateMemberName(_ context.Context, scope, id, name string, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	member, ok := s.collection(scope, "members")[id].(Member)
	if !ok {
		return errNotFound
	}
	member.Name, member.UpdatedAt = name, updatedAt
	s.collection(scope, "members")[id] = member
	return nil
}

func (s *FirestoreStore) UpdateMemberName(ctx context.Context, scope, id, name string, updatedAt time.Time) error {
	_, err := s.collection(scope, "members").Doc(id).Update(ctx, []firestore.Update{{Path: "name", Value: name}, {Path: "updatedAt", Value: updatedAt}})
	return err
}
