package operations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestRecentMailUsesMetadataCompatibleFilters(t *testing.T) {
	since := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages":
			assertMetadataListQuery(t, r.URL.Query())
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]string{{"id": "new"}, {"id": "old"}},
			})
		case "/gmail/v1/users/me/messages/new":
			writeGmailMetadata(t, w, "new", since.Add(time.Minute))
		case "/gmail/v1/users/me/messages/old":
			writeGmailMetadata(t, w, "old", since)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	provider := NewLiveGoogleProvider("client", "secret")
	provider.gmailEndpoint = server.URL + "/"
	messages, err := provider.RecentMail(context.Background(), &oauth2.Token{
		AccessToken: "access-token",
		Expiry:      time.Now().Add(time.Hour),
	}, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].ID != "new" {
		t.Fatalf("messages = %#v", messages)
	}
}

func assertMetadataListQuery(t *testing.T, query url.Values) {
	t.Helper()
	if query.Get("q") != "" {
		t.Fatalf("metadata scope does not support q, got %q", query.Get("q"))
	}
	if got := query["labelIds"]; len(got) != 1 || got[0] != "INBOX" {
		t.Fatalf("labelIds = %#v", got)
	}
	if query.Get("maxResults") != "50" {
		t.Fatalf("maxResults = %q", query.Get("maxResults"))
	}
}

func writeGmailMetadata(t *testing.T, w http.ResponseWriter, id string, receivedAt time.Time) {
	t.Helper()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":           id,
		"threadId":     "thread-" + id,
		"snippet":      "Preview",
		"internalDate": strconv.FormatInt(receivedAt.UnixMilli(), 10),
		"payload": map[string]any{
			"headers": []map[string]string{
				{"name": "From", "value": "Ada <ada@example.com>"},
				{"name": "Subject", "value": strings.ToUpper(id)},
			},
		},
	})
}
