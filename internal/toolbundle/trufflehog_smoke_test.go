package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTruffleHogSmokeOfflineDetectionAndCleanControl(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("requires jq")
	}
	script, err := filepath.Abs("../../bundles/toolbox/scripts/trufflehog-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, broken := range []string{"", "version", "exit", "result", "clean"} {
		t.Run("broken-"+broken, func(t *testing.T) {
			home := t.TempDir()
			fake := filepath.Join(t.TempDir(), "trufflehog")
			body := `#!/bin/sh
set -eu
test "$HOME" != '$CALLER_HOME'
test -z "${GITHUB_TOKEN+x}${TRUFFLEHOG_CONFIG+x}"
case "$*" in
'--no-update --version') test '$BROKEN' != version; echo 'trufflehog 3.99.2';;
'filesystem --no-update --no-verification --fail --fail-on-scan-errors --json --config '*)
 test "$#" = 9
 test -f "$8"
 grep -q 'bundledock-synthetic' "$8"
 case "$9" in
 */secret)
  grep -qx 'BUNDLEDOCK_SYNTHETIC_ABCDEFGHIJKLMNOP' "$9/fixture.txt"
  if test '$BROKEN' = result; then
   echo '{"Raw":"wrong","Verified":false}'
  else
   echo '{"Raw":"BUNDLEDOCK_SYNTHETIC_ABCDEFGHIJKLMNOP","Verified":false}'
  fi
  if test '$BROKEN' = exit; then exit 0; else exit 183; fi;;
 */clean) test '$BROKEN' != clean;;
 *) exit 2;;
 esac;;
*) exit 2;;
esac
`
			body = strings.ReplaceAll(strings.ReplaceAll(body, "$CALLER_HOME", home), "$BROKEN", broken)
			if err := os.WriteFile(fake, []byte(body), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script, fake)
			cmd.Env = append(os.Environ(), "HOME="+home, "GITHUB_TOKEN=fixture", "TRUFFLEHOG_CONFIG=caller")
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
