package toolpublish

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestOwnerMergedUpdaterFixtures(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("requires python3: ", err)
	}
	cmd := exec.Command("python3", "../../scripts/bundle_approval_test.py")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("approval fixtures: %v\n%s", err, output)
	}
}

func TestOwnerMergedBundlesShareFrozenPublication(t *testing.T) {
	b, err := os.ReadFile("../../.github/workflows/bundle-publish.yml")
	if err != nil {
		t.Fatal(err)
	}
	w := string(b)
	for _, required := range []string{"workflow_call:", "needs: preflight", "PUBLICATION_SHA: ${{ needs.preflight.outputs.sha }}", "--frozen-lock", "--bundle \"$BUNDLE\"", "run: python3 scripts/bundle-publication.py"} {
		if !strings.Contains(w, required) {
			t.Error("missing " + required)
		}
	}
	if strings.Contains(w, "environment:") {
		t.Fatal("owner merge authorization must require no environment intervention")
	}
	preflight := w[strings.Index(w, "\n  preflight:"):strings.Index(w, "\n  resolve:")]
	for _, bad := range []string{"setup-go", "go build", "go run", "docker ", "make check"} {
		if strings.Contains(preflight, bad) {
			t.Fatal("artifact compiler before validated authorization", bad)
		}
	}
	for _, name := range []string{"resolve", "native", "promote"} {
		job := w[strings.Index(w, "\n  "+name+":"):]
		if keys := regexp.MustCompile(`(?m)^  [a-z]+:`).FindAllStringIndex(job, -1); len(keys) > 1 {
			job = job[:keys[1][0]]
		}
		if !strings.Contains(job, "run: python3 scripts/bundle-publication.py") {
			t.Fatal(name + " missing owner merge provenance recheck")
		}
	}
}
