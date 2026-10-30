package main

import (
	"bundledock/internal/quay"
	"fmt"
	"net/http"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// quayOptions are shared by all quay subcommands.
type quayOptions struct {
	json   bool
	root   string
	apiURL string
	client *http.Client
}

func (o *quayOptions) api() *quay.Client { return quay.New(o.client, o.apiURL) }

// errDifference makes a command exit with status 1 without extra output.
type errDifference struct{ message string }

func (e errDifference) Error() string { return e.message }

func newQuayCommandWithClient(client *http.Client) *cobra.Command {
	o := &quayOptions{client: client}
	cmd := &cobra.Command{Use: "quay", Short: "Inspect and maintain the Quay repositories", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	cmd.PersistentFlags().BoolVar(&o.json, "json", false, "write one JSON document to stdout")
	cmd.PersistentFlags().StringVar(&o.root, "root", ".", "repository root that holds docs/ and bundles/")
	cmd.PersistentFlags().StringVar(&o.apiURL, "api-url", quay.DefaultBaseURL, "Quay endpoint (for testing)")
	_ = cmd.PersistentFlags().MarkHidden("api-url")
	cmd.AddCommand(newQuayListCommand(o))
	return cmd
}

func newQuayListCommand(o *quayOptions) *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List each bundle with its newest version, digest and staging tag count", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rows, err := quay.List(cmd.Context(), &quay.Inventory{Client: o.api()}, o.root)
			if err != nil {
				return err
			}
			if o.json {
				return quay.WriteJSON(cmd.OutOrStdout(), "bdock.quay.list/v1", map[string]any{"bundles": rows})
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "BUNDLE\tVERSION\tDIGEST\tSTAGING")
			for _, r := range rows {
				version, digest := r.Version, r.Digest
				if !r.Published && version == "" {
					version, digest = "(not published)", "-"
				} else if version == "" {
					version, digest = "-", "-"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", r.Bundle, version, digest, r.Staging)
			}
			return w.Flush()
		},
	}
}
