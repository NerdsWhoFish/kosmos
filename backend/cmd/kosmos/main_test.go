package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

func TestOpenAPICoversEveryRegisteredRoute(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	routePattern := regexp.MustCompile(`"(GET|POST|PATCH|PUT|DELETE) (/api/v1/[^" ]*)`)
	parameterPattern := regexp.MustCompile(`\{[^}]+\}`)
	implemented := map[string]struct{}{}
	err := filepath.WalkDir(filepath.Join(repositoryRoot, "backend"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, match := range routePattern.FindAllStringSubmatch(string(contents), -1) {
			route := strings.TrimPrefix(match[2], "/api/v1")
			implemented[match[1]+" "+parameterPattern.ReplaceAllString(route, "{id}")] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	contractBytes, err := os.ReadFile(filepath.Join(repositoryRoot, "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(contractBytes, &contract); err != nil {
		t.Fatalf("parse OpenAPI: %v", err)
	}
	documented := map[string]struct{}{}
	for path, operations := range contract.Paths {
		for method := range operations {
			upper := strings.ToUpper(method)
			if strings.Contains(" GET POST PATCH PUT DELETE ", " "+upper+" ") {
				documented[upper+" "+parameterPattern.ReplaceAllString(path, "{id}")] = struct{}{}
			}
		}
	}

	if missing := setDifference(implemented, documented); len(missing) > 0 {
		t.Fatalf("registered routes missing from OpenAPI: %s", strings.Join(missing, ", "))
	}
	if missing := setDifference(documented, implemented); len(missing) > 0 {
		t.Fatalf("OpenAPI routes missing from implementation: %s", strings.Join(missing, ", "))
	}
}

func setDifference(left, right map[string]struct{}) []string {
	difference := make([]string, 0)
	for value := range left {
		if _, exists := right[value]; !exists {
			difference = append(difference, value)
		}
	}
	sort.Strings(difference)
	return difference
}
