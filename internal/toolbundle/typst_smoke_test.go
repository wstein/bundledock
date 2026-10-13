package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTypstSmokeVersionOnlyAndIsolated(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/typst-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"typst 0.15.1", "wrong"} {
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
test -z "${TYPST_FONT_PATHS:-}"
printf '%s\n' '` + version + `'
`
			if err := os.WriteFile(filepath.Join(bin, "typst"), []byte(stub), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "CALLER_HOME="+os.Getenv("HOME"), "TYPST_FONT_PATHS=/host/cheatsheets")
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
