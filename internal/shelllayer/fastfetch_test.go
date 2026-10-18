package shelllayer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFastfetchInteractiveStartup(t *testing.T) {
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
			for _, mode := range []string{"interactive", "noninteractive", "missing", "disabled", "failure"} {
				t.Run(mode, func(t *testing.T) {
					dir := t.TempDir()
					layer := filepath.Join(dir, "layer"+filepath.Ext(sh.file))
					if err := os.WriteFile(layer, files[sh.file], 0644); err != nil {
						t.Fatal(err)
					}
					bin := filepath.Join(dir, "bin")
					if err := os.Mkdir(bin, 0755); err != nil {
						t.Fatal(err)
					}
					calls := filepath.Join(dir, "calls")
					if mode != "missing" {
						stub := "#!/bin/sh\ntest \"$#\" = 0 || exit 42\nprintf 'called\\n' >> \"$CALLS\"\nprintf 'FASTFETCH GREETING\\n'\n"
						if mode == "failure" {
							stub += "exit 1\n"
						}
						if err := os.WriteFile(filepath.Join(bin, "fastfetch"), []byte(stub), 0755); err != nil {
							t.Fatal(err)
						}
					}
					source := "source " + layer + "; source " + layer
					var args []string
					switch sh.name {
					case "bash":
						args = []string{"--norc", "--noprofile"}
						if mode != "noninteractive" {
							args = append(args, "-i")
						}
					case "zsh":
						args = []string{"-f"}
						if mode != "noninteractive" {
							args = append(args, "-i")
						}
					case "fish":
						args = []string{"--no-config"}
						if mode != "noninteractive" {
							args = append(args, "-i")
						}
					case "nu":
						args = []string{"--no-config-file", "--no-history"}
						if mode != "noninteractive" {
							args = append(args, "-i")
						}
					}
					args = append(args, "-c", source)
					cmd := exec.Command(executable, args...)
					cmd.Env = []string{"PATH=" + bin, "HOME=" + dir, "TERM=xterm", "CALLS=" + calls, "BDOCK_SHORTCUTS=0", "BDOCK_STARSHIP=0"}
					if mode == "disabled" {
						cmd.Env = append(cmd.Env, "BDOCK_FASTFETCH=0")
					}
					out, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("startup failed: %v\n%s", err, out)
					}
					got, err := os.ReadFile(calls)
					wantCall := mode == "interactive" || mode == "failure"
					if wantCall && (err != nil || string(got) != "called\n") {
						t.Fatalf("expected one invocation, got %q %v; output %s", got, err, out)
					}
					if !wantCall && (!os.IsNotExist(err) || strings.Contains(string(out), "FASTFETCH GREETING")) {
						t.Fatalf("unexpected invocation %q %v; output %s", got, err, out)
					}
				})
			}
		})
	}
}
