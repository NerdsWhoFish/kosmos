package operations

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
)

type fakeCloudflare struct {
	domains []CloudflareDomain
	err     error
}

func (f fakeCloudflare) Domains(context.Context, string, string) ([]CloudflareDomain, error) {
	return f.domains, f.err
}

func TestLiveCloudflareProviderPaginatesAndMergesDomains(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer dedicated-token-for-kosmos" {
			http.Error(w, "missing authorization", http.StatusUnauthorized)
			return
		}
		requests = append(requests, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/zones" && r.URL.Query().Get("page") == "1":
			fmt.Fprint(w, `{"success":true,"result":[{"id":"zone-1","name":"river.example"}],"result_info":{"page":1,"total_pages":2}}`)
		case r.URL.Path == "/zones" && r.URL.Query().Get("page") == "2":
			fmt.Fprint(w, `{"success":true,"result":[{"id":"zone-2","name":"external.example"}],"result_info":{"page":2,"total_pages":2}}`)
		case strings.HasSuffix(r.URL.Path, "/registrar/registrations") && r.URL.Query().Get("cursor") == "":
			fmt.Fprint(w, `{"success":true,"result":[{"domain_name":"river.example","expires_at":"2027-10-01T00:00:00Z","auto_renew":true,"status":"active"}],"result_info":{"cursor":"next"}}`)
		case strings.HasSuffix(r.URL.Path, "/registrar/registrations") && r.URL.Query().Get("cursor") == "next":
			fmt.Fprint(w, `{"success":true,"result":[{"domain_name":"registrar-only.example","expires_at":"2027-11-01T00:00:00Z","auto_renew":false,"status":"active"}],"result_info":{"cursor":""}}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := NewLiveCloudflareProvider(server.Client())
	provider.baseURL = server.URL
	domains, err := provider.Domains(context.Background(), "0123456789abcdef0123456789abcdef", "dedicated-token-for-kosmos")
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 4 || len(domains) != 3 {
		t.Fatalf("requests = %d, domains = %#v", len(requests), domains)
	}
	if domains[0].DomainName != "external.example" || domains[0].Registered {
		t.Fatalf("external domain = %#v", domains[0])
	}
	if domains[1].DomainName != "registrar-only.example" || !domains[1].Registered {
		t.Fatalf("registrar-only domain = %#v", domains[1])
	}
	if domains[2].DomainName != "river.example" || domains[2].ZoneID != "zone-1" || domains[2].RenewalDate != "2027-10-01" || !domains[2].AutoRenew {
		t.Fatalf("merged domain = %#v", domains[2])
	}
}

func TestCloudflareConnectionAndRenewalReminderFlow(t *testing.T) {
	module, mux, workspaceStore := newTestModule(t)
	module.cloudflare = fakeCloudflare{domains: []CloudflareDomain{{DomainName: "river.example", ZoneID: "zone-1", Registered: true, RenewalDate: "2027-10-01", AutoRenew: true, Status: "active"}}}
	account, _, err := workspaceStore.CreateAccountWithContact(context.Background(), "nerds-who-fish", workspace.Account{Name: "River Labs", Status: "prospect"}, workspace.Contact{})
	if err != nil {
		t.Fatal(err)
	}

	const accountID = "0123456789abcdef0123456789abcdef"
	const token = "dedicated-token-for-kosmos"
	connected := performJSON[map[string]any](t, mux, http.MethodPut, "/api/v1/integrations/cloudflare", `{"accountId":"`+accountID+`","apiToken":"`+token+`"}`, http.StatusOK)
	if connected["connected"] != true || connected["accountId"] != accountID || strings.Contains(fmt.Sprint(connected), token) {
		t.Fatalf("unsafe connection response: %#v", connected)
	}

	domains := performJSON[struct {
		Domains []CloudflareDomain `json:"domains"`
	}](t, mux, http.MethodGet, "/api/v1/integrations/cloudflare/domains", "", http.StatusOK)
	if len(domains.Domains) != 1 || domains.Domains[0].RenewalDate != "2027-10-01" {
		t.Fatalf("unexpected domains: %#v", domains.Domains)
	}

	target := "/api/v1/integrations/cloudflare/link"
	body := `{"accountId":"` + account.ID + `","domainName":"river.example"}`
	linked := performJSON[struct {
		Account   workspace.Account    `json:"account"`
		Reminders []workspace.Reminder `json:"reminders"`
	}](t, mux, http.MethodPost, target, body, http.StatusOK)
	if len(linked.Account.Websites) != 1 || linked.Account.Websites[0].Provider != "cloudflare" || len(linked.Reminders) != 3 {
		t.Fatalf("unexpected link response: %#v", linked)
	}
	performJSON[map[string]any](t, mux, http.MethodPost, target, body, http.StatusOK)
	reminders, err := workspaceStore.ListReminders(context.Background(), "nerds-who-fish")
	if err != nil || len(reminders) != 3 {
		t.Fatalf("idempotent reminders = %#v, %v", reminders, err)
	}
	for _, reminder := range reminders {
		if reminder.AccountID != account.ID || !strings.HasPrefix(reminder.SourceKey, "cloudflare:river.example:") {
			t.Fatalf("unexpected reminder: %#v", reminder)
		}
	}
	module.cloudflare = fakeCloudflare{domains: []CloudflareDomain{{DomainName: "river.example", ZoneID: "zone-1", Registered: true, RenewalDate: "2028-10-01", AutoRenew: true, Status: "active"}}}
	performJSON[map[string]any](t, mux, http.MethodPost, target, body, http.StatusOK)
	reminders, err = workspaceStore.ListReminders(context.Background(), "nerds-who-fish")
	if err != nil || len(reminders) != 3 {
		t.Fatalf("refreshed reminders = %#v, %v", reminders, err)
	}
	for _, reminder := range reminders {
		if !strings.Contains(reminder.SourceKey, ":2028-10-01:") {
			t.Fatalf("stale reminder survived renewal change: %#v", reminder)
		}
	}

	record := httptest.NewRecorder()
	mux.ServeHTTP(record, httptest.NewRequest(http.MethodDelete, "/api/v1/integrations/cloudflare", nil))
	if record.Code != http.StatusNoContent {
		t.Fatalf("disconnect status = %d, want %d: %s", record.Code, http.StatusNoContent, record.Body.String())
	}
	status := performJSON[map[string]any](t, mux, http.MethodGet, "/api/v1/integrations/cloudflare", "", http.StatusOK)
	if status["connected"] != false {
		t.Fatalf("status after disconnect: %#v", status)
	}
}

func TestCloudflareExternalRegistrarRequiresRenewalDate(t *testing.T) {
	module, mux, workspaceStore := newTestModule(t)
	module.cloudflare = fakeCloudflare{domains: []CloudflareDomain{{DomainName: "external.example", ZoneID: "zone-2"}}}
	account, _, err := workspaceStore.CreateAccountWithContact(context.Background(), "nerds-who-fish", workspace.Account{Name: "External Labs", Status: "prospect"}, workspace.Contact{})
	if err != nil {
		t.Fatal(err)
	}
	performJSON[map[string]any](t, mux, http.MethodPut, "/api/v1/integrations/cloudflare", `{"accountId":"0123456789abcdef0123456789abcdef","apiToken":"dedicated-token-for-kosmos"}`, http.StatusOK)
	performJSON[map[string]any](t, mux, http.MethodPost, "/api/v1/integrations/cloudflare/link", `{"accountId":"`+account.ID+`","domainName":"external.example"}`, http.StatusBadRequest)
	linked := performJSON[struct {
		Reminders []workspace.Reminder `json:"reminders"`
	}](t, mux, http.MethodPost, "/api/v1/integrations/cloudflare/link", `{"accountId":"`+account.ID+`","domainName":"external.example","renewalDate":"2027-12-01"}`, http.StatusOK)
	if len(linked.Reminders) != 3 {
		t.Fatalf("manual renewal reminders = %#v", linked.Reminders)
	}
}
