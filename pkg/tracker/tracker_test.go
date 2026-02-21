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
	t.Cleanup(func() { _ = tr.Close() })
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

func TestResolveSessionExplicit(t *testing.T) {
	tr := newTestTracker(t)
	ctx := context.Background()

	sid, err := tr.ResolveSession(ctx, "key1", "my-session", 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if sid != "my-session" {
		t.Errorf("expected my-session, got %s", sid)
	}

	// Calling again with the same ID should return the same session.
	sid2, err := tr.ResolveSession(ctx, "key1", "my-session", 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if sid2 != "my-session" {
		t.Errorf("expected my-session, got %s", sid2)
	}
}

func TestResolveSessionAutoDetect(t *testing.T) {
	tr := newTestTracker(t)
	ctx := context.Background()

	// First call creates a new session.
	sid1, err := tr.ResolveSession(ctx, "key1", "", 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if sid1 == "" {
		t.Fatal("expected non-empty session ID")
	}

	// Second call within gap should reuse.
	sid2, err := tr.ResolveSession(ctx, "key1", "", 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if sid2 != sid1 {
		t.Errorf("expected same session %s, got %s", sid1, sid2)
	}

	// With a zero gap timeout, should create new.
	sid3, err := tr.ResolveSession(ctx, "key1", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if sid3 == sid1 {
		t.Error("expected new session with zero gap timeout")
	}
}

func TestListSessions(t *testing.T) {
	tr := newTestTracker(t)
	ctx := context.Background()

	_, _ = tr.ResolveSession(ctx, "key1", "sess-a", 30*time.Minute)
	_, _ = tr.ResolveSession(ctx, "key2", "sess-b", 30*time.Minute)

	all, err := tr.ListSessions(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(all))
	}

	filtered, err := tr.ListSessions(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 session, got %d", len(filtered))
	}
}

func TestSessionRequests(t *testing.T) {
	tr := newTestTracker(t)
	ctx := context.Background()
	now := time.Now().UTC()

	sid, _ := tr.ResolveSession(ctx, "key1", "sess-detail", 30*time.Minute)

	// Record 3 requests with increasing prompt tokens (simulating context growth).
	for i, pt := range []int{500, 1200, 2800} {
		_ = tr.Record(ctx, models.UsageRecord{
			APIKey: "key1", Model: "gpt-4", SessionID: sid,
			PromptTokens: pt, CompletionTokens: 100, TotalTokens: pt + 100,
			CreatedAt: now.Add(time.Duration(i) * time.Minute),
		})
	}

	reqs, err := tr.SessionRequests(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(reqs))
	}

	// First request has no context growth.
	if reqs[0].ContextGrowth != 0 {
		t.Errorf("expected 0 context growth for first request, got %d", reqs[0].ContextGrowth)
	}
	// Second: 1200 - 500 = 700
	if reqs[1].ContextGrowth != 700 {
		t.Errorf("expected 700 context growth, got %d", reqs[1].ContextGrowth)
	}
	// Third: 2800 - 1200 = 1600
	if reqs[2].ContextGrowth != 1600 {
		t.Errorf("expected 1600 context growth, got %d", reqs[2].ContextGrowth)
	}

	// Verify session counters were updated.
	sessions, _ := tr.ListSessions(ctx, "key1")
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].RequestCount != 3 {
		t.Errorf("expected 3 requests in session, got %d", sessions[0].RequestCount)
	}
	if sessions[0].TotalTokens != 600+1300+2900 {
		t.Errorf("expected %d total tokens, got %d", 600+1300+2900, sessions[0].TotalTokens)
	}
}

func TestMigrationIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Create tracker twice — second should not fail.
	tr1, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = tr1.Close()

	tr2, err := New(dbPath)
	if err != nil {
		t.Fatal("second New() failed:", err)
	}
	_ = tr2.Close()
}
