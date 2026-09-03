package modules

import "net/http"

// Module is the extension point for a Kosmos business capability. A module
// owns its routes and can later expose events, permissions, and navigation
// metadata without coupling the platform shell to its implementation.
type Module interface {
	Name() string
	RegisterRoutes(*http.ServeMux)
}

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
