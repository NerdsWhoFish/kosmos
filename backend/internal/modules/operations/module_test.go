package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

type fakeGoogle struct {
	mail        []MailMetadata
	rows        [][]any
	sent        *int
	mailCalls   *int
	tillerCalls *int
	aliases     []string
	aliasesErr  error
	sentFrom    *string
	mailErr     error
	tillerErr   error
	upserted    *[]GoogleContact
	deleted     *[]string
	upsertErr   error
	deleteErr   error
}

func (f fakeGoogle) Send(_ context.Context, _ *oauth2.Token, from, _, _, _ string) (string, error) {
	if f.sent != nil {
		(*f.sent)++
	}
	if f.sentFrom != nil {
		*f.sentFrom = from
	}
	return "sent-1", nil
}

func (f fakeGoogle) SendAsAliases(context.Context, *oauth2.Token) ([]string, error) {
	return f.aliases, f.aliasesErr
}

func (f fakeGoogle) RecentMail(context.Context, *oauth2.Token, time.Time) ([]MailMetadata, error) {
	if f.mailCalls != nil {
		(*f.mailCalls)++
	}
	return f.mail, f.mailErr
}

func (f fakeGoogle) TillerRows(context.Context, *oauth2.Token, TillerSettings) ([][]any, error) {
	if f.tillerCalls != nil {
		(*f.tillerCalls)++
	}
	return f.rows, f.tillerErr
}

func (f fakeGoogle) UpsertContact(_ context.Context, _ *oauth2.Token, contact GoogleContact, resourceName string) (GoogleContactReference, error) {
	if f.upserted != nil {
		*f.upserted = append(*f.upserted, contact)
	}
	if f.upsertErr != nil {
		return GoogleContactReference{}, f.upsertErr
	}
	if resourceName == "" {
		resourceName = "people/" + contact.ID
	}
	return GoogleContactReference{ResourceName: resourceName, ETag: "etag-" + contact.ID}, nil
}

func (f fakeGoogle) DeleteContact(_ context.Context, _ *oauth2.Token, contactID, _ string) error {
	if f.deleted != nil {
		*f.deleted = append(*f.deleted, contactID)
	}
	return f.deleteErr
}

func newTestModule(t *testing.T) (*Module, *http.ServeMux, *workspace.MemoryStore) {
	t.Helper()
	workspaceStore := workspace.NewMemoryStore()
	module := NewModule(NewMemoryStore(), NewMemoryBlobStore(), workspaceStore, func(*http.Request) (string, Identity, error) {
		return "nerds-who-fish", Identity{Subject: "google-1", Email: "owner@nerdswhofish.com", Name: "Owner"}, nil
	}, "nerds-who-fish", []byte("0123456789abcdef0123456789abcdef"), fakeGoogle{
		mail:    []MailMetadata{{ID: "mail-1", From: "Ada <ada@example.com>", Subject: "Ready", ReceivedAt: time.Now().UTC()}},
		rows:    [][]any{{"Date", "Description", "Amount", "Merchant", "Transaction ID"}, {"2026-09-03", "River Labs deposit", "250.00", "River Labs", "row-1"}},
		aliases: []string{"hello@nerdswhofish.com"},
	}, WithJobQueue(NewMemoryJobQueue()))
	mux := http.NewServeMux()
	module.RegisterRoutes(mux)
	module.RegisterJobRoutes(mux)
	return module, mux, workspaceStore
}

func TestTeamPipelineAndNotifications(t *testing.T) {
	_, mux, _ := newTestModule(t)
	members := performJSON[struct {
		Members []Member `json:"members"`
	}](t, mux, http.MethodGet, "/api/v1/members", "", http.StatusOK)
	if len(members.Members) != 1 || members.Members[0].Role != "owner" {
		t.Fatalf("unexpected members: %#v", members.Members)
	}
	stages := performJSON[struct {
		Stages []PipelineStage `json:"stages"`
	}](t, mux, http.MethodGet, "/api/v1/pipeline-stages", "", http.StatusOK)
	if len(stages.Stages) != 5 || stages.Stages[4].ID != "lost" {
		t.Fatalf("unexpected stages: %#v", stages.Stages)
	}
	created := performJSON[PipelineStage](t, mux, http.MethodPost, "/api/v1/pipeline-stages", `{"name":"Negotiation","position":3,"probability":80}`, http.StatusCreated)
	if created.ID != "negotiation" {
		t.Fatalf("stage ID = %q", created.ID)
	}
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/pipeline-stages", `{"name":"Negotiation","position":4,"probability":90}`, http.StatusConflict)
}

