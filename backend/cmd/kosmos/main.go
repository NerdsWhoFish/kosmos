package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/landing"
	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/operations"
	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/auth"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/modules"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/observability"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2"
)

func main() {
	fallback := slog.NewJSONHandler(os.Stdout, nil)
	logger, shutdownTelemetry, err := observability.Setup(context.Background(), fallback)
	if err != nil {
		slog.Error("telemetry setup failed", "error", err)
		os.Exit(1)
	}
	defer shutdownTelemetry(context.Background())
	slog.SetDefault(logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", health)
	googleAuth := auth.NewGoogle()

	var landingStore landing.Store = landing.NewMemoryStore()
	var workspaceStore workspace.Store = workspace.NewMemoryStore()
	var operationsStore operations.Store = operations.NewMemoryStore()
	var blobStore operations.BlobStore = operations.NewMemoryBlobStore()
	projectID := os.Getenv("KOSMOS_GCP_PROJECT")
	if projectID != "" {
		firestoreClient, err := firestore.NewClient(context.Background(), projectID)
		if err != nil {
			logger.Error("workspace store setup failed", "error", err)
			os.Exit(1)
		}
		defer firestoreClient.Close()
		landingStore = landing.NewFirestoreStore(firestoreClient)
		workspaceStore = workspace.NewFirestoreStore(firestoreClient)
		operationsStore = operations.NewFirestoreStore(firestoreClient)
		if bucket := os.Getenv("KOSMOS_ATTACHMENTS_BUCKET"); bucket != "" {
			storageClient, err := storage.NewClient(context.Background())
			if err != nil {
				logger.Error("attachment store setup failed", "error", err)
				os.Exit(1)
			}
			defer storageClient.Close()
			blobStore = operations.NewGCSBlobStore(storageClient, bucket)
		}
	}
	organizationID := os.Getenv("KOSMOS_ORGANIZATION_ID")
	if organizationID == "" {
		organizationID = "local"
	}
	role, err := processRole(os.Getenv("KOSMOS_PROCESS_ROLE"))
	if err != nil {
		logger.Error("process role setup failed", "error", err)
		os.Exit(1)
	}
	jobQueue, closeJobQueue, err := newJobQueue(context.Background(), projectID, role)
	if err != nil {
		logger.Error("background job queue setup failed", "error", err)
		os.Exit(1)
	}
	defer closeJobQueue()
	identity := func(r *http.Request) (string, operations.Identity, error) {
		user, err := googleAuth.CurrentUser(r)
		return organizationID, operations.Identity{Subject: user.Subject, Email: user.Email, Name: user.Name}, err
	}
	integrationKey := integrationSecret()
	operationsModule := operations.NewModule(operationsStore, blobStore, workspaceStore, identity, organizationID, integrationKey, operations.NewLiveGoogleProvider(os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET")), operations.WithJobQueue(jobQueue))
	if role == "web" {
		migrated, err := operationsModule.MigrateGoogleConnectionSecrets(context.Background(), []byte(os.Getenv("KOSMOS_SESSION_SECRET")))
		if err != nil {
			logger.Warn("Google connection secret migration incomplete", "error", err)
		}
		if migrated > 0 {
			logger.Info("Google connection secrets migrated", "connection.count", migrated)
		}
	}
	scope := func(r *http.Request) (string, error) {
		_, actor, err := identity(r)
		if err != nil {
			return "", err
		}
		mutation := r.Method != http.MethodGet && r.Method != http.MethodHead
		if err := operationsModule.CheckAccess(r.Context(), organizationID, actor, mutation); err != nil {
			return "", err
		}
		return organizationID, nil
	}
	if role == "jobs" {
		operationsModule.RegisterJobRoutes(mux)
	} else {
		mux.HandleFunc("GET /api/v1/config", publicConfig)
		mux.HandleFunc("GET /api/", notFound)
		mux.HandleFunc("/", spaFallback)
		googleAuth.RegisterRoutes(mux)
		googleAuth.RegisterGrant("workspace", operations.GoogleScopes, func(ctx context.Context, user auth.User, token *oauth2.Token) error {
			return operationsModule.SaveGoogleGrant(ctx, operations.Identity{Subject: user.Subject, Email: user.Email, Name: user.Name}, token)
		})
		registry := modules.NewRegistry(
			landing.NewModule(landingStore, scope),
			workspace.NewModule(workspaceStore, scope),
			operationsModule,
		)
		registry.RegisterRoutes(mux)
		mux.HandleFunc("GET /api/v1/modules", func(w http.ResponseWriter, r *http.Request) {
			if _, err := scope(r); err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"modules": registry.Manifests()})
		})
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{Addr: ":" + port, Handler: otelhttp.NewHandler(observability.RequestLogger(logger, securityHeaders(mux)), "kosmos.http")}

	logger.Info("kosmos listening", "port", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func integrationSecret() []byte {
	value := os.Getenv("KOSMOS_INTEGRATION_SECRET")
	if value == "" {
		value = os.Getenv("KOSMOS_SESSION_SECRET")
	}
	return []byte(value)
}

func processRole(value string) (string, error) {
	role := strings.ToLower(strings.TrimSpace(value))
	if role == "" {
		return "web", nil
	}
	if role != "web" && role != "jobs" {
		return "", fmt.Errorf("unsupported KOSMOS_PROCESS_ROLE %q", value)
	}
	return role, nil
}

func newJobQueue(ctx context.Context, projectID, role string) (operations.JobQueue, func() error, error) {
	config := operations.CloudTasksConfig{
		ProjectID:           normalizedEnvironment("KOSMOS_TASKS_PROJECT", projectID),
		Location:            os.Getenv("KOSMOS_TASKS_LOCATION"),
		Queue:               os.Getenv("KOSMOS_TASKS_QUEUE"),
		TargetURL:           os.Getenv("KOSMOS_JOB_TARGET_URL"),
		ServiceAccountEmail: os.Getenv("KOSMOS_JOB_INVOKER_SERVICE_ACCOUNT"),
		Audience:            os.Getenv("KOSMOS_JOB_AUDIENCE"),
	}
	configured := config.Location != "" || config.Queue != "" || config.TargetURL != "" || config.ServiceAccountEmail != ""
	if !configured {
		if os.Getenv("KOSMOS_ENV") == "production" {
			return nil, nil, errors.New("Cloud Tasks configuration is required in production")
		}
		return operations.NewMemoryJobQueue(), func() error { return nil }, nil
	}
	if role == "web" && strings.TrimSpace(config.TargetURL) == "" {
		return nil, nil, errors.New("Cloud Tasks target URL is required by the web process")
	}
	queue, err := operations.NewCloudTasksQueue(ctx, config)
	if err != nil {
		return nil, nil, err
	}
	return queue, queue.Close, nil
}

func normalizedEnvironment(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self' https:; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; font-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self' https://accounts.google.com")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if os.Getenv("KOSMOS_ENV") == "production" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func publicConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"faroURL":     os.Getenv("KOSMOS_FARO_URL"),
		"faroAppName": "kosmos",
	})
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "api route not found"})
}

func spaFallback(w http.ResponseWriter, r *http.Request) {
	webRoot := os.Getenv("KOSMOS_WEB_ROOT")
	if webRoot == "" {
		webRoot = "/web"
	}
	requested := filepath.Join(webRoot, filepath.Clean("/"+r.URL.Path))
	if info, err := os.Stat(requested); err == nil && !info.IsDir() {
		http.ServeFile(w, r, requested)
		return
	}
	index := filepath.Join(webRoot, "index.html")
	if _, err := os.Stat(index); err != nil {
		http.Error(w, "frontend bundle is not available", http.StatusNotImplemented)
		return
	}
	http.ServeFile(w, r, index)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
