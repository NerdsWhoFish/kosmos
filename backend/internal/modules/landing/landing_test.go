package landing

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModuleRegistersLandingRoutes(t *testing.T) {
	mux := http.NewServeMux()
	NewModule().RegisterRoutes(mux)

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
