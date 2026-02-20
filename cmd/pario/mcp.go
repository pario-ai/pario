package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start Pario as an MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("mcp: not yet implemented (planned for week 2)")
			return nil
		},
	}
}
