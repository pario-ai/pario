package main

import (
	"fmt"

	cachepkg "github.com/pario-ai/pario/pkg/cache/sqlite"
	"github.com/pario-ai/pario/pkg/config"
	"github.com/spf13/cobra"
)

func newCacheCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage the prompt cache",
	}

	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show cache statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			c, err := cachepkg.New(cfg.DBPath, cfg.Cache.TTL)
			if err != nil {
				return err
			}
			defer func() { _ = c.Close() }()

			stats, err := c.Stats()
			if err != nil {
				return err
			}
			fmt.Printf("Entries: %d\nHits:    %d\nMisses:  %d\n", stats.Entries, stats.Hits, stats.Misses)
			return nil
		},
	}

	var expiredOnly bool
	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear cache entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			c, err := cachepkg.New(cfg.DBPath, cfg.Cache.TTL)
			if err != nil {
				return err
			}
			defer func() { _ = c.Close() }()

			if err := c.Clear(expiredOnly); err != nil {
				return err
			}
			if expiredOnly {
				fmt.Println("Expired cache entries cleared.")
			} else {
				fmt.Println("All cache entries cleared.")
			}
			return nil
		},
	}
	clearCmd.Flags().BoolVar(&expiredOnly, "expired", false, "only clear expired entries")

	cmd.PersistentFlags().StringVarP(&configPath, "config", "c", "pario.yaml", "path to config file")
	cmd.AddCommand(statsCmd, clearCmd)
	return cmd
}
