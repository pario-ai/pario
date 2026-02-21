package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pario-ai/pario/pkg/budget"
	cachepkg "github.com/pario-ai/pario/pkg/cache/sqlite"
	"github.com/pario-ai/pario/pkg/config"
	"github.com/pario-ai/pario/pkg/models"
	"github.com/pario-ai/pario/pkg/tracker"
)

func setupProxy(t *testing.T, upstream *httptest.Server) *Server {
	t.Helper()
	dir := t.TempDir()

	tr, err := tracker.New(filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })

	c, err := cachepkg.New(filepath.Join(dir, "cache.db"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })

	cfg := &config.Config{
		Listen: ":0",
		Providers: []config.ProviderConfig{
			{Name: "test", URL: upstream.URL, APIKey: "sk-provider"},
		},
		Session: config.SessionConfig{GapTimeout: 30 * time.Minute},
	}

	return New(cfg, tr, c, nil)
}

func newUpstream() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.ChatCompletionResponse{
			ID:    "chatcmpl-123",
			Model: "gpt-4",
			Choices: []models.Choice{
				{Index: 0, Message: models.ChatMessage{Role: "assistant", Content: "Hello!"}, FinishReason: "stop"},
			},
			Usage: &models.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestChatCompletions(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify provider key is used
		if r.Header.Get("Authorization") != "Bearer sk-provider" {
			t.Error("expected provider API key in upstream request")
		}
		resp := models.ChatCompletionResponse{
			ID:    "chatcmpl-123",
			Model: "gpt-4",
			Choices: []models.Choice{
				{Index: 0, Message: models.ChatMessage{Role: "assistant", Content: "Hello!"}, FinishReason: "stop"},
			},
			Usage: &models.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	srv := setupProxy(t, upstream)

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-key")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Pario-Cache") != "miss" {
		t.Error("expected cache miss on first request")
	}

	// Should get a session ID back
	if w.Header().Get("X-Pario-Session") == "" {
		t.Error("expected X-Pario-Session header in response")
	}

	// Second request should be cached
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req2.Header.Set("Authorization", "Bearer client-key")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	if w2.Header().Get("X-Pario-Cache") != "hit" {
		t.Error("expected cache hit on second request")
	}
}

func TestMissingAPIKey(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer upstream.Close()

	srv := setupProxy(t, upstream)

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBudgetExceeded(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer upstream.Close()

	dir := t.TempDir()
	tr, _ := tracker.New(filepath.Join(dir, "tracker.db"))
	defer func() { _ = tr.Close() }()

	// Record usage that exceeds the budget
	ctx := context.Background()
	_ = tr.Record(ctx, models.UsageRecord{
		APIKey: "client-key", Model: "gpt-4",
		PromptTokens: 500, CompletionTokens: 600, TotalTokens: 1100,
		CreatedAt: time.Now().UTC(),
	})

	enforcer := budget.New([]models.BudgetPolicy{
		{APIKey: "*", MaxTokens: 1000, Period: models.BudgetDaily},
	}, tr)

	cfg := &config.Config{
		Listen:    ":0",
		Providers: []config.ProviderConfig{{Name: "test", URL: upstream.URL, APIKey: "sk-provider"}},
		Session:   config.SessionConfig{GapTimeout: 30 * time.Minute},
	}

	srv := New(cfg, tr, nil, enforcer)

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestExplicitSessionHeader(t *testing.T) {
	upstream := newUpstream()
	defer upstream.Close()

	srv := setupProxy(t, upstream)

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-key")
	req.Header.Set("X-Pario-Session", "my-custom-session")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("X-Pario-Session"); got != "my-custom-session" {
		t.Errorf("expected X-Pario-Session=my-custom-session, got %s", got)
	}
}

func newAnthropicUpstream() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.AnthropicResponse{
			ID:    "msg_123",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-sonnet-4-20250514",
			Content: []models.AnthropicContent{
				{Type: "text", Text: "Hello!"},
			},
			StopReason: "end_turn",
			Usage:      &models.AnthropicUsage{InputTokens: 12, OutputTokens: 8},
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

func setupAnthropicProxy(t *testing.T, upstream *httptest.Server) *Server {
	t.Helper()
	dir := t.TempDir()

	tr, err := tracker.New(filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })

	c, err := cachepkg.New(filepath.Join(dir, "cache.db"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })

	cfg := &config.Config{
		Listen: ":0",
		Providers: []config.ProviderConfig{
			{Name: "anthropic", URL: upstream.URL, APIKey: "sk-ant-provider", Type: "anthropic"},
		},
		Session: config.SessionConfig{GapTimeout: 30 * time.Minute},
	}

	return New(cfg, tr, c, nil)
}

func TestAnthropicMessages(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify x-api-key is used for upstream
		if r.Header.Get("x-api-key") != "sk-ant-provider" {
			t.Error("expected provider x-api-key in upstream request")
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages path, got %s", r.URL.Path)
		}
		resp := models.AnthropicResponse{
			ID:    "msg_123",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-sonnet-4-20250514",
			Content: []models.AnthropicContent{
				{Type: "text", Text: "Hello!"},
			},
			StopReason: "end_turn",
			Usage:      &models.AnthropicUsage{InputTokens: 12, OutputTokens: 8},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	srv := setupAnthropicProxy(t, upstream)

	body := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}],"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("x-api-key", "client-key")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Pario-Cache") != "miss" {
		t.Error("expected cache miss on first request")
	}
	if w.Header().Get("X-Pario-Session") == "" {
		t.Error("expected X-Pario-Session header")
	}

	// Second request should be cached
	req2 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req2.Header.Set("x-api-key", "client-key")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	if w2.Header().Get("X-Pario-Cache") != "hit" {
		t.Error("expected cache hit on second request")
	}
}

func TestAnthropicXAPIKeyAuth(t *testing.T) {
	upstream := newAnthropicUpstream()
	defer upstream.Close()

	srv := setupAnthropicProxy(t, upstream)

	// Request with x-api-key should work
	body := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}],"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("x-api-key", "client-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Request with no auth should fail
	req2 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w2.Code)
	}
}

