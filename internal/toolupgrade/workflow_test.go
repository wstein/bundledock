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
	for _, tool := range []string{"git", "bash", "sh", "python3"} {
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
	for _, name := range []string{"bundles/toolbox/tools.lock.json", "bundles/toolbox/tools.packages.json", "docs/tools.md", "scripts/tool-inventory.py", "bundles/code-server/tools.txt", "bundles/code-server/tools.lock.json", "bundles/code-server/tools.packages.json", "docs/code-server.md", "bundles/node/tools.txt", "bundles/node/tools.lock.json", "bundles/node/tools.packages.json", "docs/node.md", "bundles/bun/tools.txt", "bundles/bun/tools.lock.json", "bundles/bun/tools.packages.json", "docs/bun.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			t.Fatal(err)
		}
		write(t, destination, string(data))
	}
	write(t, filepath.Join(dir, "bundles/toolbox/tools.txt"), "gh>=2.0.0,<3.0.0\n")
	must(t, dir, nil, "git", "add", ".")
	must(t, dir, nil, "git", "commit", "-m", "fixture")
	fake := t.TempDir()
	write(t, filepath.Join(fake, "go"), `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$CALLS"
case "$*" in 'run ./cmd/bundledock tools upgrade gh'|'run ./cmd/bundledock tools upgrade'|'run ./cmd/bundledock tools upgrade code-server --directory bundles/code-server'|'run ./cmd/bundledock tools upgrade --directory bundles/code-server'|'run ./cmd/tools-publish validate --bundle code-server'|'run ./cmd/bundledock tools upgrade node --directory bundles/node'|'run ./cmd/bundledock tools upgrade --directory bundles/node'|'run ./cmd/tools-publish validate --bundle node'|'run ./cmd/bundledock tools upgrade bun --directory bundles/bun'|'run ./cmd/bundledock tools upgrade --directory bundles/bun'|'run ./cmd/tools-publish validate --bundle bun') ;; *) exit 41 ;; esac
case "$*" in 'run ./cmd/tools-publish validate --bundle code-server'|'run ./cmd/tools-publish validate --bundle node'|'run ./cmd/tools-publish validate --bundle bun') exit 0 ;; esac
case "$*" in *'--directory bundles/bun') export RESOLVED_BUNDLE=bun ;; *'--directory bundles/node') export RESOLVED_BUNDLE=node ;; *'--directory bundles/code-server') export RESOLVED_BUNDLE=code-server ;; *) export RESOLVED_BUNDLE=toolbox ;; esac
if [ "${FAIL_RESOLVER:-false}" = true ]; then exit 42; fi
if [ "${NO_CHANGE:-false}" != true ]; then
python3 - <<'PYTHON'
import json,os
from pathlib import Path
bundle=os.environ['RESOLVED_BUNDLE']
p=Path('bundles')/bundle/'tools.lock.json'
lock=json.loads(p.read_text())
if os.environ.get('NO_CHANGE_'+bundle.upper().replace('-','_'))!='true':
 for arch in ('amd64','arm64'): lock['platforms'][arch]['gh' if bundle=='toolbox' else bundle]['version']={'code-server':'v4.142.0','node':'v24.22.0','toolbox':'v2.103.0','bun':'bun-v1.4.4'}[bundle]
p.write_text(json.dumps(lock,indent=2)+'\n')
PYTHON
fi
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
	result, err := os.ReadFile(filepath.Join(value(env, "UPGRADE_RESULT"), "toolbox.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), `"version": "v2.103.0"`) {
		t.Fatalf("result %s", result)
	}
	must(t, dir, nil, "git", "restore", "bundles/toolbox/tools.lock.json", "docs/tools.md")
	remote := filepath.Join(t.TempDir(), "remote.git")
	must(t, dir, nil, "git", "init", "--bare", remote)
	must(t, dir, nil, "git", "remote", "add", "origin", remote)
	out, err = adapter(t, dir, env, "publish")
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}
	got := must(t, dir, nil, "git", "show", "HEAD:bundles/toolbox/tools.lock.json")
	if !strings.Contains(got, `"version": "v2.103.0"`) {
		t.Fatal(got)
	}
	inventory := must(t, dir, nil, "git", "show", "HEAD:docs/tools.md")
	if !strings.Contains(inventory, "| GitHub CLI | 2.103.0 |") {
		t.Fatal("PR inventory missing updated version")
	}
	if branch := must(t, dir, nil, "git", "branch", "--show-current"); branch != "automation/toolbox-upgrade-123-1\n" {
		t.Fatal(branch)
	}
	if title := must(t, dir, nil, "git", "log", "-1", "--format=%s"); title != "chore(toolbox): upgrade stable toolbox lock\n" {
		t.Fatal(title)
	}
	body, err := os.ReadFile(filepath.Join(value(env, "UPGRADE_RESULT"), "pr-body.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"wstein", "exact PR head", "source CI", "protected merge", "frozen-lock publication", "without further intervention"} {
		if !strings.Contains(string(body), text) {
			t.Fatalf("approval body missing %q: %s", text, body)
		}
	}
	if strings.Contains(string(body), "Run CI manually") || strings.Contains(string(body), "Those operations need separate explicit approval") {
		t.Fatalf("approval body contradicts single approval: %s", body)
	}
	calls, err := os.ReadFile(value(env, "CALLS"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "pr create --repo example/repo --base main --head automation/toolbox-upgrade-123-1 --title chore(toolbox): upgrade stable toolbox lock") {
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
	must(t, dir, nil, "git", "restore", "bundles/toolbox/tools.lock.json", "docs/tools.md")
	remote := filepath.Join(t.TempDir(), "remote.git")
	must(t, dir, nil, "git", "init", "--bare", remote)
	must(t, dir, nil, "git", "remote", "add", "origin", remote)
	out, err = adapter(t, dir, append(env, "DENY_PR=true"), "publish")
	if err == nil || !strings.Contains(out, "Allow GitHub Actions to create and approve pull requests") {
		t.Fatalf("error %v output %s", err, out)
	}
}
func TestWorkflowManualLeastPrivilege(t *testing.T) {
	b, err := upgradeWorkflowSource()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"workflow_dispatch:", "default: true", "contents: read", "persist-credentials: false", "pull-requests: write", "scripts/tool-upgrade.sh resolve", "scripts/tool-upgrade.sh publish"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"  push:", "pull_request:", "make build", "go build", "docker ", "tools-publish", "secrets.", "workflow_run:"} {
		if strings.Contains(s, bad) {
			t.Errorf("forbidden %q", bad)
		}
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "uses:") {
			ref := strings.Fields(strings.SplitN(line, "uses:", 2)[1])[0]
			if ref == "./.github/workflows/bundle-upgrade.yml" {
				continue
			}
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

func TestDocsOnlyDriftRepair(t *testing.T) {
	dir, env := fixture(t)
	path := filepath.Join(dir, "docs/tools.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	write(t, path, strings.Replace(string(b), "| GitHub CLI | 2.102.0 |", "| GitHub CLI | 0.0.0 |", 1))
	must(t, dir, nil, "git", "add", "docs/tools.md")
	must(t, dir, nil, "git", "commit", "-m", "stale inventory fixture")
	out, err := adapter(t, dir, append(env, "NO_CHANGE=true"), "resolve")
	if err != nil {
		t.Fatalf("resolve: %v %s", err, out)
	}
	outputs, err := os.ReadFile(value(env, "GITHUB_OUTPUT"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(outputs), "changed=true") {
		t.Fatalf("docs drift ignored: %s", outputs)
	}
	must(t, dir, nil, "git", "restore", "docs/tools.md")
	remote := filepath.Join(t.TempDir(), "remote.git")
	must(t, dir, nil, "git", "init", "--bare", remote)
	must(t, dir, nil, "git", "remote", "add", "origin", remote)
	out, err = adapter(t, dir, env, "publish")
	if err != nil {
		t.Fatalf("publish: %v %s", err, out)
	}
	names := must(t, dir, nil, "git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	if names != "docs/tools.md\n" {
		t.Fatalf("docs-only repair changed %s", names)
	}
}
func TestPublisherRejectsUnrelatedDocumentation(t *testing.T) {
	dir, env := fixture(t)
	out, err := adapter(t, dir, env, "resolve")
	if err != nil {
		t.Fatalf("resolve: %v %s", err, out)
	}
	artifact := filepath.Join(value(env, "UPGRADE_RESULT"), "toolbox.md")
	b, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	write(t, artifact, string(b)+"\nUnexpected unrelated prose edit.\n")
	must(t, dir, nil, "git", "restore", "bundles/toolbox/tools.lock.json", "docs/tools.md")
	out, err = adapter(t, dir, env, "publish")
	if err == nil || !strings.Contains(out, "generated inventory") {
		t.Fatalf("unrelated documentation accepted: %v %s", err, out)
	}
	if branch := must(t, dir, nil, "git", "branch", "--show-current"); branch != "main\n" {
		t.Fatal("created branch before rejecting artifact")
	}
}

func TestWorkflowDailyAndManualWriteGates(t *testing.T) {
	b, err := upgradeWorkflowSource()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Count(s, "- cron: '50 6 * * *'") != 1 {
		t.Fatal("expected exactly one daily 06:50 UTC schedule")
	}
	for _, changed := range []string{"steps.resolve.outputs.changed", "needs.resolve.outputs.changed"} {
		gate := "if: ${{ " + changed + " == 'true' && (github.event_name == 'schedule' || (github.event_name == 'workflow_dispatch' && !inputs.dry_run)) }}"
		if !strings.Contains(s, gate) {
			t.Errorf("missing explicit changed-and-event write gate %q", gate)
		}
	}
	if !strings.Contains(s, "if: (github.event_name == 'schedule' || github.event_name == 'workflow_dispatch') && github.ref == format('refs/heads/{0}', github.event.repository.default_branch)") {
		t.Fatal("resolver must allow only schedule/manual on default branch")
	}
	if !strings.Contains(s, "TOOL: ${{ inputs.tool }}") {
		t.Fatal("scheduled runs must resolve all tools independently of manual inputs")
	}
}
func TestPublisherNoChangeCreatesNothing(t *testing.T) {
	dir, env := fixture(t)
	for _, pair := range [][2]string{{"bundles/toolbox/tools.lock.json", "toolbox.lock.json"}, {"docs/tools.md", "toolbox.md"}} {
		data, err := os.ReadFile(filepath.Join(dir, pair[0]))
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(value(env, "UPGRADE_RESULT"), pair[1]), string(data))
	}
	out, err := adapter(t, dir, env, "publish")
	if err != nil {
		t.Fatalf("no-op publish: %v %s", err, out)
	}
	if branch := must(t, dir, nil, "git", "branch", "--show-current"); branch != "main\n" {
		t.Fatal("no-op created topic")
	}
	if _, err := os.Stat(value(env, "CALLS")); !os.IsNotExist(err) {
		t.Fatal("no-op invoked external resolver or PR command")
	}
}

func TestCodeServerResolveUsesIndependentInputsAndInventory(t *testing.T) {
	dir, env := fixture(t)
	env = append(env, "BUNDLE=code-server", "TOOL=code-server")
	before := must(t, dir, nil, "git", "show", "HEAD:bundles/toolbox/tools.lock.json")
	out, err := adapter(t, dir, env, "resolve")
	if err != nil {
		t.Fatalf("code-server resolve: %v %s", err, out)
	}
	if got := must(t, dir, nil, "git", "diff", "--", "bundles/toolbox/tools.lock.json", "docs/tools.md"); got != "" {
		t.Fatal("code-server changed toolbox: " + got)
	}
	if got := must(t, dir, nil, "git", "show", "HEAD:bundles/toolbox/tools.lock.json"); got != before {
		t.Fatal("changed baseline")
	}
	for name, want := range map[string]string{"code-server.lock.json": "v4.142.0", "code-server.md": "| code-server | 4.142.0 |"} {
		b, err := os.ReadFile(filepath.Join(value(env, "UPGRADE_RESULT"), name))
		if err != nil || !strings.Contains(string(b), want) {
			t.Fatalf("artifact %s: %v %s", name, err, b)
		}
	}
}

func TestCodeServerArtifactFailuresStopBeforeTopicCreation(t *testing.T) {
	for _, kind := range []string{"stale base", "extra artifact", "linked artifact", "prose edit", "wrong architecture"} {
		t.Run(kind, func(t *testing.T) {
			dir, env := fixture(t)
			env = append(env, "BUNDLE=code-server", "TOOL=code-server")
			if out, err := adapter(t, dir, env, "resolve"); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			result := value(env, "UPGRADE_RESULT")
			switch kind {
			case "stale base":
				write(t, filepath.Join(result, "base.sha"), strings.Repeat("a", 40)+"\n")
			case "extra artifact":
				write(t, filepath.Join(result, "other.lock.json"), "unexpected")
			case "linked artifact":
				p := filepath.Join(result, "code-server.lock.json")
				if err := os.Rename(p, p+".saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(p+".saved", p); err != nil {
					t.Fatal(err)
				}
			case "prose edit":
				p := filepath.Join(result, "code-server.md")
				b, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				write(t, p, string(b)+"unrelated prose\n")
			case "wrong architecture":
				p := filepath.Join(result, "code-server.lock.json")
				b, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				write(t, p, strings.Replace(string(b), "v4.142.0", "v4.143.0", 1))
			}
			must(t, dir, nil, "git", "restore", "bundles/code-server/tools.lock.json", "docs/code-server.md")
			if out, err := adapter(t, dir, env, "publish"); err == nil {
				t.Fatalf("accepted %s: %s", kind, out)
			}
			if branch := must(t, dir, nil, "git", "branch", "--show-current"); branch != "main\n" {
				t.Fatal("created topic before rejection")
			}
		})
	}
}
func TestCodeServerNoChangeCreatesNoProposal(t *testing.T) {
	dir, env := fixture(t)
	env = append(env, "BUNDLE=code-server", "TOOL=", "NO_CHANGE=true")
	if out, err := adapter(t, dir, env, "resolve"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	b, err := os.ReadFile(value(env, "GITHUB_OUTPUT"))
	if err != nil || !strings.Contains(string(b), "changed=false") {
		t.Fatalf("outputs %v %s", err, b)
	}
	if _, err := os.Stat(filepath.Join(value(env, "UPGRADE_RESULT"), "code-server.lock.json")); !os.IsNotExist(err) {
		t.Fatal("no-op emitted proposal")
	}
}

func TestNodeResolveUsesIndependentInputsAndInventory(t *testing.T) {
	dir, env := fixture(t)
	env = append(env, "BUNDLE=node", "TOOL=node")
	before := must(t, dir, nil, "git", "show", "HEAD:bundles/toolbox/tools.lock.json")
	out, err := adapter(t, dir, env, "resolve")
	if err != nil {
		t.Fatalf("node resolve: %v %s", err, out)
	}
	if got := must(t, dir, nil, "git", "diff", "--", "bundles/toolbox/tools.lock.json", "docs/tools.md"); got != "" {
		t.Fatal("node changed toolbox: " + got)
	}
	if got := must(t, dir, nil, "git", "show", "HEAD:bundles/toolbox/tools.lock.json"); got != before {
		t.Fatal("changed baseline")
	}
	for name, want := range map[string]string{"node.lock.json": "v24.22.0", "node.md": "| node | 24.22.0 |"} {
		b, err := os.ReadFile(filepath.Join(value(env, "UPGRADE_RESULT"), name))
		if err != nil || !strings.Contains(string(b), want) {
			t.Fatalf("artifact %s: %v %s", name, err, b)
		}
	}
}

func TestNodeArtifactFailuresStopBeforeTopicCreation(t *testing.T) {
	for _, kind := range []string{"stale base", "extra artifact", "linked artifact", "prose edit", "wrong architecture"} {
		t.Run(kind, func(t *testing.T) {
			dir, env := fixture(t)
			env = append(env, "BUNDLE=node", "TOOL=node")
			if out, err := adapter(t, dir, env, "resolve"); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			result := value(env, "UPGRADE_RESULT")
			switch kind {
			case "stale base":
				write(t, filepath.Join(result, "base.sha"), strings.Repeat("a", 40)+"\n")
			case "extra artifact":
				write(t, filepath.Join(result, "other.lock.json"), "unexpected")
			case "linked artifact":
				p := filepath.Join(result, "node.lock.json")
				if err := os.Rename(p, p+".saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(p+".saved", p); err != nil {
					t.Fatal(err)
				}
			case "prose edit":
				p := filepath.Join(result, "node.md")
				b, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				write(t, p, string(b)+"unrelated prose\n")
			case "wrong architecture":
				p := filepath.Join(result, "node.lock.json")
				b, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				write(t, p, strings.Replace(string(b), "v24.22.0", "v24.23.0", 1))
			}
			must(t, dir, nil, "git", "restore", "bundles/node/tools.lock.json", "docs/node.md")
			if out, err := adapter(t, dir, env, "publish"); err == nil {
				t.Fatalf("accepted %s: %s", kind, out)
			}
			if branch := must(t, dir, nil, "git", "branch", "--show-current"); branch != "main\n" {
				t.Fatal("created topic before rejection")
			}
		})
	}
}
func TestNodeNoChangeCreatesNoProposal(t *testing.T) {
	dir, env := fixture(t)
	env = append(env, "BUNDLE=node", "TOOL=", "NO_CHANGE=true")
	if out, err := adapter(t, dir, env, "resolve"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	b, err := os.ReadFile(value(env, "GITHUB_OUTPUT"))
	if err != nil || !strings.Contains(string(b), "changed=false") {
		t.Fatalf("outputs %v %s", err, b)
	}
	if _, err := os.Stat(filepath.Join(value(env, "UPGRADE_RESULT"), "node.lock.json")); !os.IsNotExist(err) {
		t.Fatal("no-op emitted proposal")
	}
}

func TestBunResolveUsesIndependentInputsAndInventory(t *testing.T) {
	dir, env := fixture(t)
	env = append(env, "BUNDLE=bun", "TOOL=bun")
	before := must(t, dir, nil, "git", "show", "HEAD:bundles/toolbox/tools.lock.json")
	out, err := adapter(t, dir, env, "resolve")
	if err != nil {
		t.Fatalf("bun resolve: %v %s", err, out)
	}
	if got := must(t, dir, nil, "git", "diff", "--", "bundles/toolbox/tools.lock.json", "docs/tools.md"); got != "" {
		t.Fatal("bun changed toolbox: " + got)
	}
	if got := must(t, dir, nil, "git", "show", "HEAD:bundles/toolbox/tools.lock.json"); got != before {
		t.Fatal("changed baseline")
	}
	for name, want := range map[string]string{"bun.lock.json": "bun-v1.4.4", "bun.md": "| bun | 1.4.4 |"} {
		b, err := os.ReadFile(filepath.Join(value(env, "UPGRADE_RESULT"), name))
		if err != nil || !strings.Contains(string(b), want) {
			t.Fatalf("artifact %s: %v %s", name, err, b)
		}
	}
}

func TestBunArtifactFailuresStopBeforeTopicCreation(t *testing.T) {
	for _, kind := range []string{"stale base", "extra artifact", "linked artifact", "prose edit", "wrong architecture"} {
		t.Run(kind, func(t *testing.T) {
			dir, env := fixture(t)
			env = append(env, "BUNDLE=bun", "TOOL=bun")
			if out, err := adapter(t, dir, env, "resolve"); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			result := value(env, "UPGRADE_RESULT")
			switch kind {
			case "stale base":
				write(t, filepath.Join(result, "base.sha"), strings.Repeat("a", 40)+"\n")
			case "extra artifact":
				write(t, filepath.Join(result, "other.lock.json"), "unexpected")
			case "linked artifact":
				p := filepath.Join(result, "bun.lock.json")
				if err := os.Rename(p, p+".saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(p+".saved", p); err != nil {
					t.Fatal(err)
				}
			case "prose edit":
				p := filepath.Join(result, "bun.md")
				b, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				write(t, p, string(b)+"unrelated prose\n")
			case "wrong architecture":
				p := filepath.Join(result, "bun.lock.json")
				b, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				write(t, p, strings.Replace(string(b), "bun-v1.4.4", "bun-v1.4.5", 1))
			}
			must(t, dir, nil, "git", "restore", "bundles/bun/tools.lock.json", "docs/bun.md")
			if out, err := adapter(t, dir, env, "publish"); err == nil {
				t.Fatalf("accepted %s: %s", kind, out)
			}
			if branch := must(t, dir, nil, "git", "branch", "--show-current"); branch != "main\n" {
				t.Fatal("created topic before rejection")
			}
		})
	}
}
func TestBunNoChangeCreatesNoProposal(t *testing.T) {
	dir, env := fixture(t)
	env = append(env, "BUNDLE=bun", "TOOL=", "NO_CHANGE=true")
	if out, err := adapter(t, dir, env, "resolve"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	b, err := os.ReadFile(value(env, "GITHUB_OUTPUT"))
	if err != nil || !strings.Contains(string(b), "changed=false") {
		t.Fatalf("outputs %v %s", err, b)
	}
	if _, err := os.Stat(filepath.Join(value(env, "UPGRADE_RESULT"), "bun.lock.json")); !os.IsNotExist(err) {
		t.Fatal("no-op emitted proposal")
	}
}

func upgradeWorkflowSource() ([]byte, error) {
	wrapper, err := os.ReadFile("../../.github/workflows/toolbox-upgrade.yml")
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile("../../.github/workflows/bundle-upgrade.yml")
	if err != nil {
		return nil, err
	}
	return append(wrapper, body...), nil
}
