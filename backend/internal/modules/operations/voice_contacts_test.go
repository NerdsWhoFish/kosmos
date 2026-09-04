package operations

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
	"golang.org/x/oauth2"
)

func TestSharedGoogleContactsConnectionAndMutationJobs(t *testing.T) {
	module, mux, workspaceStore := newTestModule(t)
	account, err := workspaceStore.CreateAccount(context.Background(), "nerds-who-fish", workspace.Account{Name: "River Labs", Status: "prospect"})
	if err != nil {
		t.Fatal(err)
	}
	contact, err := workspaceStore.CreateContact(context.Background(), "nerds-who-fish", workspace.Contact{AccountID: account.ID, Name: "Ada Angler", Email: "ada@example.com", Phone: "+15551234567", LinkedInURL: "https://linkedin.com/in/ada"})
	if err != nil {
		t.Fatal(err)
	}
	upserted := []GoogleContact{}
	deleted := []string{}
	module.google = fakeGoogle{upserted: &upserted, deleted: &deleted}
	token := &oauth2.Token{AccessToken: "contacts-access", RefreshToken: "contacts-refresh", Expiry: time.Now().Add(time.Hour)}
	if err := module.SaveVoiceContactsGrant(
		context.Background(),
		Identity{Subject: "admin-subject", Email: "owner@nerdswhofish.com"},
		Identity{Subject: "voice-subject", Email: "shared.voice@gmail.com"},
		token,
	); err != nil {
		t.Fatal(err)
	}
	var stored VoiceContactsConnection
	if err := module.store.Get(context.Background(), "nerds-who-fish", "voiceContactsConnections", voiceContactsConnectionID, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.GoogleEmail != "shared.voice@gmail.com" || stored.GoogleSubject != "voice-subject" || stored.EncryptedToken == "" || stored.EncryptedToken == "contacts-access" {
		t.Fatalf("stored connection = %#v", stored)
	}
	status := performJSON[struct {
		Connected   bool   `json:"connected"`
		GoogleEmail string `json:"googleEmail"`
		Pending     int    `json:"pending"`
	}](t, mux, http.MethodGet, "/api/v1/integrations/google-contacts", "", http.StatusOK)
	if !status.Connected || status.GoogleEmail != "shared.voice@gmail.com" || status.Pending != 1 {
		t.Fatalf("status = %#v", status)
	}
	jobs := module.jobs.(*MemoryJobQueue).Jobs()
	if len(jobs) != 1 || jobs[0].Type != JobTypeGoogleContactSync || jobs[0].ContactID != contact.ID || jobs[0].Action != "upsert" {
		t.Fatalf("jobs = %#v", jobs)
	}
	performJSON[map[string]string](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, jobs[0]), http.StatusOK)
	if len(upserted) != 1 || upserted[0].Name != "Ada Angler" || upserted[0].Organization != "River Labs" || upserted[0].Phone != "+15551234567" {
		t.Fatalf("upserted = %#v", upserted)
	}
	var mapping GoogleContactMapping
	if err := module.store.Get(context.Background(), "nerds-who-fish", "googleContactMappings", contact.ID, &mapping); err != nil {
		t.Fatal(err)
	}
	if mapping.Status != "synced" || mapping.ResourceName != "people/"+contact.ID || mapping.LastSyncedAt == nil {
		t.Fatalf("mapping = %#v", mapping)
	}
	if err := workspaceStore.DeleteContact(context.Background(), "nerds-who-fish", contact.ID); err != nil {
		t.Fatal(err)
	}
	if err := module.EnqueueGoogleContactMutation(context.Background(), "nerds-who-fish", contact, "delete", "owner@nerdswhofish.com"); err != nil {
		t.Fatal(err)
	}
	jobs = module.jobs.(*MemoryJobQueue).Jobs()
	performJSON[map[string]string](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, jobs[len(jobs)-1]), http.StatusOK)
	if len(deleted) != 1 || deleted[0] != contact.ID {
		t.Fatalf("deleted = %#v", deleted)
	}
	if err := module.store.Get(context.Background(), "nerds-who-fish", "googleContactMappings", contact.ID, &mapping); !errors.Is(err, errNotFound) {
		t.Fatalf("mapping remained after delete: %v", err)
	}
	performJSON[map[string]any](t, mux, http.MethodDelete, "/api/v1/integrations/google-contacts", "", http.StatusNoContent)
	disconnected := performJSON[struct {
		Connected bool `json:"connected"`
	}](t, mux, http.MethodGet, "/api/v1/integrations/google-contacts", "", http.StatusOK)
	if disconnected.Connected {
		t.Fatal("shared Google Contacts connection remained after disconnect")
	}
}

func TestGoogleContactSyncFailureIsVisibleAndRetryable(t *testing.T) {
	module, mux, workspaceStore := newTestModule(t)
	contact, err := workspaceStore.CreateContact(context.Background(), "nerds-who-fish", workspace.Contact{Name: "Grace Hopper"})
	if err != nil {
		t.Fatal(err)
	}
	module.google = fakeGoogle{upsertErr: errors.New("provider unavailable")}
	if err := module.SaveVoiceContactsGrant(
		context.Background(),
		Identity{Subject: "admin-subject", Email: "owner@nerdswhofish.com"},
		Identity{Subject: "voice-subject", Email: "shared.voice@gmail.com"},
		&oauth2.Token{AccessToken: "access", RefreshToken: "refresh"},
	); err != nil {
		t.Fatal(err)
	}
	job := module.jobs.(*MemoryJobQueue).Jobs()[0]
	performJSON[map[string]any](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, job), http.StatusBadGateway)
	var mapping GoogleContactMapping
	if err := module.store.Get(context.Background(), "nerds-who-fish", "googleContactMappings", contact.ID, &mapping); err != nil {
		t.Fatal(err)
	}
	if mapping.Status != "failed" || mapping.LastError == "" {
		t.Fatalf("mapping = %#v", mapping)
	}
}

func TestSharedGoogleContactsReconnectKeepsTheRefreshToken(t *testing.T) {
	module, _, _ := newTestModule(t)
	actor := Identity{Subject: "admin-subject", Email: "owner@nerdswhofish.com"}
	connected := Identity{Subject: "voice-subject", Email: "shared.voice@gmail.com"}
	if err := module.SaveVoiceContactsGrant(context.Background(), actor, connected, &oauth2.Token{AccessToken: "first", RefreshToken: "keep-me"}); err != nil {
		t.Fatal(err)
	}
	if err := module.SaveVoiceContactsGrant(context.Background(), actor, connected, &oauth2.Token{AccessToken: "second"}); err != nil {
		t.Fatal(err)
	}
	storedToken, _, err := module.voiceContactsToken(context.Background(), "nerds-who-fish")
	if err != nil {
		t.Fatal(err)
	}
	if storedToken.AccessToken != "second" || storedToken.RefreshToken != "keep-me" {
		t.Fatalf("stored token = %#v", storedToken)
	}
}

func TestSharedGoogleContactsRequiresAdministrator(t *testing.T) {
	module, mux, _ := newTestModule(t)
	members := performJSON[struct {
		Members []Member `json:"members"`
	}](t, mux, http.MethodGet, "/api/v1/members", "", http.StatusOK)
	performJSON[Member](t, mux, http.MethodPatch, "/api/v1/members/"+members.Members[0].ID, `{"role":"member","status":"active"}`, http.StatusConflict)
	if err := module.AuthorizeVoiceContacts(context.Background(), Identity{Email: "missing@example.com"}); err == nil {
		t.Fatal("non-member was allowed to connect shared Google Contacts")
	}
}
