package main

import (
	"bundledock/internal/toolconfig"
	"bundledock/internal/toolmanager"
	"bundledock/internal/toolpublish"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func newUpgradeCommandWithClient(client *http.Client) *cobra.Command {
	bundleName := ""
	directory := toolconfig.ToolboxDirectory
	all := false

	cmd := &cobra.Command{
		Use:   "upgrade [tool]",
		Short: "Resolve the newest stable versions within packages.txt into packages.lock.json",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tool := ""
			if len(args) > 0 {
				tool = args[0]
			}
			if all {
				if tool != "" {
					return fmt.Errorf("cannot specify a tool with --all")
				}
				if cmd.Flags().Changed("bundle") {
					return fmt.Errorf("cannot combine --bundle and --all")
				}
				if cmd.Flags().Changed("directory") {
					return fmt.Errorf("cannot combine --directory and --all")
				}
				for _, name := range toolpublish.ActiveBundles() {
					b, err := toolpublish.SelectBundle(name)
					if err != nil {
						return err
					}
					if err := toolmanager.Upgrade(cmd.Context(), client, b.Directory, "", cmd.OutOrStdout()); err != nil {
						return fmt.Errorf("upgrade %s: %w", name, err)
					}
				}
				return nil
			}

			targetDir := directory
			if cmd.Flags().Changed("bundle") {
				if cmd.Flags().Changed("directory") {
					return fmt.Errorf("cannot combine --bundle and --directory")
				}
				b, err := toolpublish.SelectBundle(bundleName)
				if err != nil {
					return err
				}
				targetDir = b.Directory
			} else if bundleName != "" && !cmd.Flags().Changed("directory") {
				b, err := toolpublish.SelectBundle(bundleName)
				if err != nil {
					return err
				}
				targetDir = b.Directory
			}

			return toolmanager.Upgrade(cmd.Context(), client, targetDir, tool, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&bundleName, "bundle", "", "canonical bundle name (toolbox, code-server, node, node-current, bun, codex, agy, claude, opentofu)")
	cmd.Flags().StringVar(&directory, "directory", toolconfig.ToolboxDirectory, "directory containing the tool bundle inputs and lock")
	cmd.Flags().BoolVar(&all, "all", false, "upgrade all active canonical bundles (retired bundles are skipped)")

	return cmd
}
