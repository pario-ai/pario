package proxy

import (
	"bufio"
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

	"github.com/pario-ai/pario/pkg/audit"
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
	auditor  *audit.Logger
	router   *router.Router
	mux      *http.ServeMux
}

// New creates a proxy Server wired with all dependencies.
func New(cfg *config.Config, t tracker.Tracker, c *cachepkg.Cache, e *budget.Enforcer, a *audit.Logger) *Server {
	s := &Server{
		cfg:      cfg,
		tracker:  t,
		cache:    c,
		enforcer: e,
		auditor:  a,
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

// resolveSessionID resolves a session ID for the given client key.
func (s *Server) resolveSessionID(r *http.Request, clientKey string) string {
	if st, ok := s.tracker.(*tracker.SQLiteTracker); ok {
		explicitSession := r.Header.Get("X-Pario-Session")
		sid, err := st.ResolveSession(r.Context(), clientKey, explicitSession, s.cfg.Session.GapTimeout)
		if err != nil {
			log.Printf("session resolve error: %v", err)
			return ""
		}
		return sid
	}
	return ""
}

// doUpstreamStreamRequest sends a request to an upstream provider and returns the raw response.
// The caller owns resp.Body and must close it.
func doUpstreamStreamRequest(ctx context.Context, providerURL, path, contentType string, headers map[string]string, body []byte) (*http.Response, error) {
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

	return http.DefaultClient.Do(req)
}

// streamResult holds accumulated data from an SSE stream.
type streamResult struct {
	usage *models.Usage
	model string
	body  strings.Builder
}

// streamSSEResponse relays an SSE stream from resp to w, extracting usage data.
func streamSSEResponse(w http.ResponseWriter, resp *http.Response, format string) (*streamResult, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("response writer does not support flushing")
	}

	// Copy response headers
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	result := &streamResult{}
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()
		result.body.WriteString(line)
		result.body.WriteString("\n")

		// Write line to client
		fmt.Fprintf(w, "%s\n", line)

		// Flush on blank lines (SSE event boundary)
		if line == "" {
			flusher.Flush()
		}

		// Parse data lines for usage extraction
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}

		switch format {
		case "openai":
			var chunk models.ChatCompletionChunk
			if err := json.Unmarshal([]byte(data), &chunk); err == nil {
				if chunk.Model != "" {
					result.model = chunk.Model
				}
				if chunk.Usage != nil {
					result.usage = chunk.Usage
				}
			}
		case "anthropic":
			var evt models.AnthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &evt); err == nil {
				switch evt.Type {
				case "message_start":
					// Extract model and input tokens from the message object
					var msg struct {
						Model string               `json:"model"`
						Usage *models.AnthropicUsage `json:"usage,omitempty"`
					}
					if err := json.Unmarshal(evt.Message, &msg); err == nil {
						if msg.Model != "" {
							result.model = msg.Model
						}
						if msg.Usage != nil {
							result.usage = msg.Usage.ToUsage()
						}
					}
				case "message_delta":
					// Extract output tokens from delta usage
					if evt.Usage != nil {
						if result.usage == nil {
							result.usage = &models.Usage{}
						}
						result.usage.CompletionTokens = evt.Usage.OutputTokens
						result.usage.TotalTokens = result.usage.PromptTokens + evt.Usage.OutputTokens
					}
				}
			}
		}
	}

	// Final flush
	flusher.Flush()

	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("reading stream: %w", err)
	}
	return result, nil
}

