package config

import (
	"fmt"
	"os"
	"time"

	"github.com/pario-ai/pario/pkg/models"
	"gopkg.in/yaml.v3"
)

// Config holds all Pario configuration.
type Config struct {
	Listen    string           `yaml:"listen"`
	DBPath    string           `yaml:"db_path"`
	Providers []ProviderConfig `yaml:"providers"`
	Cache     CacheConfig      `yaml:"cache"`
	Budget    BudgetConfig     `yaml:"budget"`
	Session   SessionConfig    `yaml:"session"`
	Router    RouterConfig     `yaml:"router"`
}

// RouterConfig defines model routing and fallback chains.
type RouterConfig struct {
	Routes []RouteConfig `yaml:"routes"`
}

// RouteConfig maps a client-facing model alias to an ordered list of targets.
type RouteConfig struct {
	Model   string        `yaml:"model"`
	Targets []RouteTarget `yaml:"targets"`
}

// RouteTarget identifies a specific provider and model in a fallback chain.
type RouteTarget struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// SessionConfig controls session detection.
type SessionConfig struct {
	GapTimeout time.Duration `yaml:"gap_timeout"`
}

// ProviderConfig defines an upstream LLM provider.
// ProviderConfig defines an upstream LLM provider.
// Type is "openai" (default) or "anthropic".
type ProviderConfig struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
	Type   string `yaml:"type"`
}

// CacheConfig controls the prompt cache.
type CacheConfig struct {
	Enabled bool          `yaml:"enabled"`
	TTL     time.Duration `yaml:"ttl"`
}

// BudgetConfig controls budget enforcement.
type BudgetConfig struct {
	Enabled  bool                  `yaml:"enabled"`
	Policies []models.BudgetPolicy `yaml:"policies"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Listen: ":8080",
		DBPath: "pario.db",
		Cache: CacheConfig{
			Enabled: true,
			TTL:     time.Hour,
		},
		Budget: BudgetConfig{
			Enabled: false,
		},
		Session: SessionConfig{
			GapTimeout: 30 * time.Minute,
		},
	}
}

// Load reads a YAML config file and expands environment variables.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	cfg := Default()
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}
