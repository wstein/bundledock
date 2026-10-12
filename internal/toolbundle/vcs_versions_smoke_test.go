package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVCSVersionSmokeUsesOnlyNoninteractiveVersion(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/vcs-versions-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, broken := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "wrong-version"}[broken], func(t *testing.T) {
			bin := t.TempDir()
			home := t.TempDir()
			body := `#!/bin/sh
set -eu
test "$#" = 1
test "$1" = --version
test -z "${GIT_DIR+x}${GIT_CONFIG_COUNT+x}${LG_CONFIG_FILE+x}${LG_CONFIG_DIR+x}"
printf '%s\n' 'commit=synthetic, build date=synthetic, version=0.66.1, os=linux, arch=amd64, git version=synthetic'
`
			if broken {
				body = "#!/bin/sh\necho wrong\n"
			}
			if err := os.WriteFile(filepath.Join(bin, "lazygit"), []byte(body), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "HOME="+home, "GIT_DIR=/must-not-use", "GIT_CONFIG_COUNT=1", "LG_CONFIG_FILE=/must-not-use", "LG_CONFIG_DIR=/must-not-use")
			out, err := cmd.CombinedOutput()
			if !broken && err != nil {
				t.Fatalf("valid versions failed: %v\n%s", err, out)
			}
			if broken && err == nil {
				t.Fatal("invalid version accepted")
			}
			if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
				t.Fatalf("caller HOME changed: %v %v", entries, err)
			}
		})
	}
}
