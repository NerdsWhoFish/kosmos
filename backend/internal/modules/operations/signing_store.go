package operations

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"time"

	"cloud.google.com/go/firestore"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errSigningConflict = errors.New("signing request changed")

type signingStore interface {
	ReplaceSigningRequest(context.Context, string, int, string, SigningRequest) error
	DeleteSigningRequest(context.Context, string, string, int) error
}

type SigningCleanup struct {
	ID            string    `json:"id" firestore:"id"`
	RequestID     string    `json:"requestId,omitempty" firestore:"requestId,omitempty"`
	Objects       []string  `json:"objects" firestore:"objects"`
	CreatedAt     time.Time `json:"createdAt" firestore:"createdAt"`
	NextAttemptAt time.Time `json:"nextAttemptAt" firestore:"nextAttemptAt"`
}

func cloneSigningRequest(request SigningRequest) SigningRequest {
	request.AccessExpiresAt = nil
	request.CurrentSignerID = ""
	request.Signers = append([]SigningSigner(nil), request.Signers...)
	for i := range request.Signers {
		request.Signers[i].Values = maps.Clone(request.Signers[i].Values)
		request.Signers[i].SignedAt = cloneSigningTime(request.Signers[i].SignedAt)
		if request.Signers[i].Session != nil {
			session := *request.Signers[i].Session
			request.Signers[i].Session = &session
		}
	}
	request.ExpiresAt = cloneSigningTime(request.ExpiresAt)
	request.CompletedAt = cloneSigningTime(request.CompletedAt)
	request.DownloadExpiresAt = cloneSigningTime(request.DownloadExpiresAt)
	request.Pages = append([]SigningPage(nil), request.Pages...)
	request.Fields = append([]SigningField{}, request.Fields...)
	request.Events = append([]SigningEvent(nil), request.Events...)
	if request.Session != nil {
		session := *request.Session
		request.Session = &session
	}
	request.PostSignExpiresAt = signingPostSignExpiry(request)
	return request
}

func cloneSigningTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	next := *value
	return &next
}

func (s *MemoryStore) ReplaceSigningRequest(ctx context.Context, scope string, revision int, state string, next SigningRequest) error {
	_, span := otel.Tracer("kosmos.signing").Start(ctx, "signing.store.replace")
	defer span.End()
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.collection(scope, "signingRequests")[next.ID]
	if !ok {
		return errNotFound
	}
	current := value.(SigningRequest)
	if current.Revision != revision || current.Status != state || signingCompletionExpired(current, next) || signingSignersChanged(current, next) {
		return errSigningConflict
	}
	s.collection(scope, "signingRequests")[next.ID] = cloneSigningRequest(next)
	return nil
}

func (s *FirestoreStore) ReplaceSigningRequest(ctx context.Context, scope string, revision int, state string, next SigningRequest) error {
	ctx, span := otel.Tracer("kosmos.signing").Start(ctx, "signing.store.replace")
	defer span.End()
	ref := s.collection(scope, "signingRequests").Doc(next.ID)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(ref)
		if status.Code(err) == codes.NotFound {
			return errNotFound
		}
		if err != nil {
			return err
		}
		var current SigningRequest
		if err := doc.DataTo(&current); err != nil {
			return err
		}
		if current.Revision != revision || current.Status != state || signingCompletionExpired(current, next) || signingSignersChanged(current, next) {
			return errSigningConflict
		}
		return tx.Set(ref, next)
	})
}

func signingCompletionExpired(current, next SigningRequest) bool {
	completing := next.Status == "completed"
	for _, signer := range next.Signers {
		before := signingSignerByID(current.Signers, signer.ID)
		if signer.SignedAt != nil && before != nil && before.SignedAt == nil {
			completing = true
		}
	}
	return current.Status == "pending" && completing && (current.ExpiresAt == nil || !current.ExpiresAt.After(time.Now()))
}

func signingSignersChanged(current, next SigningRequest) bool {
	if current.Status == "draft" || len(current.Signers) == 0 {
		return false
	}
	if len(current.Signers) != len(next.Signers) || !reflect.DeepEqual(current.Fields, next.Fields) {
		return true
	}
	newlySigned := 0
	for i, before := range current.Signers {
		after := next.Signers[i]
		if before.SignedAt != nil || after.SignedAt == nil {
			if !reflect.DeepEqual(before, after) {
				return true
			}
			continue
		}
		if current.Status != "pending" || before.ID != after.ID || before.Name != after.Name || before.Email != after.Email || before.TokenHash != after.TokenHash {
			return true
		}
		newlySigned++
	}
	return newlySigned > 1 || (newlySigned > 0 && !oneOf(next.Status, "pending", "completed"))
}

func signingCleanupFor(request SigningRequest, now time.Time) SigningCleanup {
	cleanup := SigningCleanup{ID: request.ID, Objects: []string{}, CreatedAt: now, NextAttemptAt: now}
	seen := make(map[string]bool)
	objects := []string{request.OriginalObject, request.UploadedObject, request.SignedObject}
	for _, signer := range request.Signers {
		objects = append(objects, signer.SignedObject)
	}
	for _, object := range objects {
		if object != "" && !seen[object] {
			cleanup.Objects = append(cleanup.Objects, object)
			seen[object] = true
		}
	}
	return cleanup
}

func (s *MemoryStore) DeleteSigningRequest(ctx context.Context, scope, id string, revision int) error {
	_, span := otel.Tracer("kosmos.signing").Start(ctx, "signing.store.delete")
	defer span.End()
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.collection(scope, "signingRequests")[id]
	if !ok {
		return errNotFound
	}
	current := value.(SigningRequest)
	if current.Revision != revision || !oneOf(current.Status, "draft", "completed", "revoked") {
		return errSigningConflict
	}
	if _, exists := s.collection(scope, "signingCleanup")[id]; exists {
		return errAlreadyExists
	}
	s.collection(scope, "signingCleanup")[id] = signingCleanupFor(current, time.Now().UTC())
	delete(s.collection(scope, "signingRequests"), id)
	return nil
}

func (s *FirestoreStore) DeleteSigningRequest(ctx context.Context, scope, id string, revision int) error {
	ctx, span := otel.Tracer("kosmos.signing").Start(ctx, "signing.store.delete")
	defer span.End()
	ref := s.collection(scope, "signingRequests").Doc(id)
	cleanupRef := s.collection(scope, "signingCleanup").Doc(id)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(ref)
		if status.Code(err) == codes.NotFound {
			return errNotFound
		}
		if err != nil {
			return err
		}
		var current SigningRequest
		if err := doc.DataTo(&current); err != nil {
			return err
		}
		if current.Revision != revision || !oneOf(current.Status, "draft", "completed", "revoked") {
			return errSigningConflict
		}
		if err := tx.Create(cleanupRef, signingCleanupFor(current, time.Now().UTC())); err != nil {
			return err
		}
		return tx.Delete(ref)
	})
}
