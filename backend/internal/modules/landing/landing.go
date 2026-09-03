package landing

import (
	"encoding/json"
	"net/http"
	"time"
)

type Module struct{}

func NewModule() Module { return Module{} }

func (Module) Name() string { return "landing" }

func (Module) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/landing", landing)
	mux.HandleFunc("GET /api/v1/notifications", notifications)
}

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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
