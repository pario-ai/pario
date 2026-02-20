package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/pario-ai/pario/pkg/budget"
	cachepkg "github.com/pario-ai/pario/pkg/cache/sqlite"
	"github.com/pario-ai/pario/pkg/config"
	"github.com/pario-ai/pario/pkg/models"
	"github.com/pario-ai/pario/pkg/tracker"
)

// Server is the Pario reverse proxy.
type Server struct {
	cfg      *config.Config
	tracker  tracker.Tracker
	cache    *cachepkg.Cache
	enforcer *budget.Enforcer
	mux      *http.ServeMux
}

// New creates a proxy Server wired with all dependencies.
func New(cfg *config.Config, t tracker.Tracker, c *cachepkg.Cache, e *budget.Enforcer) *Server {
	s := &Server{
		cfg:      cfg,
		tracker:  t,
		cache:    c,
		enforcer: e,
		mux:      http.NewServeMux(),
	}
	s.mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	s.mux.HandleFunc("/", s.handlePassthrough)
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the proxy server with graceful shutdown support.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.cfg.Listen,
		Handler: s,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("pario proxy listening on %s", s.cfg.Listen)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	clientKey := extractAPIKey(r)
	if clientKey == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing API key")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	r.Body.Close()

	var req models.ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Cache check
	if s.cache != nil && !req.Stream {
		hash := cachepkg.HashPrompt(req.Model, req.Messages)
		if cached, ok := s.cache.Get(hash, req.Model); ok {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Pario-Cache", "hit")
			w.Write(cached)
			return
		}
	}

	// Budget check
	if s.enforcer != nil {
		if err := s.enforcer.Check(r.Context(), clientKey); err != nil {
			if errors.Is(err, budget.ErrBudgetExceeded) {
				writeJSONError(w, http.StatusTooManyRequests, "token budget exceeded")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "budget check failed")
			return
		}
	}

	// Forward to provider
	provider := s.cfg.Providers[0]
	providerURL, err := url.Parse(provider.URL)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "invalid provider URL")
		return
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		providerURL.String()+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create upstream request")
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+provider.APIKey)

	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream request failed")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to read upstream response")
		return
	}

	// Parse response for usage tracking
	if resp.StatusCode == http.StatusOK && !req.Stream {
		var chatResp models.ChatCompletionResponse
		if err := json.Unmarshal(respBody, &chatResp); err == nil && chatResp.Usage != nil {
			// Record usage
			_ = s.tracker.Record(r.Context(), models.UsageRecord{
				APIKey:           clientKey,
				Model:            chatResp.Model,
				PromptTokens:     chatResp.Usage.PromptTokens,
				CompletionTokens: chatResp.Usage.CompletionTokens,
				TotalTokens:      chatResp.Usage.TotalTokens,
				CreatedAt:        time.Now().UTC(),
			})

			// Cache the response
			if s.cache != nil {
				hash := cachepkg.HashPrompt(req.Model, req.Messages)
				_ = s.cache.Put(hash, req.Model, respBody)
			}
		}
	}

	// Forward response headers and body
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Pario-Cache", "miss")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func (s *Server) handlePassthrough(w http.ResponseWriter, r *http.Request) {
	if len(s.cfg.Providers) == 0 {
		writeJSONError(w, http.StatusServiceUnavailable, "no providers configured")
		return
	}

	provider := s.cfg.Providers[0]
	target, err := url.Parse(provider.URL)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "invalid provider URL")
		return
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			req.Header.Set("Authorization", "Bearer "+provider.APIKey)
		},
	}
	proxy.ServeHTTP(w, r)
}

func extractAPIKey(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":{"message":%q,"type":"pario_error","code":%d}}`, message, code)
}
