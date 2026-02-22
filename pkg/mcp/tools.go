package mcp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pario-ai/pario/pkg/models"
)

// Tool argument structs.

type apiKeyArgs struct {
	APIKey string `json:"api_key"`
}

type sessionDetailArgs struct {
	SessionID string `json:"session_id"`
}

// toolHandler is a function that handles a tool call.
type toolHandler func(ctx context.Context, s *Server, args json.RawMessage) ToolCallResult

// toolHandlers maps tool names to their handlers.
var toolHandlers = map[string]toolHandler{
	"pario_stats":          handleStats,
	"pario_sessions":       handleSessions,
	"pario_session_detail": handleSessionDetail,
	"pario_budget":         handleBudget,
	"pario_cache_stats":    handleCacheStats,
	"pario_cost_report":    handleCostReport,
	"pario_audit_search":   handleAuditSearch,
}

// allTools is the list of tool definitions exposed via tools/list.
var allTools = []ToolDefinition{
	{
		Name:        "pario_stats",
		Description: "Show aggregated token usage statistics, optionally filtered by API key.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"api_key": map[string]any{
					"type":        "string",
					"description": "Filter by API key (optional, omit for all keys)",
				},
			},
		},
	},
	{
		Name:        "pario_sessions",
		Description: "List all tracked sessions, optionally filtered by API key.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"api_key": map[string]any{
					"type":        "string",
					"description": "Filter by API key (optional, omit for all keys)",
				},
			},
		},
	},
	{
		Name:        "pario_session_detail",
		Description: "Show per-request detail for a specific session, including context growth.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"session_id"},
			"properties": map[string]any{
				"session_id": map[string]any{
					"type":        "string",
					"description": "The session ID to inspect",
				},
			},
		},
	},
	{
		Name:        "pario_budget",
		Description: "Show budget status (usage vs limits) for all configured policies, optionally filtered by API key.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"api_key": map[string]any{
					"type":        "string",
					"description": "Filter by API key (optional, omit for all keys)",
				},
			},
		},
	},
	{
		Name:        "pario_cost_report",
		Description: "Show estimated costs grouped by team, project, and model with optional filtering.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"team": map[string]any{
					"type":        "string",
					"description": "Filter by team (optional)",
				},
				"project": map[string]any{
					"type":        "string",
					"description": "Filter by project (optional)",
				},
				"since": map[string]any{
					"type":        "string",
					"description": "Start date in YYYY-MM-DD format (optional, defaults to start of month)",
				},
			},
		},
	},
	{
		Name:        "pario_cache_stats",
		Description: "Show prompt cache statistics (entries, hits, misses, hit rate).",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	{
		Name:        "pario_audit_search",
		Description: "Search the prompt/response audit log with optional filters.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"model": map[string]any{
					"type":        "string",
					"description": "Filter by model (optional)",
				},
				"since": map[string]any{
					"type":        "string",
					"description": "Start date in YYYY-MM-DD format (optional)",
				},
				"key_prefix": map[string]any{
					"type":        "string",
					"description": "Filter by API key prefix (optional)",
				},
				"session_id": map[string]any{
					"type":        "string",
					"description": "Filter by session ID (optional)",
				},
			},
		},
	},
}

func textResult(text string) ToolCallResult {
	return ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
	}
}

func errorResult(text string) ToolCallResult {
	return ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
		IsError: true,
	}
}

func handleStats(ctx context.Context, s *Server, rawArgs json.RawMessage) ToolCallResult {
	var args apiKeyArgs
	if len(rawArgs) > 0 {
		_ = json.Unmarshal(rawArgs, &args)
	}
	rows, err := s.tracker.Summary(ctx, args.APIKey)
	if err != nil {
		return errorResult("Error fetching stats: " + err.Error())
	}
	return textResult(formatSummary(rows))
}

func handleSessions(ctx context.Context, s *Server, rawArgs json.RawMessage) ToolCallResult {
	var args apiKeyArgs
	if len(rawArgs) > 0 {
		_ = json.Unmarshal(rawArgs, &args)
	}
	sessions, err := s.tracker.ListSessions(ctx, args.APIKey)
	if err != nil {
		return errorResult("Error fetching sessions: " + err.Error())
	}
	return textResult(formatSessions(sessions))
}

