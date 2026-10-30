package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"bundledock/internal/testdir"
)

func TestCscribeSmokeVersionOnlyAndIsolated(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/cscribe-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"cscribe v0.2.0", "cscribe 0.2.0", "wrong"} {
		t.Run(version, func(t *testing.T) {
			bin := testdir.New(t)
			// cscribe has no --version flag; its version subcommand prints "cscribe vX.Y.Z".
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
			if err := os.WriteFile(filepath.Join(bin, "cscribe"), []byte(stub), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "CALLER_HOME="+os.Getenv("HOME"), "TMPDIR="+bin)
			out, err := cmd.CombinedOutput()
			if version == "cscribe v0.2.0" && err != nil {
				t.Fatalf("isolated version failed: %v\n%s", err, out)
			}
			if version != "cscribe v0.2.0" && err == nil {
				t.Fatal("invalid version accepted")
			}
		})
	}
}
