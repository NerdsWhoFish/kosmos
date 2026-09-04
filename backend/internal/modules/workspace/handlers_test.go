package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
)

func TestWorkspaceCoreFlow(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)

	createdAccount := performJSON[struct {
		Account Account `json:"account"`
		Contact Contact `json:"contact"`
	}](t, mux, http.MethodPost, "/api/v1/accounts", `{"name":"River Labs","status":"prospect","websites":[{"url":"river.example"},{"url":"shop.river.example"}],"links":[{"label":"Project tracker","url":"https://docs.google.com/spreadsheets/d/sheet/edit#gid=12"}],"primaryContact":{"name":"Ada Angler","email":"ada@example.com","linkedinUrl":"www.linkedin.com/in/ada-angler","source":"referral"}}`, http.StatusCreated)
	account := createdAccount.Account
	contact := createdAccount.Contact
	if len(account.Websites) != 2 || account.Websites[0].Domain != "river.example" || account.Websites[1].Domain != "shop.river.example" {
		t.Fatalf("unexpected account websites: %#v", account.Websites)
	}
	if len(account.Links) != 1 || account.Links[0].Label != "Project tracker" || !strings.HasSuffix(account.Links[0].URL, "#gid=12") {
		t.Fatalf("unexpected account links: %#v", account.Links)
	}
	if contact.ID == "" || contact.Name != "Ada Angler" || contact.AccountID != account.ID || contact.LinkedInURL != "https://www.linkedin.com/in/ada-angler" {
		t.Fatalf("unexpected contact: %#v", contact)
	}
	contact = performJSON[Contact](t, mux, http.MethodPatch, "/api/v1/contacts/"+contact.ID, `{"linkedinUrl":"https://linkedin.com/in/ada-updated"}`, http.StatusOK)
	if contact.LinkedInURL != "https://linkedin.com/in/ada-updated" {
		t.Fatalf("contact LinkedIn URL = %q", contact.LinkedInURL)
	}

	opportunity := performJSON[Opportunity](t, mux, http.MethodPost, "/api/v1/opportunities", `{"name":"Website refresh","accountId":"`+account.ID+`","contactId":"`+contact.ID+`","amountCents":125000,"stage":"qualified","nextStep":"Send proposal"}`, http.StatusCreated)
	if opportunity.Stage != "qualified" || opportunity.AccountID != account.ID {
		t.Fatalf("opportunity stage = %q", opportunity.Stage)
	}

	activity := performJSON[Activity](t, mux, http.MethodPost, "/api/v1/activities", `{"contactId":"`+contact.ID+`","kind":"note","body":"Asked for a launch plan."}`, http.StatusCreated)
	if activity.ID == "" {
		t.Fatal("activity did not receive an ID")
	}

	due := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	reminder := performJSON[Reminder](t, mux, http.MethodPost, "/api/v1/reminders", `{"contactId":"`+contact.ID+`","title":"Send proposal","dueAt":"`+due+`"}`, http.StatusCreated)
	if reminder.Completed {
		t.Fatal("new reminder must be incomplete")
	}

	document := performJSON[Document](t, mux, http.MethodPost, "/api/v1/documents", `{"title":"Client kickoff","body":"# Agenda\n\nConfirm launch date."}`, http.StatusCreated)
	if document.Title != "Client kickoff" {
		t.Fatalf("document title = %q", document.Title)
	}
	updatedDocument := performJSON[Document](t, mux, http.MethodPatch, "/api/v1/documents/"+document.ID, `{"body":"# Revised agenda","links":[{"type":"account","id":"`+account.ID+`"}]}`, http.StatusOK)
	if updatedDocument.Revision != 2 || len(updatedDocument.Links) != 1 {
		t.Fatalf("unexpected updated document: %#v", updatedDocument)
	}
	revisions := performJSON[struct {
		Revisions []DocumentRevision `json:"revisions"`
	}](t, mux, http.MethodGet, "/api/v1/documents/"+document.ID+"/revisions", "", http.StatusOK)
	if len(revisions.Revisions) != 1 || revisions.Revisions[0].Revision != 1 {
		t.Fatalf("unexpected revisions: %#v", revisions.Revisions)
	}
	leads := performJSON[struct {
		Leads []Contact `json:"leads"`
	}](t, mux, http.MethodGet, "/api/v1/leads", "", http.StatusOK)
	if len(leads.Leads) != 0 {
		t.Fatalf("unexpected leads: %#v", leads.Leads)
	}
	accountDetail := performJSON[struct {
		Contacts      []Contact     `json:"contacts"`
		Opportunities []Opportunity `json:"opportunities"`
		Documents     []Document    `json:"documents"`
	}](t, mux, http.MethodGet, "/api/v1/accounts/"+account.ID, "", http.StatusOK)
	if len(accountDetail.Contacts) != 1 || len(accountDetail.Opportunities) != 1 || accountDetail.Opportunities[0].ID != opportunity.ID || len(accountDetail.Documents) != 1 || accountDetail.Documents[0].ID != document.ID {
		t.Fatalf("unexpected account detail: %#v", accountDetail)
	}

	today := time.Now().UTC().Format(time.DateOnly)
	cost := performJSON[Cost](t, mux, http.MethodPost, "/api/v1/costs", `{"vendor":"Google","description":"Workspace","amountCents":1800,"category":"Software","incurredOn":"`+today+`","recurring":true,"recurrence":"monthly","taxDeductible":true}`, http.StatusCreated)
	if cost.AmountCents != 1800 {
		t.Fatalf("cost amount = %d", cost.AmountCents)
	}

	summary := performJSON[summaryResponse](t, mux, http.MethodGet, "/api/v1/summary", "", http.StatusOK)
	if summary.Contacts != 1 || summary.OpenOpportunities != 1 || summary.PipelineAmountCents != 125000 || summary.FollowUpsDue != 1 || summary.CurrentMonthCostCents != 1800 || len(summary.RecentActivities) != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}

	updated := performJSON[Opportunity](t, mux, http.MethodPatch, "/api/v1/opportunities/"+opportunity.ID, `{"stage":"won"}`, http.StatusOK)
	if updated.Stage != "won" {
		t.Fatalf("updated stage = %q", updated.Stage)
	}
	closedSummary := performJSON[summaryResponse](t, mux, http.MethodGet, "/api/v1/summary", "", http.StatusOK)
	if closedSummary.OpenOpportunities != 0 || closedSummary.PipelineAmountCents != 0 || closedSummary.WonOpportunities != 1 || closedSummary.WonAmountCents != 125000 || closedSummary.LostOpportunities != 0 || closedSummary.LostAmountCents != 0 {
		t.Fatalf("unexpected closed summary: %#v", closedSummary)
	}
	completed := performJSON[Reminder](t, mux, http.MethodPatch, "/api/v1/reminders/"+reminder.ID, `{"completed":true}`, http.StatusOK)
	if !completed.Completed {
		t.Fatal("reminder was not completed")
	}
	performNoContent(t, mux, http.MethodDelete, "/api/v1/opportunities/"+opportunity.ID)
	performNoContent(t, mux, http.MethodDelete, "/api/v1/documents/"+document.ID)
	performNoContent(t, mux, http.MethodDelete, "/api/v1/contacts/"+contact.ID)
	performJSON[map[string]any](t, mux, http.MethodDelete, "/api/v1/opportunities/"+opportunity.ID, "", http.StatusNotFound)
	performJSON[map[string]any](t, mux, http.MethodDelete, "/api/v1/documents/"+document.ID, "", http.StatusNotFound)
	performJSON[map[string]any](t, mux, http.MethodDelete, "/api/v1/contacts/"+contact.ID, "", http.StatusNotFound)
}

