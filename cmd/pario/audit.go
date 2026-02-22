package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pario-ai/pario/pkg/audit"
	"github.com/pario-ai/pario/pkg/config"
	"github.com/pario-ai/pario/pkg/models"
	"github.com/spf13/cobra"
)

func newAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Query and manage the prompt/response audit log",
	}

	cmd.AddCommand(
		newAuditSearchCmd(),
		newAuditShowCmd(),
		newAuditStatsCmd(),
		newAuditCleanupCmd(),
	)
	return cmd
}

func newAuditSearchCmd() *cobra.Command {
	var (
		configPath string
		model      string
		since      string
		keyPrefix  string
		session    string
		limit      int
	)

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search audit log entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			l, cleanup, err := openAuditLogger(configPath)
			if err != nil {
				return err
			}
			defer cleanup()

			opts := models.AuditQueryOpts{
				Model:        model,
				APIKeyPrefix: keyPrefix,
				SessionID:    session,
				Limit:        limit,
			}
			if since != "" {
				t, err := time.Parse("2006-01-02", since)
				if err != nil {
					return fmt.Errorf("invalid --since date (use YYYY-MM-DD): %w", err)
				}
				opts.Since = t
			}

			entries, err := l.Query(context.Background(), opts)
			if err != nil {
				return err
			}
			fmt.Print(formatAuditEntries(entries))
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to pario config file")
	cmd.Flags().StringVar(&model, "model", "", "filter by model")
	cmd.Flags().StringVar(&since, "since", "", "start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&keyPrefix, "key-prefix", "", "filter by API key prefix")
	cmd.Flags().StringVar(&session, "session", "", "filter by session ID")
	cmd.Flags().IntVar(&limit, "limit", 50, "max entries to return")

	return cmd
}

func newAuditShowCmd() *cobra.Command {
	var (
		configPath string
		requestID  string
	)

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a single audit entry by request ID",
		RunE: func(cmd *cobra.Command, args []string) error {
			if requestID == "" {
				return fmt.Errorf("--request-id is required")
			}

			l, cleanup, err := openAuditLogger(configPath)
			if err != nil {
				return err
			}
			defer cleanup()

			entries, err := l.Query(context.Background(), models.AuditQueryOpts{
				RequestID: requestID,
				Limit:     1,
			})
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No entry found for that request ID.")
				return nil
			}

			e := entries[0]
			fmt.Printf("Request ID:    %s\n", e.RequestID)
			fmt.Printf("Model:         %s\n", e.Model)
			fmt.Printf("Provider:      %s\n", e.Provider)
			fmt.Printf("API Key:       %s...\n", e.APIKeyPrefix)
			fmt.Printf("Session:       %s\n", e.SessionID)
			fmt.Printf("Status:        %d\n", e.StatusCode)
			fmt.Printf("Latency:       %dms\n", e.LatencyMs)
			fmt.Printf("Tokens:        %d prompt / %d completion / %d total\n",
				e.PromptTokens, e.CompletionTokens, e.TotalTokens)
			fmt.Printf("Time:          %s\n", e.CreatedAt.Format(time.RFC3339))
			if e.RequestBody != "" {
				fmt.Printf("\n--- Request Body ---\n%s\n", e.RequestBody)
			}
			if e.ResponseBody != "" {
				fmt.Printf("\n--- Response Body ---\n%s\n", e.ResponseBody)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to pario config file")
	cmd.Flags().StringVar(&requestID, "request-id", "", "request ID to show")

	return cmd
}

func newAuditStatsCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show audit log statistics by model and day",
		RunE: func(cmd *cobra.Command, args []string) error {
			l, cleanup, err := openAuditLogger(configPath)
			if err != nil {
				return err
			}
			defer cleanup()

			stats, err := l.Stats(context.Background())
			if err != nil {
				return err
			}
			fmt.Print(formatAuditStats(stats))
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to pario config file")
	return cmd
}

func newAuditCleanupCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Delete audit entries older than the retention period",
		RunE: func(cmd *cobra.Command, args []string) error {
			l, cleanup, err := openAuditLogger(configPath)
			if err != nil {
				return err
			}
			defer cleanup()

			deleted, err := l.Cleanup(context.Background())
			if err != nil {
				return err
			}
			fmt.Printf("Deleted %d audit entries.\n", deleted)
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to pario config file")
	return cmd
}

func openAuditLogger(configPath string) (*audit.Logger, func(), error) {
	cfg := config.Default()
	if configPath != "" {
		var err error
		cfg, err = config.Load(configPath)
		if err != nil {
			return nil, nil, err
		}
	}

	l, err := audit.New(cfg.Audit)
	if err != nil {
		return nil, nil, fmt.Errorf("open audit db: %w", err)
	}
	return l, func() { _ = l.Close() }, nil
}

func formatAuditEntries(entries []models.AuditEntry) string {
	if len(entries) == 0 {
		return "No audit entries found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-38s %-20s %-10s %6s %8s %12s %-20s\n",
		"REQUEST ID", "MODEL", "PROVIDER", "STATUS", "LATENCY", "TOKENS", "TIME")
	b.WriteString(strings.Repeat("-", 118) + "\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "%-38s %-20s %-10s %6d %6dms %12d %-20s\n",
			e.RequestID, e.Model, e.Provider, e.StatusCode,
			e.LatencyMs, e.TotalTokens,
			e.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	return b.String()
}

func formatAuditStats(stats []models.AuditStat) string {
	if len(stats) == 0 {
		return "No audit stats found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-25s %-12s %8s\n", "MODEL", "DAY", "COUNT")
	b.WriteString(strings.Repeat("-", 48) + "\n")
	for _, s := range stats {
		fmt.Fprintf(&b, "%-25s %-12s %8d\n", s.Model, s.Day, s.Count)
	}
	return b.String()
}
