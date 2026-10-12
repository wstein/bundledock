package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDeadbranchSmokeChecksListingAndPreservesBranches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	script, err := filepath.Abs("../../bundles/toolbox/scripts/deadbranch-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, behavior := range []string{"valid", "wrong-output", "mutates-branch"} {
		t.Run(behavior, func(t *testing.T) {
			bin := t.TempDir()
			body := `case "$*" in
--version) echo 'deadbranch 0.4.0';;
'list --local --days 30')
 test "$(git branch --show-current)" = main
 git show-ref --verify --quiet refs/heads/feature/deadbranch-smoke
 echo feature/deadbranch-smoke
 ;;
*) exit 2;;
esac`
			if behavior == "wrong-output" {
				body = `case "$*" in --version) echo 'deadbranch 0.4.0';; 'list --local --days 30') echo wrong;; *) exit 2;; esac`
			}
			if behavior == "mutates-branch" {
				body = `case "$*" in --version) echo 'deadbranch 0.4.0';; 'list --local --days 30') git update-ref -d refs/heads/feature/deadbranch-smoke; echo feature/deadbranch-smoke;; *) exit 2;; esac`
			}
			if err := os.WriteFile(filepath.Join(bin, "deadbranch"), []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "PATH="+bin+":"+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if behavior == "valid" && err != nil {
				t.Fatalf("valid listing failed: %v\n%s", err, out)
			}
			if behavior != "valid" && err == nil {
				t.Fatalf("%s passed", behavior)
			}
		})
	}
}
