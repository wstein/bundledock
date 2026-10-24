package toolpublish

import (
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The owner's approved_digest (scripts/bundle-publication.py) covers every non-test Go file of
// the module packages that build the two binaries of the toolbox image. This test recomputes that
// dependency closure so the directory list cannot drift when an import is added.
func TestApprovalInputDirectoriesCoverTheBuildClosure(t *testing.T) {
	script, err := os.ReadFile("../../scripts/bundle-publication.py")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^APPROVAL_INPUT_DIRECTORIES = \(([^)]*)\)`).FindSubmatch(script)
	if m == nil {
		t.Fatal("APPROVAL_INPUT_DIRECTORIES not found")
	}
	listed := map[string]bool{}
	for _, d := range regexp.MustCompile(`'([^']+)'`).FindAllSubmatch(m[1], -1) {
		listed[string(d[1])] = true
	}
	list := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "./cmd/tools-bundle", "./cmd/tools-init")
	list.Dir = "../.."
	out, err := list.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	closure := map[string]bool{}
	for _, pkg := range strings.Fields(string(out)) {
		if dir, ok := strings.CutPrefix(pkg, "bundledock/"); ok {
			closure[dir] = true
		}
	}
	var missing, extra []string
	for dir := range closure {
		if !listed[dir] {
			missing = append(missing, dir)
		}
	}
	for dir := range listed {
		if !closure[dir] {
			extra = append(extra, dir)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(closure) == 0 || len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("APPROVAL_INPUT_DIRECTORIES differs from the build closure: missing %v, not in the closure %v", missing, extra)
	}
}
