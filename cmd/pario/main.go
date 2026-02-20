package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	root := &cobra.Command{
		Use:     "pario",
		Short:   "Pario — Kubernetes-native token cost control plane",
		Version: version,
	}

	root.AddCommand(
		newProxyCmd(),
		newStatsCmd(),
		newTopCmd(),
		newMCPCmd(),
		newCacheCmd(),
		newBudgetCmd(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newProxyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "proxy",
		Short: "Start the LLM API proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("proxy: not yet implemented")
			return nil
		},
	}
}

func newStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show token usage statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("stats: not yet implemented")
			return nil
		},
	}
}

func newTopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "top",
		Short: "Live view of token usage (like htop for tokens)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("top: not yet implemented")
			return nil
		},
	}
}

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start Pario as an MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("mcp: not yet implemented")
			return nil
		},
	}
}

func newCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cache",
		Short: "Manage the semantic cache",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("cache: not yet implemented")
			return nil
		},
	}
}

func newBudgetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "budget",
		Short: "Manage token budgets and policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("budget: not yet implemented")
			return nil
		},
	}
}
