package main

import (
	"bundledock/internal/quay"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// quayOptions are shared by all quay subcommands.
type quayOptions struct {
	json   bool
	root   string
	client *http.Client
}

func (o *quayOptions) api() *quay.Client { return quay.New(o.client, quay.DefaultBaseURL) }

// errDifference makes a command exit with status 1 without extra output.
type errDifference struct{ message string }

func (e errDifference) Error() string { return e.message }

func newQuayCommandWithClient(client *http.Client) *cobra.Command {
	o := &quayOptions{client: client}
	cmd := &cobra.Command{Use: "quay", Short: "Inspect and maintain the Quay repositories", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	cmd.PersistentFlags().BoolVar(&o.json, "json", false, "write one JSON document to stdout")
	cmd.PersistentFlags().StringVar(&o.root, "root", ".", "repository root that holds docs/ and bundles/")
	cmd.AddCommand(newQuayListCommand(o), newQuayStatusCommand(o), newQuayInfoCommand(o), newQuayDescribeCommand(o), newQuayPruneCommand(o))
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
	apply, initDraft := false, false
	cmd := &cobra.Command{
		Use: "describe [bundle]", Short: "Compare bundles/<name>/description.md with the live Quay description; --apply writes it", Args: cobra.MaximumNArgs(1),
		Long: "Compare bundles/<name>/description.md with the description on Quay. Without a bundle, every bundle that has a\n" +
			"description file is checked. Exit status 1 means a difference. --apply writes only the descriptions that differ,\n" +
			"using the OAuth token in the QUAY_TOKEN environment variable (never a flag or file).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if initDraft {
				return initDescription(cmd, o, args, apply)
			}
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
	cmd.Flags().BoolVar(&initDraft, "init", false, "write a draft bundles/<name>/description.md for a bundle; never overwrites; needs no network")
	return cmd
}

// newGitHub supplies the GitHub evidence for prune; tests replace it.
var newGitHub = func() quay.GitHub { return &quay.GitHubAPI{Fetch: quay.GHCLI} }

func newQuayPruneCommand(o *quayOptions) *cobra.Command {
	var apply bool
	var repository, failedRun, failedAttempt, output, approved string
	cmd := &cobra.Command{
		Use: "prune", Short: "Plan the removal of verified staging tags; --apply deletes a saved plan", Args: cobra.NoArgs,
		Long: "Plan the removal of architecture staging tags (staging-<version>-<arch>) whose publication run is complete\n" +
			"and older than seven days, or that belong to one exact failed run (--failed-run and --failed-attempt).\n" +
			"Without --apply nothing changes. --apply deletes only the tags of a saved plan (--approved-plan) that\n" +
			"are still eligible after a fresh recheck, with the robot account in REPOSITORY_USERNAME and\n" +
			"REPOSITORY_TOKEN. Version, alias and source tags are never deleted.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if apply && approved == "" {
				return fmt.Errorf("--apply requires a saved plan (--approved-plan)")
			}
			if !apply && approved != "" {
				return fmt.Errorf("--approved-plan needs --apply")
			}
			username, password := os.Getenv("REPOSITORY_USERNAME"), os.Getenv("REPOSITORY_TOKEN")
			if apply && (username == "" || password == "") {
				return fmt.Errorf("REPOSITORY_USERNAME and REPOSITORY_TOKEN required for deletion")
			}
			var target *quay.FailedRun
			if failedRun != "" || failedAttempt != "" {
				positive := regexp.MustCompile(`^[1-9][0-9]*$`)
				if !positive.MatchString(failedRun) || !positive.MatchString(failedAttempt) {
					return fmt.Errorf("both --failed-run and --failed-attempt must be positive integers")
				}
				attempt, _ := strconv.Atoi(failedAttempt)
				target = &quay.FailedRun{RunID: failedRun, Attempt: attempt}
			}
			repos := quay.Repositories()
			if repository != "all" {
				if !contains(repos, repository) {
					return fmt.Errorf("unknown repository %q", repository)
				}
				repos = []string{repository}
			}
			client := o.api()
			p := &quay.Pruner{
				Store: quay.Store{Client: client, Username: username, Password: password, Unpublished: func(r string) bool { return quay.NotYetPublished(o.root, r) }},
				Registry: func(ctx context.Context, repo string) (quay.Evidence, error) {
					return client.RegistryAs(ctx, repo, username, password)
				},
				GitHub:    newGitHub(),
				Now:       time.Now,
				Repos:     repos,
				FailedRun: target,
			}
			save := func(v any) error {
				if output == "" {
					return nil
				}
				raw, err := json.MarshalIndent(v, "", "  ")
				if err != nil {
					return err
				}
				return os.WriteFile(output, append(raw, '\n'), 0o644)
			}
			if apply {
				raw, err := os.ReadFile(approved)
				if err != nil {
					return err
				}
				var plan quay.Plan
				if err := json.Unmarshal(raw, &plan); err != nil {
					return fmt.Errorf("invalid cleanup plan: use the file saved with --output, not the --json output")
				}
				result := struct {
					Schema      int             `json:"schema"`
					Mode        string          `json:"mode"`
					GeneratedAt string          `json:"generated_at"`
					Target      *quay.FailedRun `json:"target,omitempty"`
					Actions     []quay.Action   `json:"actions"`
				}{1, plan.Mode, time.Now().UTC().Format(time.RFC3339Nano), plan.Target, []quay.Action{}}
				if err := save(result); err != nil {
					return err
				}
				actions, err := p.Apply(cmd.Context(), plan, func(a []quay.Action) error { result.Actions = a; return save(result) })
				result.Actions = actions
				if actions == nil {
					result.Actions = []quay.Action{}
				}
				if serr := save(result); serr != nil && err == nil {
					err = serr
				}
				if o.json {
					if werr := quay.WriteJSON(cmd.OutOrStdout(), "bdock.quay.prune/v1", result); werr != nil && err == nil {
						err = werr
					}
				} else {
					for _, a := range actions {
						fmt.Fprintf(cmd.OutOrStdout(), "%s: %s/%s\n", a.Action, a.Repository, a.Name)
					}
				}
				return err
			}
			plan, err := p.Plan(cmd.Context())
			if err != nil {
				return err
			}
			if err := save(plan); err != nil {
				return err
			}
			eligible := 0
			for _, r := range plan.Rows {
				if r.Eligible {
					eligible++
				}
			}
			if o.json {
				fmt.Fprintf(cmd.ErrOrStderr(), "Staging tags: %d; eligible: %d (dry run, nothing changed)\n", len(plan.Rows), eligible)
				return quay.WriteJSON(cmd.OutOrStdout(), "bdock.quay.prune/v1", plan)
			}
			out := cmd.OutOrStdout()
			for _, r := range plan.Rows {
				state := "retain"
				if r.Eligible {
					state = "eligible"
				}
				fmt.Fprintf(out, "%s %s/%s: %s\n", state, r.Repository, r.Name, r.Reason)
			}
			fmt.Fprintf(out, "Staging tags: %d; eligible: %d (dry run, nothing changed)\n", len(plan.Rows), eligible)
			return nil
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "delete the eligible tags of the saved plan (needs REPOSITORY_USERNAME and REPOSITORY_TOKEN)")
	cmd.Flags().StringVar(&repository, "repository", "all", "Quay repository to scan, or all")
	cmd.Flags().StringVar(&failedRun, "failed-run", "", "exact completed failed publication run ID")
	cmd.Flags().StringVar(&failedAttempt, "failed-attempt", "", "exact latest attempt of the failed run")
	cmd.Flags().StringVar(&output, "output", "", "save the plan (or, with --apply, the result) to this file")
	cmd.Flags().StringVar(&approved, "approved-plan", "", "saved plan to apply")
	return cmd
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// initDescription writes a draft description file. It needs neither network nor token.
func initDescription(cmd *cobra.Command, o *quayOptions, args []string, apply bool) error {
	if len(args) != 1 {
		return fmt.Errorf("--init requires a bundle")
	}
	if apply {
		return fmt.Errorf("--init and --apply cannot be combined")
	}
	b, err := quay.FindBundle(args[0])
	if err != nil {
		return err
	}
	path, chars, err := quay.InitDescription(o.root, b)
	if err != nil {
		return err
	}
	if o.json {
		return quay.WriteJSON(cmd.OutOrStdout(), "bdock.quay.describe-init/v1", map[string]any{"bundle": b.Name, "path": path, "written": true, "characters": chars})
	}
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d characters); review and edit it\n", path, chars)
	return nil
}
