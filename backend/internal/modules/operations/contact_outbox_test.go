package operations

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
	"golang.org/x/oauth2"
)

type unavailableContactQueue struct{}

func (unavailableContactQueue) Enqueue(context.Context, Job, ...string) error {
	return errors.New("Cloud Tasks temporarily unavailable")
}

func TestContactDeletionRecoversAfterQueueOutage(t *testing.T) {
	for _, recovery := range []string{"scheduler", "manual"} {
		t.Run(recovery, func(t *testing.T) {
			module, mux, store := newTestModule(t)
			ctx := context.Background()
			if err := module.SaveVoiceContactsGrant(ctx, Identity{Email: "owner@nerdswhofish.com"}, Identity{Email: "voice@gmail.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh"}); err != nil {
				t.Fatal(err)
			}
			contact, err := store.CreateContact(ctx, "nerds-who-fish", workspace.Contact{Name: "Ada"})
			if err != nil {
				t.Fatal(err)
			}
			module.jobs = unavailableContactQueue{}
			workspace.NewModule(store, func(*http.Request) (string, error) { return "nerds-who-fish", nil }, workspace.WithContactMutation(func(ctx context.Context, scope string, contact workspace.Contact, action string) error {
				return module.EnqueueGoogleContactMutation(ctx, scope, contact, action, "owner")
			})).RegisterRoutes(mux)
			performJSON[map[string]any](t, mux, http.MethodDelete, "/api/v1/contacts/"+contact.ID, "", http.StatusNoContent)
			queue := NewMemoryJobQueue()
			module.jobs = queue
			deleted := []string{}
			module.google = fakeGoogle{deleted: &deleted}
			if recovery == "manual" {
				performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/integrations/google-contacts/sync", "", http.StatusAccepted)
			} else {
				request := httptest.NewRequest(http.MethodPost, "https://worker.run.app/api/v1/jobs/schedule", strings.NewReader("{}"))
				response := httptest.NewRecorder()
				mux.ServeHTTP(response, request)
				if response.Code != http.StatusAccepted {
					t.Fatalf("scheduler: %d %s", response.Code, response.Body.String())
				}
			}
			jobs := queue.Jobs()
			if len(jobs) == 0 {
				t.Fatal("lost deletion was not recovered")
			}
			for _, job := range jobs {
				performJSON[map[string]any](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, job), http.StatusOK)
				performJSON[map[string]any](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, job), http.StatusOK)
			}
			if len(deleted) != 1 || deleted[0] != contact.ID {
				t.Fatalf("provider deletions = %#v", deleted)
			}
			mutations, err := store.ListContactMutations(ctx, "nerds-who-fish")
			if err != nil || len(mutations) != 0 {
				t.Fatalf("completed intents remain: %#v %v", mutations, err)
			}
		})
	}
}

func TestFailedContactJobSurvivesUntilProviderRecovers(t *testing.T) {
	module, mux, store := newTestModule(t)
	ctx := context.Background()
	if err := module.SaveVoiceContactsGrant(ctx, Identity{Email: "owner@nerdswhofish.com"}, Identity{Email: "voice@gmail.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateContact(ctx, "nerds-who-fish", workspace.Contact{Name: "Ada"}); err != nil {
		t.Fatal(err)
	}
	module.google = fakeGoogle{upsertErr: errors.New("Google unavailable")}
	if _, err := module.enqueuePendingContactMutations(ctx, "nerds-who-fish", "first-dispatch"); err != nil {
		t.Fatal(err)
	}
	jobs := module.jobs.(*MemoryJobQueue).Jobs()
	performJSON[map[string]any](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, jobs[0]), http.StatusBadGateway)
	pending, err := store.ListContactMutations(ctx, "nerds-who-fish")
	if err != nil || len(pending) != 1 {
		t.Fatalf("failed job lost its intent: %#v %v", pending, err)
	}
	module.google = fakeGoogle{}
	if _, err := module.enqueuePendingContactMutations(ctx, "nerds-who-fish", "next-schedule"); err != nil {
		t.Fatal(err)
	}
	jobs = module.jobs.(*MemoryJobQueue).Jobs()
	if len(jobs) != 2 || jobs[0].ID == jobs[1].ID {
		t.Fatal("replay reused a potentially exhausted Cloud Tasks name")
	}
	performJSON[map[string]any](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, jobs[1]), http.StatusOK)
	pending, err = store.ListContactMutations(ctx, "nerds-who-fish")
	if err != nil || len(pending) != 0 {
		t.Fatalf("successful retry did not acknowledge intent: %#v %v", pending, err)
	}
}

type failContactAcknowledgement struct {
	*workspace.MemoryStore
	fail bool
}

func (s *failContactAcknowledgement) CompleteContactMutation(ctx context.Context, scope, id string) error {
	if s.fail {
		return errors.New("Firestore unavailable after provider success")
	}
	return s.MemoryStore.CompleteContactMutation(ctx, scope, id)
}

func TestContactAcknowledgementFailureKeepsDurableIntent(t *testing.T) {
	module, mux, contacts := newTestModule(t)
	store := &failContactAcknowledgement{MemoryStore: contacts, fail: true}
	module.workspace = store
	ctx := context.Background()
	if err := module.SaveVoiceContactsGrant(ctx, Identity{Email: "owner@nerdswhofish.com"}, Identity{Email: "voice@gmail.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateContact(ctx, "nerds-who-fish", workspace.Contact{Name: "Ada"}); err != nil {
		t.Fatal(err)
	}
	if _, err := module.enqueuePendingContactMutations(ctx, "nerds-who-fish", "dispatch"); err != nil {
		t.Fatal(err)
	}
	job := module.jobs.(*MemoryJobQueue).Jobs()[0]
	performJSON[map[string]any](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, job), http.StatusServiceUnavailable)
	if _, found, err := store.GetContactMutation(ctx, "nerds-who-fish", job.OutboxID); err != nil || !found {
		t.Fatalf("ack failure lost intent: %v %v", found, err)
	}
	store.fail = false
	performJSON[map[string]any](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, job), http.StatusOK)
	if _, found, err := store.GetContactMutation(ctx, "nerds-who-fish", job.OutboxID); err != nil || found {
		t.Fatalf("retry did not acknowledge: %v %v", found, err)
	}
}

func TestDisconnectedContactJobKeepsItsIntent(t *testing.T) {
	module, mux, store := newTestModule(t)
	ctx := context.Background()
	if _, err := store.CreateContact(ctx, "nerds-who-fish", workspace.Contact{Name: "Ada"}); err != nil {
		t.Fatal(err)
	}
	if queued, err := module.enqueuePendingContactMutations(ctx, "nerds-who-fish", "disconnected"); err != nil || queued != 0 {
		t.Fatalf("disconnected dispatcher: %d %v", queued, err)
	}
	mutations, err := store.ListContactMutations(ctx, "nerds-who-fish")
	if err != nil || len(mutations) != 1 {
		t.Fatalf("disconnected dispatcher lost intent: %#v %v", mutations, err)
	}
	job := Job{ID: "already-queued", Type: JobTypeGoogleContactSync, Scope: "nerds-who-fish", ConnectionID: voiceContactsConnectionID, ContactID: mutations[0].ContactID, Action: "upsert", OutboxID: mutations[0].ID}
	performJSON[map[string]any](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, job), http.StatusNoContent)
	if _, found, err := store.GetContactMutation(ctx, job.Scope, job.OutboxID); err != nil || !found {
		t.Fatalf("disconnected worker lost intent: %v %v", found, err)
	}
}
