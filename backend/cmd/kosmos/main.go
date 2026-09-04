package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"cloud.google.com/go/firestore"
	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/landing"
	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/auth"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/modules"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/observability"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
	mux.HandleFunc("GET /api/v1/config", publicConfig)
	mux.HandleFunc("GET /api/", notFound)
	mux.HandleFunc("/", spaFallback)
	googleAuth := auth.NewGoogle()
	googleAuth.RegisterRoutes(mux)

	var landingStore landing.Store = landing.NewMemoryStore()
	var workspaceStore workspace.Store = workspace.NewMemoryStore()
	if projectID := os.Getenv("KOSMOS_GCP_PROJECT"); projectID != "" {
		firestoreClient, err := firestore.NewClient(context.Background(), projectID)
		if err != nil {
			logger.Error("workspace store setup failed", "error", err)
			os.Exit(1)
		}
		defer firestoreClient.Close()
		landingStore = landing.NewFirestoreStore(firestoreClient)
		workspaceStore = workspace.NewFirestoreStore(firestoreClient)
	}
	organizationID := os.Getenv("KOSMOS_ORGANIZATION_ID")
	if organizationID == "" {
		organizationID = "local"
	}
	scope := func(r *http.Request) (string, error) {
		_, err := googleAuth.CurrentUser(r)
		if err != nil {
			return "", err
		}
		return organizationID, nil
	}
	modules.NewRegistry(
		landing.NewModule(landingStore, scope),
		workspace.NewModule(workspaceStore, scope),
	).RegisterRoutes(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{Addr: ":" + port, Handler: otelhttp.NewHandler(observability.RequestLogger(logger, mux), "kosmos.http")}

	logger.Info("kosmos listening", "port", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
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
