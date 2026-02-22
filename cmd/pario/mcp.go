package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/pario-ai/pario/pkg/audit"
	"github.com/pario-ai/pario/pkg/budget"
	cachepkg "github.com/pario-ai/pario/pkg/cache/sqlite"
	"github.com/pario-ai/pario/pkg/config"
	"github.com/pario-ai/pario/pkg/mcp"
	"github.com/pario-ai/pario/pkg/tracker"
	"github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start Pario as an MCP server (stdio JSON-RPC)",
		Long:  "Runs Pario as a Model Context Protocol server over stdin/stdout for use with Claude Code and other MCP clients.",
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

			var cache mcp.CacheStatter
			if cfg.Cache.Enabled {
				c, err := cachepkg.New(cfg.DBPath, cfg.Cache.TTL)
				if err != nil {
					return err
				}
				defer func() { _ = c.Close() }()
				cache = c
			}

			var enforcer *budget.Enforcer
			if cfg.Budget.Enabled {
				enforcer = budget.New(cfg.Budget.Policies, tr)
			}

			var auditor *audit.Logger
			if cfg.Audit.Enabled {
				auditor, err = audit.New(cfg.Audit)
				if err != nil {
					return err
				}
				defer func() { _ = auditor.Close() }()
			}

			srv := mcp.New(tr, cache, enforcer, auditor, cfg.Attribution.Pricing, version)

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			return srv.Run(ctx, os.Stdin, os.Stdout)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to pario config file")

	return cmd
}
