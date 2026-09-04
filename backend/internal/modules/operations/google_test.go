package operations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestGmailMessagePreservesOnlyAuthoredLineBreaks(t *testing.T) {
	body := "Hey Joey,\n\nThis deliberately long paragraph should remain one logical line after Gmail decodes the MIME transport even though its encoded form uses safe transport line lengths.\n\nThanks"
	raw := encodeGmailMessage("joey@nerdswhofish.com", "customer@example.com", "Renewal amount: $25", body)
	message, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if message.Header.Get("Content-Transfer-Encoding") != "base64" {
		t.Fatalf("encoding = %q", message.Header.Get("Content-Transfer-Encoding"))
	}
	decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, message.Body))
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != body {
		t.Fatalf("decoded body = %q, want exact authored body", decoded)
	}
}

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

func TestGoogleContactsCreateUpdateAndDelete(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer contacts-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/people/me/connections":
			if r.URL.Query().Get("personFields") != "externalIds" {
				t.Fatalf("personFields = %q", r.URL.Query().Get("personFields"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"connections": []any{}})
		case "POST /v1/people:createContact":
			var person map[string]any
			if err := json.NewDecoder(r.Body).Decode(&person); err != nil {
				t.Fatal(err)
			}
			assertGoogleContactPayload(t, person, "Ada Angler", "ada@example.com")
			_ = json.NewEncoder(w).Encode(map[string]string{"resourceName": "people/created", "etag": "created-etag"})
		case "GET /v1/people/created":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceName": "people/created", "etag": "current-etag", "metadata": map[string]any{"sources": []map[string]string{{"type": "CONTACT", "id": "source"}}}})
		case "GET /v1/people/missing":
			http.Error(w, `{"error":{"code":404,"message":"missing"}}`, http.StatusNotFound)
		case "PATCH /v1/people/created:updateContact":
			if got := r.URL.Query().Get("updatePersonFields"); got != "names,emailAddresses,phoneNumbers,organizations,urls,externalIds" {
				t.Fatalf("updatePersonFields = %q", got)
			}
			var person map[string]any
			if err := json.NewDecoder(r.Body).Decode(&person); err != nil {
				t.Fatal(err)
			}
			assertGoogleContactPayload(t, person, "Ada Lovelace", "")
			if person["etag"] != "current-etag" {
				t.Fatalf("etag = %#v", person["etag"])
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"resourceName": "people/created", "etag": "updated-etag"})
		case "DELETE /v1/people/created:deleteContact":
			http.Error(w, `{"error":{"code":404,"message":"missing"}}`, http.StatusNotFound)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	provider := NewLiveGoogleProvider("client", "secret")
	provider.peopleEndpoint = server.URL + "/"
	token := &oauth2.Token{AccessToken: "contacts-token", Expiry: time.Now().Add(time.Hour)}
	created, err := provider.UpsertContact(context.Background(), token, GoogleContact{
		ID: "contact-1", Name: "Ada Angler", Email: "ada@example.com", Phone: "+15551234567", Organization: "River Labs", LinkedInURL: "https://linkedin.com/in/ada",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if created.ResourceName != "people/created" || created.ETag != "created-etag" {
		t.Fatalf("created = %#v", created)
	}
	updated, err := provider.UpsertContact(context.Background(), token, GoogleContact{ID: "contact-1", Name: "Ada Lovelace"}, created.ResourceName)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ETag != "updated-etag" {
		t.Fatalf("updated = %#v", updated)
	}
	recreated, err := provider.UpsertContact(context.Background(), token, GoogleContact{ID: "contact-1", Name: "Ada Angler", Email: "ada@example.com"}, "people/missing")
	if err != nil || recreated.ResourceName != "people/created" {
		t.Fatalf("recreated = %#v, %v", recreated, err)
	}
	if err := provider.DeleteContact(context.Background(), token, "contact-1", created.ResourceName); err != nil {
		t.Fatalf("404 delete must be idempotent: %v", err)
	}
	want := []string{"GET /v1/people/me/connections", "POST /v1/people:createContact", "GET /v1/people/created", "PATCH /v1/people/created:updateContact", "GET /v1/people/missing", "POST /v1/people:createContact", "DELETE /v1/people/created:deleteContact"}
	if strings.Join(requests, "|") != strings.Join(want, "|") {
		t.Fatalf("requests = %#v", requests)
	}
}

func assertGoogleContactPayload(t *testing.T, person map[string]any, name, email string) {
	t.Helper()
	names, ok := person["names"].([]any)
	if !ok || len(names) != 1 || names[0].(map[string]any)["unstructuredName"] != name {
		t.Fatalf("names = %#v", person["names"])
	}
	externalIDs, ok := person["externalIds"].([]any)
	if !ok || len(externalIDs) != 1 || externalIDs[0].(map[string]any)["type"] != "kosmos" || externalIDs[0].(map[string]any)["value"] != "contact-1" {
		t.Fatalf("externalIds = %#v", person["externalIds"])
	}
	emails, ok := person["emailAddresses"].([]any)
	if !ok {
		t.Fatalf("emailAddresses missing: %#v", person)
	}
	if email == "" && len(emails) != 0 {
		t.Fatalf("emailAddresses = %#v", emails)
	}
	if email != "" && (len(emails) != 1 || emails[0].(map[string]any)["value"] != email) {
		t.Fatalf("emailAddresses = %#v", emails)
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
