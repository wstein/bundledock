package toolupgrade

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, env []string, name string, args ...string) (string, error) {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	c.Env = append(c.Env, env...)
	b, err := c.CombinedOutput()
	return string(b), err
}
func must(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	out, err := run(t, dir, env, name, args...)
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return out
}
func write(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) (string, []string) {
	t.Helper()
	for _, tool := range []string{"git", "bash", "sh"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("requires ", tool, ": ", err)
		}
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	must(t, dir, nil, "git", "init", "-b", "main")
	must(t, dir, nil, "git", "config", "user.name", "Test")
	must(t, dir, nil, "git", "config", "user.email", "test@example.invalid")
	must(t, dir, nil, "git", "config", "commit.gpgsign", "false")
	if err := os.MkdirAll(filepath.Join(dir, "bundles/toolbox"), 0755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "bundles/toolbox/tools.lock.json"), "old lock\n")
	write(t, filepath.Join(dir, "bundles/toolbox/tools.txt"), "gh>=2.0.0,<3.0.0\n")
	must(t, dir, nil, "git", "add", ".")
	must(t, dir, nil, "git", "commit", "-m", "fixture")
	fake := t.TempDir()
	write(t, filepath.Join(fake, "go"), `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$CALLS"
case "$*" in 'run ./cmd/bundledock tools upgrade gh'|'run ./cmd/bundledock tools upgrade') ;; *) exit 41 ;; esac
if [ "${FAIL_RESOLVER:-false}" = true ]; then exit 42; fi
if [ "${NO_CHANGE:-false}" != true ]; then printf 'new lock\n' > bundles/toolbox/tools.lock.json; fi
if [ "${EXTRA_EDIT:-false}" = true ]; then printf bad > extra-file; fi
`)
	write(t, filepath.Join(fake, "gh"), `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$CALLS"
if [ "${DENY_PR:-false}" = true ]; then echo 'GitHub Actions is not permitted to create pull requests' >&2; exit 1; fi
printf 'https://github.com/example/repo/pull/1\n'
`)
	env := []string{"PATH=" + fake + ":" + os.Getenv("PATH"), "CALLS=" + filepath.Join(t.TempDir(), "calls"), "TOOL=gh", "UPGRADE_RESULT=" + t.TempDir(), "GITHUB_OUTPUT=" + filepath.Join(t.TempDir(), "outputs"), "GITHUB_STEP_SUMMARY=" + filepath.Join(t.TempDir(), "summary"), "UPGRADE_SCRIPT=" + filepath.Join(root, "scripts/tool-upgrade.sh"), "GITHUB_REPOSITORY=example/repo", "GITHUB_RUN_ID=123", "GITHUB_RUN_ATTEMPT=1", "DEFAULT_BRANCH=main"}
	return dir, env
}
func value(env []string, key string) string {
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			return strings.TrimPrefix(e, key+"=")
		}
	}
	return ""
}
func adapter(t *testing.T, dir string, env []string, phase string) (string, error) {
	t.Helper()
	return run(t, dir, env, "bash", value(env, "UPGRADE_SCRIPT"), phase)
}

func TestResolveAndPublish(t *testing.T) {
	dir, env := fixture(t)
	out, err := adapter(t, dir, env, "resolve")
	if err != nil {
		t.Fatalf("resolve: %v\n%s", err, out)
	}
	b, err := os.ReadFile(value(env, "GITHUB_OUTPUT"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "changed=true") {
		t.Fatalf("outputs: %s", b)
	}
	result, err := os.ReadFile(filepath.Join(value(env, "UPGRADE_RESULT"), "tools.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "new lock\n" {
		t.Fatalf("result %s", result)
	}
	must(t, dir, nil, "git", "restore", "bundles/toolbox/tools.lock.json")
	remote := filepath.Join(t.TempDir(), "remote.git")
	must(t, dir, nil, "git", "init", "--bare", remote)
	must(t, dir, nil, "git", "remote", "add", "origin", remote)
	out, err = adapter(t, dir, env, "publish")
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}
	got := must(t, dir, nil, "git", "show", "HEAD:bundles/toolbox/tools.lock.json")
	if got != "new lock\n" {
		t.Fatal(got)
	}
	if branch := must(t, dir, nil, "git", "branch", "--show-current"); branch != "automation/tool-upgrade-123-1\n" {
		t.Fatal(branch)
	}
	calls, err := os.ReadFile(value(env, "CALLS"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "pr create --repo example/repo --base main --head automation/tool-upgrade-123-1") {
		t.Fatalf("calls %s", calls)
	}
}
func TestResolveFailuresAndNoChange(t *testing.T) {
	for _, tc := range []struct {
		name, setting string
		fail          bool
	}{
		{"no change", "NO_CHANGE=true", false}, {"all tools", "TOOL=", false}, {"resolver failure", "FAIL_RESOLVER=true", true}, {"extra edit", "EXTRA_EDIT=true", true}, {"unsafe input", "TOOL=gh; touch /tmp/never", true}, {"unknown input", "TOOL=unknown", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, env := fixture(t)
			env = append(env, tc.setting)
			out, err := adapter(t, dir, env, "resolve")
			if (err != nil) != tc.fail {
				t.Fatalf("error %v output %s", err, out)
			}
			if !tc.fail && tc.name == "no change" {
				b, err := os.ReadFile(value(env, "GITHUB_OUTPUT"))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(b), "changed=false") {
					t.Fatal(string(b))
				}
			}
		})
	}
}
func TestDeniedPRExplainsPrerequisite(t *testing.T) {
	dir, env := fixture(t)
	out, err := adapter(t, dir, env, "resolve")
	if err != nil {
		t.Fatalf("%v %s", err, out)
	}
	must(t, dir, nil, "git", "restore", "bundles/toolbox/tools.lock.json")
	remote := filepath.Join(t.TempDir(), "remote.git")
	must(t, dir, nil, "git", "init", "--bare", remote)
	must(t, dir, nil, "git", "remote", "add", "origin", remote)
	out, err = adapter(t, dir, append(env, "DENY_PR=true"), "publish")
	if err == nil || !strings.Contains(out, "Allow GitHub Actions to create and approve pull requests") {
		t.Fatalf("error %v output %s", err, out)
	}
}
func TestWorkflowManualLeastPrivilege(t *testing.T) {
	b, err := os.ReadFile("../../.github/workflows/tool-upgrade.yml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"workflow_dispatch:", "default: true", "contents: read", "persist-credentials: false", "if: ${{ !inputs.dry_run && needs.resolve.outputs.changed == 'true' }}", "pull-requests: write", "scripts/tool-upgrade.sh resolve", "scripts/tool-upgrade.sh publish"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"schedule:", "  push:", "pull_request:", "make build", "go build", "docker ", "tools-publish", "secrets.", "workflow_run:"} {
		if strings.Contains(s, bad) {
			t.Errorf("forbidden %q", bad)
		}
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "uses:") {
			ref := strings.Fields(strings.SplitN(line, "uses:", 2)[1])[0]
			parts := strings.Split(ref, "@")
			if len(parts) != 2 || len(parts[1]) != 40 {
				t.Errorf("unpinned action %s", line)
			}
		}
	}
	resolve := strings.Split(strings.Split(s, "  resolve:\n")[1], "  publish:\n")[0]
	if strings.Contains(resolve, ": write") {
		t.Fatal("resolver has write permission")
	}
}
