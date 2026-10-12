package toolpublish

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeServerWorkflowSourcesSeparateApprovalAndArtifacts(t *testing.T) {
	for name, required := range map[string][]string{
		"bundledock-code-server.yml": {"name: Publish code-server", "workflow_dispatch:", "inputs.approved_sha == github.sha", "scripts/approve-publication.sh", "--bundle code-server", "resolved-code-server-${{ github.run_attempt }}", "code-server-published-${{ matrix.architecture }}-${{ github.run_attempt }}", "ubuntu-24.04-arm"},
		"code-server-upgrade.yml":    {"name: Propose code-server upgrades", "- cron: '50 6 * * *'", "default: true", "BUNDLE: code-server", "code-server-upgrade-${{ github.run_attempt }}", "code-server.lock.json", "code-server.md", "base.sha"},
	} {
		b, err := os.ReadFile("../../.github/workflows/" + name)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, want := range required {
			if !strings.Contains(s, want) {
				t.Errorf("%s missing %s", name, want)
			}
		}
		for _, bad := range []string{"  push:", "pull_request:", "workflow_run:"} {
			if strings.Contains(s, bad) {
				t.Errorf("%s forbidden %s", name, bad)
			}
		}
		if name == "bundledock-code-server.yml" && strings.Contains(s, "cron:") {
			t.Fatal("native publication must remain manual-only")
		}
		for _, line := range strings.Split(s, "\n") {
			if strings.Contains(line, "uses:") {
				ref := strings.Fields(strings.SplitN(line, "uses:", 2)[1])[0]
				parts := strings.Split(ref, "@")
				if len(parts) != 2 || len(parts[1]) != 40 {
					t.Fatal("unpinned action " + line)
				}
			}
		}
	}
}
func TestCodeServerSmokeAdapterUsesOwnReadOnlyVolume(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	docker := `#!/bin/sh
printf '%s\n' "$*" >> "$SMOKE_CALLS"
case "$1" in
image) printf 'sha256:fixture\n' ;;
volume) if [ "$2" = create ]; then printf 'fixture-volume\n'; fi ;;
run)
case "$*" in
*sha256:fixture*) case "$*" in *'dst=/bundle,volume-nocopy sha256:fixture') ;; *) exit 31 ;; esac ;;
*consumer:fixture*) case "$*" in *'dst=/opt/bundledock/code-server,readonly,volume-nocopy'*'consumer:fixture /code-server-smoke.sh') ;; *) exit 32 ;; esac ;;
*) exit 33 ;;
esac ;;
*) exit 34 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(docker), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "../../bundles/code-server/smoke.sh", "carrier:fixture", "consumer:fixture")
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "SMOKE_CALLS="+log)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("smoke adapter: %v %s", err, b)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Count(s, "dst=/bundle,volume-nocopy sha256:fixture") != 2 || strings.Count(s, "consumer:fixture /code-server-smoke.sh") != 2 || !strings.Contains(s, "volume rm fixture-volume") {
		t.Fatal("missing initialization/concurrent consumers/cleanup: " + s)
	}
	source, err := os.ReadFile("../../bundles/code-server/consumer-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--auth password", "--bind-addr 127.0.0.1:", "/login", "payload/lib/node", "ThirdPartyNotices.txt", "config.yaml", "Unexpected write", "readlink"} {
		if !strings.Contains(string(source), want) {
			t.Error("consumer smoke missing " + want)
		}
	}
}
