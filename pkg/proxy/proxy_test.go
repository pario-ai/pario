package proxy

import (
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
	t.Cleanup(func() { tr.Close() })

	c, err := cachepkg.New(filepath.Join(dir, "cache.db"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })

	cfg := &config.Config{
		Listen: ":0",
		Providers: []config.ProviderConfig{
			{Name: "test", URL: upstream.URL, APIKey: "sk-provider"},
		},
	}

	return New(cfg, tr, c, nil)
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
	defer tr.Close()

	// Record usage that exceeds the budget
	_ = tr.Record(nil, models.UsageRecord{
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
