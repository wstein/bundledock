package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"bundledock/internal/testdir"
)

func TestStwSmokeVersionOnlyAndIsolated(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/stw-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"SourceSteward v0.1.0", "SourceSteward 0.1.0", "wrong"} {
		t.Run(version, func(t *testing.T) {
			bin := testdir.New(t)
			// stw version prints "SourceSteward vX.Y.Z".
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
			if err := os.WriteFile(filepath.Join(bin, "stw"), []byte(stub), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "CALLER_HOME="+os.Getenv("HOME"), "TMPDIR="+bin)
			out, err := cmd.CombinedOutput()
			if version == "SourceSteward v0.1.0" && err != nil {
				t.Fatalf("isolated version failed: %v\n%s", err, out)
			}
			if version != "SourceSteward v0.1.0" && err == nil {
				t.Fatal("invalid version accepted")
			}
		})
	}
}