func TestOrganizationMustKeepAnActiveOwner(t *testing.T) {
	_, mux, _ := newTestModule(t)
	members := performJSON[struct {
		Members []Member `json:"members"`
	}](t, mux, http.MethodGet, "/api/v1/members", "", http.StatusOK)
	if len(members.Members) != 1 {
		t.Fatalf("members = %d, want 1", len(members.Members))
	}
	performJSON[map[string]any](t, mux, http.MethodPatch, "/api/v1/members/"+members.Members[0].ID, `{"role":"owner","status":"disabled"}`, http.StatusConflict)
	performJSON[map[string]any](t, mux, http.MethodPatch, "/api/v1/members/"+members.Members[0].ID, `{"role":"admin","status":"active"}`, http.StatusConflict)
}

func TestEmailTemplateRequiresBody(t *testing.T) {
	_, mux, _ := newTestModule(t)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/email/templates", `{"name":"Introduction","subject":"Hello","body":"  "}`, http.StatusBadRequest)
}

func TestEmailTemplateCanBeEditedAndDeleted(t *testing.T) {
	_, mux, _ := newTestModule(t)
	created := performJSON[EmailTemplate](t, mux, http.MethodPost, "/api/v1/email/templates", `{"name":"Introduction","subject":"Hello {{name}}","body":"Welcome to {{company}}."}`, http.StatusCreated)
	updated := performJSON[EmailTemplate](t, mux, http.MethodPatch, "/api/v1/email/templates/"+created.ID, `{"name":"Warm introduction","subject":"Hi {{name}}","body":"Welcome to {{company}}. Domains: {{domains}}."}`, http.StatusOK)
	if updated.Name != "Warm introduction" || updated.Subject != "Hi {{name}}" || updated.CreatedAt != created.CreatedAt || !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("unexpected updated template: %#v", updated)
	}
	performJSON[map[string]any](t, mux, http.MethodDelete, "/api/v1/email/templates/"+created.ID, "", http.StatusNoContent)
	performJSON[map[string]any](t, mux, http.MethodPatch, "/api/v1/email/templates/"+created.ID, `{"name":"Missing","subject":"Missing","body":"Missing"}`, http.StatusNotFound)
}

func TestEmailTemplateCustomInputsAreValidatedAndPersisted(t *testing.T) {
	_, mux, _ := newTestModule(t)
	created := performJSON[EmailTemplate](t, mux, http.MethodPost, "/api/v1/email/templates", `{"name":"Renewal","subject":"Your renewal is {{ renewal_amount }}","body":"Hi {{name}}, answer by {{renewal_date}}.","inputs":[{"key":"renewal_amount","label":"How much is their renewal?","defaultValue":"$100"},{"key":"renewal_date","label":"When is it due?","defaultValue":""}]}`, http.StatusCreated)
	if len(created.Inputs) != 2 || created.Inputs[0].Key != "renewal_amount" || created.Inputs[0].DefaultValue != "$100" {
		t.Fatalf("custom inputs were not persisted: %#v", created.Inputs)
	}
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/email/templates", `{"name":"Undefined","subject":"Renewal","body":"Amount: {{renewal_amount}}"}`, http.StatusBadRequest)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/email/templates", `{"name":"Unused","subject":"Renewal","body":"Hello","inputs":[{"key":"renewal_amount","label":"Amount","defaultValue":""}]}`, http.StatusBadRequest)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/email/templates", `{"name":"Reserved","subject":"Hello {{name}}","body":"Hello","inputs":[{"key":"name","label":"Name","defaultValue":""}]}`, http.StatusBadRequest)
}

