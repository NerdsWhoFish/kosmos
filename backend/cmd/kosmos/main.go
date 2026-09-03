package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/observability"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type landingResponse struct {
	Buttons       []landingButton `json:"buttons"`
	Notifications []notification  `json:"notifications"`
}

type landingButton struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Href        string `json:"href"`
	Icon        string `json:"icon"`
}

type notification struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
	Href      string    `json:"href"`
}

func main() {
	shutdownTelemetry, err := observability.Setup(context.Background())
	if err != nil {
		slog.Error("telemetry setup failed", "error", err)
		os.Exit(1)
	}
	defer shutdownTelemetry()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", health)
	mux.HandleFunc("GET /api/v1/landing", landing)
	mux.HandleFunc("GET /api/v1/notifications", notifications)
	mux.HandleFunc("GET /api/", notFound)
	mux.HandleFunc("/", spaFallback)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{Addr: ":" + port, Handler: otelhttp.NewHandler(mux, "kosmos.http")}

	logger.Info("kosmos listening", "port", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func landing(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, landingResponse{
		Buttons: []landingButton{
			{ID: "website", Label: "Open website", Description: "Jump straight to the public business site.", Href: "https://www.nerdswhofish.com", Icon: "globe"},
			{ID: "bookings", Label: "Bookings", Description: "Manage meetings and availability.", Href: "https://book.nerdswhofish.com", Icon: "calendar"},
			{ID: "contacts", Label: "Contacts", Description: "Keep every relationship in one place.", Href: "/contacts", Icon: "users"},
		},
		Notifications: sampleNotifications(),
	})
}

func notifications(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]notification{"notifications": sampleNotifications()})
}

func sampleNotifications() []notification {
	return []notification{
		{ID: "welcome", Title: "Kosmos is ready", Summary: "Your business home base is ready to customize.", Kind: "system", CreatedAt: time.Now().UTC(), Href: "/docs/getting-started"},
	}
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "api route not found"})
}

func spaFallback(w http.ResponseWriter, r *http.Request) {
	webRoot := "/web"
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