func TestSummaryCountsRemindersOneWeekBeforeDue(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	if _, err := store.CreateReminder(context.Background(), "nerds-who-fish", Reminder{Title: "Soon", DueAt: now.Add(6 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateReminder(context.Background(), "nerds-who-fish", Reminder{Title: "Later", DueAt: now.Add(8 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewModule(store, func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)

	summary := performJSON[summaryResponse](t, mux, http.MethodGet, "/api/v1/summary", "", http.StatusOK)
	if summary.FollowUpsDue != 1 {
		t.Fatalf("follow-ups coming up = %d, want 1", summary.FollowUpsDue)
	}
}

func TestAccountEventsAreNewestFirstFilterableAndAttributed(t *testing.T) {
	store := NewMemoryStore()
	mux := http.NewServeMux()
	NewModule(
		store,
		func(*http.Request) (string, error) { return "nerds-who-fish", nil },
		WithActor(func(*http.Request) string { return "owner@nerdswhofish.com" }),
	).RegisterRoutes(mux)
	created := performJSON[struct {
		Account Account `json:"account"`
		Contact Contact `json:"contact"`
	}](t, mux, http.MethodPost, "/api/v1/accounts", `{"name":"River Labs","primaryContact":{"name":"Ada"}}`, http.StatusCreated)
	performJSON[Opportunity](t, mux, http.MethodPost, "/api/v1/opportunities", `{"name":"Website refresh","accountId":"`+created.Account.ID+`","stage":"qualified"}`, http.StatusCreated)
	performJSON[Contact](t, mux, http.MethodPatch, "/api/v1/contacts/"+created.Contact.ID, `{"phone":"+15551234567"}`, http.StatusOK)

	type eventResponse struct {
		Events []AccountEvent `json:"events"`
		Page   struct {
			NextCursor string `json:"nextCursor"`
		} `json:"page"`
	}
	first := performJSON[eventResponse](t, mux, http.MethodGet, "/api/v1/accounts/"+created.Account.ID+"/events?limit=2", "", http.StatusOK)
	if len(first.Events) != 2 || first.Page.NextCursor == "" || first.Events[0].Action != "contact.updated" {
		t.Fatalf("unexpected first event page: %#v", first)
	}
	if first.Events[0].Actor != "owner@nerdswhofish.com" {
		t.Fatalf("event actor = %q", first.Events[0].Actor)
	}
	contacts := performJSON[eventResponse](t, mux, http.MethodGet, "/api/v1/accounts/"+created.Account.ID+"/events?kind=contact", "", http.StatusOK)
	if len(contacts.Events) != 2 {
		t.Fatalf("contact events = %#v", contacts.Events)
	}
}

func TestContactMutationsPublishGoogleSyncEvents(t *testing.T) {
	type mutation struct {
		contact Contact
		action  string
	}
	mutations := []mutation{}
	mux := http.NewServeMux()
	NewModule(
		NewMemoryStore(),
		func(*http.Request) (string, error) { return "nerds-who-fish", nil },
		WithContactMutation(func(_ context.Context, scope string, contact Contact, action string) error {
			if scope != "nerds-who-fish" {
				t.Fatalf("scope = %q", scope)
			}
			mutations = append(mutations, mutation{contact: contact, action: action})
			return nil
		}),
	).RegisterRoutes(mux)
	created := performJSON[Contact](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"Ada","phone":"+15551234567"}`, http.StatusCreated)
	performJSON[Contact](t, mux, http.MethodPatch, "/api/v1/contacts/"+created.ID, `{"phone":"+15557654321"}`, http.StatusOK)
	performNoContent(t, mux, http.MethodDelete, "/api/v1/contacts/"+created.ID)
	if len(mutations) != 3 || mutations[0].action != "upsert" || mutations[1].action != "upsert" || mutations[2].action != "delete" {
		t.Fatalf("mutations = %#v", mutations)
	}
	if mutations[1].contact.Phone != "+15557654321" || mutations[2].contact.ID != created.ID {
		t.Fatalf("mutation contacts = %#v", mutations)
	}
}

func TestDeleteAccountCascadesLinkedWorkspaceRecords(t *testing.T) {
	type mutation struct {
		contact Contact
		action  string
	}
	mutations := make([]mutation, 0)
	mux := http.NewServeMux()
	NewModule(
		NewMemoryStore(),
		func(*http.Request) (string, error) { return "nerds-who-fish", nil },
		WithContactMutation(func(_ context.Context, _ string, contact Contact, action string) error {
			mutations = append(mutations, mutation{contact: contact, action: action})
			return nil
		}),
	).RegisterRoutes(mux)
	created := performJSON[struct {
		Account Account `json:"account"`
		Contact Contact `json:"contact"`
	}](t, mux, http.MethodPost, "/api/v1/accounts", `{"name":"River Labs","status":"prospect","primaryContact":{"name":"Ada Angler","email":"ada@example.com"}}`, http.StatusCreated)
	opportunity := performJSON[Opportunity](t, mux, http.MethodPost, "/api/v1/opportunities", `{"name":"Website refresh","accountId":"`+created.Account.ID+`","stage":"qualified"}`, http.StatusCreated)
	performJSON[Activity](t, mux, http.MethodPost, "/api/v1/activities", `{"contactId":"`+created.Contact.ID+`","opportunityId":"`+opportunity.ID+`","kind":"note","body":"Interested"}`, http.StatusCreated)
	performJSON[Reminder](t, mux, http.MethodPost, "/api/v1/reminders", `{"accountId":"`+created.Account.ID+`","contactId":"`+created.Contact.ID+`","title":"Follow up","dueAt":"`+time.Now().UTC().Add(time.Hour).Format(time.RFC3339)+`"}`, http.StatusCreated)
	document := performJSON[Document](t, mux, http.MethodPost, "/api/v1/documents", `{"title":"Account notes","body":"Details","links":[{"type":"account","id":"`+created.Account.ID+`"}]}`, http.StatusCreated)
	performJSON[Document](t, mux, http.MethodPatch, "/api/v1/documents/"+document.ID, `{"body":"More details"}`, http.StatusOK)

	performNoContent(t, mux, http.MethodDelete, "/api/v1/accounts/"+created.Account.ID)
	performJSON[map[string]any](t, mux, http.MethodGet, "/api/v1/accounts/"+created.Account.ID, "", http.StatusNotFound)
	for path, key := range map[string]string{
		"/api/v1/accounts":      "accounts",
		"/api/v1/contacts":      "contacts",
		"/api/v1/opportunities": "opportunities",
		"/api/v1/activities":    "activities",
		"/api/v1/reminders":     "reminders",
		"/api/v1/documents":     "documents",
	} {
		response := performJSON[map[string]any](t, mux, http.MethodGet, path, "", http.StatusOK)
		items, ok := response[key].([]any)
		if !ok || len(items) != 0 {
			t.Fatalf("%s after account deletion = %#v", key, response[key])
		}
	}
	if len(mutations) != 2 || mutations[1].action != "delete" || mutations[1].contact.ID != created.Contact.ID {
		t.Fatalf("unexpected contact mutations: %#v", mutations)
	}
}

func TestContactSourcesCombineDefaultsAndOrganizationChoices(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)
	created := performJSON[ContactSource](t, mux, http.MethodPost, "/api/v1/contact-sources", `{"name":"Fishing expo"}`, http.StatusCreated)
	if created.Name != "Fishing expo" {
		t.Fatalf("source = %#v", created)
	}
	response := performJSON[struct {
		Sources []ContactSource `json:"sources"`
	}](t, mux, http.MethodGet, "/api/v1/contact-sources", "", http.StatusOK)
	if len(response.Sources) < 6 || response.Sources[len(response.Sources)-1].Name != "Website" {
		t.Fatalf("sources = %#v", response.Sources)
	}
}

func TestWorkspaceSearchesRecords(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)
	performJSON[Contact](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"Grace Hopper","email":"grace@compiler.example"}`, http.StatusCreated)
	performJSON[Cost](t, mux, http.MethodPost, "/api/v1/costs", `{"vendor":"Google","description":"Workspace","amountCents":1800,"incurredOn":"2026-09-04"}`, http.StatusCreated)

	response := performJSON[struct {
		Results []searchResult `json:"results"`
	}](t, mux, http.MethodGet, "/api/v1/search?q=compiler", "", http.StatusOK)
	if len(response.Results) != 1 || response.Results[0].Title != "Grace Hopper" || response.Results[0].Href != "/contacts/"+response.Results[0].ID {
		t.Fatalf("unexpected search results: %#v", response.Results)
	}
	costResponse := performJSON[struct {
		Results []searchResult `json:"results"`
	}](t, mux, http.MethodGet, "/api/v1/search?q=workspace", "", http.StatusOK)
	if len(costResponse.Results) != 1 || costResponse.Results[0].Href != "/operations" {
		t.Fatalf("unexpected cost search results: %#v", costResponse.Results)
	}
}

func TestWorkspaceManifestKeepsCostsInsideBusinessOperations(t *testing.T) {
	manifest := (Module{}).Manifest()
	for _, navigation := range manifest.Navigation {
		if navigation.Path == "/costs" {
			t.Fatal("costs must not be a separate top-level destination")
		}
	}
	found := false
	for _, resource := range manifest.Resources {
		found = found || resource == "costs"
	}
	if !found {
		t.Fatal("costs resource must remain owned by the workspace module")
	}
}

func TestWorkspaceRequiresAuthentication(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "", errors.New("missing session") }).RegisterRoutes(mux)
	record := httptest.NewRecorder()
	mux.ServeHTTP(record, httptest.NewRequest(http.MethodGet, "/api/v1/contacts", nil))
	if record.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", record.Code, http.StatusUnauthorized)
	}
}

func TestWorkspaceEmptySummaryUsesArray(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)
	record := httptest.NewRecorder()
	mux.ServeHTTP(record, httptest.NewRequest(http.MethodGet, "/api/v1/summary", nil))

	if record.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", record.Code, http.StatusOK)
	}
	var response struct {
		RecentActivities []Activity `json:"recentActivities"`
	}
	if err := json.NewDecoder(record.Body).Decode(&response); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if response.RecentActivities == nil {
		t.Fatal("recentActivities must be an empty array, not null")
	}
}