func TestVoiceLinkSelectsSharedGoogleAccount(t *testing.T) {
	module, mux, _ := newTestModule(t)
	const googleEmail = "shared.voice@gmail.com"
	if err := module.store.Put(context.Background(), "nerds-who-fish", "voiceContactsConnections", voiceContactsConnectionID, VoiceContactsConnection{ID: voiceContactsConnectionID, GoogleEmail: googleEmail}); err != nil {
		t.Fatal(err)
	}

	response := performJSON[map[string]string](t, mux, http.MethodGet, "/api/v1/voice/link?phone=%2B15551234567&mode=call", "", http.StatusOK)
	chooser, err := url.Parse(response["googleVoiceUrl"])
	if err != nil {
		t.Fatal(err)
	}
	if chooser.Host != "accounts.google.com" || chooser.Path != "/AccountChooser" || chooser.Query().Get("Email") != googleEmail {
		t.Fatalf("unexpected account chooser URL: %s", chooser)
	}
	destination, err := url.Parse(chooser.Query().Get("continue"))
	if err != nil {
		t.Fatal(err)
	}
	if destination.Host != "voice.google.com" || destination.Path != "/search" || destination.Query().Get("authuser") != googleEmail || destination.Query().Get("from") != "[]" || destination.Query().Get("q") != `["+15551234567"]` {
		t.Fatalf("unexpected Voice destination: %s", destination)
	}
	if response["googleAccount"] != googleEmail {
		t.Fatalf("google account = %q", response["googleAccount"])
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/voice/link?phone=%2B15551234567&redirect=1", nil)
	request.Header.Set("X-Test-User", "owner@nerdswhofish.com")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther || !strings.Contains(recorder.Header().Get("Location"), "accounts.google.com/AccountChooser") {
		t.Fatalf("redirect = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestGoogleMailAndTillerFlow(t *testing.T) {
	module, mux, workspaceStore := newTestModule(t)
	account, _, err := workspaceStore.CreateAccountWithContact(context.Background(), "nerds-who-fish", workspace.Account{Name: "River Labs", Status: "prospect"}, workspace.Contact{Name: "Ada", Email: "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := module.SaveGoogleGrant(context.Background(), Identity{Email: "owner@nerdswhofish.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := module.connectionToken(context.Background(), "nerds-who-fish", "owner@nerdswhofish.com"); err != nil {
		t.Fatalf("open saved token: %v", err)
	}
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/email/send", `{"to":"ada@example.com","subject":"Hello","body":"Ready to fish?"}`, http.StatusCreated)
	queuedMail := performJSON[map[string]string](t, mux, http.MethodPost, "/api/v1/email/sync", `{}`, http.StatusAccepted)
	mailJob := module.jobs.(*MemoryJobQueue).Jobs()[0]
	performJSON[map[string]string](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, mailJob), http.StatusOK)
	if queuedMail["status"] != "accepted" {
		t.Fatalf("mail sync status = %q", queuedMail["status"])
	}
	performJSON[GoogleConnection](t, mux, http.MethodPut, "/api/v1/integrations/tiller", `{"spreadsheetId":"sheet-1","range":"Transactions!A:Z"}`, http.StatusOK)
	performJSON[map[string]string](t, mux, http.MethodPost, "/api/v1/integrations/tiller/sync", `{}`, http.StatusAccepted)
	tillerJob := module.jobs.(*MemoryJobQueue).Jobs()[1]
	performJSON[map[string]string](t, mux, http.MethodPost, jobsBasePath+"/execute", mustJSON(t, tillerJob), http.StatusOK)
	transactions := performJSON[struct {
		Transactions []Transaction `json:"transactions"`
	}](t, mux, http.MethodGet, "/api/v1/transactions", "", http.StatusOK)
	if len(transactions.Transactions) != 1 || transactions.Transactions[0].MatchStatus != "matched" || transactions.Transactions[0].AccountID != account.ID {
		t.Fatalf("unexpected transactions: %#v", transactions.Transactions)
	}
	notifications := performJSON[struct {
		Notifications []Notification `json:"notifications"`
	}](t, mux, http.MethodGet, "/api/v1/notifications", "", http.StatusOK)
	var tillerNotification *Notification
	for index := range notifications.Notifications {
		if notifications.Notifications[index].Kind == "transaction" {
			tillerNotification = &notifications.Notifications[index]
			break
		}
	}
	if tillerNotification == nil || tillerNotification.Href != "/operations" {
		t.Fatalf("unexpected business operation notifications: %#v", notifications.Notifications)
	}
}

func TestGoogleConnectionSecretMigration(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	legacyKey := []byte("legacy-secret-0123456789abcdef0123")
	currentKey := []byte("current-secret-0123456789abcdef012")
	legacy := NewModule(store, NewMemoryBlobStore(), workspace.NewMemoryStore(), nil, "nerds-who-fish", legacyKey, fakeGoogle{})
	if err := legacy.SaveGoogleGrant(ctx, Identity{Email: "owner@nerdswhofish.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh"}); err != nil {
		t.Fatal(err)
	}
	current := NewModule(store, NewMemoryBlobStore(), workspace.NewMemoryStore(), nil, "nerds-who-fish", currentKey, fakeGoogle{})
	migrated, err := current.MigrateGoogleConnectionSecrets(ctx, legacyKey)
	if err != nil || migrated != 1 {
		t.Fatalf("migration = %d, %v", migrated, err)
	}
	if _, _, err := current.connectionToken(ctx, "nerds-who-fish", "owner@nerdswhofish.com"); err != nil {
		t.Fatalf("open migrated token: %v", err)
	}
	migrated, err = current.MigrateGoogleConnectionSecrets(ctx, legacyKey)
	if err != nil || migrated != 0 {
		t.Fatalf("repeat migration = %d, %v", migrated, err)
	}
}

func TestOverlappingIntegrationJobsCreateBusinessEffectsOnce(t *testing.T) {
	module, _, workspaceStore := newTestModule(t)
	ctx := context.Background()
	if _, err := workspaceStore.CreateContact(ctx, "nerds-who-fish", workspace.Contact{Name: "Ada", Email: "ada@example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := module.SaveGoogleGrant(ctx, Identity{Email: "owner@nerdswhofish.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	connectionID := memberID("owner@nerdswhofish.com")
	var connection GoogleConnection
	if err := module.store.Get(ctx, "nerds-who-fish", "googleConnections", connectionID, &connection); err != nil {
		t.Fatal(err)
	}
	connection.Tiller = &TillerSettings{SpreadsheetID: "sheet-1", Range: "Transactions!A:Z"}
	if err := module.store.Put(ctx, "nerds-who-fish", "googleConnections", connectionID, connection); err != nil {
		t.Fatal(err)
	}

	runTwice := func(run func(string) error) {
		t.Helper()
		errors := make(chan error, 2)
		for _, jobID := range []string{"manual-job", "scheduled-job"} {
			go func(id string) { errors <- run(id) }(jobID)
		}
		for range 2 {
			if err := <-errors; err != nil {
				t.Fatal(err)
			}
		}
	}
	runTwice(func(jobID string) error {
		_, err := module.syncEmailConnection(ctx, "nerds-who-fish", connectionID, "system", jobID)
		return err
	})
	runTwice(func(jobID string) error {
		_, _, err := module.syncTillerConnection(ctx, "nerds-who-fish", connectionID, "system", jobID)
		return err
	})

	assertCount := func(collection string, target any, want int) {
		t.Helper()
		if err := module.store.List(ctx, "nerds-who-fish", collection, target); err != nil {
			t.Fatal(err)
		}
		value := reflect.ValueOf(target)
		if got := value.Elem().Len(); got != want {
			t.Fatalf("%s count = %d, want %d", collection, got, want)
		}
	}
	assertCount("mailMetadata", &[]MailMetadata{}, 1)
	assertCount("transactions", &[]Transaction{}, 1)
	assertCount("notifications", &[]Notification{}, 2)
	audits := []AuditEntry{}
	assertCount("audit", &audits, 3)
	actions := map[string]int{}
	for _, entry := range audits {
		actions[entry.Action]++
	}
	if actions["email.synced"] != 1 || actions["tiller.synced"] != 1 {
		t.Fatalf("integration audit actions = %#v", actions)
	}
}

func TestManualSyncQueuesWithoutCallingGoogle(t *testing.T) {
	module, mux, _ := newTestModule(t)
	mailCalls, tillerCalls := 0, 0
	module.google = fakeGoogle{mailCalls: &mailCalls, tillerCalls: &tillerCalls}
	if err := module.SaveGoogleGrant(context.Background(), Identity{Email: "owner@nerdswhofish.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var connection GoogleConnection
	connectionID := memberID("owner@nerdswhofish.com")
	if err := module.store.Get(context.Background(), "nerds-who-fish", "googleConnections", connectionID, &connection); err != nil {
		t.Fatal(err)
	}
	connection.Tiller = &TillerSettings{SpreadsheetID: "sheet-1", Range: "Transactions!A:Z"}
	if err := module.store.Put(context.Background(), "nerds-who-fish", "googleConnections", connectionID, connection); err != nil {
		t.Fatal(err)
	}

	mail := performJSON[map[string]string](t, mux, http.MethodPost, "/api/v1/email/sync", `{}`, http.StatusAccepted)
	tiller := performJSON[map[string]string](t, mux, http.MethodPost, "/api/v1/integrations/tiller/sync", `{}`, http.StatusAccepted)
	if mail["status"] != "accepted" || tiller["status"] != "accepted" || mailCalls != 0 || tillerCalls != 0 {
		t.Fatalf("manual sync performed inline: mail=%#v tiller=%#v calls=%d/%d", mail, tiller, mailCalls, tillerCalls)
	}
	jobs := module.jobs.(*MemoryJobQueue).Jobs()
	if len(jobs) != 2 || jobs[0].Type != JobTypeGmailSync || jobs[1].Type != JobTypeTillerSync {
		t.Fatalf("queued jobs = %#v", jobs)
	}
}

func TestSchedulerQueuesEveryGoogleConnectionAndConfiguredTiller(t *testing.T) {
	module, mux, _ := newTestModule(t)
	for _, email := range []string{"owner@nerdswhofish.com", "admin@nerdswhofish.com"} {
		if err := module.SaveGoogleGrant(context.Background(), Identity{Email: email}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	ownerID := memberID("owner@nerdswhofish.com")
	var owner GoogleConnection
	if err := module.store.Get(context.Background(), "nerds-who-fish", "googleConnections", ownerID, &owner); err != nil {
		t.Fatal(err)
	}
	owner.Tiller = &TillerSettings{SpreadsheetID: "sheet-1", Range: "Transactions!A:Z"}
	if err := module.store.Put(context.Background(), "nerds-who-fish", "googleConnections", ownerID, owner); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "https://kosmos-jobs-hash-ue.a.run.app"+jobsBasePath+"/schedule", nil)
	request.Header.Set("X-CloudScheduler-ScheduleTime", "2026-09-04T13:00:00Z")
	record := httptest.NewRecorder()
	mux.ServeHTTP(record, request)
	if record.Code != http.StatusAccepted {
		t.Fatalf("schedule status = %d: %s", record.Code, record.Body.String())
	}
	jobs := module.jobs.(*MemoryJobQueue).Jobs()
	counts := map[string]int{}
	for _, job := range jobs {
		counts[job.Type]++
		if job.Actor != "system" || job.ID == "" {
			t.Fatalf("scheduled job = %#v", job)
		}
	}
	if len(jobs) != 3 || counts[JobTypeGmailSync] != 2 || counts[JobTypeTillerSync] != 1 {
		t.Fatalf("scheduled jobs = %#v", jobs)
	}
	repeat := httptest.NewRecorder()
	repeatRequest := httptest.NewRequest(http.MethodPost, "https://kosmos-jobs-hash-ue.a.run.app"+jobsBasePath+"/schedule", nil)
	repeatRequest.Header.Set("X-CloudScheduler-ScheduleTime", "2026-09-04T13:00:00Z")
	mux.ServeHTTP(repeat, repeatRequest)
	if repeat.Code != http.StatusAccepted || len(module.jobs.(*MemoryJobQueue).Jobs()) != 3 {
		t.Fatalf("repeated schedule = %d, jobs = %#v", repeat.Code, module.jobs.(*MemoryJobQueue).Jobs())
	}
}

func TestWorkerExecutionIsIdempotent(t *testing.T) {
	module, mux, workspaceStore := newTestModule(t)
	mailCalls := 0
	module.google = fakeGoogle{mailCalls: &mailCalls, mail: []MailMetadata{{ID: "mail-1", From: "Ada <ada@example.com>", Subject: "Ready", ReceivedAt: time.Now().UTC()}}}
	if _, err := workspaceStore.CreateContact(context.Background(), "nerds-who-fish", workspace.Contact{Name: "Ada", Email: "ada@example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := module.SaveGoogleGrant(context.Background(), Identity{Email: "owner@nerdswhofish.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	job := Job{ID: "job-idempotent", Type: JobTypeGmailSync, Scope: "nerds-who-fish", ConnectionID: memberID("owner@nerdswhofish.com"), Actor: "system"}
	body := mustJSON(t, job)
	performJSON[map[string]string](t, mux, http.MethodPost, jobsBasePath+"/execute", body, http.StatusOK)
	performJSON[map[string]string](t, mux, http.MethodPost, jobsBasePath+"/execute", body, http.StatusOK)
	if mailCalls != 1 {
		t.Fatalf("mail provider calls = %d, want 1", mailCalls)
	}
	var execution JobExecution
	if err := module.store.Get(context.Background(), job.Scope, "jobExecutions", job.ID, &execution); err != nil || execution.Status != "completed" {
		t.Fatalf("execution = %#v, err = %v", execution, err)
	}
}

func TestWorkerFailureRemainsRetryable(t *testing.T) {
	module, mux, _ := newTestModule(t)
	mailCalls := 0
	module.google = fakeGoogle{mailCalls: &mailCalls, mailErr: errors.New("google unavailable")}
	if err := module.SaveGoogleGrant(context.Background(), Identity{Email: "owner@nerdswhofish.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	job := Job{ID: "job-retry", Type: JobTypeGmailSync, Scope: "nerds-who-fish", ConnectionID: memberID("owner@nerdswhofish.com"), Actor: "system"}
	body := mustJSON(t, job)
	performJSON[map[string]any](t, mux, http.MethodPost, jobsBasePath+"/execute", body, http.StatusBadGateway)
	performJSON[map[string]any](t, mux, http.MethodPost, jobsBasePath+"/execute", body, http.StatusBadGateway)
	if mailCalls != 2 {
		t.Fatalf("mail provider calls = %d, want 2", mailCalls)
	}
}

func TestEmailSendIsIdempotent(t *testing.T) {
	module, mux, _ := newTestModule(t)
	count := 0
	module.google = fakeGoogle{sent: &count}
	if err := module.SaveGoogleGrant(context.Background(), Identity{Email: "owner@nerdswhofish.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	body := `{"to":"ada@example.com","subject":"Hello","body":"Ready to fish?"}`
	performJSON[map[string]string](t, mux, http.MethodPost, "/api/v1/email/send", body, http.StatusCreated)
	performJSON[map[string]string](t, mux, http.MethodPost, "/api/v1/email/send", body, http.StatusOK)
	if count != 1 {
		t.Fatalf("provider sends = %d, want 1", count)
	}
}

func TestAdminMapsVerifiedGmailSendAsAlias(t *testing.T) {
	module, mux, _ := newTestModule(t)
	from := ""
	module.google = fakeGoogle{aliases: []string{"hello@nerdswhofish.com"}, sentFrom: &from}
	members := performJSON[struct {
		Members []Member `json:"members"`
	}](t, mux, http.MethodGet, "/api/v1/members", "", http.StatusOK)
	if err := module.SaveGoogleGrant(context.Background(), Identity{Email: "owner@nerdswhofish.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	mapping := performJSON[SendAsMapping](t, mux, http.MethodPut, "/api/v1/members/"+members.Members[0].ID+"/send-as", `{"email":"hello@nerdswhofish.com"}`, http.StatusOK)
	if mapping.Email != "hello@nerdswhofish.com" {
		t.Fatalf("send-as mapping = %#v", mapping)
	}
	performJSON[map[string]any](t, mux, http.MethodPut, "/api/v1/members/"+members.Members[0].ID+"/send-as", `{"email":"spoof@example.com"}`, http.StatusBadRequest)
	performJSON[map[string]string](t, mux, http.MethodPost, "/api/v1/email/send", `{"to":"ada@example.com","subject":"Hello","body":"Ready to fish?"}`, http.StatusCreated)
	if from != mapping.Email {
		t.Fatalf("email sent from %q, want %q", from, mapping.Email)
	}
	listed := performJSON[struct {
		Mappings []SendAsMapping `json:"mappings"`
	}](t, mux, http.MethodGet, "/api/v1/email/send-as", "", http.StatusOK)
	if len(listed.Mappings) != 1 || listed.Mappings[0].MemberID != members.Members[0].ID {
		t.Fatalf("send-as mappings = %#v", listed.Mappings)
	}
}

func TestSendAsPermissionFailureRequestsReconnect(t *testing.T) {
	module, mux, _ := newTestModule(t)
	module.google = fakeGoogle{aliasesErr: &googleapi.Error{Code: http.StatusForbidden, Message: "insufficient authentication scopes"}}
	members := performJSON[struct {
		Members []Member `json:"members"`
	}](t, mux, http.MethodGet, "/api/v1/members", "", http.StatusOK)
	if err := module.SaveGoogleGrant(context.Background(), Identity{Email: "owner@nerdswhofish.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	response := performJSON[struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}](t, mux, http.MethodPut, "/api/v1/members/"+members.Members[0].ID+"/send-as", `{"email":"hello@nerdswhofish.com"}`, http.StatusConflict)
	if response.Error.Code != "send_as_permission_required" {
		t.Fatalf("error code = %q", response.Error.Code)
	}
}

func TestContactIntakeDeduplicatesAndNotifies(t *testing.T) {
	_, mux, workspaceStore := newTestModule(t)
	body := `{"name":"Ada Angler","email":"ada@example.com","message":"Need a website","source":"website"}`
	performJSON[map[string]string](t, mux, http.MethodPost, "/api/v1/intake/contact", body, http.StatusCreated)
	record := httptest.NewRecorder()
	mux.ServeHTTP(record, httptest.NewRequest(http.MethodPost, "/api/v1/intake/contact", strings.NewReader(body)))
	if record.Code != http.StatusAccepted {
		t.Fatalf("duplicate status = %d", record.Code)
	}
	contacts, _ := workspaceStore.ListContacts(context.Background(), "nerds-who-fish")
	if len(contacts) != 1 || contacts[0].Source != "website" {
		t.Fatalf("unexpected contacts: %#v", contacts)
	}
	notifications := performJSON[struct {
		Notifications []Notification `json:"notifications"`
	}](t, mux, http.MethodGet, "/api/v1/notifications", "", http.StatusOK)
	if len(notifications.Notifications) != 1 || notifications.Notifications[0].Kind != "lead" {
		t.Fatalf("unexpected notifications: %#v", notifications.Notifications)
	}
}

func TestContactIntakeReusesAnExistingAccount(t *testing.T) {
	_, mux, workspaceStore := newTestModule(t)
	performJSON[map[string]string](t, mux, http.MethodPost, "/api/v1/intake/contact", `{"name":"Ada","email":"ada@example.com","company":"River Labs","website":""}`, http.StatusCreated)
	performJSON[map[string]string](t, mux, http.MethodPost, "/api/v1/intake/contact", `{"name":"Grace","email":"grace@example.com","company":"river labs","website":""}`, http.StatusCreated)
	contacts, err := workspaceStore.ListContacts(context.Background(), "nerds-who-fish")
	if err != nil || len(contacts) != 2 || contacts[0].AccountID == "" || contacts[0].AccountID != contacts[1].AccountID {
		t.Fatalf("contacts = %#v, %v", contacts, err)
	}
	accounts, err := workspaceStore.ListAccounts(context.Background(), "nerds-who-fish")
	if err != nil || len(accounts) != 1 {
		t.Fatalf("accounts = %#v, %v", accounts, err)
	}
}

func TestPrivateAttachmentUsesExpiringDownload(t *testing.T) {
	_, mux, _ := newTestModule(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "receipt.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("receipt"))
	_ = writer.WriteField("kind", "attachment")
	_ = writer.WriteField("recordType", "cost")
	_ = writer.WriteField("recordId", "cost-1")
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/attachments", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	record := httptest.NewRecorder()
	mux.ServeHTTP(record, request)
	if record.Code != http.StatusCreated {
		t.Fatalf("upload status = %d: %s", record.Code, record.Body.String())
	}
	var attachment Attachment
	if err := json.NewDecoder(record.Body).Decode(&attachment); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(attachment.DownloadURL, "expires=") || !strings.Contains(attachment.DownloadURL, "signature=") {
		t.Fatalf("download URL is not signed: %q", attachment.DownloadURL)
	}
	if !strings.Contains(attachment.ViewURL, "disposition=inline") {
		t.Fatalf("view URL is not inline: %q", attachment.ViewURL)
	}
	download := httptest.NewRecorder()
	mux.ServeHTTP(download, httptest.NewRequest(http.MethodGet, attachment.DownloadURL, nil))
	if download.Code != http.StatusOK || download.Body.String() != "receipt" {
		t.Fatalf("download = %d %q", download.Code, download.Body.String())
	}
	deleted := httptest.NewRecorder()
	mux.ServeHTTP(deleted, httptest.NewRequest(http.MethodDelete, "/api/v1/attachments/"+attachment.ID, nil))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", deleted.Code, deleted.Body.String())
	}
	missing := httptest.NewRecorder()
	mux.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, attachment.DownloadURL, nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted attachment download = %d", missing.Code)
	}
}

func TestCostDeletionRemovesLinkedAttachments(t *testing.T) {
	module, _, _ := newTestModule(t)
	attachment := Attachment{ID: "receipt-1", ObjectName: "organizations/nerds-who-fish/receipt-1", RecordType: "cost", RecordID: "cost-1"}
	if err := module.store.Put(context.Background(), "nerds-who-fish", "attachments", attachment.ID, attachment); err != nil {
		t.Fatal(err)
	}
	if err := module.blobs.Put(context.Background(), attachment.ObjectName, "text/plain", strings.NewReader("receipt")); err != nil {
		t.Fatal(err)
	}
	if err := module.DeleteCostAttachments(context.Background(), "nerds-who-fish", "cost-1"); err != nil {
		t.Fatal(err)
	}
	var stored Attachment
	if err := module.store.Get(context.Background(), "nerds-who-fish", "attachments", attachment.ID, &stored); !errors.Is(err, errNotFound) {
		t.Fatalf("attachment still stored: %v", err)
	}
	if _, err := module.blobs.Open(context.Background(), attachment.ObjectName); !errors.Is(err, errNotFound) {
		t.Fatalf("attachment blob still stored: %v", err)
	}
}

func TestViewerCannotMutateCoreModules(t *testing.T) {
	module, _, _ := newTestModule(t)
	actor := Identity{Email: "viewer@nerdswhofish.com", Name: "Viewer"}
	member, err := module.ensureMember(context.Background(), "nerds-who-fish", actor)
	if err != nil {
		t.Fatal(err)
	}
	member.Role = "viewer"
	if err := module.store.Put(context.Background(), "nerds-who-fish", "members", member.ID, member); err != nil {
		t.Fatal(err)
	}
	if err := module.CheckAccess(context.Background(), "nerds-who-fish", actor, true); err == nil {
		t.Fatal("viewer mutation should be denied")
	}
}

func TestViewerCannotMutateOperations(t *testing.T) {
	module, mux, _ := newTestModule(t)
	actor := Identity{Email: "viewer@nerdswhofish.com", Name: "Viewer"}
	member, err := module.ensureMember(context.Background(), "nerds-who-fish", actor)
	if err != nil {
		t.Fatal(err)
	}
	member.Role = "viewer"
	if err := module.store.Put(context.Background(), "nerds-who-fish", "members", member.ID, member); err != nil {
		t.Fatal(err)
	}
	module.identity = func(*http.Request) (string, Identity, error) { return "nerds-who-fish", actor, nil }
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/email/templates", `{"name":"Nope","subject":"Nope","body":"Nope"}`, http.StatusForbidden)
}

func TestOperationsListsArePaginated(t *testing.T) {
	module, mux, _ := newTestModule(t)
	for _, actor := range []Identity{{Email: "admin@nerdswhofish.com", Name: "Admin"}, {Email: "member@nerdswhofish.com", Name: "Member"}} {
		if _, err := module.ensureMember(context.Background(), "nerds-who-fish", actor); err != nil {
			t.Fatal(err)
		}
	}
	first := performJSON[struct {
		Members []Member `json:"members"`
		Page    struct {
			Limit      int    `json:"limit"`
			NextCursor string `json:"nextCursor"`
		} `json:"page"`
	}](t, mux, http.MethodGet, "/api/v1/members?limit=2", "", http.StatusOK)
	if len(first.Members) != 2 || first.Page.Limit != 2 || first.Page.NextCursor == "" {
		t.Fatalf("first page = %#v", first)
	}
	second := performJSON[struct {
		Members []Member `json:"members"`
	}](t, mux, http.MethodGet, "/api/v1/members?limit=2&cursor="+first.Page.NextCursor, "", http.StatusOK)
	if len(second.Members) != 1 || second.Members[0].ID == first.Members[0].ID || second.Members[0].ID == first.Members[1].ID {
		t.Fatalf("second page = %#v", second)
	}
	performJSON[map[string]any](t, mux, http.MethodGet, "/api/v1/members?cursor=broken", "", http.StatusBadRequest)
}

func TestOperationsListHandlersUsePagedStore(t *testing.T) {
	module, mux, _ := newTestModule(t)
	store := &operationsPageSpy{Store: module.store}
	module.store = store
	performJSON[map[string]any](t, mux, http.MethodGet, "/api/v1/members?limit=1", "", http.StatusOK)
	if store.calls != 1 || store.collection != "members" || store.request.Limit != 1 {
		t.Fatalf("paged store calls = %d, collection = %q, request = %#v", store.calls, store.collection, store.request)
	}
}

type operationsPageSpy struct {
	Store
	calls      int
	collection string
	request    pagination.Request
}

func (s *operationsPageSpy) ListPage(ctx context.Context, scope, collection string, request pagination.Request, spec pagination.Spec, target any) (pagination.Metadata, error) {
	s.calls++
	s.collection = collection
	s.request = request
	return s.Store.ListPage(ctx, scope, collection, request, spec, target)
}

func performJSON[T any](t *testing.T, handler http.Handler, method, target, body string, wantStatus int) T {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if method == http.MethodPost {
		request.Header.Set("Idempotency-Key", "test-"+deterministicID(target+body))
	}
	record := httptest.NewRecorder()
	handler.ServeHTTP(record, request)
	if record.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d: %s", method, target, record.Code, wantStatus, record.Body.String())
	}
	var response T
	if record.Body.Len() != 0 {
		if err := json.NewDecoder(record.Body).Decode(&response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return response
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
