package toolpublish

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCISmokeBuildsLockedImageOnce(t *testing.T) {
	for _, failure := range []string{"", "build", "pull", "smoke"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			source, err := os.ReadFile("../../bundles/toolbox/scripts/ci-smoke.sh")
			if err != nil {
				t.Fatal(err)
			}
			write := func(name, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			write("ci-smoke.sh", string(source))
			write("docker", `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
case "$1" in
build)
 [ "$FAIL" != build ] || exit 21
 while [ "$1" != --iidfile ]; do shift; done
 printf '%s\n' sha256:tested > "$2" ;;
pull) [ "$FAIL" != pull ] || exit 22 ;;
*) exit 23 ;;
esac
`)
			write("smoke.sh", `#!/bin/sh
printf 'smoke %s\n' "$*" >> "$CALLS"
[ "$FAIL" != smoke ] || exit 24
`)
			calls := filepath.Join(dir, "calls")
			cmd := exec.Command("sh", filepath.Join(dir, "ci-smoke.sh"))
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "CALLS="+calls, "FAIL="+failure, "RUNNER_TEMP="+dir)
			output, err := cmd.CombinedOutput()
			if (err != nil) != (failure != "") {
				t.Fatalf("failure %q: %v %s", failure, err, output)
			}
			data, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			if strings.Count(got, "build ") != 1 || !strings.Contains(got, "build -f bundles/toolbox/Dockerfile --iidfile ") {
				t.Fatalf("must build checked-in lock exactly once: %s", got)
			}
			if strings.Contains(got, "push") || strings.Contains(got, "upgrade") {
				t.Fatalf("CI must not resolve or publish: %s", got)
			}
			if failure == "build" {
				if strings.Contains(got, "pull ") || strings.Contains(got, "smoke ") {
					t.Fatal(got)
				}
				return
			}
			consumer := "mcr.microsoft.com/devcontainers/go:2-1.27-trixie"
			if !strings.Contains(got, "pull "+consumer+"\n") {
				t.Fatal(got)
			}
			if failure == "pull" {
				if strings.Contains(got, "smoke ") {
					t.Fatal(got)
				}
				return
			}
			if !strings.HasSuffix(got, "smoke sha256:tested "+consumer+"\n") {
				t.Fatalf("must smoke exact built ID after consumer pull: %s", got)
			}
		})
	}
}
