package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMisspellSmokeVersionOnlyAndIsolated(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/misspell-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"0.8.0", "v0.8.0", "dev", "wrong"} {
		t.Run(version, func(t *testing.T) {
			bin := t.TempDir()
			// misspell -v prints the bare version (goreleaser sets main.version to X.Y.Z).
			stub := `#!/bin/sh
set -eu
test "$#" = 1 && test "$1" = -v
test -z "${CALLER_HOME:-}"
test "$PWD" = "$HOME"
test "$XDG_CONFIG_HOME" = "$HOME/.config"
test "$XDG_CACHE_HOME" = "$HOME/.cache"
test "$XDG_DATA_HOME" = "$HOME/.local/share"
printf '%s\n' '` + version + `'
`
			if err := os.WriteFile(filepath.Join(bin, "misspell"), []byte(stub), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "CALLER_HOME="+os.Getenv("HOME"))
			out, err := cmd.CombinedOutput()
			if version == "0.8.0" && err != nil {
				t.Fatalf("isolated version failed: %v\n%s", err, out)
			}
			if version != "0.8.0" && err == nil {
				t.Fatal("invalid version accepted")
			}
		})
	}
}