func handleSessionDetail(ctx context.Context, s *Server, rawArgs json.RawMessage) ToolCallResult {
	var args sessionDetailArgs
	if len(rawArgs) > 0 {
		_ = json.Unmarshal(rawArgs, &args)
	}
	if args.SessionID == "" {
		return errorResult("session_id is required")
	}
	reqs, err := s.tracker.SessionRequests(ctx, args.SessionID)
	if err != nil {
		return errorResult("Error fetching session detail: " + err.Error())
	}
	return textResult(formatSessionRequests(reqs))
}

func handleBudget(ctx context.Context, s *Server, rawArgs json.RawMessage) ToolCallResult {
	if s.enforcer == nil {
		return textResult("Budget enforcement is not configured.")
	}
	var args apiKeyArgs
	if len(rawArgs) > 0 {
		_ = json.Unmarshal(rawArgs, &args)
	}
	statuses, err := s.enforcer.Status(ctx, args.APIKey)
	if err != nil {
		return errorResult("Error fetching budget status: " + err.Error())
	}
	return textResult(formatBudgetStatus(statuses))
}

type costReportArgs struct {
	Team    string `json:"team"`
	Project string `json:"project"`
	Since   string `json:"since"`
}

func handleCostReport(ctx context.Context, s *Server, rawArgs json.RawMessage) ToolCallResult {
	var args costReportArgs
	if len(rawArgs) > 0 {
		_ = json.Unmarshal(rawArgs, &args)
	}

	since := beginningOfMonth()
	if args.Since != "" {
		t, err := time.Parse("2006-01-02", args.Since)
		if err != nil {
			return errorResult("Invalid since date (use YYYY-MM-DD): " + err.Error())
		}
		since = t
	}

	reports, err := s.tracker.CostReport(ctx, since, args.Team, args.Project)
	if err != nil {
		return errorResult("Error fetching cost report: " + err.Error())
	}

	pricingMap := make(map[string]struct{ prompt, completion float64 }, len(s.pricing))
	for _, p := range s.pricing {
		pricingMap[p.Model] = struct{ prompt, completion float64 }{p.PromptCost, p.CompletionCost}
	}
	for i := range reports {
		if p, ok := pricingMap[reports[i].Model]; ok {
			reports[i].EstimatedCost = (float64(reports[i].PromptTokens)/1000)*p.prompt +
				(float64(reports[i].CompletionTokens)/1000)*p.completion
		}
	}

	return textResult(formatCostReport(reports))
}

func beginningOfMonth() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

type auditSearchArgs struct {
	Model     string `json:"model"`
	Since     string `json:"since"`
	KeyPrefix string `json:"key_prefix"`
	SessionID string `json:"session_id"`
}

func handleAuditSearch(ctx context.Context, s *Server, rawArgs json.RawMessage) ToolCallResult {
	if s.auditor == nil {
		return textResult("Audit logging is not configured.")
	}
	var args auditSearchArgs
	if len(rawArgs) > 0 {
		_ = json.Unmarshal(rawArgs, &args)
	}

	opts := models.AuditQueryOpts{
		Model:        args.Model,
		APIKeyPrefix: args.KeyPrefix,
		SessionID:    args.SessionID,
		Limit:        50,
	}
	if args.Since != "" {
		t, err := time.Parse("2006-01-02", args.Since)
		if err != nil {
			return errorResult("Invalid since date (use YYYY-MM-DD): " + err.Error())
		}
		opts.Since = t
	}

	entries, err := s.auditor.Query(ctx, opts)
	if err != nil {
		return errorResult("Error searching audit log: " + err.Error())
	}
	return textResult(formatAuditEntries(entries))
}

func handleCacheStats(_ context.Context, s *Server, _ json.RawMessage) ToolCallResult {
	if s.cache == nil {
		return textResult("Cache is not configured.")
	}
	stats, err := s.cache.Stats()
	if err != nil {
		return errorResult("Error fetching cache stats: " + err.Error())
	}
	return textResult(formatCacheStats(stats))
}