// handleStreamingOpenAI handles streaming OpenAI chat completion requests.
func (s *Server) handleStreamingOpenAI(w http.ResponseWriter, r *http.Request, clientKey string, body []byte, routes []router.Route, reqStart time.Time) {
	var resp *http.Response
	var usedRoute router.Route
	for _, route := range routes {
		reqBody := rewriteModel(body, route.Model)
		headers := map[string]string{
			"Authorization": "Bearer " + route.Provider.APIKey,
		}

		res, err := doUpstreamStreamRequest(r.Context(), route.Provider.URL, "/v1/chat/completions", "application/json", headers, reqBody)
		if err != nil {
			log.Printf("upstream %s failed: %v, trying next", route.Provider.Name, err)
			continue
		}
		if res.StatusCode >= 500 {
			res.Body.Close()
			log.Printf("upstream %s returned %d, trying next", route.Provider.Name, res.StatusCode)
			continue
		}
		resp = res
		usedRoute = route
		break
	}

	if resp == nil {
		writeJSONError(w, http.StatusBadGateway, "all upstream providers failed")
		return
	}
	defer resp.Body.Close()
	_ = usedRoute // used for future provider attribution

	sessionID := s.resolveSessionID(r, clientKey)
	if sessionID != "" {
		w.Header().Set("X-Pario-Session", sessionID)
	}

	result, err := streamSSEResponse(w, resp, "openai")
	if err != nil {
		log.Printf("streaming error: %v", err)
	}

	// Record usage
	if result != nil && result.usage != nil {
		team, project, env := s.resolveLabels(r, clientKey)
		modelName := result.model
		_ = s.tracker.Record(r.Context(), models.UsageRecord{
			APIKey:           clientKey,
			Model:            modelName,
			SessionID:        sessionID,
			PromptTokens:     result.usage.PromptTokens,
			CompletionTokens: result.usage.CompletionTokens,
			TotalTokens:      result.usage.TotalTokens,
			Team:             team,
			Project:          project,
			Env:              env,
			CreatedAt:        time.Now().UTC(),
		})
	}

	// Audit log
	if s.auditor != nil && result != nil {
		latency := time.Since(reqStart).Milliseconds()
		keyHash, keyPrefix := audit.HashAPIKey(clientKey)
		respBody := result.body.String()
		if len(respBody) > 8192 {
			respBody = respBody[:8192]
		}
		entry := models.AuditEntry{
			RequestID:    r.Header.Get("X-Request-ID"),
			APIKeyHash:   keyHash,
			APIKeyPrefix: keyPrefix,
			Model:        result.model,
			SessionID:    sessionID,
			Provider:     "openai",
			RequestBody:  string(body),
			ResponseBody: respBody,
			StatusCode:   resp.StatusCode,
			LatencyMs:    latency,
			CreatedAt:    time.Now().UTC(),
		}
		if result.usage != nil {
			entry.PromptTokens = result.usage.PromptTokens
			entry.CompletionTokens = result.usage.CompletionTokens
			entry.TotalTokens = result.usage.TotalTokens
		}
		go func() {
			if err := s.auditor.Log(context.Background(), entry); err != nil {
				log.Printf("audit log error: %v", err)
			}
		}()
	}
}

