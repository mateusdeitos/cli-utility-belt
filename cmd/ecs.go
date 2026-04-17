package cmd

import "github.com/spf13/cobra"

var ecsCmd = &cobra.Command{
	Use:   "ecs",
	Short: "AWS ECS utilities",
}

func init() {
	rootCmd.AddCommand(ecsCmd)
}
