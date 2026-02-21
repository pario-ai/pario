package models

// BudgetPeriod defines the time window for a budget policy.
type BudgetPeriod string

const (
	BudgetDaily   BudgetPeriod = "daily"
	BudgetMonthly BudgetPeriod = "monthly"
)

// BudgetPolicy defines max tokens per API key per period.
type BudgetPolicy struct {
	APIKey    string       `json:"api_key" yaml:"api_key"`
	Model     string       `json:"model,omitempty" yaml:"model,omitempty"`
	MaxTokens int64        `json:"max_tokens" yaml:"max_tokens"`
	Period    BudgetPeriod `json:"period" yaml:"period"`
}

// BudgetStatus shows current usage against a policy.
type BudgetStatus struct {
	Policy    BudgetPolicy `json:"policy"`
	Used      int64        `json:"used"`
	Remaining int64        `json:"remaining"`
}