// handleStreamingAnthropic handles streaming Anthropic message requests.
func (s *Server) handleStreamingAnthropic(w http.ResponseWriter, r *http.Request, clientKey string, body []byte, routes []router.Route, reqStart time.Time) {
	anthropicVersion := r.Header.Get("anthropic-version")
	var resp *http.Response
	var usedRoute router.Route
	for _, route := range routes {
		reqBody := rewriteModel(body, route.Model)
		headers := map[string]string{
			"x-api-key": route.Provider.APIKey,
		}
		if anthropicVersion != "" {
			headers["anthropic-version"] = anthropicVersion
		}

		res, err := doUpstreamStreamRequest(r.Context(), route.Provider.URL, "/v1/messages", "application/json", headers, reqBody)
		if err != nil {
			log.Printf("upstream %s failed: %v, trying next", route.Provider.Name, err)
			continue
		}
		if res.StatusCode >= 500 {
			res.Body.Close()
			log.Printf("upstream %s returned %d, trying next", route.Provider.Name, res.StatusCode)
			continue
		}
		resp = res
		usedRoute = route
		break
	}

	if resp == nil {
		writeJSONError(w, http.StatusBadGateway, "all upstream providers failed")
		return
	}
	defer resp.Body.Close()
	_ = usedRoute

	sessionID := s.resolveSessionID(r, clientKey)
	if sessionID != "" {
		w.Header().Set("X-Pario-Session", sessionID)
	}

	result, err := streamSSEResponse(w, resp, "anthropic")
	if err != nil {
		log.Printf("streaming error: %v", err)
	}

	// Record usage
	if result != nil && result.usage != nil {
		team, project, env := s.resolveLabels(r, clientKey)
		_ = s.tracker.Record(r.Context(), models.UsageRecord{
			APIKey:           clientKey,
			Model:            result.model,
			SessionID:        sessionID,
			PromptTokens:     result.usage.PromptTokens,
			CompletionTokens: result.usage.CompletionTokens,
			TotalTokens:      result.usage.TotalTokens,
			Team:             team,
			Project:          project,
			Env:              env,
			CreatedAt:        time.Now().UTC(),
		})
	}

	// Audit log
	if s.auditor != nil && result != nil {
		latency := time.Since(reqStart).Milliseconds()
		keyHash, keyPrefix := audit.HashAPIKey(clientKey)
		respBody := result.body.String()
		if len(respBody) > 8192 {
			respBody = respBody[:8192]
		}
		entry := models.AuditEntry{
			RequestID:    r.Header.Get("X-Request-ID"),
			APIKeyHash:   keyHash,
			APIKeyPrefix: keyPrefix,
			Model:        result.model,
			SessionID:    sessionID,
			Provider:     "anthropic",
			RequestBody:  string(body),
			ResponseBody: respBody,
			StatusCode:   resp.StatusCode,
			LatencyMs:    latency,
			CreatedAt:    time.Now().UTC(),
		}
		if result.usage != nil {
			entry.PromptTokens = result.usage.PromptTokens
			entry.CompletionTokens = result.usage.CompletionTokens
			entry.TotalTokens = result.usage.TotalTokens
		}
		go func() {
			if err := s.auditor.Log(context.Background(), entry); err != nil {
				log.Printf("audit log error: %v", err)
			}
		}()
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

	reqStart := time.Now()

	// Streaming branch
	if req.Stream {
		s.handleStreamingOpenAI(w, r, clientKey, body, routes, reqStart)
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
	sessionID := s.resolveSessionID(r, clientKey)
	if sessionID != "" {
		w.Header().Set("X-Pario-Session", sessionID)
	}

	// Parse response for usage tracking
	var usage *models.Usage
	if result.statusCode == http.StatusOK {
		var chatResp models.ChatCompletionResponse
		if err := json.Unmarshal(result.body, &chatResp); err == nil && chatResp.Usage != nil {
			usage = chatResp.Usage
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

	// Audit log
	if s.auditor != nil {
		latency := time.Since(reqStart).Milliseconds()
		keyHash, keyPrefix := audit.HashAPIKey(clientKey)
		entry := models.AuditEntry{
			RequestID:    r.Header.Get("X-Request-ID"),
			APIKeyHash:   keyHash,
			APIKeyPrefix: keyPrefix,
			Model:        req.Model,
			SessionID:    sessionID,
			Provider:     "openai",
			RequestBody:  string(body),
			ResponseBody: string(result.body),
			StatusCode:   result.statusCode,
			LatencyMs:    latency,
			CreatedAt:    time.Now().UTC(),
		}
		if usage != nil {
			entry.PromptTokens = usage.PromptTokens
			entry.CompletionTokens = usage.CompletionTokens
			entry.TotalTokens = usage.TotalTokens
		}
		go func() {
			if err := s.auditor.Log(context.Background(), entry); err != nil {
				log.Printf("audit log error: %v", err)
			}
		}()
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

	reqStart := time.Now()

	// Streaming branch
	if req.Stream {
		s.handleStreamingAnthropic(w, r, clientKey, body, routes, reqStart)
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
	sessionID := s.resolveSessionID(r, clientKey)
	if sessionID != "" {
		w.Header().Set("X-Pario-Session", sessionID)
	}

	// Parse response for usage tracking
	var usage *models.Usage
	if result.statusCode == http.StatusOK {
		var anthResp models.AnthropicResponse
		if err := json.Unmarshal(result.body, &anthResp); err == nil && anthResp.Usage != nil {
			usage = anthResp.Usage.ToUsage()
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

	// Audit log
	if s.auditor != nil {
		latency := time.Since(reqStart).Milliseconds()
		keyHash, keyPrefix := audit.HashAPIKey(clientKey)
		entry := models.AuditEntry{
			RequestID:    r.Header.Get("X-Request-ID"),
			APIKeyHash:   keyHash,
			APIKeyPrefix: keyPrefix,
			Model:        req.Model,
			SessionID:    sessionID,
			Provider:     "anthropic",
			RequestBody:  string(body),
			ResponseBody: string(result.body),
			StatusCode:   result.statusCode,
			LatencyMs:    latency,
			CreatedAt:    time.Now().UTC(),
		}
		if usage != nil {
			entry.PromptTokens = usage.PromptTokens
			entry.CompletionTokens = usage.CompletionTokens
			entry.TotalTokens = usage.TotalTokens
		}
		go func() {
			if err := s.auditor.Log(context.Background(), entry); err != nil {
				log.Printf("audit log error: %v", err)
			}
		}()
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
