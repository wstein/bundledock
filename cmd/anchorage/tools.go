package main

import (
	"anchorage/internal/dockercli"
	"anchorage/internal/toolmanager"
	"github.com/spf13/cobra"
	"net/http"
	"os"
)

func newToolsCommand() *cobra.Command { return newToolsCommandWithClient(nil) }
func newToolsCommandWithClient(client *http.Client) *cobra.Command {
	tools := &cobra.Command{Use: "tools", Short: "Manage shared OCI tools", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	image := "anchorage/tools:poc"
	volume := &cobra.Command{
		Use: "volume", Short: "Initialize and verify a versioned Docker tool volume", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dockercli.Volume(cmd.Context(), dockercli.Run, image, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	volume.Flags().StringVar(&image, "image", image, "OCI tool bundle image")
	upgrade := &cobra.Command{Use: "upgrade [tool]", Short: "Resolve the newest stable versions within tools.txt into tools.lock.json", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		directory, err := os.Getwd()
		if err != nil {
			return err
		}
		tool := ""
		if len(args) > 0 {
			tool = args[0]
		}
		return toolmanager.Upgrade(cmd.Context(), client, directory, tool, cmd.OutOrStdout())
	}}
	tools.AddCommand(volume, upgrade)
	return tools
}
