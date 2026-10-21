package toolupgrade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resolveInto resolves one bundle into its own result directory, as one matrix
// job does, and restores the checkout so the next bundle starts clean.
func resolveInto(t *testing.T, dir string, env []string, bundle, tool, result string) {
	t.Helper()
	env = append(env, "BUNDLE="+bundle, "TOOL="+tool, "UPGRADE_RESULT="+result)
	if out, err := adapter(t, dir, env, "resolve"); err != nil {
		t.Fatalf("resolve %s: %v\n%s", bundle, err, out)
	}
	must(t, dir, nil, "git", "restore", ".")
}

// gather copies the resolved files of several jobs into one directory, as the
// artifact download of the proposal job does.
func gather(t *testing.T, target string, sources ...string) {
	t.Helper()
	for _, source := range sources {
		entries, err := os.ReadDir(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			b, err := os.ReadFile(filepath.Join(source, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(target, entry.Name()), string(b))
		}
	}
}

func withRemote(t *testing.T, dir string) {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "remote.git")
	must(t, dir, nil, "git", "init", "--bare", remote)
	must(t, dir, nil, "git", "remote", "add", "origin", remote)
}

func TestSeveralBundlesShareOneCommitAndPullRequest(t *testing.T) {
	dir, env := fixture(t)
	var parts []string
	for _, bundle := range []string{"agy", "node", "toolbox"} {
		part := t.TempDir()
		tool := bundle
		if bundle == "toolbox" {
			tool = "gh"
		}
		resolveInto(t, dir, env, bundle, tool, part)
		parts = append(parts, part)
	}
	result := t.TempDir()
	gather(t, result, parts...)
	failed := t.TempDir()
	write(t, filepath.Join(failed, "claude"), "claude\n")
	withRemote(t, dir)
	publish := append(env, "UPGRADE_RESULT="+result, "FAILED_BUNDLES="+failed)
	if out, err := adapter(t, dir, publish, "publish"); err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}
	if branch := must(t, dir, nil, "git", "branch", "--show-current"); branch != "automation/upgrades-123-1\n" {
		t.Fatal(branch)
	}
	if n := strings.TrimSpace(must(t, dir, nil, "git", "rev-list", "--count", "main..HEAD")); n != "1" {
		t.Fatalf("want one commit, got %s", n)
	}
	files := strings.Fields(must(t, dir, nil, "git", "diff", "--name-only", "main", "HEAD"))
	want := []string{"bundles/agy/packages.lock.json", "bundles/node/packages.lock.json", "bundles/toolbox/packages.lock.json", "docs/agy.md", "docs/node.md", "docs/toolbox.md"}
	if strings.Join(files, " ") != strings.Join(want, " ") {
		t.Fatalf("files %v, want %v", files, want)
	}
	title := must(t, dir, nil, "git", "log", "-1", "--format=%s")
	if title != "chore(upgrades): upgrade toolbox, node, agy\n" {
		t.Fatalf("title %q", title)
	}
	commit := must(t, dir, nil, "git", "log", "-1", "--format=%b")
	for _, text := range []string{"toolbox:\n- gh: ", "node:\n- node: ", "agy:\n- agy: "} {
		if !strings.Contains(commit, text) {
			t.Fatalf("commit body missing %q:\n%s", text, commit)
		}
	}
	body, err := os.ReadFile(filepath.Join(result, "pr-body.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"### toolbox", "### node", "### agy", "| Tool | Previous | New | Source |", "Resolution failed for: claude", "Run: https://github.com/wstein/bundledock/actions/runs/123 (attempt 1)"} {
		if !strings.Contains(string(body), text) {
			t.Fatalf("PR body missing %q:\n%s", text, body)
		}
	}
	calls, err := os.ReadFile(value(env, "CALLS"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(calls), "pr create") != 1 || strings.Count(string(calls), "--method POST") != 1 ||
		!strings.Contains(string(calls), "--head automation/upgrades-123-1 --title chore(upgrades): upgrade toolbox, node, agy") {
		t.Fatalf("want one PR and one CI dispatch:\n%s", calls)
	}
	summary, err := os.ReadFile(value(env, "GITHUB_STEP_SUMMARY"))
	if err != nil || !strings.Contains(string(summary), "Resolution failed for: claude") {
		t.Fatalf("summary %q %v", summary, err)
	}
}

func TestOneChangedBundleKeepsItsOwnTopicName(t *testing.T) {
	dir, env := fixture(t)
	part := t.TempDir()
	resolveInto(t, dir, env, "agy", "agy", part)
	withRemote(t, dir)
	if out, err := adapter(t, dir, append(env, "UPGRADE_RESULT="+part), "publish"); err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}
	if branch := must(t, dir, nil, "git", "branch", "--show-current"); branch != "automation/agy-upgrade-123-1\n" {
		t.Fatal(branch)
	}
}

func TestNothingChangedOpensNothingAndReportsFailures(t *testing.T) {
	dir, env := fixture(t)
	empty, failed := t.TempDir(), t.TempDir()
	write(t, filepath.Join(failed, "bun"), "bun\n")
	if out, err := adapter(t, dir, append(env, "UPGRADE_RESULT="+empty, "FAILED_BUNDLES="+failed), "publish"); err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}
	if branch := must(t, dir, nil, "git", "branch", "--show-current"); branch != "main\n" {
		t.Fatal("created a branch without changes: ", branch)
	}
	summary, err := os.ReadFile(value(env, "GITHUB_STEP_SUMMARY"))
	if err != nil || !strings.Contains(string(summary), "Resolution failed for: bun") {
		t.Fatalf("summary %q %v", summary, err)
	}
	if _, err := os.Stat(value(env, "CALLS")); err == nil {
		t.Fatal("contacted the forge without changes")
	}
}

func TestCombinedProposalRefusesUnknownOrIncompleteArtifacts(t *testing.T) {
	for _, tc := range []struct{ name, file, content string }{
		{"unknown bundle files", "mystery.lock.json", "x"},
		{"lock without inventory", "node.lock.json", "x"},
		{"unknown failed bundle", "FAILED:mystery", "mystery\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, env := fixture(t)
			part := t.TempDir()
			resolveInto(t, dir, env, "agy", "agy", part)
			failed := t.TempDir()
			if strings.HasPrefix(tc.file, "FAILED:") {
				write(t, filepath.Join(failed, strings.TrimPrefix(tc.file, "FAILED:")), tc.content)
			} else {
				write(t, filepath.Join(part, tc.file), tc.content)
			}
			withRemote(t, dir)
			if out, err := adapter(t, dir, append(env, "UPGRADE_RESULT="+part, "FAILED_BUNDLES="+failed), "publish"); err == nil {
				t.Fatal("accepted", out)
			}
			if branch := must(t, dir, nil, "git", "branch", "--show-current"); branch != "main\n" {
				t.Fatal("created branch before rejection: ", branch)
			}
		})
	}
}
