package models

import "time"

// AuditEntry represents a single audited LLM request/response pair.
type AuditEntry struct {
	RequestID    string    `json:"request_id"`
	APIKeyHash   string    `json:"api_key_hash"`
	APIKeyPrefix string    `json:"api_key_prefix"`
	Model        string    `json:"model"`
	SessionID    string    `json:"session_id"`
	Provider     string    `json:"provider"`
	RequestBody  string    `json:"request_body,omitempty"`
	ResponseBody string    `json:"response_body,omitempty"`
	RequestHeaders map[string]string `json:"request_headers,omitempty"`
	StatusCode     int       `json:"status_code"`
	PromptTokens   int       `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens    int       `json:"total_tokens"`
	LatencyMs      int64     `json:"latency_ms"`
	CreatedAt      time.Time `json:"created_at"`
}

// AuditConfig controls the audit logging subsystem.
type AuditConfig struct {
	Enabled       bool     `yaml:"enabled"`
	DBPath        string   `yaml:"db_path"`
	RetentionDays int      `yaml:"retention_days"`
	RedactKeys    bool     `yaml:"redact_keys"`
	Include       []string `yaml:"include"`       // "prompts", "responses", "metadata"
	ExcludeModels []string `yaml:"exclude_models"`
	MaxBodySize   int      `yaml:"max_body_size"` // bytes
}

// AuditQueryOpts specifies filters for querying audit entries.
type AuditQueryOpts struct {
	Model        string
	Since        time.Time
	APIKeyPrefix string
	SessionID    string
	RequestID    string
	Limit        int
}

// AuditStat holds aggregate audit counts for a model/day combination.
type AuditStat struct {
	Model string
	Day   string
	Count int
}
