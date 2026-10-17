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
	for _, broken := range []string{"", "fzf", "bat", "lsd", "regctl", "actionlint", "hadolint", "pinact", "zizmor", "cosign", "git-cliff", "zoxide", "difft", "jsonschema-cli", "fail:actionlint", "fail:hadolint", "fail:pinact", "fail:zizmor", "fail:cosign", "fail:git-cliff", "fail:zoxide", "fail:difft", "fail:jsonschema-cli", "loose:zizmor"} {
		t.Run("broken-"+broken, func(t *testing.T) {
			home := t.TempDir()
			bin := t.TempDir()
			versionEcho := map[string]string{
				"actionlint": "echo '1.7.12'", "hadolint": "echo 'Haskell Dockerfile Linter 2.15.1'", "pinact": "echo 'pinact version 5.0.0'",
				"zizmor": "echo 'zizmor 1.31.1'", "cosign": "echo 'GitVersion:    v3.1.3'", "git-cliff": "echo 'git-cliff 2.14.2'",
				"zoxide": "echo 'zoxide 0.10.0'", "difft": "echo 'Difftastic 0.71.0'", "jsonschema-cli": "echo 'jsonschema-cli 0.58.5'",
			}
			stubs := map[string]string{
				"fzf":            "case \"$*\" in --version) echo 0.74.5;; '--filter beta') cat >/dev/null; echo beta;; *) exit 2;; esac",
				"bat":            "case \"$*\" in --version) echo 'bat 0.26.1';; '--paging=never --color=never --style=plain --language=go -') cat;; *) exit 2;; esac",
				"lsd":            "case \"$*\" in --version) echo 'lsd 1.2.0';; '--ignore-config --color never --icon never --oneline '*) test \"$#\" = 7 && test -f \"$7/entry.txt\" && echo entry.txt;; *) exit 2;; esac",
				"actionlint":     "case \"$*\" in -version) echo '1.7.12';; *) exit 2;; esac",
				"hadolint":       "case \"$*\" in --version) echo 'Haskell Dockerfile Linter 2.15.1';; *) exit 2;; esac",
				"pinact":         "case \"$*\" in --version) echo 'pinact version 5.0.0';; *) exit 2;; esac",
				"zizmor":         "case \"$*\" in --version) echo 'zizmor 1.31.1';; *) exit 2;; esac",
				"cosign":         "case \"$*\" in version) echo 'GitVersion:    v3.1.3';; *) exit 2;; esac",
				"git-cliff":      "case \"$*\" in --version) echo 'git-cliff 2.14.2';; *) exit 2;; esac",
				"zoxide":         "case \"$*\" in --version) echo 'zoxide 0.10.0';; *) exit 2;; esac",
				"difft":          "case \"$*\" in --version) echo 'Difftastic 0.71.0';; *) exit 2;; esac",
				"jsonschema-cli": "case \"$*\" in --version) echo 'jsonschema-cli 0.58.5';; *) exit 2;; esac",
				"regctl":         "case \"$*\" in version) echo 'VCSTag: v0.11.6';; --help) echo help;; 'image digest --help') echo help;; 'ref --format {{.Repository}} example.invalid/team/image:v1') echo team/image;; *) exit 2;; esac",
			}
			for name, body := range stubs {
				switch {
				case name == broken:
					body = "echo wrong"
				case broken == "fail:"+name:
					// Matching text with a failing exit status must not pass.
					body = "case \"$*\" in -version|--version|version) " + versionEcho[name] + "; exit 1;; *) exit 2;; esac"
				case broken == "loose:"+name:
					// A glibc loader error contains a dotted number but is no version banner.
					body = "echo '/lib/libc.so.6: version `GLIBC_2.34.0` not found' >&2; exit 1"
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
