package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Listen != ":8080" {
		t.Errorf("expected :8080, got %s", cfg.Listen)
	}
	if cfg.Cache.TTL != time.Hour {
		t.Errorf("expected 1h TTL, got %v", cfg.Cache.TTL)
	}
}

func TestLoad(t *testing.T) {
	t.Setenv("TEST_API_KEY", "sk-test-123")

	content := `
listen: ":9090"
db_path: "test.db"
providers:
  - name: openai
    url: https://api.openai.com
    api_key: ${TEST_API_KEY}
cache:
  enabled: true
  ttl: 30m
budget:
  enabled: true
  policies:
    - api_key: "*"
      max_tokens: 500000
      period: daily
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Listen != ":9090" {
		t.Errorf("expected :9090, got %s", cfg.Listen)
	}
	if cfg.Providers[0].APIKey != "sk-test-123" {
		t.Errorf("env var not expanded: got %s", cfg.Providers[0].APIKey)
	}
	if cfg.Cache.TTL != 30*time.Minute {
		t.Errorf("expected 30m TTL, got %v", cfg.Cache.TTL)
	}
	if !cfg.Budget.Enabled {
		t.Error("expected budget enabled")
	}
	if len(cfg.Budget.Policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(cfg.Budget.Policies))
	}
	if cfg.Budget.Policies[0].MaxTokens != 500000 {
		t.Errorf("expected 500000 max tokens, got %d", cfg.Budget.Policies[0].MaxTokens)
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
