package tracker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pario-ai/pario/pkg/models"
)

func newTestTracker(t *testing.T) *SQLiteTracker {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	tr, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	return tr
}

func TestRecordAndQuery(t *testing.T) {
	tr := newTestTracker(t)
	ctx := context.Background()
	now := time.Now().UTC()

	rec := models.UsageRecord{
		APIKey:           "key1",
		Model:            "gpt-4",
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		CreatedAt:        now,
	}
	if err := tr.Record(ctx, rec); err != nil {
		t.Fatal(err)
	}

	records, err := tr.QueryByKey(ctx, "key1", now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].TotalTokens != 150 {
		t.Errorf("expected 150 tokens, got %d", records[0].TotalTokens)
	}
}

func TestTotalByKey(t *testing.T) {
	tr := newTestTracker(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for i := range 3 {
		_ = tr.Record(ctx, models.UsageRecord{
			APIKey: "key1", Model: "gpt-4",
			PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		})
	}

	total, err := tr.TotalByKey(ctx, "key1", now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if total != 450 {
		t.Errorf("expected 450, got %d", total)
	}
}

func TestSummary(t *testing.T) {
	tr := newTestTracker(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_ = tr.Record(ctx, models.UsageRecord{
		APIKey: "key1", Model: "gpt-4",
		PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
		CreatedAt: now,
	})
	_ = tr.Record(ctx, models.UsageRecord{
		APIKey: "key2", Model: "gpt-3.5-turbo",
		PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300,
		CreatedAt: now,
	})

	summaries, err := tr.Summary(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	// Filter by key
	summaries, err = tr.Summary(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
}