func TestWorkspaceListsUseCursorPagination(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)
	firstCreated := performJSON[Contact](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"First"}`, http.StatusCreated)
	secondCreated := performJSON[Contact](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"Second"}`, http.StatusCreated)
	thirdCreated := performJSON[Contact](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"Third"}`, http.StatusCreated)

	type listResponse struct {
		Contacts []Contact `json:"contacts"`
		Page     struct {
			Limit      int    `json:"limit"`
			NextCursor string `json:"nextCursor"`
		} `json:"page"`
	}
	firstPage := performJSON[listResponse](t, mux, http.MethodGet, "/api/v1/contacts?limit=2", "", http.StatusOK)
	if len(firstPage.Contacts) != 2 || firstPage.Page.Limit != 2 || firstPage.Page.NextCursor == "" {
		t.Fatalf("unexpected first page: %#v", firstPage)
	}
	if firstPage.Contacts[0].ID != thirdCreated.ID || firstPage.Contacts[1].ID != secondCreated.ID {
		t.Fatalf("first page order = %q, %q", firstPage.Contacts[0].ID, firstPage.Contacts[1].ID)
	}

	secondPage := performJSON[listResponse](t, mux, http.MethodGet, "/api/v1/contacts?limit=2&cursor="+firstPage.Page.NextCursor, "", http.StatusOK)
	if len(secondPage.Contacts) != 1 || secondPage.Contacts[0].ID != firstCreated.ID || secondPage.Page.NextCursor != "" {
		t.Fatalf("unexpected second page: %#v", secondPage)
	}
}

func TestWorkspaceListsRejectMalformedPagination(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)

	response := performJSON[struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}](t, mux, http.MethodGet, "/api/v1/contacts?cursor=garbage", "", http.StatusBadRequest)
	if response.Error.Code != "invalid_pagination" || response.Error.Message != "cursor is invalid" {
		t.Fatalf("unexpected error: %#v", response.Error)
	}
}

func TestWorkspaceListHandlersUsePagedStore(t *testing.T) {
	base := NewMemoryStore()
	if _, err := base.CreateContact(context.Background(), "nerds-who-fish", Contact{Name: "Paged"}); err != nil {
		t.Fatal(err)
	}
	store := &workspacePageSpy{Store: base}
	mux := http.NewServeMux()
	NewModule(store, func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)
	performJSON[map[string]any](t, mux, http.MethodGet, "/api/v1/contacts?limit=1", "", http.StatusOK)
	if store.calls != 1 || store.collection != "contacts" || store.request.Limit != 1 {
		t.Fatalf("paged store calls = %d, collection = %q, request = %#v", store.calls, store.collection, store.request)
	}
}

type workspacePageSpy struct {
	Store
	calls      int
	collection string
	request    pagination.Request
}

func (s *workspacePageSpy) ListPage(ctx context.Context, scope, collection string, request pagination.Request, spec pagination.Spec, target any) (pagination.Metadata, error) {
	s.calls++
	s.collection = collection
	s.request = request
	return s.Store.ListPage(ctx, scope, collection, request, spec, target)
}

func TestWorkspaceRejectsInvalidRecords(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":""}`, http.StatusBadRequest)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"Ada","status":"prospect"}`, http.StatusBadRequest)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"Ada","linkedinUrl":"https://example.com/ada"}`, http.StatusBadRequest)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/accounts", `{"name":"River Labs","websites":[{"url":"not a website"}]}`, http.StatusBadRequest)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/accounts", `{"name":"River Labs","links":[{"label":"Tracker","url":"javascript:alert(1)"}]}`, http.StatusBadRequest)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/costs", `{"description":"Hosting","amountCents":-1,"incurredOn":"not-a-date"}`, http.StatusBadRequest)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/opportunities", `{"name":"Website refresh","stage":"qualified","ownerEmail":"not-an-email"}`, http.StatusBadRequest)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/opportunities", `{"name":"Website refresh","accountId":"missing","stage":"qualified"}`, http.StatusBadRequest)
}

