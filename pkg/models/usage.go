package models

import "time"

// Usage represents token usage from an LLM response.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// UsageRecord tracks per-request token usage.
type UsageRecord struct {
	ID               int64     `json:"id"`
	APIKey           string    `json:"api_key"`
	Model            string    `json:"model"`
	SessionID        string    `json:"session_id,omitempty"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	Team             string    `json:"team,omitempty"`
	Project          string    `json:"project,omitempty"`
	Env              string    `json:"env,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// Session groups related requests into a conversation.
type Session struct {
	ID            string    `json:"id"`
	APIKey        string    `json:"api_key"`
	StartedAt     time.Time `json:"started_at"`
	LastActivity  time.Time `json:"last_activity"`
	RequestCount  int       `json:"request_count"`
	TotalTokens   int       `json:"total_tokens"`
}

// SessionRequest represents a single request within a session, with context growth info.
type SessionRequest struct {
	Seq              int       `json:"seq"`
	CreatedAt        time.Time `json:"created_at"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	ContextGrowth    int       `json:"context_growth"`
}

// UsageSummary aggregates usage across requests.
type UsageSummary struct {
	APIKey           string `json:"api_key"`
	Model            string `json:"model"`
	RequestCount     int    `json:"request_count"`
	TotalPrompt      int    `json:"total_prompt"`
	TotalCompletion  int    `json:"total_completion"`
	TotalTokens      int    `json:"total_tokens"`
}