func TestAnthropicSessionTracking(t *testing.T) {
	upstream := newAnthropicUpstream()
	defer upstream.Close()

	srv := setupAnthropicProxy(t, upstream)

	body := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}],"max_tokens":1024}`

	// Auto session
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("x-api-key", "client-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	sessionID := w.Header().Get("X-Pario-Session")
	if sessionID == "" {
		t.Fatal("expected auto-assigned session ID")
	}
	if !strings.HasPrefix(sessionID, "sess_") {
		t.Errorf("expected session ID to start with sess_, got %s", sessionID)
	}

	// Explicit session (different body to avoid cache hit)
	body2 := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hello"}],"max_tokens":1024}`
	req2 := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body2))
	req2.Header.Set("x-api-key", "client-key")
	req2.Header.Set("X-Pario-Session", "my-session")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	if got := w2.Header().Get("X-Pario-Session"); got != "my-session" {
		t.Errorf("expected X-Pario-Session=my-session, got %s", got)
	}
}

func TestAutoSessionAssigned(t *testing.T) {
	upstream := newUpstream()
	defer upstream.Close()

	srv := setupProxy(t, upstream)

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-key")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	sessionID := w.Header().Get("X-Pario-Session")
	if sessionID == "" {
		t.Fatal("expected auto-assigned session ID")
	}
	if !strings.HasPrefix(sessionID, "sess_") {
		t.Errorf("expected session ID to start with sess_, got %s", sessionID)
	}
}

