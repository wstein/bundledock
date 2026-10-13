package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNaviSmokeVersionOnlyAndIsolated(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/navi-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"navi 2.24.0", "wrong"} {
		t.Run(version, func(t *testing.T) {
			bin := t.TempDir()
			stub := `#!/bin/sh
set -eu
test "$#" = 1 && test "$1" = --version
test -z "${CALLER_HOME:-}"
test "$PWD" = "$HOME"
test "$XDG_CONFIG_HOME" = "$HOME/.config"
test "$XDG_CACHE_HOME" = "$HOME/.cache"
test "$XDG_DATA_HOME" = "$HOME/.local/share"
test -z "${NAVI_PATH:-}"
printf '%s\n' '` + version + `'
`
			if err := os.WriteFile(filepath.Join(bin, "navi"), []byte(stub), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "CALLER_HOME="+os.Getenv("HOME"), "NAVI_PATH=/host/cheatsheets")
			out, err := cmd.CombinedOutput()
			if version != "wrong" && err != nil {
				t.Fatalf("isolated version failed: %v\n%s", err, out)
			}
			if version == "wrong" && err == nil {
				t.Fatal("invalid version accepted")
			}
		})
	}
}
