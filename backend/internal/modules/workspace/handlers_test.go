package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

	response := performJSON[struct {
		Results []searchResult `json:"results"`
	}](t, mux, http.MethodGet, "/api/v1/search?q=compiler", "", http.StatusOK)
	if len(response.Results) != 1 || response.Results[0].Title != "Grace Hopper" || response.Results[0].Href != "/contacts/"+response.Results[0].ID {
		t.Fatalf("unexpected search results: %#v", response.Results)
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
