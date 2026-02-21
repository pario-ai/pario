package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"github.com/pario-ai/pario/pkg/budget"
	cachepkg "github.com/pario-ai/pario/pkg/cache/sqlite"
	"github.com/pario-ai/pario/pkg/config"
	"github.com/pario-ai/pario/pkg/proxy"
	"github.com/pario-ai/pario/pkg/tracker"
	"github.com/spf13/cobra"
)

func newProxyCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Start the LLM API proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			tr, err := tracker.New(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("init tracker: %w", err)
			}
			defer func() { _ = tr.Close() }()

			var cache *cachepkg.Cache
			if cfg.Cache.Enabled {
				cache, err = cachepkg.New(cfg.DBPath, cfg.Cache.TTL)
				if err != nil {
					return fmt.Errorf("init cache: %w", err)
				}
				defer func() { _ = cache.Close() }()
			}

			var enforcer *budget.Enforcer
			if cfg.Budget.Enabled {
				enforcer = budget.New(cfg.Budget.Policies, tr)
			}

			srv := proxy.New(cfg, tr, cache, enforcer)

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			log.Printf("starting pario proxy with config: %s", configPath)
			return srv.ListenAndServe(ctx)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "pario.yaml", "path to config file")
	return cmd
}
