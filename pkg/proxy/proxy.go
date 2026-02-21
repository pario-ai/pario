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
	"github.com/pario-ai/pario/pkg/router"
	"github.com/pario-ai/pario/pkg/tracker"
)

// Server is the Pario reverse proxy.
type Server struct {
	cfg      *config.Config
	tracker  tracker.Tracker
	cache    *cachepkg.Cache
	enforcer *budget.Enforcer
	router   *router.Router
	mux      *http.ServeMux
}

// New creates a proxy Server wired with all dependencies.
func New(cfg *config.Config, t tracker.Tracker, c *cachepkg.Cache, e *budget.Enforcer) *Server {
	s := &Server{
		cfg:      cfg,
		tracker:  t,
		cache:    c,
		enforcer: e,
		router:   router.New(cfg),
		mux:      http.NewServeMux(),
	}
	s.mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	s.mux.HandleFunc("/v1/messages", s.handleMessages)
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

// upstreamResult holds the response from a single upstream attempt.
type upstreamResult struct {
	statusCode int
	body       []byte
	header     http.Header
}

// doUpstreamRequest sends a request to an upstream provider and returns the result.
func doUpstreamRequest(ctx context.Context, providerURL, path, contentType string, headers map[string]string, body []byte) (*upstreamResult, error) {
	target, err := url.Parse(providerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid provider URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String()+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return &upstreamResult{
		statusCode: resp.StatusCode,
		body:       respBody,
		header:     resp.Header,
	}, nil
}

// isRetryable returns true if the error or status code warrants trying the next route.
func isRetryable(err error, statusCode int) bool {
	if err != nil {
		return true
	}
	return statusCode >= 500
}

// rewriteModel replaces the "model" field in a JSON body with the given model name.
func rewriteModel(body []byte, model string) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return body
	}
	modelJSON, err := json.Marshal(model)
	if err != nil {
		return body
	}
	raw["model"] = modelJSON
	out, err := json.Marshal(raw)
	if err != nil {
		return body
	}
	return out
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
		if err := s.enforcer.Check(r.Context(), clientKey, req.Model); err != nil {
			if errors.Is(err, budget.ErrBudgetExceeded) {
				writeJSONError(w, http.StatusTooManyRequests, "token budget exceeded")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "budget check failed")
			return
		}
	}

	// Resolve routes
	routes, err := s.router.Resolve(req.Model)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "no providers available")
		return
	}

	// Fallback loop
	var result *upstreamResult
	for _, route := range routes {
		reqBody := rewriteModel(body, route.Model)
		headers := map[string]string{
			"Authorization": "Bearer " + route.Provider.APIKey,
		}

		res, err := doUpstreamRequest(r.Context(), route.Provider.URL, "/v1/chat/completions", "application/json", headers, reqBody)
		if isRetryable(err, 0) {
			log.Printf("upstream %s failed: %v, trying next", route.Provider.Name, err)
			continue
		}
		if res != nil && isRetryable(nil, res.statusCode) {
			log.Printf("upstream %s returned %d, trying next", route.Provider.Name, res.statusCode)
			result = res
			continue
		}
		result = res
		break
	}

	if result == nil {
		writeJSONError(w, http.StatusBadGateway, "all upstream providers failed")
		return
	}

	// Resolve session
	var sessionID string
	if st, ok := s.tracker.(*tracker.SQLiteTracker); ok {
		explicitSession := r.Header.Get("X-Pario-Session")
		sid, err := st.ResolveSession(r.Context(), clientKey, explicitSession, s.cfg.Session.GapTimeout)
		if err != nil {
			log.Printf("session resolve error: %v", err)
		} else {
			sessionID = sid
		}
	}
	if sessionID != "" {
		w.Header().Set("X-Pario-Session", sessionID)
	}

	// Parse response for usage tracking
	if result.statusCode == http.StatusOK && !req.Stream {
		var chatResp models.ChatCompletionResponse
		if err := json.Unmarshal(result.body, &chatResp); err == nil && chatResp.Usage != nil {
			team, project, env := s.resolveLabels(r, clientKey)
			_ = s.tracker.Record(r.Context(), models.UsageRecord{
				APIKey:           clientKey,
				Model:            chatResp.Model,
				SessionID:        sessionID,
				PromptTokens:     chatResp.Usage.PromptTokens,
				CompletionTokens: chatResp.Usage.CompletionTokens,
				TotalTokens:      chatResp.Usage.TotalTokens,
				Team:             team,
				Project:          project,
				Env:              env,
				CreatedAt:        time.Now().UTC(),
			})

			if s.cache != nil {
				hash := cachepkg.HashPrompt(req.Model, req.Messages)
				_ = s.cache.Put(hash, req.Model, result.body)
			}
		}
	}

	// Forward response headers and body
	for k, vals := range result.header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Pario-Cache", "miss")
	w.WriteHeader(result.statusCode)
	w.Write(result.body)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
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

	var req models.AnthropicRequest
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
		if err := s.enforcer.Check(r.Context(), clientKey, req.Model); err != nil {
			if errors.Is(err, budget.ErrBudgetExceeded) {
				writeJSONError(w, http.StatusTooManyRequests, "token budget exceeded")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "budget check failed")
			return
		}
	}

	// Resolve routes
	routes, err := s.router.Resolve(req.Model)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "no providers available")
		return
	}

	// Fallback loop
	anthropicVersion := r.Header.Get("anthropic-version")
	var result *upstreamResult
	for _, route := range routes {
		reqBody := rewriteModel(body, route.Model)
		headers := map[string]string{
			"x-api-key": route.Provider.APIKey,
		}
		if anthropicVersion != "" {
			headers["anthropic-version"] = anthropicVersion
		}

		res, err := doUpstreamRequest(r.Context(), route.Provider.URL, "/v1/messages", "application/json", headers, reqBody)
		if isRetryable(err, 0) {
			log.Printf("upstream %s failed: %v, trying next", route.Provider.Name, err)
			continue
		}
		if res != nil && isRetryable(nil, res.statusCode) {
			log.Printf("upstream %s returned %d, trying next", route.Provider.Name, res.statusCode)
			result = res
			continue
		}
		result = res
		break
	}

	if result == nil {
		writeJSONError(w, http.StatusBadGateway, "all upstream providers failed")
		return
	}

	// Resolve session
	var sessionID string
	if st, ok := s.tracker.(*tracker.SQLiteTracker); ok {
		explicitSession := r.Header.Get("X-Pario-Session")
		sid, err := st.ResolveSession(r.Context(), clientKey, explicitSession, s.cfg.Session.GapTimeout)
		if err != nil {
			log.Printf("session resolve error: %v", err)
		} else {
			sessionID = sid
		}
	}
	if sessionID != "" {
		w.Header().Set("X-Pario-Session", sessionID)
	}

	// Parse response for usage tracking
	if result.statusCode == http.StatusOK && !req.Stream {
		var anthResp models.AnthropicResponse
		if err := json.Unmarshal(result.body, &anthResp); err == nil && anthResp.Usage != nil {
			usage := anthResp.Usage.ToUsage()
			team, project, env := s.resolveLabels(r, clientKey)
			_ = s.tracker.Record(r.Context(), models.UsageRecord{
				APIKey:           clientKey,
				Model:            anthResp.Model,
				SessionID:        sessionID,
				PromptTokens:     usage.PromptTokens,
				CompletionTokens: usage.CompletionTokens,
				TotalTokens:      usage.TotalTokens,
				Team:             team,
				Project:          project,
				Env:              env,
				CreatedAt:        time.Now().UTC(),
			})

			if s.cache != nil {
				hash := cachepkg.HashPrompt(req.Model, req.Messages)
				_ = s.cache.Put(hash, req.Model, result.body)
			}
		}
	}

	// Forward response headers and body
	for k, vals := range result.header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Pario-Cache", "miss")
	w.WriteHeader(result.statusCode)
	w.Write(result.body)
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

// resolveLabels extracts attribution labels from headers, falling back to config key_labels.
func (s *Server) resolveLabels(r *http.Request, clientKey string) (team, project, env string) {
	team = r.Header.Get("X-Pario-Team")
	project = r.Header.Get("X-Pario-Project")
	env = r.Header.Get("X-Pario-Env")

	if team == "" && project == "" && env == "" {
		if labels, ok := s.cfg.Attribution.KeyLabels[clientKey]; ok {
			team = labels.Team
			project = labels.Project
			env = labels.Env
		}
	}
	return team, project, env
}

func extractAPIKey(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if key := r.Header.Get("x-api-key"); key != "" {
		return key
	}
	return ""
}

func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":{"message":%q,"type":"pario_error","code":%d}}`, message, code)
}
