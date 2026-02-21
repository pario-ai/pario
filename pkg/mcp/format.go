package mcp

import (
	"fmt"
	"strings"

	"github.com/pario-ai/pario/pkg/models"
)

// formatSummary formats usage summaries as a text table.
func formatSummary(rows []models.UsageSummary) string {
	if len(rows) == 0 {
		return "No usage data found."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-20s %-25s %8s %10s %10s %10s\n",
		"API Key", "Model", "Requests", "Prompt", "Completion", "Total")
	b.WriteString(strings.Repeat("-", 87) + "\n")
	for _, r := range rows {
		key := r.APIKey
		if len(key) > 20 {
			key = key[:8] + "..." + key[len(key)-8:]
		}
		fmt.Fprintf(&b, "%-20s %-25s %8d %10d %10d %10d\n",
			key, r.Model, r.RequestCount, r.TotalPrompt, r.TotalCompletion, r.TotalTokens)
	}
	return b.String()
}

// formatSessions formats sessions as a text table.
func formatSessions(sessions []models.Session) string {
	if len(sessions) == 0 {
		return "No sessions found."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-38s %-20s %-20s %-20s %8s %10s\n",
		"Session ID", "API Key", "Started", "Last Activity", "Requests", "Tokens")
	b.WriteString(strings.Repeat("-", 120) + "\n")
	for _, s := range sessions {
		key := s.APIKey
		if len(key) > 20 {
			key = key[:8] + "..." + key[len(key)-8:]
		}
		fmt.Fprintf(&b,"%-38s %-20s %-20s %-20s %8d %10d\n",
			s.ID, key,
			s.StartedAt.Format("2006-01-02 15:04:05"),
			s.LastActivity.Format("2006-01-02 15:04:05"),
			s.RequestCount, s.TotalTokens)
	}
	return b.String()
}

// formatSessionRequests formats session requests as a text table.
func formatSessionRequests(reqs []models.SessionRequest) string {
	if len(reqs) == 0 {
		return "No requests found for this session."
	}
	var b strings.Builder
	fmt.Fprintf(&b,"%4s  %-20s %10s %10s %10s %10s\n",
		"Seq", "Time", "Prompt", "Completion", "Total", "Ctx Growth")
	b.WriteString(strings.Repeat("-", 70) + "\n")
	for _, r := range reqs {
		fmt.Fprintf(&b,"%4d  %-20s %10d %10d %10d %+10d\n",
			r.Seq,
			r.CreatedAt.Format("2006-01-02 15:04:05"),
			r.PromptTokens, r.CompletionTokens, r.TotalTokens, r.ContextGrowth)
	}
	return b.String()
}

// formatBudgetStatus formats budget statuses as a text table.
func formatBudgetStatus(statuses []models.BudgetStatus) string {
	if len(statuses) == 0 {
		return "No budget policies found."
	}
	var b strings.Builder
	fmt.Fprintf(&b,"%-20s %-8s %12s %12s %12s %6s\n",
		"API Key", "Period", "Max Tokens", "Used", "Remaining", "Usage%")
	b.WriteString(strings.Repeat("-", 74) + "\n")
	for _, s := range statuses {
		key := s.Policy.APIKey
		if len(key) > 20 {
			key = key[:8] + "..." + key[len(key)-8:]
		}
		pct := float64(0)
		if s.Policy.MaxTokens > 0 {
			pct = float64(s.Used) / float64(s.Policy.MaxTokens) * 100
		}
		fmt.Fprintf(&b,"%-20s %-8s %12d %12d %12d %5.1f%%\n",
			key, s.Policy.Period, s.Policy.MaxTokens, s.Used, s.Remaining, pct)
	}
	return b.String()
}

// formatCacheStats formats cache stats as text.
func formatCacheStats(stats models.CacheStats) string {
	total := stats.Hits + stats.Misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(stats.Hits) / float64(total) * 100
	}
	return fmt.Sprintf("Cache Statistics\n"+
		"  Entries:  %d\n"+
		"  Hits:     %d\n"+
		"  Misses:   %d\n"+
		"  Hit Rate: %.1f%%\n",
		stats.Entries, stats.Hits, stats.Misses, hitRate)
}
