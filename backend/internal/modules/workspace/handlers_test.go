package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
)

func TestWorkspaceCoreFlow(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)

	account := performJSON[Account](t, mux, http.MethodPost, "/api/v1/accounts", `{"name":"River Labs","status":"prospect"}`, http.StatusCreated)
	contact := performJSON[Contact](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"Ada Angler","accountId":"`+account.ID+`","company":"River Labs","email":"ada@example.com","status":"lead","source":"referral"}`, http.StatusCreated)
	if contact.ID == "" || contact.Name != "Ada Angler" {
		t.Fatalf("unexpected contact: %#v", contact)
	}

	opportunity := performJSON[Opportunity](t, mux, http.MethodPost, "/api/v1/opportunities", `{"name":"Website refresh","contactId":"`+contact.ID+`","amountCents":125000,"stage":"qualified","nextStep":"Send proposal"}`, http.StatusCreated)
	if opportunity.Stage != "qualified" {
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
	if len(leads.Leads) != 1 || leads.Leads[0].Source != "referral" {
		t.Fatalf("unexpected leads: %#v", leads.Leads)
	}
	accountDetail := performJSON[struct {
		Contacts []Contact `json:"contacts"`
	}](t, mux, http.MethodGet, "/api/v1/accounts/"+account.ID, "", http.StatusOK)
	if len(accountDetail.Contacts) != 1 {
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
	completed := performJSON[Reminder](t, mux, http.MethodPatch, "/api/v1/reminders/"+reminder.ID, `{"completed":true}`, http.StatusOK)
	if !completed.Completed {
		t.Fatal("reminder was not completed")
	}
}

func TestWorkspaceSearchesRecords(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "nerds-who-fish", nil }).RegisterRoutes(mux)
	performJSON[Contact](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"Grace Hopper","company":"Compiler Co","status":"customer"}`, http.StatusCreated)
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
	firstCreated := performJSON[Contact](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"First","status":"lead"}`, http.StatusCreated)
	secondCreated := performJSON[Contact](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"Second","status":"lead"}`, http.StatusCreated)
	thirdCreated := performJSON[Contact](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"Third","status":"lead"}`, http.StatusCreated)

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
	if _, err := base.CreateContact(context.Background(), "nerds-who-fish", Contact{Name: "Paged", Status: "lead"}); err != nil {
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
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/contacts", `{"name":"","status":"stranger"}`, http.StatusBadRequest)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/costs", `{"description":"Hosting","amountCents":-1,"incurredOn":"not-a-date"}`, http.StatusBadRequest)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/opportunities", `{"name":"Website refresh","stage":"qualified","ownerEmail":"not-an-email"}`, http.StatusBadRequest)
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
