package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUtilitiesSmokeRejectsWrongOutput(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/utilities-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, broken := range []string{"", "fzf", "bat", "lsd", "regctl", "actionlint", "hadolint", "pinact", "zizmor", "cosign", "git-cliff", "zoxide", "difft", "jsonschema-cli", "gh-stack", "gh", "fail:gh-stack", "fail:gh", "missing:gh-stack", "no-gh", "dotted:gh-stack", "prefixed:gh-stack", "fail:actionlint", "fail:hadolint", "fail:pinact", "fail:zizmor", "fail:cosign", "fail:git-cliff", "fail:zoxide", "fail:difft", "fail:jsonschema-cli", "loose:zizmor", "dotted:actionlint", "dotted:hadolint", "dotted:pinact", "dotted:zizmor", "dotted:cosign", "dotted:git-cliff", "dotted:zoxide", "dotted:difft", "dotted:jsonschema-cli", "prefixed:actionlint", "prefixed:hadolint", "prefixed:pinact", "prefixed:zizmor", "prefixed:cosign", "prefixed:git-cliff", "prefixed:zoxide", "prefixed:difft", "prefixed:jsonschema-cli", "keyword:pinact"} {
		t.Run("broken-"+broken, func(t *testing.T) {
			home := t.TempDir()
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			if err := os.MkdirAll(filepath.Join(root, "libexec"), 0755); err != nil || os.MkdirAll(bin, 0755) != nil {
				t.Fatal("cannot create toolbox layout")
			}
			versionEcho := map[string]string{
				"actionlint": "printf '1.7.12\\ninstalled by downloading from release page\\nbuilt with go1.25.0 compiler for linux/amd64\\n'", "hadolint": "echo 'Haskell Dockerfile Linter 2.15.1'", "pinact": "echo 'pinact version 5.0.0'",
				"zizmor": "echo 'zizmor 1.31.1'", "cosign": "echo 'GitVersion:    v3.1.3'", "git-cliff": "echo 'git-cliff 2.14.2'",
				"zoxide": "echo 'zoxide 0.10.0'", "difft": "echo 'Difftastic 0.71.0'", "jsonschema-cli": "echo 'Version: 0.58.5'", "gh-stack": "echo 'gh stack version 0.2.1'",
			}
			stubs := map[string]string{
				"fzf":            "case \"$*\" in --version) echo 0.74.5;; '--filter beta') cat >/dev/null; echo beta;; *) exit 2;; esac",
				"bat":            "case \"$*\" in --version) echo 'bat 0.26.1';; '--paging=never --color=never --style=plain --language=go -') cat;; *) exit 2;; esac",
				"lsd":            "case \"$*\" in --version) echo 'lsd 1.2.0';; '--ignore-config --color never --icon never --oneline '*) test \"$#\" = 7 && test -f \"$7/entry.txt\" && echo entry.txt;; *) exit 2;; esac",
				"actionlint":     "case \"$*\" in -version) printf '1.7.12\\ninstalled by downloading from release page\\nbuilt with go1.25.0 compiler for linux/amd64\\n';; *) exit 2;; esac",
				"hadolint":       "case \"$*\" in --version) echo 'Haskell Dockerfile Linter 2.15.1';; *) exit 2;; esac",
				"pinact":         "case \"$*\" in --version) echo 'pinact version 5.0.0';; *) exit 2;; esac",
				"zizmor":         "case \"$*\" in --version) echo 'zizmor 1.31.1';; *) exit 2;; esac",
				"cosign":         "case \"$*\" in version) echo 'GitVersion:    v3.1.3';; *) exit 2;; esac",
				"git-cliff":      "case \"$*\" in --version) echo 'git-cliff 2.14.2';; *) exit 2;; esac",
				"zoxide":         "case \"$*\" in --version) echo 'zoxide 0.10.0';; *) exit 2;; esac",
				"difft":          "case \"$*\" in --version) echo 'Difftastic 0.71.0';; *) exit 2;; esac",
				"jsonschema-cli": "case \"$*\" in --version) echo 'Version: 0.58.5';; *) exit 2;; esac",
				"gh":             "test \"${GH_PROMPT_DISABLED:-}\" = 1 || exit 3; IFS= read -r _ && exit 4; case \"$*\" in 'stack --version') exec \"$XDG_DATA_HOME/gh/extensions/gh-stack/gh-stack\" --version;; *) exit 2;; esac",
				"gh-stack":       "case \"$*\" in --version) echo 'gh stack version 0.2.1';; *) exit 2;; esac",
				"regctl":         "case \"$*\" in version) echo 'VCSTag: v0.11.6';; --help) echo help;; 'image digest --help') echo help;; 'ref --format {{.Repository}} example.invalid/team/image:v1') echo team/image;; *) exit 2;; esac",
			}
			for name, body := range stubs {
				switch {
				case name == broken:
					body = "echo wrong"
				case broken == "fail:gh" && name == "gh":
					// gh reaches the extension but exits non-zero after printing the banner.
					body = "case \"$*\" in 'stack --version') echo 'gh stack version 0.2.1'; exit 1;; *) exit 2;; esac"
				case broken == "fail:"+name:
					// Matching text with a failing exit status must not pass.
					body = "case \"$*\" in -version|--version|version) " + versionEcho[name] + "; exit 1;; *) exit 2;; esac"
				case broken == "dotted:"+name:
					// Exit 0 with a dotted number outside any anchored banner must fail.
					body = "echo 'warning: unsupported loader 2.34.0'"
				case broken == "prefixed:"+name:
					// The real banner after a prefix is not a version line: the anchor must hold.
					body = "case \"$*\" in -version|--version|version) { " + versionEcho[name] + "; } | sed 's/^/error: /';; *) exit 2;; esac"
				case broken == "keyword:"+name:
					// Leading keyword without the exact banner shape must fail.
					body = "echo 'pinact: cannot run, needs 2.34.0'"
				case broken == "loose:"+name:
					// A glibc loader error contains a dotted number but is no version banner.
					body = "echo '/lib/libc.so.6: version `GLIBC_2.34.0` not found' >&2; exit 1"
				}
				if broken == "no-gh" && name == "gh" || broken == "missing:gh-stack" && name == "gh-stack" {
					continue
				}
				dir := bin
				if name == "gh-stack" {
					// The extension binary lives at libexec/gh-stack, beside bin.
					dir = filepath.Join(root, "libexec")
				}
				if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("sh", script)
			path := bin + ":" + os.Getenv("PATH")
			if broken == "no-gh" {
				// Without gh the discovery check is skipped with a message.
				path = bin + ":/usr/bin:/bin"
				// Only the utilities the script needs, so gh is never on PATH.
				tools := filepath.Join(root, "tools")
				if err := os.MkdirAll(tools, 0755); err != nil {
					t.Fatal(err)
				}
				for _, tool := range []string{"sh", "grep", "sed", "cat", "mktemp", "rm", "mkdir", "ln", "printf", "dirname", "env", "head", "tr", "sort", "uname"} {
					if p, err := exec.LookPath(tool); err == nil {
						if err := os.Symlink(p, filepath.Join(tools, tool)); err != nil {
							t.Fatal(err)
						}
					}
				}
				path = bin + ":" + tools
			}
			cmd.Env = append(os.Environ(), "HOME="+home, "BUNDLEDOCK_TOOLBOX="+root, "PATH="+path)
			// A live stdin: an unguarded gh call would read it (a prompt hazard).
			cmd.Stdin = strings.NewReader("y\n")
			out, err := cmd.CombinedOutput()
			if broken == "missing:gh-stack" && !strings.Contains(string(out), "libexec/gh-stack version probe failed") {
				t.Fatalf("missing extension binary must fail its direct probe: %v\n%s", err, out)
			}
			if broken == "no-gh" {
				if err != nil || !strings.Contains(string(out), "skipping gh stack discovery check") {
					t.Fatalf("missing gh must skip with a message: %v\n%s", err, out)
				}
				return
			}
			if broken == "" && err != nil {
				t.Fatalf("valid synthetic tools failed: %v\n%s", err, out)
			}
			if broken != "" && err == nil {
				t.Fatalf("broken %s passed", broken)
			}
		})
	}
}
