package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pario-ai/pario/pkg/models"
)

// fakeTracker implements tracker.Tracker for testing.
type fakeTracker struct {
	summaries []models.UsageSummary
	sessions  []models.Session
	requests  []models.SessionRequest
}

func (f *fakeTracker) Record(_ context.Context, _ models.UsageRecord) error              { return nil }
func (f *fakeTracker) QueryByKey(_ context.Context, _ string, _ time.Time) ([]models.UsageRecord, error) {
	return nil, nil
}
func (f *fakeTracker) TotalByKey(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeTracker) TotalByKeyAndModel(_ context.Context, _, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeTracker) Summary(_ context.Context, _ string) ([]models.UsageSummary, error) {
	return f.summaries, nil
}
func (f *fakeTracker) ResolveSession(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (f *fakeTracker) ListSessions(_ context.Context, _ string) ([]models.Session, error) {
	return f.sessions, nil
}
func (f *fakeTracker) SessionRequests(_ context.Context, _ string) ([]models.SessionRequest, error) {
	return f.requests, nil
}
func (f *fakeTracker) Close() error { return nil }

// fakeCache implements CacheStatter for testing.
type fakeCache struct {
	stats models.CacheStats
}

func (f *fakeCache) Stats() (models.CacheStats, error) { return f.stats, nil }

func sendAndReceive(t *testing.T, srv *Server, req Request) Response {
	t.Helper()
	line, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	line = append(line, '\n')

	var out bytes.Buffer
	if err := srv.Run(context.Background(), bytes.NewReader(line), &out); err != nil {
		t.Fatal(err)
	}

	var resp Response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nraw: %s", err, out.String())
	}
	return resp
}

func TestInitialize(t *testing.T) {
	srv := New(&fakeTracker{}, nil, nil, "test")
	resp := sendAndReceive(t, srv, Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	var result InitializeResult
	json.Unmarshal(data, &result)

	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("protocol version = %s, want 2024-11-05", result.ProtocolVersion)
	}
	if result.ServerInfo.Name != "pario" {
		t.Errorf("server name = %s, want pario", result.ServerInfo.Name)
	}
}

func TestToolsList(t *testing.T) {
	srv := New(&fakeTracker{}, nil, nil, "test")
	resp := sendAndReceive(t, srv, Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	var result ToolsListResult
	json.Unmarshal(data, &result)

	if len(result.Tools) != 5 {
		t.Errorf("got %d tools, want 5", len(result.Tools))
	}

	names := make(map[string]bool)
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"pario_stats", "pario_sessions", "pario_session_detail", "pario_budget", "pario_cache_stats"} {
		if !names[want] {
			t.Errorf("missing tool: %s", want)
		}
	}
}

func TestToolCallStats(t *testing.T) {
	tr := &fakeTracker{
		summaries: []models.UsageSummary{
			{APIKey: "sk-test", Model: "gpt-4", RequestCount: 10, TotalPrompt: 500, TotalCompletion: 200, TotalTokens: 700},
		},
	}
	srv := New(tr, nil, nil, "test")

	params, _ := json.Marshal(ToolCallParams{Name: "pario_stats", Arguments: json.RawMessage(`{}`)})
	resp := sendAndReceive(t, srv, Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`3`),
		Method:  "tools/call",
		Params:  params,
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	json.Unmarshal(data, &result)

	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}
	if !strings.Contains(result.Content[0].Text, "gpt-4") {
		t.Errorf("expected gpt-4 in output, got: %s", result.Content[0].Text)
	}
}

func TestToolCallCacheNotConfigured(t *testing.T) {
	srv := New(&fakeTracker{}, nil, nil, "test")

	params, _ := json.Marshal(ToolCallParams{Name: "pario_cache_stats"})
	resp := sendAndReceive(t, srv, Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`4`),
		Method:  "tools/call",
		Params:  params,
	})

	data, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	json.Unmarshal(data, &result)

	if !strings.Contains(result.Content[0].Text, "not configured") {
		t.Errorf("expected 'not configured', got: %s", result.Content[0].Text)
	}
}

func TestToolCallBudgetNotConfigured(t *testing.T) {
	srv := New(&fakeTracker{}, nil, nil, "test")

	params, _ := json.Marshal(ToolCallParams{Name: "pario_budget"})
	resp := sendAndReceive(t, srv, Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`5`),
		Method:  "tools/call",
		Params:  params,
	})

	data, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	json.Unmarshal(data, &result)

	if !strings.Contains(result.Content[0].Text, "not configured") {
		t.Errorf("expected 'not configured', got: %s", result.Content[0].Text)
	}
}

func TestToolCallCacheStats(t *testing.T) {
	cache := &fakeCache{stats: models.CacheStats{Entries: 42, Hits: 10, Misses: 5}}
	srv := New(&fakeTracker{}, cache, nil, "test")

	params, _ := json.Marshal(ToolCallParams{Name: "pario_cache_stats"})
	resp := sendAndReceive(t, srv, Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`6`),
		Method:  "tools/call",
		Params:  params,
	})

	data, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	json.Unmarshal(data, &result)

	text := result.Content[0].Text
	if !strings.Contains(text, "42") || !strings.Contains(text, "66.7%") {
		t.Errorf("unexpected cache stats output: %s", text)
	}
}

func TestToolCallSessionDetail(t *testing.T) {
	tr := &fakeTracker{
		requests: []models.SessionRequest{
			{Seq: 1, PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150, ContextGrowth: 100},
		},
	}
	srv := New(tr, nil, nil, "test")

	params, _ := json.Marshal(ToolCallParams{
		Name:      "pario_session_detail",
		Arguments: json.RawMessage(`{"session_id":"abc-123"}`),
	})
	resp := sendAndReceive(t, srv, Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`7`),
		Method:  "tools/call",
		Params:  params,
	})

	data, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	json.Unmarshal(data, &result)

	if !strings.Contains(result.Content[0].Text, "150") {
		t.Errorf("expected 150 in output, got: %s", result.Content[0].Text)
	}
}

func TestToolCallSessionDetailMissingID(t *testing.T) {
	srv := New(&fakeTracker{}, nil, nil, "test")

	params, _ := json.Marshal(ToolCallParams{
		Name:      "pario_session_detail",
		Arguments: json.RawMessage(`{}`),
	})
	resp := sendAndReceive(t, srv, Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`8`),
		Method:  "tools/call",
		Params:  params,
	})

	data, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	json.Unmarshal(data, &result)

	if !result.IsError {
		t.Error("expected isError=true for missing session_id")
	}
}

func TestNotificationNoResponse(t *testing.T) {
	srv := New(&fakeTracker{}, nil, nil, "test")

	line, _ := json.Marshal(Request{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	})
	line = append(line, '\n')

	var out bytes.Buffer
	_ = srv.Run(context.Background(), bytes.NewReader(line), &out)

	if out.Len() != 0 {
		t.Errorf("expected no output for notification, got: %s", out.String())
	}
}

func TestUnknownMethod(t *testing.T) {
	srv := New(&fakeTracker{}, nil, nil, "test")
	resp := sendAndReceive(t, srv, Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`9`),
		Method:  "unknown/method",
	})

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != CodeMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, CodeMethodNotFound)
	}
}
