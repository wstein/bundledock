package shelllayer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellLayerDefaults(t *testing.T) {
	files, err := Files()
	if err != nil {
		t.Fatal(err)
	}
	for _, sh := range shells {
		t.Run(sh.name, func(t *testing.T) {
			executable, err := exec.LookPath(sh.name)
			if err != nil {
				t.Skip(err)
			}
			_ = executable

			t.Run("umask widening", func(t *testing.T) {
				for _, tc := range []struct {
					name       string
					startUmask string
					optout     bool
					wantUmask  string
				}{
					{name: "0000 widened to 022", startUmask: "0000", optout: false, wantUmask: "0022"},
					{name: "000 widened to 022", startUmask: "000", optout: false, wantUmask: "0022"},
					{name: "0027 kept", startUmask: "0027", optout: false, wantUmask: "0027"},
					{name: "0077 kept", startUmask: "0077", optout: false, wantUmask: "0077"},
					{name: "0022 kept", startUmask: "0022", optout: false, wantUmask: "0022"},
					{name: "optout keeps 0000", startUmask: "0000", optout: true, wantUmask: "0000"},
				} {
					t.Run(tc.name, func(t *testing.T) {
						dir := t.TempDir()
						layer := filepath.Join(dir, "layer"+filepath.Ext(sh.file))
						if err := os.WriteFile(layer, files[sh.file], 0644); err != nil {
							t.Fatal(err)
						}

						var script string
						switch sh.name {
						case "fish":
							script = "umask " + tc.startUmask + "; source " + layer + "; umask"
						case "nu":
							if tc.startUmask == "0000" || tc.startUmask == "000" {
								script = "umask rwxrwxrwx | ignore; source " + layer + "; umask"
							} else if tc.startUmask == "0077" {
								script = "umask rwx------ | ignore; source " + layer + "; umask"
							} else if tc.startUmask == "0027" {
								script = "umask rwxr-x--- | ignore; source " + layer + "; umask"
							} else {
								script = "umask rwxr-xr-x | ignore; source " + layer + "; umask"
							}
						default: // bash, zsh
							script = "umask " + tc.startUmask + "; . " + layer + "; umask"
						}

						cmd := exec.Command(sh.name, "-i", "-c", script)
						if sh.name == "bash" {
							cmd.Args = []string{sh.name, "--norc", "--noprofile", "-i", "-c", script}
						} else if sh.name == "zsh" {
							cmd.Args = []string{sh.name, "-f", "-i", "-c", script}
						} else if sh.name == "fish" {
							cmd.Args = []string{sh.name, "--no-config", "-i", "-c", script}
						} else if sh.name == "nu" {
							cmd.Args = []string{sh.name, "--no-config-file", "--no-history", "-i", "-c", script}
						}
						cmd.Env = []string{
							"PATH=" + os.Getenv("PATH"),
							"HOME=" + dir,
							"TERM=dumb",
							"BDOCK_STARSHIP=0",
							"BDOCK_FASTFETCH=0",
							"BDOCK_ZOXIDE=0",
							"BDOCK_SHORTCUTS=0",
						}
						if tc.optout {
							cmd.Env = append(cmd.Env, "BDOCK_UMASK=0")
						}

						out, err := cmd.CombinedOutput()
						if err != nil {
							t.Fatalf("cmd failed: %v, output: %s", err, string(out))
						}
						got := strings.TrimSpace(string(out))
						// nu umask outputs symbolic format like rwxr-xr-x
						if sh.name == "nu" {
							if tc.wantUmask == "0022" && got != "rwxr-xr-x" {
								t.Errorf("got %q, want rwxr-xr-x (0022)", got)
							} else if tc.wantUmask == "0000" && got != "rwxrwxrwx" {
								t.Errorf("got %q, want rwxrwxrwx (0000)", got)
							} else if tc.wantUmask == "0077" && got != "rwx------" {
								t.Errorf("got %q, want rwx------ (0077)", got)
							} else if tc.wantUmask == "0027" && got != "rwxr-x---" {
								t.Errorf("got %q, want rwxr-x--- (0027)", got)
							}
						} else {
							// standard umask outputs 0022, 022, 0000, 000
							gotNorm := strings.TrimLeft(got, "0")
							wantNorm := strings.TrimLeft(tc.wantUmask, "0")
							if gotNorm == "" {
								gotNorm = "0"
							}
							if wantNorm == "" {
								wantNorm = "0"
							}
							if gotNorm != wantNorm {
								t.Errorf("got %q (%q), want %q (%q)", got, gotNorm, tc.wantUmask, wantNorm)
							}
						}
					})
				}
			})

			t.Run("editor default", func(t *testing.T) {
				bin := t.TempDir()
				// Create fake hx
				hxPath := filepath.Join(bin, "hx")
				if err := os.WriteFile(hxPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
					t.Fatal(err)
				}

				for _, tc := range []struct {
					name       string
					env        []string
					optout     bool
					wantEditor string
				}{
					{name: "defaults to hx when unset and outside IDE", env: []string{}, optout: false, wantEditor: "hx"},
					{name: "preserved when already set", env: []string{"EDITOR=nano"}, optout: false, wantEditor: "nano"},
					{name: "not set in vscode", env: []string{"TERM_PROGRAM=vscode"}, optout: false, wantEditor: ""},
					{name: "not set in jetbrains", env: []string{"TERMINAL_EMULATOR=JetBrains-JediTerm"}, optout: false, wantEditor: ""},
					{name: "optout keeps unset", env: []string{}, optout: true, wantEditor: ""},
				} {
					t.Run(tc.name, func(t *testing.T) {
						dir := t.TempDir()
						layer := filepath.Join(dir, "layer"+filepath.Ext(sh.file))
						if err := os.WriteFile(layer, files[sh.file], 0644); err != nil {
							t.Fatal(err)
						}

						var script string
						switch sh.name {
						case "fish":
							script = "source " + layer + `; echo "EDITOR=$EDITOR"`
						case "nu":
							script = "source " + layer + "; if ($env.EDITOR? | is-empty) { echo 'EDITOR=' } else { echo $'EDITOR=($env.EDITOR)' }"
						default: // bash, zsh
							script = ". " + layer + "; echo EDITOR=$EDITOR"
						}

						cmd := exec.Command(sh.name, "-i", "-c", script)
						if sh.name == "bash" {
							cmd.Args = []string{sh.name, "--norc", "--noprofile", "-i", "-c", script}
						} else if sh.name == "zsh" {
							cmd.Args = []string{sh.name, "-f", "-i", "-c", script}
						} else if sh.name == "fish" {
							cmd.Args = []string{sh.name, "--no-config", "-i", "-c", script}
						} else if sh.name == "nu" {
							cmd.Args = []string{sh.name, "--no-config-file", "--no-history", "-i", "-c", script}
						}

						cmd.Env = []string{
							"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
							"HOME=" + dir,
							"TERM=dumb",
							"BDOCK_STARSHIP=0",
							"BDOCK_FASTFETCH=0",
							"BDOCK_ZOXIDE=0",
							"BDOCK_SHORTCUTS=0",
						}
						cmd.Env = append(cmd.Env, tc.env...)
						if tc.optout {
							cmd.Env = append(cmd.Env, "BDOCK_EDITOR=0")
						}

						out, err := cmd.CombinedOutput()
						if err != nil {
							t.Fatalf("cmd failed: %v, output: %s", err, string(out))
						}
						got := strings.TrimSpace(string(out))
						want := "EDITOR=" + tc.wantEditor
						if got != want {
							t.Errorf("got %q, want %q", got, want)
						}
					})
				}
			})

			if sh.name == "bash" || sh.name == "zsh" {
				t.Run("history deduplication", func(t *testing.T) {
					dir := t.TempDir()
					layer := filepath.Join(dir, "layer"+filepath.Ext(sh.file))
					if err := os.WriteFile(layer, files[sh.file], 0644); err != nil {
						t.Fatal(err)
					}

					for _, tc := range []struct {
						name   string
						optout bool
					}{
						{name: "enabled by default", optout: false},
						{name: "optout disabled", optout: true},
					} {
						t.Run(tc.name, func(t *testing.T) {
							var script string
							if sh.name == "bash" {
								script = ". " + layer + "; echo HISTCONTROL=$HISTCONTROL"
							} else {
								script = ". " + layer + "; setopt | grep -E 'histignorealldups|histignoredups|histsavenodups|histignorespace' || true"
							}

							cmd := exec.Command(sh.name)
							if sh.name == "bash" {
								cmd.Args = []string{sh.name, "--norc", "--noprofile", "-i", "-c", script}
							} else {
								cmd.Args = []string{sh.name, "-f", "-i", "-c", script}
							}
							cmd.Env = []string{
								"PATH=" + os.Getenv("PATH"),
								"HOME=" + dir,
								"TERM=dumb",
								"BDOCK_STARSHIP=0",
								"BDOCK_FASTFETCH=0",
								"BDOCK_ZOXIDE=0",
								"BDOCK_SHORTCUTS=0",
							}
							if tc.optout {
								cmd.Env = append(cmd.Env, "BDOCK_HISTORY=0")
							}

							out, err := cmd.CombinedOutput()
							if err != nil {
								t.Fatalf("cmd failed: %v, output: %s", err, string(out))
							}
							got := strings.TrimSpace(string(out))
							if sh.name == "bash" {
								if tc.optout {
									if strings.Contains(got, "ignoreboth:erasedups") {
										t.Errorf("expected HISTCONTROL not to be set on optout, got %q", got)
									}
								} else {
									if got != "HISTCONTROL=ignoreboth:erasedups" {
										t.Errorf("got %q, want HISTCONTROL=ignoreboth:erasedups", got)
									}
								}
							} else if sh.name == "zsh" {
								if tc.optout {
									if strings.Contains(got, "hist") {
										t.Errorf("expected no hist options on optout, got %q", got)
									}
								} else {
									if !strings.Contains(got, "histignorealldups") || !strings.Contains(got, "histsavenodups") || !strings.Contains(got, "histignorespace") {
										t.Errorf("got %q, expected histignorealldups, histsavenodups, histignorespace", got)
									}
								}
							}
						})
					}
				})
			}
		})
	}
}
