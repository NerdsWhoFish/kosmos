package operations

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
)

type signingCleanupBlobs struct {
	BlobStore
	mu    sync.Mutex
	fail  map[string]error
	calls map[string]int
}

func (b *signingCleanupBlobs) Delete(ctx context.Context, object string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls[object]++
	if err := b.fail[object]; err != nil {
		return err
	}
	return b.BlobStore.Delete(ctx, object)
}

func seedSigningCleanup(t *testing.T, store Store, scope, id string, objects []string, now time.Time) SigningCleanup {
	t.Helper()
	item := SigningCleanup{ID: id, Objects: objects, CreatedAt: now, NextAttemptAt: now}
	if err := store.Create(context.Background(), scope, "signingCleanup", id, item); err != nil {
		t.Fatal(err)
	}
	return item
}

func TestSigningCleanupRetentionPartialProgressAndRetry(t *testing.T) {
	m, _, _ := newTestModule(t)
	ctx := context.Background()
	now := time.Now().UTC()
	blobs := &signingCleanupBlobs{BlobStore: m.blobs, fail: map[string]error{"retained": errors.New("retention prevents private/path deletion"), "missing": storage.ErrObjectNotExist}, calls: map[string]int{}}
	m.blobs = blobs
	for _, object := range []string{"retained", "ready"} {
		if err := blobs.Put(ctx, object, "application/pdf", strings.NewReader("PDF")); err != nil {
			t.Fatal(err)
		}
	}
	seedSigningCleanup(t, m.store, m.publicScope, "request", []string{"retained", "ready", "missing"}, now)
	job := Job{ID: "cleanup", Type: JobTypeSigningCleanup, Scope: m.publicScope, OutboxID: "request", Actor: "system"}
	if err := m.runSigningCleanup(ctx, job, now); err != nil {
		t.Fatal(err)
	}
	var pending SigningCleanup
	if err := m.store.Get(ctx, m.publicScope, "signingCleanup", "request", &pending); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pending.Objects, []string{"retained"}) || !pending.NextAttemptAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("progress = %#v", pending)
	}
	if err := m.runSigningCleanup(ctx, job, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if blobs.calls["retained"] != 1 {
		t.Fatal("duplicate job retried retained object before due")
	}
	delete(blobs.fail, "retained")
	if err := m.runSigningCleanup(ctx, job, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := m.runSigningCleanup(ctx, job, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := m.store.Get(ctx, m.publicScope, "signingCleanup", "request", &pending); !errors.Is(err, errNotFound) {
		t.Fatalf("completed outbox = %v", err)
	}
	if blobs.calls["ready"] != 1 || blobs.calls["missing"] != 1 || blobs.calls["retained"] != 2 {
		t.Fatalf("delete calls = %#v", blobs.calls)
	}
}

type signingCleanupProgressFailure struct {
	*MemoryStore
	fail bool
}

func (s *signingCleanupProgressFailure) AdvanceSigningCleanup(ctx context.Context, scope, id, removed string, next time.Time) error {
	if s.fail {
		return errors.New("private/path progress failure")
	}
	return s.MemoryStore.AdvanceSigningCleanup(ctx, scope, id, removed, next)
}

func TestSigningCleanupRecoversAfterDeletedBlobProgressFailure(t *testing.T) {
	m, _, _ := newTestModule(t)
	ctx := context.Background()
	now := time.Now().UTC()
	store := &signingCleanupProgressFailure{MemoryStore: m.store.(*MemoryStore), fail: true}
	m.store = store
	seedSigningCleanup(t, store, m.publicScope, "request", []string{"ready"}, now)
	if err := m.blobs.Put(ctx, "ready", "application/pdf", strings.NewReader("PDF")); err != nil {
		t.Fatal(err)
	}
	job := Job{Type: JobTypeSigningCleanup, Scope: m.publicScope, OutboxID: "request"}
	if err := m.runSigningCleanup(ctx, job, now); err == nil || strings.Contains(err.Error(), "private/path") {
		t.Fatalf("unredacted/missing failure: %v", err)
	}
	var pending SigningCleanup
	if err := store.Get(ctx, m.publicScope, "signingCleanup", "request", &pending); err != nil || len(pending.Objects) != 1 {
		t.Fatalf("lost intent: %#v %v", pending, err)
	}
	store.fail = false
	if err := m.runSigningCleanup(ctx, job, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Get(ctx, m.publicScope, "signingCleanup", "request", &pending); !errors.Is(err, errNotFound) {
		t.Fatalf("outbox not removed: %v", err)
	}
}

type signingCleanupPageSpy struct {
	*MemoryStore
	pages         int
	configFailure bool
}

func (s *signingCleanupPageSpy) List(ctx context.Context, scope, collection string, target any) error {
	if collection == "signingCleanup" {
		return errors.New("unbounded cleanup list")
	}
	if collection == "googleConnections" && s.configFailure {
		return errors.New("integration configuration unavailable")
	}
	return s.MemoryStore.List(ctx, scope, collection, target)
}

func (s *signingCleanupPageSpy) ListPage(ctx context.Context, scope, collection string, request pagination.Request, spec pagination.Spec, target any) (pagination.Metadata, error) {
	if collection == "signingCleanup" {
		s.pages++
		if request.Limit != signingCleanupBatchSize || spec.OrderBy != "nextAttemptAt" {
			return pagination.Metadata{}, errors.New("cleanup query must be bounded and fair")
		}
	}
	return s.MemoryStore.ListPage(ctx, scope, collection, request, spec, target)
}

func TestSigningCleanupRetainedHeadDoesNotStarveLaterRecords(t *testing.T) {
	m, _, _ := newTestModule(t)
	ctx := context.Background()
	now := time.Now().UTC()
	store := &signingCleanupPageSpy{MemoryStore: m.store.(*MemoryStore)}
	m.store = store
	blobs := &signingCleanupBlobs{BlobStore: m.blobs, fail: map[string]error{"retained": errors.New("retention")}, calls: map[string]int{}}
	m.blobs = blobs
	for i := 0; i < signingCleanupBatchSize+1; i++ {
		seedSigningCleanup(t, store, m.publicScope, fmt.Sprintf("request-%03d", i), []string{"retained"}, now.Add(-time.Hour).Add(time.Duration(i)*time.Second))
	}
	if count, err := m.enqueueSigningCleanup(ctx, m.publicScope, now); err != nil || count != 50 {
		t.Fatalf("first batch %d %v", count, err)
	}
	queue := m.jobs.(*MemoryJobQueue)
	for _, job := range queue.Jobs() {
		if err := m.runSigningCleanup(ctx, job, now); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := m.enqueueSigningCleanup(ctx, m.publicScope, now.Add(time.Minute)); err != nil || count != 1 {
		t.Fatalf("later batch %d %v", count, err)
	}
	jobs := queue.Jobs()
	if len(jobs) != 51 || jobs[50].OutboxID != "request-050" || store.pages != 2 {
		t.Fatalf("tail starved: count%d pages%d", len(jobs), store.pages)
	}
	if _, err := m.enqueueSigningCleanup(ctx, m.publicScope, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(queue.Jobs()) != 101 {
		t.Fatalf("hourly retry reused exhausted task IDs: %d", len(queue.Jobs()))
	}
}

func TestSigningCleanupSchedulingSurvivesQueueAndIntegrationOutages(t *testing.T) {
	for _, configFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(configFailure), func(t *testing.T) {
			m, mux, _ := newTestModule(t)
			store := &signingCleanupPageSpy{MemoryStore: m.store.(*MemoryStore), configFailure: configFailure}
			m.store = store
			seedSigningCleanup(t, store, m.publicScope, "request", []string{"missing"}, time.Now().Add(-time.Minute))
			m.jobs = unavailableContactQueue{}
			if _, err := m.enqueueSigningCleanup(context.Background(), m.publicScope, time.Now()); err == nil {
				t.Fatal("queue outage not returned")
			}
			queue := NewMemoryJobQueue()
			m.jobs = queue
			r := httptest.NewRequest(http.MethodPost, "https://worker.run.app/api/v1/jobs/schedule", nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			want := http.StatusAccepted
			if configFailure {
				want = http.StatusInternalServerError
			}
			if w.Code != want || len(queue.Jobs()) != 1 {
				t.Fatalf("schedule %d jobs%d", w.Code, len(queue.Jobs()))
			}
			job := queue.Jobs()[0]
			for i := 0; i < 2; i++ {
				performJSON[map[string]any](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, job), http.StatusOK)
			}
			var execution JobExecution
			if err := store.Get(context.Background(), m.publicScope, "jobExecutions", job.ID, &execution); !errors.Is(err, errNotFound) {
				t.Fatalf("cleanup execution left redundant record: %v", err)
			}
		})
	}
}

func TestSigningCleanupConcurrentProgressNeverResurrects(t *testing.T) {
	factories := map[string]func(*testing.T) Store{
		"memory": func(*testing.T) Store { return NewMemoryStore() },
		"firestore": func(t *testing.T) Store {
			if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
				t.Skip("FIRESTORE_EMULATOR_HOST is not configured")
			}
			client, err := firestore.NewClient(context.Background(), "kosmos-signing-tests")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			return NewFirestoreStore(client)
		},
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			store := factory(t)
			ctx := context.Background()
			scope := fmt.Sprintf("signing-cleanup-%d", time.Now().UnixNano())
			now := time.Now().UTC()
			original := seedSigningCleanup(t, store, scope, "request", []string{"a", "b", "c"}, now)
			progress := store.(signingCleanupStore)
			var wg sync.WaitGroup
			results := make(chan error, 8)
			for _, object := range []string{"a", "b", "c", "a", "b", "c", "", ""} {
				wg.Add(1)
				go func(object string) {
					defer wg.Done()
					results <- progress.AdvanceSigningCleanup(ctx, scope, "request", object, now.Add(time.Hour))
				}(object)
			}
			wg.Wait()
			close(results)
			for err := range results {
				if err != nil {
					t.Fatal(err)
				}
			}
			var pending SigningCleanup
			if err := store.Get(ctx, scope, "signingCleanup", "request", &pending); !errors.Is(err, errNotFound) {
				t.Fatalf("outbox resurrected: %#v %v", pending, err)
			}
			if !reflect.DeepEqual(original.Objects, []string{"a", "b", "c"}) {
				t.Fatal("progress mutated previously-read snapshot")
			}
			if err := progress.AdvanceSigningCleanup(ctx, scope, "request", "", now.Add(2*time.Hour)); err != nil {
				t.Fatal(err)
			}
			if err := store.Get(ctx, scope, "signingCleanup", "request", &pending); !errors.Is(err, errNotFound) {
				t.Fatalf("delayed retry resurrected outbox: %v", err)
			}
		})
	}
}

