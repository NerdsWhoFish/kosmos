package landing

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	platformmodules "github.com/NerdsWhoFish/kosmos/backend/internal/platform/modules"
)

type OwnerFunc func(*http.Request) (string, error)
type ManagerFunc func(*http.Request) error

type Module struct {
	store   Store
	owner   OwnerFunc
	manager ManagerFunc
}

func NewModule(store Store, owner OwnerFunc, managers ...ManagerFunc) Module {
	module := Module{store: store, owner: owner}
	if len(managers) > 0 {
		module.manager = managers[0]
	}
	return module
}

func (Module) Name() string { return "landing" }

func (Module) Manifest() platformmodules.Manifest {
	return platformmodules.Manifest{Name: "landing", Navigation: []platformmodules.Navigation{{Path: "/", Label: "Overview", Icon: "overview"}}, Permissions: []string{"landing.read", "landing.manage"}, Resources: []string{"buttons", "notifications"}}
}

func (m Module) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/landing", m.landing)
	mux.HandleFunc("POST /api/v1/landing/buttons", m.createButton)
	mux.HandleFunc("PATCH /api/v1/landing/buttons/{id}", m.updateButton)
	mux.HandleFunc("DELETE /api/v1/landing/buttons/{id}", m.deleteButton)
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
	writeJSON(w, http.StatusOK, landingResponse{Buttons: buttons, Notifications: []notification{}})
}

func (m Module) createButton(w http.ResponseWriter, r *http.Request) {
	owner, ok := m.requireOwner(w, r)
	if !ok {
		return
	}
	if m.manager != nil && m.manager(r) != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "owner or administrator access required"})
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

func (m Module) updateButton(w http.ResponseWriter, r *http.Request) {
	owner, ok := m.requireOwner(w, r)
	if !ok {
		return
	}
	if m.manager != nil && m.manager(r) != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "owner or administrator access required"})
		return
	}
	var button Button
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&button); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid shortcut"})
		return
	}
	button.ID = r.PathValue("id")
	button.Label = strings.TrimSpace(button.Label)
	button.Description = strings.TrimSpace(button.Description)
	button.Href = strings.TrimSpace(button.Href)
	button.Icon = "globe"
	if err := validateButton(button); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	updated, err := m.store.UpdateButton(r.Context(), owner, button.ID, button)
	if errors.Is(err, errNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "shortcut not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save shortcut"})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (m Module) deleteButton(w http.ResponseWriter, r *http.Request) {
	owner, ok := m.requireOwner(w, r)
	if !ok {
		return
	}
	if m.manager != nil && m.manager(r) != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "owner or administrator access required"})
		return
	}
	if err := m.store.DeleteButton(r.Context(), owner, r.PathValue("id")); errors.Is(err, errNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "shortcut not found"})
		return
	} else if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete shortcut"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
