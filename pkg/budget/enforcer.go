package budget

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pario-ai/pario/pkg/models"
	"github.com/pario-ai/pario/pkg/tracker"
)

// ErrBudgetExceeded is returned when a request exceeds the budget.
var ErrBudgetExceeded = errors.New("budget exceeded")

// Enforcer checks token usage against budget policies.
type Enforcer struct {
	policies []models.BudgetPolicy
	tracker  tracker.Tracker
}

// New creates an Enforcer with the given policies and tracker.
func New(policies []models.BudgetPolicy, t tracker.Tracker) *Enforcer {
	return &Enforcer{policies: policies, tracker: t}
}

// Check returns ErrBudgetExceeded if the API key has exceeded any applicable policy.
func (e *Enforcer) Check(ctx context.Context, apiKey, model string) error {
	for _, p := range e.applicablePolicies(apiKey, model) {
		since := periodStart(p.Period)
		var used int64
		var err error
		if p.Model != "" {
			used, err = e.tracker.TotalByKeyAndModel(ctx, apiKey, p.Model, since)
		} else {
			used, err = e.tracker.TotalByKey(ctx, apiKey, since)
		}
		if err != nil {
			return fmt.Errorf("budget check: %w", err)
		}
		if used >= p.MaxTokens {
			return ErrBudgetExceeded
		}
	}
	return nil
}

// Status returns the budget status for an API key across all applicable policies.
func (e *Enforcer) Status(ctx context.Context, apiKey string) ([]models.BudgetStatus, error) {
	policies := e.policiesForKey(apiKey)
	statuses := make([]models.BudgetStatus, 0, len(policies))

	for _, p := range policies {
		since := periodStart(p.Period)
		var used int64
		var err error
		if p.Model != "" {
			used, err = e.tracker.TotalByKeyAndModel(ctx, apiKey, p.Model, since)
		} else {
			used, err = e.tracker.TotalByKey(ctx, apiKey, since)
		}
		if err != nil {
			return nil, fmt.Errorf("budget status: %w", err)
		}
		remaining := p.MaxTokens - used
		if remaining < 0 {
			remaining = 0
		}
		statuses = append(statuses, models.BudgetStatus{
			Policy:    p,
			Used:      used,
			Remaining: remaining,
		})
	}
	return statuses, nil
}

// policiesForKey returns all policies matching an API key (ignoring model filter).
func (e *Enforcer) policiesForKey(apiKey string) []models.BudgetPolicy {
	var result []models.BudgetPolicy
	for _, p := range e.policies {
		if p.APIKey == "*" || p.APIKey == apiKey {
			result = append(result, p)
		}
	}
	return result
}

func (e *Enforcer) applicablePolicies(apiKey, model string) []models.BudgetPolicy {
	var result []models.BudgetPolicy
	for _, p := range e.policies {
		if p.APIKey == "*" || p.APIKey == apiKey {
			if p.Model == "" || p.Model == model {
				result = append(result, p)
			}
		}
	}
	return result
}

func periodStart(period models.BudgetPeriod) time.Time {
	now := time.Now().UTC()
	switch period {
	case models.BudgetMonthly:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	default: // daily
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
}
