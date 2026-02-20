package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/pario-ai/pario/pkg/config"
	"github.com/pario-ai/pario/pkg/tracker"
	"github.com/spf13/cobra"
)

func newStatsCmd() *cobra.Command {
	var (
		configPath string
		apiKey     string
	)

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show token usage statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			tr, err := tracker.New(cfg.DBPath)
			if err != nil {
				return err
			}
			defer tr.Close()

			summaries, err := tr.Summary(context.Background(), apiKey)
			if err != nil {
				return err
			}

			if len(summaries) == 0 {
				fmt.Println("No usage data found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "API KEY\tMODEL\tREQUESTS\tPROMPT\tCOMPLETION\tTOTAL")
			for _, s := range summaries {
				fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\n",
					s.APIKey, s.Model, s.RequestCount, s.TotalPrompt, s.TotalCompletion, s.TotalTokens)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "pario.yaml", "path to config file")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "filter by API key")
	return cmd
}
