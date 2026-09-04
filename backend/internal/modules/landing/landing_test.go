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
