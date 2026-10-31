package quay

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Git verifies the Git side of a publication: that the published commit is on
// main and that the immutable source tag matches the verified provenance.
type Git interface {
	// VerifySource checks that origin is the official repository and that
	// commit is an ancestor of origin/main.
	VerifySource(ctx context.Context, commit string) error
	// RemoteTag reports whether the annotated tag name exists on origin. An
	// existing tag must point at commit and carry exactly the annotation.
	RemoteTag(ctx context.Context, name, commit, annotation string) (bool, error)
}

// GitCLI implements Git with the git command line tool (port of the Git checks
// of scripts/bundle-git-tags.py). Run executes one git command; nil selects the
// real one.
type GitCLI struct {
	Run func(ctx context.Context, stdin string, args ...string) (string, error)
}

// gitEnvironment hands a GITHUB_TOKEN to git only through the environment of
// the subprocess, never through arguments, files or diagnostics.
func gitEnvironment() []string {
	env := os.Environ()
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		header := "AUTHORIZATION: basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token))
		env = append(env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader", "GIT_CONFIG_VALUE_0="+header)
	}
	return env
}

func runGit(ctx context.Context, stdin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = gitEnvironment()
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout = &out
	// Diagnostics may name URLs or credentials: they are discarded.
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Git operation failed: %s", args[0])
	}
	return out.String(), nil
}

func (g *GitCLI) git(ctx context.Context, args ...string) (string, error) {
	if g.Run != nil {
		return g.Run(ctx, "", args...)
	}
	return runGit(ctx, "", args...)
}

func verifyRef() string {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	return "refs/bundledock-verify/" + hex.EncodeToString(raw)
}

// VerifySource implements Git.
func (g *GitCLI) VerifySource(ctx context.Context, commit string) error {
	for _, args := range [][]string{{"remote", "get-url", "--all", "origin"}, {"remote", "get-url", "--push", "--all", "origin"}} {
		out, err := g.git(ctx, args...)
		if err != nil {
			return err
		}
		remotes := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if out == "" || len(remotes) != 1 {
			return reasonError("origin fetch and push URLs must be the official BundleDock repository")
		}
		if url := strings.TrimSuffix(remotes[0], ".git"); url != SourceURL && url != "git@github.com:"+GitHubRepository {
			return reasonError("origin fetch and push URLs must be the official BundleDock repository")
		}
	}
	ref := verifyRef()
	defer g.git(ctx, "update-ref", "-d", ref) //nolint:errcheck // best-effort cleanup of a private ref
	if _, err := g.git(ctx, "fetch", "--no-tags", "origin", "refs/heads/main:"+ref); err != nil {
		return err
	}
	if _, err := g.git(ctx, "cat-file", "-e", commit+"^{commit}"); err != nil {
		return err
	}
	_, err := g.git(ctx, "merge-base", "--is-ancestor", commit, ref)
	return err
}

// RemoteTag implements Git.
func (g *GitCLI) RemoteTag(ctx context.Context, name, commit, annotation string) (bool, error) {
	full := "refs/tags/" + name
	out, err := g.git(ctx, "ls-remote", "--refs", "origin", full)
	if err != nil {
		return false, err
	}
	found := strings.TrimSpace(out)
	if found == "" {
		return false, nil
	}
	fields := strings.Fields(found)
	if len(fields) != 2 {
		return false, reasonError("unexpected remote ref")
	}
	oid, actual := fields[0], fields[1]
	if actual != full {
		return false, reasonError("unexpected remote ref")
	}
	ref := verifyRef()
	defer g.git(ctx, "update-ref", "-d", ref) //nolint:errcheck // best-effort cleanup of a private ref
	if _, err := g.git(ctx, "fetch", "--no-tags", "origin", full+":"+ref); err != nil {
		return false, err
	}
	rev, err := g.git(ctx, "rev-parse", ref)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(rev) != oid {
		return false, reasonError("remote tag changed during verification")
	}
	raw, err := g.git(ctx, "cat-file", "tag", ref)
	if err != nil {
		return false, err
	}
	header, message, ok := strings.Cut(raw, "\n\n")
	if !ok {
		return false, reasonError("existing remote tag conflicts with verified provenance")
	}
	values := map[string]string{}
	for _, line := range strings.Split(header, "\n") {
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			return false, reasonError("existing remote tag conflicts with verified provenance")
		}
		values[key] = value
	}
	if values["object"] != commit || values["type"] != "commit" || values["tag"] != name || message != annotation {
		return false, reasonError("existing remote tag conflicts with verified provenance")
	}
	return true, nil
}
