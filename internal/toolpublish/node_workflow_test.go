package toolpublish

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeWorkflowSourcesSeparateApprovalAndArtifacts(t *testing.T) {
	for _, bundle := range []string{"node", "node-current"} {
		for _, file := range []string{"bundledock-" + bundle + ".yml", bundle + "-upgrade.yml"} {
			data, err := os.ReadFile("../../.github/workflows/" + file)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			for _, required := range []string{"bundle: " + bundle, "workflow_dispatch:"} {
				if !strings.Contains(source, required) {
					t.Fatal(file + " missing " + required)
				}
			}
			if strings.HasPrefix(file, "bundledock-") {
				for _, required := range []string{"name: Publish " + bundle, "uses: ./.github/workflows/bundle-publish.yml", "workflows: [CI]", "approved_sha:"} {
					if !strings.Contains(source, required) {
						t.Fatal(file + " missing " + required)
					}
				}
				if strings.Contains(source, "cron:") {
					t.Fatal("publication must have no schedule")
				}
			} else {
				for _, required := range []string{"name: Propose " + bundle + " upgrades", "uses: ./.github/workflows/bundle-upgrade.yml", "- cron: '50 6 * * *'", "default: true"} {
					if !strings.Contains(source, required) {
						t.Fatal(file + " missing " + required)
					}
				}
			}
		}
	}
}

func TestNodeSmokeAdapterUsesOwnReadOnlyVolume(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip(err)
	}
	for _, bundle := range []string{"node", "node-current"} {
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
*consumer:fixture*) case "$*" in *'dst=/opt/bdock/node,readonly,volume-nocopy'*'consumer:fixture /node-smoke.sh') ;; *) exit 32 ;; esac ;;
*) exit 33 ;;
esac ;;
*) exit 34 ;;
esac
`
		if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(strings.ReplaceAll(docker, "/opt/bdock/node,", "/opt/bdock/"+bundle+",")), 0700); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", "../../bundles/"+bundle+"/scripts/smoke.sh", "carrier:fixture", "consumer:fixture")
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "SMOKE_CALLS="+log)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("smoke adapter: %v %s", err, b)
		}
		b, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if strings.Count(s, "dst=/bundle,volume-nocopy sha256:fixture") != 2 || strings.Count(s, "consumer:fixture /node-smoke.sh") != 2 || !strings.Contains(s, "volume rm fixture-volume") {
			t.Fatal("missing initialization/concurrent consumers/cleanup: " + s)
		}
		source, err := os.ReadFile("../../bundles/" + bundle + "/scripts/consumer-smoke.sh")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"npm --version", "npx --version", "npm_config_cache", "npm_config_prefix", "--offline", "Unexpected write", "readlink"} {
			if !strings.Contains(string(source), want) {
				t.Error("consumer smoke missing " + want)
			}
		}
	}

}
