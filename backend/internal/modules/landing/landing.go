package landing

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type OwnerFunc func(*http.Request) (string, error)

type Module struct {
	store Store
	owner OwnerFunc
}

func NewModule(store Store, owner OwnerFunc) Module {
	return Module{store: store, owner: owner}
}

func (Module) Name() string { return "landing" }

func (m Module) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/landing", m.landing)
	mux.HandleFunc("POST /api/v1/landing/buttons", m.createButton)
	mux.HandleFunc("GET /api/v1/notifications", m.notifications)
}

type landingResponse struct {
	Buttons       []Button       `json:"buttons"`
	Notifications []notification `json:"notifications"`
}

type notification struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
	Href      string    `json:"href"`
}

func (m Module) landing(w http.ResponseWriter, r *http.Request) {
	owner, ok := m.requireOwner(w, r)
	if !ok {
		return
	}
	buttons, err := m.store.ListButtons(r.Context(), owner)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load landing zone"})
		return
	}
	writeJSON(w, http.StatusOK, landingResponse{Buttons: buttons, Notifications: sampleNotifications()})
}

func (m Module) createButton(w http.ResponseWriter, r *http.Request) {
	owner, ok := m.requireOwner(w, r)
	if !ok {
		return
	}
	var button Button
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&button); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid shortcut"})
		return
	}
	button.ID = ""
	button.Label = strings.TrimSpace(button.Label)
	button.Description = strings.TrimSpace(button.Description)
	button.Href = strings.TrimSpace(button.Href)
	button.Icon = "globe"
	if err := validateButton(button); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	created, err := m.store.CreateButton(r.Context(), owner, button)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save shortcut"})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (m Module) notifications(w http.ResponseWriter, r *http.Request) {
	if _, ok := m.requireOwner(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string][]notification{"notifications": sampleNotifications()})
}

func (m Module) requireOwner(w http.ResponseWriter, r *http.Request) (string, bool) {
	owner, err := m.owner(r)
	if err != nil || owner == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return "", false
	}
	return owner, true
}

func validateButton(button Button) error {
	if button.Label == "" || len(button.Label) > 80 {
		return &validationError{"label must be between 1 and 80 characters"}
	}
	if len(button.Description) > 180 {
		return &validationError{"description must be 180 characters or fewer"}
	}
	parsed, err := url.ParseRequestURI(button.Href)
	if err != nil || (parsed.IsAbs() && ((parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "")) || (!parsed.IsAbs() && !strings.HasPrefix(button.Href, "/")) {
		return &validationError{"link must be an HTTPS, HTTP, or workspace-relative URL"}
	}
	return nil
}

type validationError struct{ message string }

func (e *validationError) Error() string { return e.message }

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