func TestFallbackOn5xx(t *testing.T) {
	callCount := 0
	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer upstream1.Close()

	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := models.ChatCompletionResponse{
			ID:    "chatcmpl-fallback",
			Model: "gpt-4o-mini",
			Choices: []models.Choice{
				{Index: 0, Message: models.ChatMessage{Role: "assistant", Content: "fallback!"}, FinishReason: "stop"},
			},
			Usage: &models.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream2.Close()

	dir := t.TempDir()
	tr, _ := tracker.New(filepath.Join(dir, "tracker.db"))
	defer func() { _ = tr.Close() }()

	cfg := &config.Config{
		Listen: ":0",
		Providers: []config.ProviderConfig{
			{Name: "primary", URL: upstream1.URL, APIKey: "sk-1"},
			{Name: "fallback", URL: upstream2.URL, APIKey: "sk-2"},
		},
		Router: config.RouterConfig{
			Routes: []config.RouteConfig{
				{
					Model: "gpt-4",
					Targets: []config.RouteTarget{
						{Provider: "primary", Model: "gpt-4"},
						{Provider: "fallback", Model: "gpt-4o-mini"},
					},
				},
			},
		},
		Session: config.SessionConfig{GapTimeout: 30 * time.Minute},
	}

	srv := New(cfg, tr, nil, nil)

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if callCount != 2 {
		t.Errorf("expected 2 upstream calls (1 fail + 1 success), got %d", callCount)
	}
}

func TestNoFallbackOn4xx(t *testing.T) {
	callCount := 0
	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer upstream1.Close()

	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream2.Close()

	dir := t.TempDir()
	tr, _ := tracker.New(filepath.Join(dir, "tracker.db"))
	defer func() { _ = tr.Close() }()

	cfg := &config.Config{
		Listen: ":0",
		Providers: []config.ProviderConfig{
			{Name: "primary", URL: upstream1.URL, APIKey: "sk-1"},
			{Name: "fallback", URL: upstream2.URL, APIKey: "sk-2"},
		},
		Router: config.RouterConfig{
			Routes: []config.RouteConfig{
				{
					Model: "gpt-4",
					Targets: []config.RouteTarget{
						{Provider: "primary", Model: "gpt-4"},
						{Provider: "fallback", Model: "gpt-4"},
					},
				},
			},
		},
		Session: config.SessionConfig{GapTimeout: 30 * time.Minute},
	}

	srv := New(cfg, tr, nil, nil)

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if callCount != 1 {
		t.Errorf("expected 1 upstream call (no fallback on 4xx), got %d", callCount)
	}
}

func TestAllProvidersFail502(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"fail"}`))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	tr, _ := tracker.New(filepath.Join(dir, "tracker.db"))
	defer func() { _ = tr.Close() }()

	cfg := &config.Config{
		Listen: ":0",
		Providers: []config.ProviderConfig{
			{Name: "p1", URL: upstream.URL, APIKey: "sk-1"},
			{Name: "p2", URL: upstream.URL, APIKey: "sk-2"},
		},
		Router: config.RouterConfig{
			Routes: []config.RouteConfig{
				{
					Model: "gpt-4",
					Targets: []config.RouteTarget{
						{Provider: "p1", Model: "gpt-4"},
						{Provider: "p2", Model: "gpt-4"},
					},
				},
			},
		},
		Session: config.SessionConfig{GapTimeout: 30 * time.Minute},
	}

	srv := New(cfg, tr, nil, nil)

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// Last 5xx result should be returned (not 502 from pario) since we have a result
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 (last upstream response), got %d", w.Code)
	}
}

func TestModelRewriteInBody(t *testing.T) {
	var receivedModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		receivedModel = body["model"].(string)
		resp := models.ChatCompletionResponse{
			ID:    "chatcmpl-rw",
			Model: receivedModel,
			Choices: []models.Choice{
				{Index: 0, Message: models.ChatMessage{Role: "assistant", Content: "ok"}, FinishReason: "stop"},
			},
			Usage: &models.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	dir := t.TempDir()
	tr, _ := tracker.New(filepath.Join(dir, "tracker.db"))
	defer func() { _ = tr.Close() }()

	cfg := &config.Config{
		Listen: ":0",
		Providers: []config.ProviderConfig{
			{Name: "openai", URL: upstream.URL, APIKey: "sk-1"},
		},
		Router: config.RouterConfig{
			Routes: []config.RouteConfig{
				{
					Model: "fast",
					Targets: []config.RouteTarget{
						{Provider: "openai", Model: "gpt-4o-mini"},
					},
				},
			},
		},
		Session: config.SessionConfig{GapTimeout: 30 * time.Minute},
	}

	srv := New(cfg, tr, nil, nil)

	body := `{"model":"fast","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if receivedModel != "gpt-4o-mini" {
		t.Errorf("expected upstream to receive model gpt-4o-mini, got %s", receivedModel)
	}
}

func TestTransportErrorFallback(t *testing.T) {
	// upstream1 is a closed server (transport error)
	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	upstream1.Close() // close immediately to cause transport error

	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.ChatCompletionResponse{
			ID:    "chatcmpl-ok",
			Model: "gpt-4",
			Choices: []models.Choice{
				{Index: 0, Message: models.ChatMessage{Role: "assistant", Content: "ok"}, FinishReason: "stop"},
			},
			Usage: &models.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream2.Close()

	dir := t.TempDir()
	tr, _ := tracker.New(filepath.Join(dir, "tracker.db"))
	defer func() { _ = tr.Close() }()

	cfg := &config.Config{
		Listen: ":0",
		Providers: []config.ProviderConfig{
			{Name: "dead", URL: upstream1.URL, APIKey: "sk-1"},
			{Name: "alive", URL: upstream2.URL, APIKey: "sk-2"},
		},
		Router: config.RouterConfig{
			Routes: []config.RouteConfig{
				{
					Model: "gpt-4",
					Targets: []config.RouteTarget{
						{Provider: "dead", Model: "gpt-4"},
						{Provider: "alive", Model: "gpt-4"},
					},
				},
			},
		},
		Session: config.SessionConfig{GapTimeout: 30 * time.Minute},
	}

	srv := New(cfg, tr, nil, nil)

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
