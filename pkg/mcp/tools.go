package mcp

import (
	"context"
	"encoding/json"
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
		Name:        "pario_cache_stats",
		Description: "Show prompt cache statistics (entries, hits, misses, hit rate).",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
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
