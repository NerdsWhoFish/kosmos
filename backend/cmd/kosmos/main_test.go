package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicConfigReturnsRuntimeFaroSettings(t *testing.T) {
	t.Setenv("KOSMOS_FARO_URL", "https://faro.example.com/collect/test")
	record := httptest.NewRecorder()

	publicConfig(record, httptest.NewRequest(http.MethodGet, "/api/v1/config", nil))

	if record.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", record.Code, http.StatusOK)
	}
	var config map[string]string
	if err := json.NewDecoder(record.Body).Decode(&config); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if config["faroURL"] != "https://faro.example.com/collect/test" || config["faroAppName"] != "kosmos" {
		t.Fatalf("unexpected config: %#v", config)
	}
}
