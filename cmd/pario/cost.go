package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pario-ai/pario/pkg/config"
	"github.com/pario-ai/pario/pkg/models"
	"github.com/pario-ai/pario/pkg/tracker"
	"github.com/spf13/cobra"
)

func newCostCmd() *cobra.Command {
	var (
		configPath string
		team       string
		project    string
		since      string
	)

	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Show estimated costs by team, project, and model",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Default()
			if configPath != "" {
				var err error
				cfg, err = config.Load(configPath)
				if err != nil {
					return err
				}
			}

			tr, err := tracker.New(cfg.DBPath)
			if err != nil {
				return err
			}
			defer func() { _ = tr.Close() }()

			sinceTime := beginningOfMonth()
			if since != "" {
				t, err := time.Parse("2006-01-02", since)
				if err != nil {
					return fmt.Errorf("invalid --since date (use YYYY-MM-DD): %w", err)
				}
				sinceTime = t
			}

			reports, err := tr.CostReport(context.Background(), sinceTime, team, project)
			if err != nil {
				return err
			}

			pricingMap := buildPricingMap(cfg.Attribution.Pricing)
			applyCosts(reports, pricingMap)

			fmt.Print(formatCostTable(reports))
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to pario config file")
	cmd.Flags().StringVar(&team, "team", "", "filter by team")
	cmd.Flags().StringVar(&project, "project", "", "filter by project")
	cmd.Flags().StringVar(&since, "since", "", "start date (YYYY-MM-DD, default: start of month)")

	return cmd
}

func beginningOfMonth() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func buildPricingMap(pricing []models.ModelPricing) map[string]models.ModelPricing {
	m := make(map[string]models.ModelPricing, len(pricing))
	for _, p := range pricing {
		m[p.Model] = p
	}
	return m
}

func applyCosts(reports []models.CostReport, pricing map[string]models.ModelPricing) {
	for i := range reports {
		if p, ok := pricing[reports[i].Model]; ok {
			reports[i].EstimatedCost = (float64(reports[i].PromptTokens)/1000)*p.PromptCost +
				(float64(reports[i].CompletionTokens)/1000)*p.CompletionCost
		}
	}
}

func formatCostTable(reports []models.CostReport) string {
	if len(reports) == 0 {
		return "No cost data found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-15s %-15s %-25s %8s %12s %10s\n",
		"TEAM", "PROJECT", "MODEL", "REQUESTS", "TOKENS", "EST. COST")
	b.WriteString(strings.Repeat("-", 89) + "\n")

	var totalCost float64
	for _, r := range reports {
		fmt.Fprintf(&b, "%-15s %-15s %-25s %8d %12d $%9.4f\n",
			defaultStr(r.Team, "(none)"),
			defaultStr(r.Project, "(none)"),
			r.Model, r.RequestCount, r.TotalTokens, r.EstimatedCost)
		totalCost += r.EstimatedCost
	}
	b.WriteString(strings.Repeat("-", 89) + "\n")
	fmt.Fprintf(&b, "%77s $%9.4f\n", "TOTAL:", totalCost)
	return b.String()
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
