package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSmillSmokeVersionOnlyAndIsolated(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/smill-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"smill v0.1.1", "smill 0.1.1", "wrong"} {
		t.Run(version, func(t *testing.T) {
			bin := t.TempDir()
			// smill has no --version flag; its version subcommand prints "smill vX.Y.Z".
			stub := `#!/bin/sh
set -eu
test "$#" = 1 && test "$1" = version
test -z "${CALLER_HOME:-}"
test "$PWD" = "$HOME"
test "$XDG_CONFIG_HOME" = "$HOME/.config"
test "$XDG_CACHE_HOME" = "$HOME/.cache"
test "$XDG_DATA_HOME" = "$HOME/.local/share"
printf '%s\n' '` + version + `'
`
			if err := os.WriteFile(filepath.Join(bin, "smill"), []byte(stub), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "CALLER_HOME="+os.Getenv("HOME"))
			out, err := cmd.CombinedOutput()
			if version == "smill v0.1.1" && err != nil {
				t.Fatalf("isolated version failed: %v\n%s", err, out)
			}
			if version != "smill v0.1.1" && err == nil {
				t.Fatal("invalid version accepted")
			}
		})
	}
}
