package main

import "github.com/spf13/cobra"

func newRootCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "anchorage",
		Short:        "AI-native Docker development environments",
		Version:      "dev",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}