func TestSigningOrphanQueueIsScopedDurableAndIdempotent(t *testing.T) {
	m, _, _ := newTestModule(t)
	ctx := context.Background()
	object := m.publicScope + "/signing/request/signed-attempt.pdf"
	for i := 0; i < 2; i++ {
		if err := m.queueSigningOrphan(ctx, m.publicScope, "request", object); err != nil {
			t.Fatal(err)
		}
	}
	var items []SigningCleanup
	if err := m.store.List(ctx, m.publicScope, "signingCleanup", &items); err != nil || len(items) != 1 || !reflect.DeepEqual(items[0].Objects, []string{object}) || items[0].ID == "request" {
		t.Fatalf("orphan queue = %#v %v", items, err)
	}
	if items[0].RequestID != "request" || !items[0].NextAttemptAt.Equal(items[0].CreatedAt.Add(time.Hour)) {
		t.Fatalf("orphan lost its ownership check or grace period: %#v", items[0])
	}
	for _, invalid := range []string{"other/signing/request/signed-attempt.pdf", m.publicScope + "/signing/other/signed-attempt.pdf", m.publicScope + "/signing/request/../original.pdf", m.publicScope + "/signing/request/arbitrary.pdf"} {
		if err := m.queueSigningOrphan(ctx, m.publicScope, "request", invalid); err == nil {
			t.Fatal("out-of-scope object accepted")
		}
	}
}

