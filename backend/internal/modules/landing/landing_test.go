package landing

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModuleRegistersLandingRoutes(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "user-1", nil }).RegisterRoutes(mux)

	record := httptest.NewRecorder()
	mux.ServeHTTP(record, httptest.NewRequest(http.MethodGet, "/api/v1/landing", nil))

	if record.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", record.Code, http.StatusOK)
	}
	var response landingResponse
	if err := json.NewDecoder(record.Body).Decode(&response); err != nil {
		t.Fatalf("decode landing response: %v", err)
	}
	if len(response.Buttons) != 3 {
		t.Fatalf("buttons = %d, want 3", len(response.Buttons))
	}
	if response.Notifications == nil {
		t.Fatal("notifications must be an array")
	}
}

func TestModuleCreatesPersistentShortcut(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "user-1", nil }).RegisterRoutes(mux)

	body := bytes.NewBufferString(`{"label":"Fishing reports","description":"Open the latest reports.","href":"https://example.com/reports"}`)
	create := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/landing/buttons", body)
	request.Header.Set("X-Kosmos-CSRF", "1")
	mux.ServeHTTP(create, request)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", create.Code, http.StatusCreated, create.Body.String())
	}

	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/landing", nil))
	var response landingResponse
	if err := json.NewDecoder(list.Body).Decode(&response); err != nil {
		t.Fatalf("decode landing response: %v", err)
	}
	if len(response.Buttons) != 4 {
		t.Fatalf("buttons = %d, want 4", len(response.Buttons))
	}
	if response.Buttons[3].Label != "Fishing reports" {
		t.Fatalf("created label = %q", response.Buttons[3].Label)
	}
}

func TestOrganizationSharesShortcutsAndRestrictsManagement(t *testing.T) {
	store := NewMemoryStore()
	manager := func(request *http.Request) error {
		if request.Header.Get("X-Test-Role") != "admin" {
			return errors.New("not an administrator")
		}
		return nil
	}
	mux := http.NewServeMux()
	NewModule(store, func(*http.Request) (string, error) { return "organization-1", nil }, manager).RegisterRoutes(mux)

	denied := httptest.NewRecorder()
	mux.ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/api/v1/landing/buttons", bytes.NewBufferString(`{"label":"Denied","href":"/documents"}`)))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("member create status = %d, want %d", denied.Code, http.StatusForbidden)
	}

	created := httptest.NewRequest(http.MethodPost, "/api/v1/landing/buttons", bytes.NewBufferString(`{"label":"Shared docs","href":"/documents"}`))
	created.Header.Set("X-Test-Role", "admin")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, created)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("admin create status = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}

	listing := httptest.NewRecorder()
	mux.ServeHTTP(listing, httptest.NewRequest(http.MethodGet, "/api/v1/landing", nil))
	var response landingResponse
	if err := json.NewDecoder(listing.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Buttons[len(response.Buttons)-1].Label != "Shared docs" {
		t.Fatalf("organization shortcut missing: %#v", response.Buttons)
	}

	buttonID := response.Buttons[len(response.Buttons)-1].ID
	deniedEdit := httptest.NewRecorder()
	mux.ServeHTTP(deniedEdit, httptest.NewRequest(http.MethodPatch, "/api/v1/landing/buttons/"+buttonID, bytes.NewBufferString(`{"label":"Nope","href":"/"}`)))
	if deniedEdit.Code != http.StatusForbidden {
		t.Fatalf("member edit status = %d, want %d", deniedEdit.Code, http.StatusForbidden)
	}

	edit := httptest.NewRequest(http.MethodPatch, "/api/v1/landing/buttons/"+buttonID, bytes.NewBufferString(`{"label":"Team docs","description":"Shared knowledge","href":"/documents"}`))
	edit.Header.Set("X-Test-Role", "admin")
	edited := httptest.NewRecorder()
	mux.ServeHTTP(edited, edit)
	if edited.Code != http.StatusOK {
		t.Fatalf("admin edit status = %d, want %d: %s", edited.Code, http.StatusOK, edited.Body.String())
	}
	var updated Button
	if err := json.NewDecoder(edited.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Label != "Team docs" || updated.Href != "/documents" {
		t.Fatalf("updated shortcut = %#v", updated)
	}

	remove := httptest.NewRequest(http.MethodDelete, "/api/v1/landing/buttons/"+buttonID, nil)
	remove.Header.Set("X-Test-Role", "admin")
	deleted := httptest.NewRecorder()
	mux.ServeHTTP(deleted, remove)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("admin delete status = %d, want %d: %s", deleted.Code, http.StatusNoContent, deleted.Body.String())
	}

	after := httptest.NewRecorder()
	mux.ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/api/v1/landing", nil))
	if err := json.NewDecoder(after.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Buttons) != 3 {
		t.Fatalf("buttons after delete = %d, want 3", len(response.Buttons))
	}
}

func TestModuleRequiresAuthentication(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "", errors.New("missing session") }).RegisterRoutes(mux)

	record := httptest.NewRecorder()
	mux.ServeHTTP(record, httptest.NewRequest(http.MethodGet, "/api/v1/landing", nil))
	if record.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", record.Code, http.StatusUnauthorized)
	}
}

func TestModuleRejectsUnsafeShortcut(t *testing.T) {
	mux := http.NewServeMux()
	NewModule(NewMemoryStore(), func(*http.Request) (string, error) { return "user-1", nil }).RegisterRoutes(mux)

	body := bytes.NewBufferString(`{"label":"Unsafe","href":"javascript:alert(1)"}`)
	record := httptest.NewRecorder()
	mux.ServeHTTP(record, httptest.NewRequest(http.MethodPost, "/api/v1/landing/buttons", body))
	if record.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", record.Code, http.StatusBadRequest)
	}
}
