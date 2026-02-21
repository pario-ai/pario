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
		sessions   bool
		sessionID  string
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

			ctx := context.Background()

			// Session detail view
			if sessionID != "" {
				reqs, err := tr.SessionRequests(ctx, sessionID)
				if err != nil {
					return err
				}
				if len(reqs) == 0 {
					fmt.Println("No requests found for session.")
					return nil
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "#\tTIME\tPROMPT\tCOMPLETION\tTOTAL\tCONTEXT GROWTH")
				for _, r := range reqs {
					growth := "-"
					if r.Seq > 1 {
						growth = fmt.Sprintf("%+d", r.ContextGrowth)
					}
					fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%d\t%s\n",
						r.Seq, r.CreatedAt.Format("2006-01-02T15:04:05"), r.PromptTokens, r.CompletionTokens, r.TotalTokens, growth)
				}
				return w.Flush()
			}

			// Session list view
			if sessions {
				sess, err := tr.ListSessions(ctx, apiKey)
				if err != nil {
					return err
				}
				if len(sess) == 0 {
					fmt.Println("No sessions found.")
					return nil
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "SESSION ID\tAPI KEY\tSTARTED\tLAST ACTIVITY\tREQUESTS\tTOTAL TOKENS")
				for _, s := range sess {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\n",
						s.ID, s.APIKey, s.StartedAt.Format("2006-01-02T15:04:05"), s.LastActivity.Format("2006-01-02T15:04:05"), s.RequestCount, s.TotalTokens)
				}
				return w.Flush()
			}

			// Default: usage summary
			summaries, err := tr.Summary(ctx, apiKey)
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
	cmd.Flags().BoolVar(&sessions, "sessions", false, "list sessions")
	cmd.Flags().StringVar(&sessionID, "session-id", "", "show detail for a specific session")
	return cmd
}
