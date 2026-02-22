package audit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pario-ai/pario/pkg/models"
)

func tempCfg(t *testing.T) models.AuditConfig {
	t.Helper()
	return models.AuditConfig{
		Enabled:       true,
		DBPath:        filepath.Join(t.TempDir(), "audit_test.db"),
		RetentionDays: 90,
		RedactKeys:    true,
		MaxBodySize:   1024,
		Include:       []string{"prompts", "responses", "metadata"},
	}
}

func mustNew(t *testing.T, cfg models.AuditConfig) *Logger {
	t.Helper()
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func sampleEntry() models.AuditEntry {
	return models.AuditEntry{
		RequestID:        "req-001",
		APIKeyHash:       "abc123hash",
		APIKeyPrefix:     "sk-test-",
		Model:            "gpt-4",
		SessionID:        "sess-1",
		Provider:         "openai",
		RequestBody:      `{"model":"gpt-4","messages":[]}`,
		ResponseBody:     `{"choices":[]}`,
		RequestHeaders:   map[string]string{"Content-Type": "application/json"},
		StatusCode:       200,
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
		LatencyMs:        150,
		CreatedAt:        time.Now(),
	}
}

func TestLogAndQuery(t *testing.T) {
	l := mustNew(t, tempCfg(t))
	ctx := context.Background()

	entry := sampleEntry()
	if err := l.Log(ctx, entry); err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, err := l.Query(ctx, models.AuditQueryOpts{Model: "gpt-4"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].RequestID != "req-001" {
		t.Errorf("expected req-001, got %s", entries[0].RequestID)
	}
}

func TestQueryByRequestID(t *testing.T) {
	l := mustNew(t, tempCfg(t))
	ctx := context.Background()

	_ = l.Log(ctx, sampleEntry())

	entries, err := l.Query(ctx, models.AuditQueryOpts{RequestID: "req-001"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1, got %d", len(entries))
	}
}

func TestExcludeModels(t *testing.T) {
	cfg := tempCfg(t)
	cfg.ExcludeModels = []string{"gpt-4"}
	l := mustNew(t, cfg)
	ctx := context.Background()

	entry := sampleEntry()
	if err := l.Log(ctx, entry); err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, err := l.Query(ctx, models.AuditQueryOpts{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for excluded model, got %d", len(entries))
	}
}

func TestBodyTruncation(t *testing.T) {
	cfg := tempCfg(t)
	cfg.MaxBodySize = 16
	l := mustNew(t, cfg)
	ctx := context.Background()

	entry := sampleEntry()
	entry.RequestBody = strings.Repeat("x", 100)
	if err := l.Log(ctx, entry); err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, err := l.Query(ctx, models.AuditQueryOpts{RequestID: "req-001"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries[0].RequestBody) != 16 {
		t.Errorf("expected truncated body len 16, got %d", len(entries[0].RequestBody))
	}
}

func TestIncludeFiltering(t *testing.T) {
	cfg := tempCfg(t)
	cfg.Include = []string{"metadata"} // no prompts or responses
	l := mustNew(t, cfg)
	ctx := context.Background()

	if err := l.Log(ctx, sampleEntry()); err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, err := l.Query(ctx, models.AuditQueryOpts{RequestID: "req-001"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if entries[0].RequestBody != "" {
		t.Errorf("expected empty request body, got %q", entries[0].RequestBody)
	}
	if entries[0].ResponseBody != "" {
		t.Errorf("expected empty response body, got %q", entries[0].ResponseBody)
	}
}

func TestCleanup(t *testing.T) {
	cfg := tempCfg(t)
	cfg.RetentionDays = 0 // everything is old
	l := mustNew(t, cfg)
	ctx := context.Background()

	entry := sampleEntry()
	entry.CreatedAt = time.Now().AddDate(0, 0, -1)
	_ = l.Log(ctx, entry)

	deleted, err := l.Cleanup(ctx)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}
}

func TestStats(t *testing.T) {
	l := mustNew(t, tempCfg(t))
	ctx := context.Background()

	_ = l.Log(ctx, sampleEntry())
	e2 := sampleEntry()
	e2.RequestID = "req-002"
	_ = l.Log(ctx, e2)

	stats, err := l.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if len(stats) == 0 {
		t.Fatal("expected stats")
	}
	if stats[0].Count != 2 {
		t.Errorf("expected count 2, got %d", stats[0].Count)
	}
}

func TestHashAPIKey(t *testing.T) {
	hash, prefix := HashAPIKey("sk-test-abc123xyz")
	if len(hash) != 64 {
		t.Errorf("expected 64-char hash, got %d", len(hash))
	}
	if prefix != "sk-test-" {
		t.Errorf("expected prefix sk-test-, got %s", prefix)
	}
}

func TestNilLoggerSafe(t *testing.T) {
	var l *Logger
	if err := l.Log(context.Background(), sampleEntry()); err != nil {
		t.Errorf("nil logger should be safe: %v", err)
	}
}

func TestNewInvalidPath(t *testing.T) {
	cfg := models.AuditConfig{
		Enabled: true,
		DBPath:  filepath.Join(os.TempDir(), "nonexistent", "deep", "path", "audit.db"),
		Include: []string{"prompts"},
	}
	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for invalid path")
	}
}
