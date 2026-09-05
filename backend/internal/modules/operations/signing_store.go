package operations

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errSigningConflict = errors.New("signing request changed")

type signingStore interface {
	ReplaceSigningRequest(context.Context, string, int, string, SigningRequest) error
}

func cloneSigningRequest(request SigningRequest) SigningRequest {
	request.Pages = append([]SigningPage(nil), request.Pages...)
	request.Fields = append([]SigningField{}, request.Fields...)
	request.Events = append([]SigningEvent(nil), request.Events...)
	return request
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
	if current.Revision != revision || current.Status != state || signingCompletionExpired(current, next) {
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
		if current.Revision != revision || current.Status != state || signingCompletionExpired(current, next) {
			return errSigningConflict
		}
		return tx.Set(ref, next)
	})
}

func signingCompletionExpired(current, next SigningRequest) bool {
	return next.Status == "completed" && (current.ExpiresAt == nil || !current.ExpiresAt.After(time.Now()))
}
