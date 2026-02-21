package router

import (
	"fmt"

	"github.com/pario-ai/pario/pkg/config"
)

// Route represents a resolved provider and model to try.
type Route struct {
	Provider config.ProviderConfig
	Model    string
}

// Router resolves requested model names to ordered provider+model chains.
type Router struct {
	cfg *config.Config
}

// New creates a Router from the given configuration.
func New(cfg *config.Config) *Router {
	return &Router{cfg: cfg}
}

// Resolve returns an ordered list of routes for the requested model.
// If the model matches a configured route, the route's targets are returned.
// Otherwise, the first provider is used with the original model name.
func (r *Router) Resolve(requestedModel string) ([]Route, error) {
	if len(r.cfg.Providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}

	// Build provider index by name
	providerIndex := make(map[string]config.ProviderConfig, len(r.cfg.Providers))
	for _, p := range r.cfg.Providers {
		providerIndex[p.Name] = p
	}

	// Check configured routes
	for _, route := range r.cfg.Router.Routes {
		if route.Model != requestedModel {
			continue
		}
		var routes []Route
		for _, target := range route.Targets {
			provider, ok := providerIndex[target.Provider]
			if !ok {
				continue // skip unknown providers
			}
			model := target.Model
			if model == "" {
				model = requestedModel
			}
			routes = append(routes, Route{Provider: provider, Model: model})
		}
		if len(routes) == 0 {
			return nil, fmt.Errorf("route %q: all providers unknown", requestedModel)
		}
		return routes, nil
	}

	// No matching route — default to first provider
	return []Route{{Provider: r.cfg.Providers[0], Model: requestedModel}}, nil
}
