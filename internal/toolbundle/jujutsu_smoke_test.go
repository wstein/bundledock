package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bundledock/internal/testdir"
)

// The fixture validates the smoke's exact native CLI arguments and rejects
// inherited repository/config selectors. It never executes upstream Jujutsu.
func TestJujutsuSmokeIsolatedAndRejectsWrongBehavior(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git required for hostile repository fixture")
	}
	script, err := filepath.Abs("../../bundles/toolbox/scripts/jujutsu-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	external := testdir.New(t)
	init := exec.Command(git, "init", "--bare", external)
	init.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + testdir.New(t), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	if out, err := init.CombinedOutput(); err != nil {
		t.Fatalf("git fixture: %v\n%s", err, out)
	}
	config := filepath.Join(external, "config")
	before, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, broken := range []string{"", "init", "new", "log"} {
		t.Run("broken-"+broken, func(t *testing.T) {
			home := testdir.New(t)
			fake := filepath.Join(testdir.New(t), "jj")
			body := `#!/bin/sh
set -eu
# env -i must remove every caller-controlled Git selector and Jujutsu config.
test -z "${GIT_DIR+x}${GIT_WORK_TREE+x}${GIT_INDEX_FILE+x}${GIT_OBJECT_DIRECTORY+x}${GIT_ALTERNATE_OBJECT_DIRECTORIES+x}${GIT_CONFIG_COUNT+x}${GIT_CONFIG_PARAMETERS+x}${JJ_CONFIG_TOML+x}"
test "$JJ_CONFIG" = ''
test "$HOME" != "$CALLER_HOME"
test -d "$HOME"
case "$*" in
'--version') echo 'jj 0.46.0';;
'--no-pager git init --no-colocate repo')
 test "$BROKEN" != init
 mkdir -p repo/.jj
 # Real Git in the temporary workspace catches any inherited selector leak.
 git init -q repo
 git -C repo config bundledock.smoke isolated
 ;;
'--no-pager new --message bundledock-jj-smoke')
 test "$BROKEN" != new
 test -d .jj
 printf '%s\n' bundledock-jj-smoke > .jj/description
 ;;
'--no-pager log --no-graph --revisions @ --template description')
 if test "$BROKEN" = log; then echo wrong; else cat .jj/description; fi
 ;;
*) exit 2;;
esac
`
			// Substitute fixture-only controls literally: the smoke must not inherit them.
			body = strings.ReplaceAll(body, "$BROKEN", broken)
			body = strings.ReplaceAll(body, "$CALLER_HOME", home)
			if err := os.WriteFile(fake, []byte(body), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script, fake)
			cmd.Env = append(os.Environ(), "HOME="+home, "GIT_DIR="+external, "GIT_WORK_TREE="+external, "GIT_INDEX_FILE="+filepath.Join(external, "protected-index"), "GIT_OBJECT_DIRECTORY="+filepath.Join(external, "objects"), "GIT_ALTERNATE_OBJECT_DIRECTORIES="+filepath.Join(external, "objects"), "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.hooksPath", "GIT_CONFIG_VALUE_0="+external, "GIT_CONFIG_PARAMETERS='core.bare=true'", "JJ_CONFIG="+config, "JJ_CONFIG_TOML=invalid")
			out, err := cmd.CombinedOutput()
			if broken == "" && err != nil {
				t.Fatalf("valid isolated fixture failed: %v\n%s", err, out)
			}
			if broken != "" && err == nil {
				t.Fatalf("broken %s passed", broken)
			}
			after, err := os.ReadFile(config)
			if err != nil || string(after) != string(before) {
				t.Fatalf("caller Git config changed: %v", err)
			}
			if _, err := os.Stat(filepath.Join(external, "protected-index")); !os.IsNotExist(err) {
				t.Fatalf("caller Git index touched: %v", err)
			}
			if entries, err := os.ReadDir(filepath.Join(external, "refs", "heads")); err != nil || len(entries) != 0 {
				t.Fatalf("caller refs changed: %v %v", entries, err)
			}
			if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
				t.Fatalf("caller home changed: %v %v", entries, err)
			}
		})
	}
}
