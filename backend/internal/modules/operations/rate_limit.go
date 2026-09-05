package operations

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type rateLimitWindow struct {
	Requests  []time.Time `firestore:"requests"`
	ExpiresAt time.Time   `firestore:"expiresAt"`
}

func (w rateLimitWindow) admit(limit int, duration time.Duration, now time.Time) (rateLimitWindow, bool, time.Duration) {
	recent := make([]time.Time, 0, limit)
	expiresAt := now.Add(duration)
	for _, seen := range w.Requests {
		if seen.After(now.Add(-duration)) {
			recent = append(recent, seen)
			if seen.Add(duration).After(expiresAt) {
				expiresAt = seen.Add(duration)
			}
		}
	}
	if len(recent) >= limit {
		oldest := recent[0]
		for _, seen := range recent[1:] {
			if seen.Before(oldest) {
				oldest = seen
			}
		}
		return w, false, oldest.Add(duration).Sub(now)
	}
	return rateLimitWindow{Requests: append(recent, now), ExpiresAt: expiresAt}, true, 0
}

func (s *MemoryStore) AllowRateLimit(_ context.Context, scope, key string, limit int, duration time.Duration, now time.Time) (bool, time.Duration, error) {
	if limit < 1 || duration <= 0 {
		return false, 0, errors.New("invalid rate limit")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	windows := s.collection(scope, "intakeRateLimits")
	for id, value := range windows {
		if !value.(rateLimitWindow).ExpiresAt.After(now) {
			delete(windows, id)
		}
	}
	current, _ := windows[key].(rateLimitWindow)
	updated, allowed, retryAfter := current.admit(limit, duration, now)
	if allowed {
		windows[key] = updated
	}
	return allowed, retryAfter, nil
}

func (s *FirestoreStore) AllowRateLimit(ctx context.Context, scope, key string, limit int, duration time.Duration, now time.Time) (bool, time.Duration, error) {
	if limit < 1 || duration <= 0 {
		return false, 0, errors.New("invalid rate limit")
	}
	ref := s.collection(scope, "intakeRateLimits").Doc(key)
	for attempts := 0; attempts <= limit; attempts++ {
		var current rateLimitWindow
		doc, err := ref.Get(ctx)
		if err == nil {
			if err := doc.DataTo(&current); err != nil {
				return false, 0, err
			}
		} else if status.Code(err) != codes.NotFound {
			return false, 0, err
		}
		updated, allowed, retryAfter := current.admit(limit, duration, now)
		if !allowed {
			return false, retryAfter, nil
		}
		if status.Code(err) == codes.NotFound {
			_, err = ref.Create(ctx, updated)
		} else {
			_, err = ref.Update(ctx, []firestore.Update{{Path: "requests", Value: updated.Requests}, {Path: "expiresAt", Value: updated.ExpiresAt}}, firestore.LastUpdateTime(doc.UpdateTime))
		}
		if err == nil {
			return true, 0, nil
		}
		if code := status.Code(err); code != codes.AlreadyExists && code != codes.FailedPrecondition && code != codes.NotFound && code != codes.Aborted {
			return false, 0, err
		}
	}
	return false, 0, errors.New("rate limit admission conflicted")
}