func TestSigningOrphanCleanupProtectsLateCommittedObjects(t *testing.T) {
	for _, ownership := range []string{"original", "uploaded", "latest", "earlier signer", "later signer"} {
		t.Run(ownership, func(t *testing.T) {
			m, _, _ := newTestModule(t)
			ctx := context.Background()
			object := m.publicScope + "/signing/request/signed-attempt.pdf"
			blobs := &signingCleanupBlobs{BlobStore: m.blobs, fail: map[string]error{}, calls: map[string]int{}}
			m.blobs = blobs
			if err := blobs.Put(ctx, object, "application/pdf", strings.NewReader("signed PDF")); err != nil {
				t.Fatal(err)
			}
			if err := m.queueSigningOrphan(ctx, m.publicScope, "request", object); err != nil {
				t.Fatal(err)
			}
			var items []SigningCleanup
			if err := m.store.List(ctx, m.publicScope, "signingCleanup", &items); err != nil {
				t.Fatal(err)
			}
			item := items[0]
			job := Job{Type: JobTypeSigningCleanup, Scope: m.publicScope, OutboxID: item.ID}
			if err := m.runSigningCleanup(ctx, job, item.CreatedAt.Add(30*time.Minute)); err != nil {
				t.Fatal(err)
			}
			if blobs.calls[object] != 0 {
				t.Fatal("orphan was deleted before commit grace period elapsed")
			}
			request := SigningRequest{ID: "request", Status: "pending", Signers: []SigningSigner{{ID: "first"}, {ID: "second"}}}
			switch ownership {
			case "original":
				request.OriginalObject = object
			case "uploaded":
				request.UploadedObject = object
			case "latest":
				request.SignedObject = object
			case "earlier signer":
				request.Signers[0].SignedObject = object
			case "later signer":
				request.Signers[1].SignedObject = object
			}
			if err := m.store.Create(ctx, m.publicScope, "signingRequests", request.ID, request); err != nil {
				t.Fatal(err)
			}
			for count := 0; count < 2; count++ {
				if err := m.runSigningCleanup(ctx, job, item.NextAttemptAt); err != nil {
					t.Fatal(err)
				}
			}
			if blobs.calls[object] != 0 {
				t.Fatal("orphan cleanup deleted a late-committed live object")
			}
			if err := m.store.Get(ctx, m.publicScope, "signingCleanup", item.ID, &item); !errors.Is(err, errNotFound) {
				t.Fatalf("owned object retained a cleanup obligation: %v", err)
			}
			reader, err := blobs.Open(ctx, object)
			if err != nil {
				t.Fatal("late-committed PDF disappeared")
			}
			_ = reader.Close()
		})
	}
}

