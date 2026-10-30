package main

import (
	"bundledock/internal/quay"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

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
	cmd.AddCommand(newQuayListCommand(o), newQuayStatusCommand(o), newQuayInfoCommand(o), newQuayDescribeCommand(o))
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

func newQuayStatusCommand(o *quayOptions) *cobra.Command {
	return &cobra.Command{
		Use: "status [bundle]", Short: "Compare Quay with the publication records; exit 1 on any difference", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bundles := quay.Bundles()
			if len(args) == 1 {
				b, err := quay.FindBundle(args[0])
				if err != nil {
					return err
				}
				bundles = []quay.Bundle{b}
			}
			rows, err := quay.Status(cmd.Context(), &quay.Inventory{Client: o.api()}, o.root, bundles)
			if err != nil {
				return err
			}
			differs := 0
			for _, r := range rows {
				if r.State == "differs" {
					differs++
				}
			}
			if o.json {
				if err := quay.WriteJSON(cmd.OutOrStdout(), "bdock.quay.status/v1", map[string]any{"bundles": rows}); err != nil {
					return err
				}
			} else {
				out := cmd.OutOrStdout()
				for _, r := range rows {
					fmt.Fprintf(out, "%s: %s\n", r.Bundle, r.State)
					for _, c := range r.Checks {
						switch {
						case c.Kind == "residue":
							fmt.Fprintf(out, "  residue: staging tag %s of the published version\n", c.Tag)
						case !c.OK && c.Actual == "":
							fmt.Fprintf(out, "  %s %s: missing (expected %s)\n", c.Kind, c.Tag, c.Expected)
						case !c.OK:
							fmt.Fprintf(out, "  %s %s: %s (expected %s)\n", c.Kind, c.Tag, c.Actual, c.Expected)
						}
					}
				}
			}
			if differs > 0 {
				return errDifference{fmt.Sprintf("%d bundle(s) differ from the publication records", differs)}
			}
			return nil
		},
	}
}

func newQuayInfoCommand(o *quayOptions) *cobra.Command {
	return &cobra.Command{
		Use: "info <bundle>", Short: "Show all tags of a bundle with digest, age, architectures and labels", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := quay.FindBundle(args[0])
			if err != nil {
				return err
			}
			rows, err := quay.Info(cmd.Context(), o.api(), b, time.Now())
			if err != nil {
				return err
			}
			if o.json {
				return quay.WriteJSON(cmd.OutOrStdout(), "bdock.quay.info/v1", map[string]any{"bundle": b.Name, "repository": b.Repository, "tags": rows})
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "TAG\tDIGEST\tAGE\tARCH\tVERSION\tREVISION")
			for _, r := range rows {
				if r.Error != "" {
					fmt.Fprintf(w, "%s\t%s\t%dd\t-\t(%s)\t-\n", r.Tag, r.Digest, r.AgeDays, r.Error)
					continue
				}
				fmt.Fprintf(w, "%s\t%s\t%dd\t%s\t%s\t%s\n", r.Tag, r.Digest, r.AgeDays, strings.Join(r.Architectures, ","), r.Version, r.Revision)
			}
			return w.Flush()
		},
	}
}

func newQuayDescribeCommand(o *quayOptions) *cobra.Command {
	apply := false
	cmd := &cobra.Command{
		Use: "describe [bundle]", Short: "Compare bundles/<name>/description.md with the live Quay description; --apply writes it", Args: cobra.MaximumNArgs(1),
		Long: "Compare bundles/<name>/description.md with the description on Quay. Without a bundle, every bundle that has a\n" +
			"description file is checked. Exit status 1 means a difference. --apply writes only the descriptions that differ,\n" +
			"using the OAuth token in the QUAY_TOKEN environment variable (never a flag or file).",
		RunE: func(cmd *cobra.Command, args []string) error {
			bundles := quay.Bundles()
			if len(args) == 1 {
				b, err := quay.FindBundle(args[0])
				if err != nil {
					return err
				}
				if _, ok, err := quay.LoadDescription(o.root, b.Name); err != nil || !ok {
					if err == nil {
						err = fmt.Errorf("%s not found", quay.DescriptionPath(o.root, b.Name))
					}
					return err
				}
				bundles = []quay.Bundle{b}
			}
			rows, err := quay.Describe(cmd.Context(), o.api(), o.root, bundles, apply, os.Getenv("QUAY_TOKEN"))
			if err != nil {
				return err
			}
			differs := 0
			for _, r := range rows {
				if r.State == "differs" {
					differs++
				}
			}
			if o.json {
				if err := quay.WriteJSON(cmd.OutOrStdout(), "bdock.quay.describe/v1", map[string]any{"bundles": rows}); err != nil {
					return err
				}
			} else {
				for _, r := range rows {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", r.Bundle, r.State)
					if r.State != "skipped" && !r.Public {
						fmt.Fprintf(cmd.OutOrStdout(), "  repository is not public\n")
					}
				}
			}
			if differs > 0 {
				return errDifference{fmt.Sprintf("%d description(s) differ; run with --apply to write them", differs)}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "write differing descriptions (needs QUAY_TOKEN)")
	return cmd
}
