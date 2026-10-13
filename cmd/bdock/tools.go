package main

import (
	"bundledock/internal/dockercli"
	"bundledock/internal/toolconfig"
	"bundledock/internal/toolmanager"
	"github.com/spf13/cobra"
	"net/http"
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
	directory := toolconfig.ToolboxDirectory
	upgrade := &cobra.Command{Use: "upgrade [tool]", Short: "Resolve the newest stable versions within packages.txt into packages.lock.json", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		tool := ""
		if len(args) > 0 {
			tool = args[0]
		}
		return toolmanager.Upgrade(cmd.Context(), client, directory, tool, cmd.OutOrStdout())
	}}
	upgrade.Flags().StringVar(&directory, "directory", directory, "directory containing the tool bundle inputs and lock")
	tools.AddCommand(volume, upgrade)
	return tools
}
