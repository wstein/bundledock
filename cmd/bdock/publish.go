package main

import (
	"bufio"
	"bundledock/internal/toolpublish"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var gitPreflight = func() (sha, branch string, clean, synced bool, err error) {
	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return "", "", false, false, fmt.Errorf("checking git status: %w", err)
	}
	clean = len(bytes.TrimSpace(out)) == 0

	out, err = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", "", false, false, fmt.Errorf("checking git branch: %w", err)
	}
	branch = strings.TrimSpace(string(out))

	out, err = exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", "", false, false, fmt.Errorf("resolving git HEAD SHA: %w", err)
	}
	sha = strings.TrimSpace(string(out))

	outOrigin, err := exec.Command("git", "rev-parse", "origin/main").Output()
	if err == nil {
		synced = sha == strings.TrimSpace(string(outOrigin))
	} else {
		synced = true
	}

	return sha, branch, clean, synced, nil
}

func newPublishCommandWithClient(client *http.Client) *cobra.Command {
	all := false
	yes := false
	dryRun := false
	repo := "wstein/bundledock"
	token := ""
	serverURL := ""

	cmd := &cobra.Command{
		Use:   "publish [bundle]",
		Short: "Publish official OCI tool bundles to Quay.io via verified GitHub Actions workflow dispatch",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all && len(args) > 0 {
				return fmt.Errorf("cannot combine bundle argument and --all")
			}
			if !all && len(args) == 0 {
				return fmt.Errorf("requires a bundle name or --all (available: %s)", strings.Join(toolpublish.CanonicalBundles, ", "))
			}

			var bundles []string
			if all {
				bundles = toolpublish.CanonicalBundles
			} else {
				b, err := toolpublish.SelectBundle(args[0])
				if err != nil {
					return err
				}
				bundles = []string{b.Name}
			}

			sha, branch, clean, synced, err := gitPreflight()
			if err != nil {
				return err
			}
			if !clean {
				return fmt.Errorf("working tree has uncommitted changes; commit or stash before publishing")
			}
			if branch != "main" {
				return fmt.Errorf("must be on branch main (currently on %s)", branch)
			}
			if !synced {
				return fmt.Errorf("local main differs from origin/main; push or pull before publishing")
			}

			tok, err := toolpublish.ResolveToken(token)
			if err != nil {
				return err
			}

			pub := &toolpublish.GitHubPublisher{
				Client:  client,
				BaseURL: serverURL,
				Repo:    repo,
				Token:   tok,
			}

			ciRun, err := pub.VerifyCommitCI(cmd.Context(), sha)
			if err != nil {
				return fmt.Errorf("CI verification failed: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Preflight verification successful:\n")
			fmt.Fprintf(out, "  Commit SHA: %s\n", sha)
			fmt.Fprintf(out, "  Official CI: SUCCESS (run %d: %s)\n", ciRun.ID, ciRun.HTMLURL)
			fmt.Fprintf(out, "  Target bundles: %s\n", strings.Join(bundles, ", "))

			if dryRun {
				fmt.Fprintf(out, "[dry-run] Workflow dispatch skipped.\n")
				return nil
			}

			if !yes {
				fmt.Fprintf(out, "\nProceed with publishing %s from commit %s? [y/N]: ", strings.Join(bundles, ", "), sha)
				reader := bufio.NewReader(os.Stdin)
				line, _ := reader.ReadString('\n')
				ans := strings.TrimSpace(strings.ToLower(line))
				if ans != "y" && ans != "yes" {
					return fmt.Errorf("publication cancelled by user")
				}
			}

			for _, b := range bundles {
				if err := pub.DispatchPublication(cmd.Context(), b, sha); err != nil {
					return fmt.Errorf("dispatch %s: %w", b, err)
				}
				fmt.Fprintf(out, "Dispatched publication for %s (approved_sha: %s)\n", b, sha)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "publish all canonical bundles sequentially")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip interactive confirmation prompt")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "run preflights and print verification without dispatching")
	cmd.Flags().StringVar(&repo, "repo", repo, "target GitHub repository (owner/repo)")
	cmd.Flags().StringVar(&token, "token", "", "GitHub API token (falls back to GITHUB_TOKEN or gh auth token)")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "GitHub API server URL (for testing)")
	_ = cmd.Flags().MarkHidden("server-url")

	return cmd
}
