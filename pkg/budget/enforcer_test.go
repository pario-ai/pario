package budget

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pario-ai/pario/pkg/models"
	"github.com/pario-ai/pario/pkg/tracker"
)

func setup(t *testing.T) (tracker.Tracker, context.Context) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "budget_test.db")
	tr, err := tracker.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	return tr, context.Background()
}

func TestCheckUnderBudget(t *testing.T) {
	tr, ctx := setup(t)

	_ = tr.Record(ctx, models.UsageRecord{
		APIKey: "key1", Model: "gpt-4",
		PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
		CreatedAt: time.Now().UTC(),
	})

	e := New([]models.BudgetPolicy{
		{APIKey: "*", MaxTokens: 1000, Period: models.BudgetDaily},
	}, tr)

	if err := e.Check(ctx, "key1"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestCheckExceeded(t *testing.T) {
	tr, ctx := setup(t)

	_ = tr.Record(ctx, models.UsageRecord{
		APIKey: "key1", Model: "gpt-4",
		PromptTokens: 500, CompletionTokens: 600, TotalTokens: 1100,
		CreatedAt: time.Now().UTC(),
	})

	e := New([]models.BudgetPolicy{
		{APIKey: "*", MaxTokens: 1000, Period: models.BudgetDaily},
	}, tr)

	err := e.Check(ctx, "key1")
	if err == nil {
		t.Fatal("expected budget exceeded error")
	}
	if err != ErrBudgetExceeded {
		t.Errorf("expected ErrBudgetExceeded, got %v", err)
	}
}

func TestStatus(t *testing.T) {
	tr, ctx := setup(t)

	_ = tr.Record(ctx, models.UsageRecord{
		APIKey: "key1", Model: "gpt-4",
		PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
		CreatedAt: time.Now().UTC(),
	})

	e := New([]models.BudgetPolicy{
		{APIKey: "*", MaxTokens: 1000, Period: models.BudgetDaily},
	}, tr)

	statuses, err := e.Status(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Used != 150 {
		t.Errorf("expected 150 used, got %d", statuses[0].Used)
	}
	if statuses[0].Remaining != 850 {
		t.Errorf("expected 850 remaining, got %d", statuses[0].Remaining)
	}
}

func TestSpecificKeyPolicy(t *testing.T) {
	tr, ctx := setup(t)

	e := New([]models.BudgetPolicy{
		{APIKey: "key1", MaxTokens: 500, Period: models.BudgetDaily},
		{APIKey: "*", MaxTokens: 10000, Period: models.BudgetDaily},
	}, tr)

	// key2 should only match wildcard
	statuses, err := e.Status(ctx, "key2")
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status for key2, got %d", len(statuses))
	}

	// key1 should match both
	statuses, err = e.Status(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses for key1, got %d", len(statuses))
	}
}
