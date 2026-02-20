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
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	CreatedAt        time.Time `json:"created_at"`
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