func TestOpportunityUsesTheContactsAccount(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)
	created := performJSON[struct {
		Account Account `json:"account"`
		Contact Contact `json:"contact"`
	}](t, mux, http.MethodPost, "/api/v1/accounts", `{"name":"River Labs","primaryContact":{"name":"Ada"}}`, http.StatusCreated)
	opportunity := performJSON[Opportunity](t, mux, http.MethodPost, "/api/v1/opportunities", `{"name":"Website refresh","contactId":"`+created.Contact.ID+`","stage":"new"}`, http.StatusCreated)
	if opportunity.AccountID != created.Account.ID {
		t.Fatalf("opportunity account = %q, want %q", opportunity.AccountID, created.Account.ID)
	}
}

func TestCostCanBeReviewedAndUpdated(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)
	created := performJSON[Cost](t, mux, http.MethodPost, "/api/v1/costs", `{"description":"Domain","amountCents":2400,"incurredOn":"2026-09-04","reviewState":"review"}`, http.StatusCreated)
	updated := performJSON[Cost](t, mux, http.MethodPatch, "/api/v1/costs/"+created.ID, `{"reviewState":"complete","paymentMethod":"Business card"}`, http.StatusOK)
	if updated.ReviewState != "complete" || updated.PaymentMethod != "Business card" {
		t.Fatalf("unexpected updated cost: %#v", updated)
	}
	performJSON[map[string]any](t, mux, http.MethodPatch, "/api/v1/costs/"+created.ID, `{"recurrence":"sometimes"}`, http.StatusBadRequest)
	performNoContent(t, mux, http.MethodDelete, "/api/v1/costs/"+created.ID)
	performJSON[map[string]any](t, mux, http.MethodDelete, "/api/v1/costs/"+created.ID, "", http.StatusNotFound)
}

