package router

import (
	"testing"

	"github.com/pario-ai/pario/pkg/config"
)

func TestResolveNoRoutes(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "openai", URL: "https://api.openai.com", APIKey: "sk-1"},
		},
	}
	r := New(cfg)
	routes, err := r.Resolve("gpt-4")
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].Provider.Name != "openai" || routes[0].Model != "gpt-4" {
		t.Errorf("unexpected route: %+v", routes[0])
	}
}

func TestResolveWithAlias(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "openai", URL: "https://api.openai.com", APIKey: "sk-1"},
			{Name: "anthropic", URL: "https://api.anthropic.com", APIKey: "sk-2"},
		},
		Router: config.RouterConfig{
			Routes: []config.RouteConfig{
				{
					Model: "fast",
					Targets: []config.RouteTarget{
						{Provider: "openai", Model: "gpt-4o-mini"},
						{Provider: "anthropic", Model: "claude-haiku-4-5"},
					},
				},
			},
		},
	}
	r := New(cfg)
	routes, err := r.Resolve("fast")
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
	if routes[0].Model != "gpt-4o-mini" || routes[0].Provider.Name != "openai" {
		t.Errorf("unexpected first route: %+v", routes[0])
	}
	if routes[1].Model != "claude-haiku-4-5" || routes[1].Provider.Name != "anthropic" {
		t.Errorf("unexpected second route: %+v", routes[1])
	}
}

func TestResolveEmptyModelUsesRequested(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "openai", URL: "https://api.openai.com", APIKey: "sk-1"},
		},
		Router: config.RouterConfig{
			Routes: []config.RouteConfig{
				{
					Model: "gpt-4",
					Targets: []config.RouteTarget{
						{Provider: "openai"},
					},
				},
			},
		},
	}
	r := New(cfg)
	routes, err := r.Resolve("gpt-4")
	if err != nil {
		t.Fatal(err)
	}
	if routes[0].Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", routes[0].Model)
	}
}

func TestResolveSkipsUnknownProvider(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "openai", URL: "https://api.openai.com", APIKey: "sk-1"},
		},
		Router: config.RouterConfig{
			Routes: []config.RouteConfig{
				{
					Model: "fast",
					Targets: []config.RouteTarget{
						{Provider: "unknown", Model: "x"},
						{Provider: "openai", Model: "gpt-4o-mini"},
					},
				},
			},
		},
	}
	r := New(cfg)
	routes, err := r.Resolve("fast")
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].Provider.Name != "openai" {
		t.Errorf("expected openai, got %s", routes[0].Provider.Name)
	}
}

func TestResolveAllUnknownProviders(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "openai", URL: "https://api.openai.com", APIKey: "sk-1"},
		},
		Router: config.RouterConfig{
			Routes: []config.RouteConfig{
				{
					Model:   "bad",
					Targets: []config.RouteTarget{{Provider: "unknown", Model: "x"}},
				},
			},
		},
	}
	r := New(cfg)
	_, err := r.Resolve("bad")
	if err == nil {
		t.Fatal("expected error for all unknown providers")
	}
}

func TestResolveNoProviders(t *testing.T) {
	cfg := &config.Config{}
	r := New(cfg)
	_, err := r.Resolve("gpt-4")
	if err == nil {
		t.Fatal("expected error for no providers")
	}
}