type signingCleanupOwnershipFailure struct {
	*MemoryStore
	fail bool
}

func (s *signingCleanupOwnershipFailure) Get(ctx context.Context, scope, collection, id string, target any) error {
	if collection == "signingRequests" && s.fail {
		return errors.New("private/path ownership read failed")
	}
	return s.MemoryStore.Get(ctx, scope, collection, id, target)
}

func TestSigningOrphanCleanupDefersWhenOwnershipReadFails(t *testing.T) {
	m, _, _ := newTestModule(t)
	ctx := context.Background()
	store := &signingCleanupOwnershipFailure{MemoryStore: m.store.(*MemoryStore), fail: true}
	m.store = store
	object := m.publicScope + "/signing/request/signed-attempt.pdf"
	blobs := &signingCleanupBlobs{BlobStore: m.blobs, fail: map[string]error{}, calls: map[string]int{}}
	m.blobs = blobs
	if err := blobs.Put(ctx, object, "application/pdf", strings.NewReader("orphan PDF")); err != nil {
		t.Fatal(err)
	}
	if err := m.queueSigningOrphan(ctx, m.publicScope, "request", object); err != nil {
		t.Fatal(err)
	}
	var items []SigningCleanup
	if err := store.List(ctx, m.publicScope, "signingCleanup", &items); err != nil {
		t.Fatal(err)
	}
	item := items[0]
	job := Job{Type: JobTypeSigningCleanup, Scope: m.publicScope, OutboxID: item.ID}
	if err := m.runSigningCleanup(ctx, job, item.NextAttemptAt); err != nil {
		t.Fatal(err)
	}
	var pending SigningCleanup
	if err := store.Get(ctx, m.publicScope, "signingCleanup", item.ID, &pending); err != nil {
		t.Fatal(err)
	}
	if blobs.calls[object] != 0 || !reflect.DeepEqual(pending.Objects, item.Objects) || !pending.NextAttemptAt.Equal(item.NextAttemptAt.Add(time.Hour)) {
		t.Fatal("failed ownership check deleted an object or lost its durable retry")
	}
	if err := store.Create(ctx, m.publicScope, "signingRequests", "request", SigningRequest{ID: "request", OriginalObject: m.publicScope + "/signing/request/original.pdf"}); err != nil {
		t.Fatal(err)
	}
	store.fail = false
	if err := m.runSigningCleanup(ctx, job, pending.NextAttemptAt); err != nil {
		t.Fatal(err)
	}
	if blobs.calls[object] != 1 {
		t.Fatal("unowned PDF was not deleted after ownership reads recovered")
	}
	if err := store.Get(ctx, m.publicScope, "signingCleanup", item.ID, &pending); !errors.Is(err, errNotFound) {
		t.Fatalf("finished orphan still queued: %v", err)
	}
}

func TestSigningDeletedDocumentCleanupDoesNotNeedOwnershipRead(t *testing.T) {
	m, _, _ := newTestModule(t)
	store := &signingCleanupOwnershipFailure{MemoryStore: m.store.(*MemoryStore), fail: true}
	m.store = store
	now := time.Now().UTC()
	seedSigningCleanup(t, store, m.publicScope, "deleted", []string{"missing"}, now)
	job := Job{Type: JobTypeSigningCleanup, Scope: m.publicScope, OutboxID: "deleted"}
	if err := m.runSigningCleanup(context.Background(), job, now); err != nil {
		t.Fatal(err)
	}
	var pending SigningCleanup
	if err := store.Get(context.Background(), m.publicScope, "signingCleanup", "deleted", &pending); !errors.Is(err, errNotFound) {
		t.Fatalf("authoritative document deletion blocked by ownership lookup: %v", err)
	}
}