func TestCostDeletionRunsCleanupBeforeRemovingRecord(t *testing.T) {
	store := NewMemoryStore()
	cleanupCalled := false
	mux := http.NewServeMux()
	NewModule(
		store,
		func(*http.Request) (string, error) { return "nerds-who-fish", nil },
		WithCostDeletion(func(_ context.Context, scope, id string) error {
			cleanupCalled = scope == "nerds-who-fish" && id != ""
			return nil
		}),
	).RegisterRoutes(mux)
	created := performJSON[Cost](t, mux, http.MethodPost, "/api/v1/costs", `{"description":"Domain","amountCents":2400,"incurredOn":"2026-09-04"}`, http.StatusCreated)
	performNoContent(t, mux, http.MethodDelete, "/api/v1/costs/"+created.ID)
	if !cleanupCalled {
		t.Fatal("cost cleanup was not called")
	}
	items, err := store.ListCosts(context.Background(), "nerds-who-fish")
	if err != nil || len(items) != 0 {
		t.Fatalf("costs after delete = %#v, %v", items, err)
	}
}

func TestAccountUpdatePreservesManagedWebsiteMetadata(t *testing.T) {
	store := NewMemoryStore()
	account, _, err := store.CreateAccountWithContact(context.Background(), "workspace", Account{
		Name:   "River Labs",
		Status: "prospect",
		Websites: []Website{{
			URL: "https://river.example", Domain: "river.example", Provider: "cloudflare", ExternalID: "zone-1", RenewalDate: "2027-10-01", AutoRenew: true, Status: "active",
		}},
	}, Contact{})
	if err != nil {
		t.Fatal(err)
	}
	websites := []Website{{URL: "https://www.river.example", Domain: "river.example"}}
	updated, err := store.UpdateAccount(context.Background(), "workspace", account.ID, AccountPatch{Websites: &websites})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Websites) != 1 || updated.Websites[0].Provider != "cloudflare" || updated.Websites[0].ExternalID != "zone-1" || updated.Websites[0].RenewalDate != "2027-10-01" || !updated.Websites[0].AutoRenew {
		t.Fatalf("managed website metadata was lost: %#v", updated.Websites)
	}
}

