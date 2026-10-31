package toolpublish

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bundledock/internal/testdir"
)

// OpenTofu is retired (2026-10-31): no dispatch choice for publication or the updater.
func TestOpenTofuIsAbsentFromAutomaticWorkflowChoices(t *testing.T) {
	for _, file := range []string{"publish-bundles.yml", "propose-upgrades.yml"} {
		data, err := os.ReadFile("../../.github/workflows/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "          - opentofu") {
			t.Fatal(file + " still offers the retired opentofu bundle")
		}
	}
	data, err := os.ReadFile("../../.github/workflows/staging-cleanup.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "        repository: [agy, bun, claude, code-server, codex, nixbox, node, opentofu, toolbox]\n") {
		t.Fatal("staging cleanup must keep cleaning stale staging tags of the retired repository")
	}
}

func TestOpenTofuSmokeAdapterUsesOwnReadOnlyVolume(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip(err)
	}
	dir := testdir.New(t)
	log := filepath.Join(dir, "calls")
	docker := `#!/bin/sh
printf '%s\n' "$*" >> "$SMOKE_CALLS"
case "$1" in
image) printf 'sha256:fixture\n' ;;
volume) if [ "$2" = create ]; then printf 'fixture-volume\n'; fi ;;
run)
case "$*" in
*sha256:fixture*) case "$*" in *'dst=/bundle,volume-nocopy sha256:fixture'|*'sha256:fixture --license licenses/opentofu/LICENSE'|*'sha256:fixture --license licenses/opentofu/README.md'|*'sha256:fixture --license licenses/opentofu/CHANGELOG.md'|*'sha256:fixture --license licenses/opentofu/SOURCE') ;; *) exit 31 ;; esac ;;
*consumer:fixture*) case "$*" in *'dst=/opt/bdock/opentofu,readonly,volume-nocopy'*'consumer:fixture /opentofu-smoke.sh') ;; *) exit 32 ;; esac ;;
*) exit 33 ;;
esac ;;
*) exit 34 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(docker), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "../../bundles/opentofu/scripts/smoke.sh", "carrier:fixture", "consumer:fixture")
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "SMOKE_CALLS="+log)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("smoke adapter: %v %s", err, b)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Count(s, "dst=/bundle,volume-nocopy sha256:fixture") != 2 || strings.Count(s, "consumer:fixture /opentofu-smoke.sh") != 2 || !strings.Contains(s, "volume rm fixture-volume") {
		t.Fatal("missing initialization/concurrent consumers/cleanup: " + s)
	}
	for _, notice := range []string{"LICENSE", "README.md", "CHANGELOG.md", "SOURCE"} {
		if !strings.Contains(s, "--license licenses/opentofu/"+notice) {
			t.Fatal("missing verified notice read: " + notice)
		}
	}
	source, err := os.ReadFile("../../bundles/opentofu/scripts/consumer-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tofu version", "tofu init -backend=false", "tofu validate", "tofu plan", "tofu apply", "TF_PLUGIN_CACHE_DIR", "TF_DATA_DIR", "terraform.tfstate", "unexpected write", "config-before"} {
		if !strings.Contains(string(source), want) {
			t.Error("consumer smoke missing " + want)
		}
	}
}
