package toolpublish

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestMergedToolboxApprovalFixtures(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("requires python3: ", err)
	}
	cmd := exec.Command("python3", "../../scripts/toolbox_publication_test.py")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("approval fixtures: %v\n%s", err, output)
	}
}

func TestMergedToolboxBuildDependsOnHumanApproval(t *testing.T) {
	b, err := os.ReadFile("../../.github/workflows/bundledock-toolbox.yml")
	if err != nil {
		t.Fatal(err)
	}
	w := string(b)
	for _, required := range []string{"workflow_run:", "workflows: [CI]", "types: [completed]", "  preflight:", "  approval:", "name: Approve toolbox ${{ needs.preflight.outputs.sha }}", "name: toolbox-publication", "url: ${{ needs.preflight.outputs.pr_url }}", "needs: [preflight, approval]", "needs.approval.result == 'success'", "PUBLICATION_SHA: ${{ needs.preflight.outputs.sha }}", "--frozen-lock"} {
		if !strings.Contains(w, required) {
			t.Errorf("missing approval contract %s", required)
		}
	}
	start := strings.Index(w, "\n  preflight:")
	if start < 0 {
		t.Fatal("missing preflight")
	}
	preflight := w[start:strings.Index(w, "\n  resolve:")]
	trusted := w[start:strings.Index(w, "\n  approval:")]
	if !strings.Contains(trusted, "ref: ${{ github.sha }}") || strings.Contains(trusted, "ref: ${{ github.event.workflow_run.head_sha }}") {
		t.Fatal("preflight must never execute CI-controlled source")
	}
	for _, forbidden := range []string{"setup-go", "go build", "go run", "docker ", "make check"} {
		if strings.Contains(preflight, forbidden) {
			t.Errorf("compiler/build before approval: %s", forbidden)
		}
	}
	for _, name := range []string{"resolve", "native", "promote"} {
		job := w[strings.Index(w, "\n  "+name+":"):]
		if keys := regexp.MustCompile(`(?m)^  [a-z]+:`).FindAllStringIndex(job, -1); len(keys) > 1 {
			job = job[:keys[1][0]]
		}
		if !strings.Contains(job, "run: python3 scripts/toolbox-publication.py") {
			t.Errorf("%s lacks automatic provenance recheck", name)
		}
	}
}
