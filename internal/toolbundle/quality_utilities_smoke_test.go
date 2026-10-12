package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestQualityUtilitiesSmokeRejectsWrongBehavior(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/quality-utilities-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	stubs := map[string]string{
		"shellcheck": `case "$*" in --version) echo ShellCheck;; *) cat >/dev/null;; esac`,
		"shfmt":      `case "$*" in --version) echo v3.14.1;; *) cat >/dev/null; echo 'echo smoke';; esac`,
		"just":       `case "$*" in --version) echo 'just 1.58.0';; *) echo recipe-smoke;; esac`,
		"ruff":       `case "$*" in --version) echo 'ruff 0.17.0';; *) cat >/dev/null; echo 'x = 1';; esac`,
		"delta":      `case "$*" in --version) echo 'delta 0.20.1';; '--no-gitconfig --paging=never') cat;; *) exit 2;; esac`,
		"watchexec":  `case "$*" in --version) echo 'watchexec 2.8.0';; *) echo watch-smoke;; esac`,
		"hyperfine":  `case "$*" in --version) echo 'hyperfine 2.0.0';; *) while test "$1" != --export-json; do shift; done; shift; echo '{"results":[{"command":"true","times":[0.001]}]}' > "$1";; esac`,
		"age-keygen": `case "$*" in --version) echo v1.3.2;; -o*) echo AGE-SECRET-KEY-SYNTHETIC > "$2";; -y*) echo age1synthetic;; esac`,
		"age":        `case "$*" in --version) echo v1.3.2;; *) while test "$1" != -o; do shift; done; shift; output=$1; shift; cat "$1" > "$output";; esac`,
	}
	for _, broken := range []string{"", "shellcheck", "shfmt", "just", "ruff", "delta", "watchexec", "hyperfine", "age-keygen", "age"} {
		t.Run("broken-"+broken, func(t *testing.T) {
			home := t.TempDir()
			bin := t.TempDir()
			for name, body := range stubs {
				if name == broken {
					body = "echo wrong"
				}
				if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+":"+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if broken == "" && err != nil {
				t.Fatalf("valid synthetic tools failed: %v\n%s", err, out)
			}
			if broken != "" && err == nil {
				t.Fatalf("broken %s passed", broken)
			}
		})
	}
}
