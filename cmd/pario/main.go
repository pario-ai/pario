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
