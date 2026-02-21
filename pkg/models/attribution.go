package models

// CostLabel holds attribution labels for a request.
type CostLabel struct {
	Team    string `json:"team,omitempty" yaml:"team"`
	Project string `json:"project,omitempty" yaml:"project"`
	Env     string `json:"env,omitempty" yaml:"env"`
}

// ModelPricing defines per-1K token costs for a model.
type ModelPricing struct {
	Model          string  `json:"model" yaml:"model"`
	PromptCost     float64 `json:"prompt_cost_per_1k" yaml:"prompt_cost_per_1k"`
	CompletionCost float64 `json:"completion_cost_per_1k" yaml:"completion_cost_per_1k"`
}

// CostReport is an aggregated cost row grouped by team, project, and model.
type CostReport struct {
	Team             string  `json:"team"`
	Project          string  `json:"project"`
	Model            string  `json:"model"`
	RequestCount     int     `json:"request_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	EstimatedCost    float64 `json:"estimated_cost"`
}
