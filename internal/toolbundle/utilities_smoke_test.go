package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestUtilitiesSmokeRejectsWrongOutput(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/utilities-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, broken := range []string{"", "fzf", "bat", "regctl"} {
		t.Run("broken-"+broken, func(t *testing.T) {
			home := t.TempDir()
			bin := t.TempDir()
			stubs := map[string]string{
				"fzf":    "case \"$*\" in --version) echo 0.74.5;; '--filter beta') cat >/dev/null; echo beta;; *) exit 2;; esac",
				"bat":    "case \"$*\" in --version) echo 'bat 0.26.1';; '--paging=never --color=never --style=plain --language=go -') cat;; *) exit 2;; esac",
				"regctl": "case \"$*\" in version) echo 'VCSTag: v0.11.6';; --help) echo help;; 'image digest --help') echo help;; 'ref --format {{.Repository}} example.invalid/team/image:v1') echo team/image;; *) exit 2;; esac",
			}
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
