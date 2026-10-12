package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLeafSmokeIsolatedAndRejectsWrongRendering(t *testing.T) {
	script, err := filepath.Abs("../../bundles/toolbox/scripts/leaf-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, broken := range []string{"", "version", "render"} {
		t.Run("broken-"+broken, func(t *testing.T) {
			home := t.TempDir()
			fake := filepath.Join(t.TempDir(), "leaf")
			body := `#!/bin/sh
set -eu
test "$HOME" != '$CALLER_HOME'
test -d "$HOME"
test "$XDG_CONFIG_HOME" != '$CALLER_HOME'
test -z "${LEAF_CONFIG+x}${EDITOR+x}${VISUAL+x}"
case "$*" in
--version) test '$BROKEN' != version; echo 'leaf 1.29.0';;
'--inline plain '*)
 test "$#" = 3
 test "$3" = "$PWD/preview.md"
 test "$(cat "$3")" = '# bundledock-leaf-smoke'
 if test '$BROKEN' = render; then echo wrong; else echo bundledock-leaf-smoke; fi;;
*) exit 2;;
esac
`
			body = strings.ReplaceAll(body, "$CALLER_HOME", home)
			body = strings.ReplaceAll(body, "$BROKEN", broken)
			if err := os.WriteFile(fake, []byte(body), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script, fake)
			cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home, "LEAF_CONFIG="+home, "EDITOR=false", "VISUAL=false")
			out, err := cmd.CombinedOutput()
			if broken == "" && err != nil {
				t.Fatalf("valid fixture: %v\n%s", err, out)
			}
			if broken != "" && err == nil {
				t.Fatal("broken fixture passed")
			}
			if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
				t.Fatalf("caller home changed: %v %v", entries, err)
			}
		})
	}
}
