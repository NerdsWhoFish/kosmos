package modules

import "net/http"

// Module is the extension point for a Kosmos business capability. A module
// owns its routes and can later expose events, permissions, and navigation
// metadata without coupling the platform shell to its implementation.
type Module interface {
	Name() string
	RegisterRoutes(*http.ServeMux)
}

type Manifest struct {
	Name                string       `json:"name"`
	Navigation          []Navigation `json:"navigation"`
	Permissions         []string     `json:"permissions"`
	Resources           []string     `json:"resources"`
	EventTypes          []string     `json:"eventTypes"`
	BackgroundJobs      []string     `json:"backgroundJobs"`
	SearchProviders     []string     `json:"searchProviders"`
	DocumentLinkTargets []string     `json:"documentLinkTargets"`
}

type Navigation struct {
	Path  string `json:"path"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
}

type Described interface{ Manifest() Manifest }

type Registry struct {
	modules []Module
}

func NewRegistry(modules ...Module) *Registry {
	return &Registry{modules: append([]Module(nil), modules...)}
}

func (r *Registry) RegisterRoutes(mux *http.ServeMux) {
	for _, module := range r.modules {
		module.RegisterRoutes(mux)
	}
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.modules))
	for _, module := range r.modules {
		names = append(names, module.Name())
	}
	return names
}

func (r *Registry) Manifests() []Manifest {
	manifests := make([]Manifest, 0, len(r.modules))
	for _, module := range r.modules {
		if described, ok := module.(Described); ok {
			manifests = append(manifests, described.Manifest())
		}
	}
	return manifests
}
