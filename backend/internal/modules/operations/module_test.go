package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
	"golang.org/x/oauth2"
)

type fakeGoogle struct {
	mail []MailMetadata
	rows [][]any
	sent *int
}

func (f fakeGoogle) Send(context.Context, *oauth2.Token, string, string, string) (string, error) {
	if f.sent != nil {
		(*f.sent)++
	}
	return "sent-1", nil
}

func (f fakeGoogle) RecentMail(context.Context, *oauth2.Token, time.Time) ([]MailMetadata, error) {
	return f.mail, nil
}

func (f fakeGoogle) TillerRows(context.Context, *oauth2.Token, TillerSettings) ([][]any, error) {
	return f.rows, nil
}

func newTestModule(t *testing.T) (*Module, *http.ServeMux, *workspace.MemoryStore) {
	t.Helper()
	workspaceStore := workspace.NewMemoryStore()
	module := NewModule(NewMemoryStore(), NewMemoryBlobStore(), workspaceStore, func(*http.Request) (string, Identity, error) {
		return "nerds-who-fish", Identity{Subject: "google-1", Email: "owner@nerdswhofish.com", Name: "Owner"}, nil
	}, "nerds-who-fish", []byte("0123456789abcdef0123456789abcdef"), fakeGoogle{
		mail: []MailMetadata{{ID: "mail-1", From: "Ada <ada@example.com>", Subject: "Ready", ReceivedAt: time.Now().UTC()}},
		rows: [][]any{{"Date", "Description", "Amount", "Merchant", "Transaction ID"}, {"2026-09-03", "River Labs deposit", "250.00", "River Labs", "row-1"}},
	})
	mux := http.NewServeMux()
	module.RegisterRoutes(mux)
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
}

func TestGoogleMailAndTillerFlow(t *testing.T) {
	module, mux, workspaceStore := newTestModule(t)
	_, err := workspaceStore.CreateContact(context.Background(), "nerds-who-fish", workspace.Contact{Name: "Ada", Company: "River Labs", Email: "ada@example.com", Status: "lead"})
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
	synced := performJSON[map[string]int](t, mux, http.MethodPost, "/api/v1/email/sync", `{}`, http.StatusOK)
	if synced["newMessages"] != 1 {
		t.Fatalf("newMessages = %d", synced["newMessages"])
	}
	performJSON[GoogleConnection](t, mux, http.MethodPut, "/api/v1/integrations/tiller", `{"spreadsheetId":"sheet-1","range":"Transactions!A:Z"}`, http.StatusOK)
	result := performJSON[map[string]int](t, mux, http.MethodPost, "/api/v1/integrations/tiller/sync", `{}`, http.StatusOK)
	if result["newTransactions"] != 1 {
		t.Fatalf("newTransactions = %d", result["newTransactions"])
	}
	transactions := performJSON[struct {
		Transactions []Transaction `json:"transactions"`
	}](t, mux, http.MethodGet, "/api/v1/transactions", "", http.StatusOK)
	if len(transactions.Transactions) != 1 || transactions.Transactions[0].MatchStatus != "matched" {
		t.Fatalf("unexpected transactions: %#v", transactions.Transactions)
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
	if len(contacts) != 1 || contacts[0].Status != "lead" {
		t.Fatalf("unexpected contacts: %#v", contacts)
	}
	notifications := performJSON[struct {
		Notifications []Notification `json:"notifications"`
	}](t, mux, http.MethodGet, "/api/v1/notifications", "", http.StatusOK)
	if len(notifications.Notifications) != 1 || notifications.Notifications[0].Kind != "lead" {
		t.Fatalf("unexpected notifications: %#v", notifications.Notifications)
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
	download := httptest.NewRecorder()
	mux.ServeHTTP(download, httptest.NewRequest(http.MethodGet, attachment.DownloadURL, nil))
	if download.Code != http.StatusOK || download.Body.String() != "receipt" {
		t.Fatalf("download = %d %q", download.Code, download.Body.String())
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
