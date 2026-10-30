package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bundledock/internal/testdir"
)

// Synthetic commands verify the smoke script's argument and failure handling;
// they do not establish native executable or consumer compatibility.
func TestHTTPDiskEnvSecretsSmoke(t *testing.T) {
	for _, tool := range []string{"sh", "timeout", "grep"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("synthetic shell smoke requires host tool %s: %v", tool, err)
		}
	}
	tools := map[string]string{"xh": "xh 0.26.2", "dust": "Dust 1.2.6", "direnv": "2.38.2", "sops": "sops 3.13.3"}
	for _, broken := range []string{"", "xh", "dust", "direnv", "sops"} {
		for _, mode := range []string{"valid", "wrong-output", "failed-status"} {
			if broken == "" && mode != "valid" || broken != "" && mode == "valid" {
				continue
			}
			t.Run(broken+"/"+mode, func(t *testing.T) {
				bin, home := testdir.New(t), testdir.New(t)
				for tool, version := range tools {
					args := "--version"
					if tool == "direnv" {
						args = "version"
					}
					if tool == "sops" {
						args = "--disable-version-check --version"
					}
					body := "#!/bin/sh\nset -eu\ntest \"$*\" = \"" + args + "\" || exit 42\nprintf '%s\\n' '" + version + "'\n"
					if tool == broken && mode == "wrong-output" {
						body = "#!/bin/sh\necho unrelated\n"
					}
					if tool == broken && mode == "failed-status" {
						body += "exit 23\n"
					}
					if err := os.WriteFile(filepath.Join(bin, tool), []byte(body), 0755); err != nil {
						t.Fatal(err)
					}
				}
				script, err := filepath.Abs("../../bundles/toolbox/scripts/http-disk-env-secrets-smoke.sh")
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("sh", script)
				cmd.Dir = home
				cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+":"+os.Getenv("PATH"))
				output, err := cmd.CombinedOutput()
				if broken == "" {
					if err != nil {
						t.Fatalf("valid commands failed: %v\n%s", err, output)
					}
					return
				}
				if err == nil {
					t.Fatalf("accepted %s %s", broken, mode)
				}
				if mode == "failed-status" && !strings.Contains(string(output), "failed: "+broken+" version (exit 23)") {
					t.Fatalf("lost diagnostic: %s", output)
				}
			})
		}
	}
}
