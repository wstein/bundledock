package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestToolboxTofuSmokeVersionOnlyAndIsolated(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/tofu-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"OpenTofu v1.13.2", "OpenTofu 1.13.2", "wrong"} {
		t.Run(version, func(t *testing.T) {
			bin := t.TempDir()
			// tofu version prints "OpenTofu vX.Y.Z".
			stub := `#!/bin/sh
set -eu
test "$#" = 1 && test "$1" = version
test "${CHECKPOINT_DISABLE:-}" = 1
test -z "${CALLER_HOME:-}"
test "$PWD" = "$HOME"
test "$XDG_CONFIG_HOME" = "$HOME/.config"
test "$XDG_CACHE_HOME" = "$HOME/.cache"
test "$XDG_DATA_HOME" = "$HOME/.local/share"
printf '%s\n' '` + version + `'
`
			if err := os.WriteFile(filepath.Join(bin, "tofu"), []byte(stub), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "CALLER_HOME="+os.Getenv("HOME"))
			out, err := cmd.CombinedOutput()
			if version == "OpenTofu v1.13.2" && err != nil {
				t.Fatalf("isolated version failed: %v\n%s", err, out)
			}
			if version != "OpenTofu v1.13.2" && err == nil {
				t.Fatal("invalid version accepted")
			}
		})
	}
}
