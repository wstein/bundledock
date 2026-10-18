package main

import (
	"bundledock/internal/dockercli"
	"net/http"

	"github.com/spf13/cobra"
)

func newToolsCommand() *cobra.Command { return newToolsCommandWithClient(nil) }
func newToolsCommandWithClient(client *http.Client) *cobra.Command {
	tools := &cobra.Command{Use: "tools", Short: "Manage shared OCI tools", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	image := "bundledock-toolbox:poc"
	bundle := "toolbox"
	volume := &cobra.Command{
		Use: "volume", Short: "Initialize and verify a versioned Docker tool volume", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dockercli.BundleVolume(cmd.Context(), dockercli.Run, image, bundle, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	volume.Flags().StringVar(&image, "image", image, "OCI tool bundle image")
	volume.Flags().StringVar(&bundle, "bundle", bundle, "bundle name used for the image-derived volume namespace")
	tools.AddCommand(volume, newUpgradeCommandWithClient(client))
	return tools
}
