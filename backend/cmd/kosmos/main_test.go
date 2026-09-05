package main

import (
	"context"
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

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/operations"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/auth"
	"gopkg.in/yaml.v3"
)

func TestRequestIdentityPrefersBearerCredentialsWithoutBrowserCSRF(t *testing.T) {
	currentUserCalls := 0
	authenticateCalls := 0
	identity := requestIdentity("nerds-who-fish", func(*http.Request) (auth.User, error) {
		currentUserCalls++
		return auth.User{Email: "owner@nerdswhofish.com"}, nil
	}, func(_ context.Context, token string) (operations.Identity, error) {
		authenticateCalls++
		if token != "workflow-token" {
			t.Fatalf("token = %q", token)
		}
		return operations.Identity{Kind: "api", Access: "write"}, nil
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/documents", nil)
	request.Header.Set("Authorization", "Bearer workflow-token")
	_, actor, err := identity(request)
	if err != nil || actor.Kind != "api" || authenticateCalls != 1 || currentUserCalls != 0 {
		t.Fatalf("identity = %#v, %v, API calls %d, Google calls %d", actor, err, authenticateCalls, currentUserCalls)
	}
}

func TestRequestIdentityDoesNotFallBackFromMalformedAuthorization(t *testing.T) {
	currentUserCalls := 0
	identity := requestIdentity("nerds-who-fish", func(*http.Request) (auth.User, error) {
		currentUserCalls++
		return auth.User{Email: "owner@nerdswhofish.com"}, nil
	}, func(context.Context, string) (operations.Identity, error) {
		t.Fatal("malformed authorization reached API authenticator")
		return operations.Identity{}, nil
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/documents", nil)
	request.Header.Set("Authorization", "Basic nope")
	if _, _, err := identity(request); err == nil || currentUserCalls != 0 {
		t.Fatalf("malformed authorization fell back to Google, calls = %d", currentUserCalls)
	}
}

type openAPIContract struct {
	Paths      map[string]openAPIPathItem `yaml:"paths"`
	Components struct {
		Responses       map[string]openAPIResponse `yaml:"responses"`
		Schemas         map[string]openAPISchema   `yaml:"schemas"`
		SecuritySchemes map[string]struct {
			Type string `yaml:"type"`
			In   string `yaml:"in"`
			Name string `yaml:"name"`
		} `yaml:"securitySchemes"`
	} `yaml:"components"`
}

type openAPIPathItem struct {
	Get    openAPIOperation `yaml:"get"`
	Post   openAPIOperation `yaml:"post"`
	Delete openAPIOperation `yaml:"delete"`
}

type openAPIOperation struct {
	Parameters []openAPIReference         `yaml:"parameters"`
	Responses  map[string]openAPIResponse `yaml:"responses"`
	Security   []map[string][]string      `yaml:"security"`
}

type openAPIReference struct {
	Ref string `yaml:"$ref"`
}

type openAPIResponse struct {
	Ref     string                      `yaml:"$ref"`
	Content map[string]openAPIMediaType `yaml:"content"`
}

type openAPIMediaType struct {
	Schema openAPISchema `yaml:"schema"`
}

type openAPISchema struct {
	Ref                  string                   `yaml:"$ref"`
	Type                 string                   `yaml:"type"`
	Const                any                      `yaml:"const"`
	AdditionalProperties any                      `yaml:"additionalProperties"`
	Required             []string                 `yaml:"required"`
	Properties           map[string]openAPISchema `yaml:"properties"`
	Items                *openAPISchema           `yaml:"items"`
}

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

func TestProcessRole(t *testing.T) {
	for input, want := range map[string]string{"": "web", "WEB": "web", " jobs ": "jobs"} {
		got, err := processRole(input)
		if err != nil || got != want {
			t.Fatalf("processRole(%q) = %q, %v, want %q", input, got, err, want)
		}
	}
	if _, err := processRole("scheduler"); err == nil {
		t.Fatal("unsupported role should fail")
	}
}

func TestReleaseBuildsArtifactsBeforeTheContainer(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	files := map[string]string{}
	for _, path := range []string{".github/workflows/release.yml", ".gitignore", ".goreleaser.yaml", "Dockerfile"} {
		contents, err := os.ReadFile(filepath.Join(repositoryRoot, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		files[path] = string(contents)
	}

	if !strings.Contains(files[".github/workflows/release.yml"], "publish: goreleaser,docker") {
		t.Fatal("release must run GoReleaser before Docker")
	}
	if !strings.Contains(files[".goreleaser.yaml"], "main: ./backend/cmd/kosmos") {
		t.Fatal("GoReleaser must build the Kosmos server")
	}
	if !strings.Contains(files[".gitignore"], "gha-creds-*.json") {
		t.Fatal("Google authentication credentials must not dirty the GoReleaser checkout")
	}
	dockerfile := files["Dockerfile"]
	for _, forbidden := range []string{"FROM golang:", "FROM node:", "go build", "npm run build"} {
		if strings.Contains(dockerfile, forbidden) {
			t.Fatalf("Dockerfile recompiles application artifacts with %q", forbidden)
		}
	}
	for _, required := range []string{"COPY dist/kosmos_linux_${TARGETARCH}*/kosmos /kosmos", "COPY frontend/dist /web"} {
		if !strings.Contains(dockerfile, required) {
			t.Fatalf("Dockerfile missing artifact copy %q", required)
		}
	}
}

func TestIntegrationSecretIsSeparateFromSessionSigning(t *testing.T) {
	t.Setenv("KOSMOS_SESSION_SECRET", "session-only")
	t.Setenv("KOSMOS_INTEGRATION_SECRET", "integration-only")
	if got := string(integrationSecret()); got != "integration-only" {
		t.Fatalf("integration secret = %q", got)
	}
	t.Setenv("KOSMOS_INTEGRATION_SECRET", "")
	if got := string(integrationSecret()); got != "session-only" {
		t.Fatalf("local fallback = %q", got)
	}
}

func TestUnconfiguredJobQueueUsesMemory(t *testing.T) {
	for _, name := range []string{"KOSMOS_TASKS_PROJECT", "KOSMOS_TASKS_LOCATION", "KOSMOS_TASKS_QUEUE", "KOSMOS_JOB_TARGET_URL", "KOSMOS_JOB_INVOKER_SERVICE_ACCOUNT", "KOSMOS_JOB_AUDIENCE"} {
		t.Setenv(name, "")
	}
	queue, closeQueue, err := newJobQueue(context.Background(), "", "web")
	if err != nil {
		t.Fatal(err)
	}
	defer closeQueue()
	if _, ok := queue.(*operations.MemoryJobQueue); !ok {
		t.Fatalf("queue type = %T", queue)
	}
	t.Setenv("KOSMOS_ENV", "production")
	if _, _, err := newJobQueue(context.Background(), "project-1", "web"); err == nil {
		t.Fatal("production should require Cloud Tasks configuration")
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

func TestOpenAPIPaginatedListResponseSchemas(t *testing.T) {
	contract := loadOpenAPIContract(t)
	expected := map[string]struct {
		collection string
		itemSchema string
	}{
		"/accounts":                 {"accounts", "Account"},
		"/api-credentials":          {"credentials", "APICredential"},
		"/accounts/{id}/events":     {"events", "AccountEvent"},
		"/activities":               {"activities", "Activity"},
		"/attachments":              {"attachments", "Attachment"},
		"/audit":                    {"entries", "AuditEntry"},
		"/contacts":                 {"contacts", "Contact"},
		"/costs":                    {"costs", "Cost"},
		"/documents":                {"documents", "Document"},
		"/documents/{id}/revisions": {"revisions", "DocumentRevision"},
		"/email/messages":           {"messages", "MailMetadata"},
		"/email/templates":          {"templates", "EmailTemplate"},
		"/leads":                    {"leads", "Contact"},
		"/members":                  {"members", "Member"},
		"/notifications":            {"notifications", "Notification"},
		"/opportunities":            {"opportunities", "Opportunity"},
		"/pipeline-stages":          {"stages", "PipelineStage"},
		"/reminders":                {"reminders", "Reminder"},
		"/signing-requests":         {"requests", "SigningRequest"},
		"/transactions":             {"transactions", "Transaction"},
	}

	found := map[string]struct{}{}
	for path, pathItem := range contract.Paths {
		operation := pathItem.Get
		if !hasParameter(operation.Parameters, "#/components/parameters/Limit") || !hasParameter(operation.Parameters, "#/components/parameters/Cursor") {
			continue
		}
		found[path] = struct{}{}
		want, exists := expected[path]
		if !exists {
			t.Errorf("paginated GET %s is not covered by the response contract test", path)
			continue
		}

		response, exists := operation.Responses["200"]
		if !exists {
			t.Errorf("paginated GET %s has no 200 response", path)
			continue
		}
		response = resolveResponse(t, contract, response)
		mediaType, exists := response.Content["application/json"]
		if !exists {
			t.Errorf("paginated GET %s has no application/json response", path)
			continue
		}
		schema := resolveSchema(t, contract, mediaType.Schema)
		if schema.Type != "object" || !sameStrings(schema.Required, []string{want.collection, "page"}) {
			t.Errorf("paginated GET %s schema type/required = %q/%v", path, schema.Type, schema.Required)
		}
		collection, exists := schema.Properties[want.collection]
		if !exists || collection.Type != "array" || collection.Items == nil || collection.Items.Ref != "#/components/schemas/"+want.itemSchema {
			t.Errorf("paginated GET %s collection %q does not contain %s items", path, want.collection, want.itemSchema)
		} else {
			resolveSchema(t, contract, *collection.Items)
		}
		page, exists := schema.Properties["page"]
		if !exists || page.Ref != "#/components/schemas/Page" {
			t.Errorf("paginated GET %s does not use Page metadata", path)
		}
	}

	if missing := setDifference(stringSetKeys(expected), found); len(missing) > 0 {
		t.Fatalf("expected paginated GETs missing from OpenAPI: %s", strings.Join(missing, ", "))
	}

	page := contract.Components.Schemas["Page"]
	if page.Type != "object" || !sameStrings(page.Required, []string{"limit", "nextCursor"}) {
		t.Fatalf("Page schema type/required = %q/%v", page.Type, page.Required)
	}
	if page.Properties["limit"].Type != "integer" || page.Properties["nextCursor"].Type != "string" {
		t.Fatalf("Page properties = %#v", page.Properties)
	}
}

func TestOpenAPIAsyncSyncResponses(t *testing.T) {
	contract := loadOpenAPIContract(t)
	for _, path := range []string{"/email/sync", "/integrations/tiller/sync"} {
		operation := contract.Paths[path].Post
		response, exists := operation.Responses["202"]
		if !exists {
			t.Errorf("POST %s has no 202 response", path)
			continue
		}
		if _, exists := operation.Responses["200"]; exists {
			t.Errorf("POST %s incorrectly documents a synchronous 200 response", path)
		}
		response = resolveResponse(t, contract, response)
		mediaType, exists := response.Content["application/json"]
		if !exists {
			t.Errorf("POST %s has no application/json response", path)
			continue
		}
		schema := resolveSchema(t, contract, mediaType.Schema)
		if schema.Type != "object" || !sameStrings(schema.Required, []string{"id", "status"}) {
			t.Errorf("POST %s schema type/required = %q/%v", path, schema.Type, schema.Required)
		}
		if schema.AdditionalProperties != false {
			t.Errorf("POST %s permits fields other than id and status", path)
		}
		if schema.Properties["id"].Type != "string" {
			t.Errorf("POST %s id is not a string", path)
		}
		status := schema.Properties["status"]
		if status.Type != "string" || status.Const != "accepted" {
			t.Errorf("POST %s status = type %q const %#v", path, status.Type, status.Const)
		}
	}
}

func TestOpenAPIPublicSigningSecurityAndPDFResponses(t *testing.T) {
	contract := loadOpenAPIContract(t)
	scheme := contract.Components.SecuritySchemes["signingToken"]
	if scheme.Type != "apiKey" || scheme.In != "header" || scheme.Name != "X-Kosmos-Signing-Token" {
		t.Fatalf("public signing security scheme = %#v", scheme)
	}
	for _, path := range []string{"/signing/{id}", "/signing/{id}/pdf", "/signing/{id}/complete"} {
		operation := contract.Paths[path].Get
		if strings.HasSuffix(path, "/complete") {
			operation = contract.Paths[path].Post
			if !hasParameter(operation.Parameters, "#/components/parameters/SigningCSRF") {
				t.Errorf("public signing completion omits its required CSRF header")
			}
		}
		if len(operation.Security) != 1 || len(operation.Security[0]) != 1 {
			t.Errorf("%s must require only signingToken, got %#v", path, operation.Security)
			continue
		}
		if _, exists := operation.Security[0]["signingToken"]; !exists {
			t.Errorf("%s does not require signingToken", path)
		}
		for _, status := range []string{"404", "410", "429"} {
			if _, exists := operation.Responses[status]; !exists {
				t.Errorf("%s omits public signing response %s", path, status)
			}
		}
	}
	for _, path := range []string{"/signing-requests/{id}/pdf", "/signing/{id}/pdf"} {
		operation := contract.Paths[path].Get
		response := resolveResponse(t, contract, operation.Responses["200"])
		if _, exists := response.Content["application/pdf"]; !exists {
			t.Errorf("%s does not return application/pdf", path)
		}
		if !hasParameter(operation.Parameters, "#/components/parameters/CompletedPDF") {
			t.Errorf("%s does not expose the completed artifact parameter", path)
		}
	}
}

func loadOpenAPIContract(t *testing.T) openAPIContract {
	t.Helper()
	contractBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var contract openAPIContract
	if err := yaml.Unmarshal(contractBytes, &contract); err != nil {
		t.Fatalf("parse OpenAPI: %v", err)
	}
	return contract
}

func resolveResponse(t *testing.T, contract openAPIContract, response openAPIResponse) openAPIResponse {
	t.Helper()
	if response.Ref == "" {
		return response
	}
	name := strings.TrimPrefix(response.Ref, "#/components/responses/")
	resolved, exists := contract.Components.Responses[name]
	if name == response.Ref || !exists {
		t.Fatalf("unresolved response reference %q", response.Ref)
	}
	return resolved
}

func resolveSchema(t *testing.T, contract openAPIContract, schema openAPISchema) openAPISchema {
	t.Helper()
	if schema.Ref == "" {
		return schema
	}
	name := strings.TrimPrefix(schema.Ref, "#/components/schemas/")
	resolved, exists := contract.Components.Schemas[name]
	if name == schema.Ref || !exists {
		t.Fatalf("unresolved schema reference %q", schema.Ref)
	}
	return resolved
}

func hasParameter(parameters []openAPIReference, reference string) bool {
	for _, parameter := range parameters {
		if parameter.Ref == reference {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	return strings.Join(left, "\x00") == strings.Join(right, "\x00")
}

func stringSetKeys[V any](values map[string]V) map[string]struct{} {
	keys := make(map[string]struct{}, len(values))
	for key := range values {
		keys[key] = struct{}{}
	}
	return keys
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
