package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"bundledock/internal/testdir"
)

func TestVCSVersionSmokeUsesOnlyNoninteractiveVersion(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/vcs-versions-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, broken := range []string{"", "lazygit", "git-lfs"} {
		t.Run("broken-"+broken, func(t *testing.T) {
			bin := testdir.New(t)
			home := testdir.New(t)
			stubs := map[string]string{
				"lazygit": `test "$#" = 1
test "$1" = --version
printf '%s\n' 'commit=synthetic, build date=synthetic, version=0.66.1, os=linux, arch=amd64, git version=synthetic'`,
				"git-lfs": `test "$#" = 1
test "$1" = version
printf '%s\n' 'git-lfs/3.8.0 (GitHub; linux amd64; go synthetic)'`,
			}
			for name, body := range stubs {
				body = "#!/bin/sh\nset -eu\ntest -z \"${GIT_DIR+x}${GIT_CONFIG_COUNT+x}${LG_CONFIG_FILE+x}${LG_CONFIG_DIR+x}\"\n" + body + "\n"
				if name == broken {
					body = "#!/bin/sh\necho wrong\n"
				}
				if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0755); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "HOME="+home, "GIT_DIR=/must-not-use", "GIT_CONFIG_COUNT=1", "LG_CONFIG_FILE=/must-not-use", "LG_CONFIG_DIR=/must-not-use")
			out, err := cmd.CombinedOutput()
			if broken == "" && err != nil {
				t.Fatalf("valid versions failed: %v\n%s", err, out)
			}
			if broken != "" && err == nil {
				t.Fatalf("invalid %s version accepted", broken)
			}
			if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
				t.Fatalf("caller HOME changed: %v %v", entries, err)
			}
		})
	}
}
