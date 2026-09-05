package operations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
	"golang.org/x/oauth2"
)

func TestRecentMailFollowsEveryPage(t *testing.T) {
	since := time.Now().Add(-time.Hour)
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gmail/v1/users/me/messages" {
			pages++
			if r.URL.Query().Get("pageToken") == "page2" {
				_ = json.NewEncoder(w).Encode(map[string]any{"messages": []map[string]string{{"id": "second"}}})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"messages": []map[string]string{{"id": "first"}}, "nextPageToken": "page2"})
			}
			return
		}
		writeGmailMetadata(t, w, r.URL.Path, since.Add(time.Minute))
	}))
	defer server.Close()
	provider := NewLiveGoogleProvider("client", "secret")
	provider.gmailEndpoint = server.URL + "/"
	items, err := provider.RecentMail(context.Background(), &oauth2.Token{AccessToken: "access", Expiry: time.Now().Add(time.Hour)}, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || pages != 2 {
		t.Fatalf("received %d messages from %d pages, expected both pages", len(items), pages)
	}
}

func TestRecentMailDoesNotReturnPartialPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gmail/v1/users/me/messages" {
			if r.URL.Query().Get("pageToken") == "page2" {
				http.Error(w, `{"error":{"code":403,"message":"permission changed"}}`, http.StatusForbidden)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"messages": []map[string]string{{"id": "first"}}, "nextPageToken": "page2"})
			return
		}
		writeGmailMetadata(t, w, "first", time.Now())
	}))
	defer server.Close()
	provider := NewLiveGoogleProvider("client", "secret")
	provider.gmailEndpoint = server.URL + "/"
	items, err := provider.RecentMail(context.Background(), &oauth2.Token{AccessToken: "access", Expiry: time.Now().Add(time.Hour)}, time.Now().Add(-time.Hour))
	if err == nil || len(items) != 0 {
		t.Fatalf("partial success would advance the checkpoint: %#v %v", items, err)
	}
}

func TestRecentMailStopsWhenRequestIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	}))
	defer server.Close()
	provider := NewLiveGoogleProvider("client", "secret")
	provider.gmailEndpoint = server.URL + "/"
	_, err := provider.RecentMail(ctx, &oauth2.Token{AccessToken: "access", Expiry: time.Now().Add(time.Hour)}, time.Now())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled sync = %v", err)
	}
}

type observedMailProvider struct {
	fakeGoogle
	beforeReturn func()
	err          error
}

func (p observedMailProvider) RecentMail(context.Context, *oauth2.Token, time.Time) ([]MailMetadata, error) {
	if p.beforeReturn != nil {
		p.beforeReturn()
	}
	return []MailMetadata{{ID: "mail-1", From: "ada@example.com", Subject: "Hello", ReceivedAt: time.Now()}}, p.err
}

func TestMailCheckpointUsesSyncStartAndRetainsFailures(t *testing.T) {
	module, _, contacts := newTestModule(t)
	ctx := context.Background()
	if _, err := contacts.CreateContact(ctx, "nerds-who-fish", workspace.Contact{Name: "Ada", Email: "ada@example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := module.SaveGoogleGrant(ctx, Identity{Email: "owner@nerdswhofish.com"}, &oauth2.Token{AccessToken: "access", RefreshToken: "refresh"}); err != nil {
		t.Fatal(err)
	}
	var returnedAt time.Time
	module.google = observedMailProvider{beforeReturn: func() { returnedAt = time.Now().UTC() }}
	id := memberID("owner@nerdswhofish.com")
	if _, err := module.syncEmailConnection(ctx, "nerds-who-fish", id, "owner", "job"); err != nil {
		t.Fatal(err)
	}
	var connection GoogleConnection
	if err := module.store.Get(ctx, "nerds-who-fish", "googleConnections", id, &connection); err != nil {
		t.Fatal(err)
	}
	if connection.LastMailSyncAt == nil || !connection.LastMailSyncAt.Before(returnedAt) {
		t.Fatalf("checkpoint = %v, provider returned at %v", connection.LastMailSyncAt, returnedAt)
	}
	previous := *connection.LastMailSyncAt
	module.google = observedMailProvider{err: errors.New("second page failed")}
	if _, err := module.syncEmailConnection(ctx, "nerds-who-fish", id, "owner", "retry"); err == nil {
		t.Fatal("provider failure was ignored")
	}
	if err := module.store.Get(ctx, "nerds-who-fish", "googleConnections", id, &connection); err != nil {
		t.Fatal(err)
	}
	if !connection.LastMailSyncAt.Equal(previous) {
		t.Fatalf("failure advanced checkpoint: %v", connection.LastMailSyncAt)
	}
}