func TestAccountLinksCanBeManaged(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)
	created := performJSON[struct {
		Account Account `json:"account"`
	}](t, mux, http.MethodPost, "/api/v1/accounts", `{"name":"River Labs"}`, http.StatusCreated)
	updated := performJSON[Account](t, mux, http.MethodPatch, "/api/v1/accounts/"+created.Account.ID, `{"links":[{"label":" Proposal ","url":"https://docs.google.com/document/d/proposal/edit"}]}`, http.StatusOK)
	if len(updated.Links) != 1 || updated.Links[0].Label != "Proposal" {
		t.Fatalf("unexpected account links: %#v", updated.Links)
	}
	updated = performJSON[Account](t, mux, http.MethodPatch, "/api/v1/accounts/"+created.Account.ID, `{"links":[]}`, http.StatusOK)
	if len(updated.Links) != 0 {
		t.Fatalf("account links were not removed: %#v", updated.Links)
	}
}

func performJSON[T any](t *testing.T, handler http.Handler, method, target, body string, wantStatus int) T {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	record := httptest.NewRecorder()
	handler.ServeHTTP(record, request)
	if record.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d: %s", method, target, record.Code, wantStatus, record.Body.String())
	}
	var response T
	if err := json.NewDecoder(record.Body).Decode(&response); err != nil {
		t.Fatalf("decode %s %s: %v", method, target, err)
	}
	return response
}

func performNoContent(t *testing.T, handler http.Handler, method, target string) {
	t.Helper()
	record := httptest.NewRecorder()
	handler.ServeHTTP(record, httptest.NewRequest(method, target, nil))
	if record.Code != http.StatusNoContent {
		t.Fatalf("%s %s status = %d, want %d: %s", method, target, record.Code, http.StatusNoContent, record.Body.String())
	}
}
