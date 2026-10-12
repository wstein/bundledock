package toolpublish

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeServerWorkflowSourcesSeparateApprovalAndArtifacts(t *testing.T) {
	for _, file := range []string{"bundledock-code-server.yml", "code-server-upgrade.yml"} {
		data, err := os.ReadFile("../../.github/workflows/" + file)
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		for _, required := range []string{"bundle: code-server", "workflow_dispatch:"} {
			if !strings.Contains(source, required) {
				t.Fatal(file + " missing " + required)
			}
		}
		if strings.HasPrefix(file, "bundledock-") {
			for _, required := range []string{"name: Publish code-server", "uses: ./.github/workflows/bundle-publish.yml", "workflows: [CI]", "approved_sha:"} {
				if !strings.Contains(source, required) {
					t.Fatal(file + " missing " + required)
				}
			}
			if strings.Contains(source, "cron:") {
				t.Fatal("publication must have no schedule")
			}
		} else {
			for _, required := range []string{"name: Propose code-server upgrades", "uses: ./.github/workflows/bundle-upgrade.yml", "- cron: '50 6 * * *'", "default: true"} {
				if !strings.Contains(source, required) {
					t.Fatal(file + " missing " + required)
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
	cmd := exec.Command("sh", "../../bundles/code-server/scripts/smoke.sh", "carrier:fixture", "consumer:fixture")
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
	source, err := os.ReadFile("../../bundles/code-server/scripts/consumer-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--auth password", "--bind-addr 127.0.0.1:", "/login", "payload/lib/node", "ThirdPartyNotices.txt", "config.yaml", "Unexpected write", "readlink"} {
		if !strings.Contains(string(source), want) {
			t.Error("consumer smoke missing " + want)
		}
	}
}
