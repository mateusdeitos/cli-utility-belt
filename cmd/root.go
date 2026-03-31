package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "belt",
	Short: "A personal CLI utility belt",
	Long:  `belt is a collection of everyday developer utilities available as a single binary.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
