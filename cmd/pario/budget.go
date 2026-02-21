package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/pario-ai/pario/pkg/budget"
	"github.com/pario-ai/pario/pkg/config"
	"github.com/pario-ai/pario/pkg/tracker"
	"github.com/spf13/cobra"
)

func newBudgetCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "budget",
		Short: "Manage token budgets and policies",
	}

	var apiKey string
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show budget usage vs limits",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if !cfg.Budget.Enabled {
				fmt.Println("Budget enforcement is disabled.")
				return nil
			}

			tr, err := tracker.New(cfg.DBPath)
			if err != nil {
				return err
			}
			defer func() { _ = tr.Close() }()

			enforcer := budget.New(cfg.Budget.Policies, tr)

			key := apiKey
			if key == "" {
				key = "*"
			}

			statuses, err := enforcer.Status(context.Background(), key)
			if err != nil {
				return err
			}

			if len(statuses) == 0 {
				fmt.Println("No budget policies found for this key.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "API KEY\tPERIOD\tMAX TOKENS\tUSED\tREMAINING")
			for _, s := range statuses {
				fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\n",
					s.Policy.APIKey, s.Policy.Period, s.Policy.MaxTokens, s.Used, s.Remaining)
			}
			return w.Flush()
		},
	}
	statusCmd.Flags().StringVar(&apiKey, "api-key", "", "filter by API key")

	cmd.PersistentFlags().StringVarP(&configPath, "config", "c", "pario.yaml", "path to config file")
	cmd.AddCommand(statusCmd)
	return cmd
}
