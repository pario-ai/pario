package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newTopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "top",
		Short: "Live view of token usage (like htop for tokens)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("top: not yet implemented (planned for week 2)")
			return nil
		},
	}
}
