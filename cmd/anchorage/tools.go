package main

import (
	"anchorage/internal/dockercli"
	"github.com/spf13/cobra"
)

func newToolsCommand() *cobra.Command {
	tools := &cobra.Command{Use: "tools", Short: "Manage shared OCI tools", Args: cobra.NoArgs}
	image := "anchorage/tools:poc"
	volume := &cobra.Command{
		Use: "volume", Short: "Initialize and verify a versioned Docker tool volume", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dockercli.Volume(cmd.Context(), dockercli.Run, image, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	volume.Flags().StringVar(&image, "image", image, "OCI tool bundle image")
	tools.AddCommand(volume)
	return tools
}
