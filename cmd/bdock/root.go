package main

import (
	"net/http"

	"github.com/spf13/cobra"
)

var version = "dev"

func newRootCommand() *cobra.Command { return newRootCommandWithClient(nil) }

func newRootCommandWithClient(client *http.Client) *cobra.Command {
	root := &cobra.Command{
		Use:          "bdock",
		Short:        "Build and manage OCI bundles",
		Version:      version,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.AddCommand(newToolsCommandWithClient(client))
	root.AddCommand(newUpgradeCommandWithClient(client))
	return root
}
