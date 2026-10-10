package toolpublish

import (
	"os"
	"strings"
	"testing"
)

// These source checks run without Hugo, uploads, or deployment credentials.
func TestPagesWorkflowApprovalBoundary(t *testing.T) {
	w := pagesWorkflow(t)
	header, jobs, ok := strings.Cut(w, "\njobs:\n")
	if !ok {
		t.Fatal("missing jobs")
	}
	for _, required := range []string{"workflow_dispatch:", "approved_sha:", "required: true", "permissions:\n  contents: read", "cancel-in-progress: false"} {
		if !strings.Contains(header, required) {
			t.Errorf("missing manual publication contract: %s", required)
		}
	}
	for _, forbidden := range []string{"push:", "pull_request:", "schedule:", "workflow_run:", "repository_dispatch:", "workflow_call:", "write", "enablement"} {
		if strings.Contains(header, forbidden) {
			t.Errorf("unexpected trigger or global authority: %s", forbidden)
		}
	}
	build, deploy, ok := strings.Cut(jobs, "\n  deploy:\n")
	if !ok || !strings.HasPrefix(build, "  build:\n") {
		t.Fatal("expected separate build and deploy jobs")
	}
	for name, job := range map[string]string{"build": build, "deploy": deploy} {
		t.Run(name, func(t *testing.T) {
			for _, required := range []string{
				"github.event_name == 'workflow_dispatch'", "github.event.repository.default_branch == 'main'",
				"github.ref == 'refs/heads/main'", "github.event.sender.type == 'User'", "inputs.approved_sha == github.sha",
				"persist-credentials: false", "ref: ${{ github.sha }}",
				"APPROVED_SHA: ${{ inputs.approved_sha }}", "DEFAULT_BRANCH: ${{ github.event.repository.default_branch }}",
				"SENDER_TYPE: ${{ github.event.sender.type }}", "TRIGGERING_ACTOR: ${{ github.triggering_actor }}",
			} {
				if !strings.Contains(job, required) {
					t.Errorf("job must independently recheck exact human approval: %s", required)
				}
			}
			if !pagesGatePrecedesEffects(job) {
				t.Error("approval must precede every build, artifact upload, and deployment, including partial reruns")
			}
		})
	}
	if strings.Contains(build, "write") || strings.Contains(build, "configure-pages") {
		t.Error("build must have read-only source access and cannot enable Pages")
	}
	for _, required := range []string{"needs: build", "contents: read", "pages: write", "id-token: write", "name: github-pages", "url: ${{ steps.deployment.outputs.page_url }}"} {
		if !strings.Contains(deploy, required) {
			t.Errorf("missing deployment contract: %s", required)
		}
	}
	if strings.Count(w, ": write") != 2 || strings.Contains(w, "contents: write") || strings.Contains(w, "secrets.") {
		t.Error("only deploy may receive Pages and OIDC write permissions; no separate credentials")
	}
	for _, required := range []string{"run: make docs", "HUGO_BASEURL: https://wstein.github.io/bundledock/", "path: public/", "name: github-pages-${{ github.run_attempt }}", "artifact_name: github-pages-${{ github.run_attempt }}"} {
		if !strings.Contains(w, required) {
			t.Errorf("missing pinned Hugo project-subpath or attempt-specific artifact contract: %s", required)
		}
	}
	for _, line := range strings.Split(w, "\n") {
		if _, action, ok := strings.Cut(line, "uses: "); ok {
			_, ref, ok := strings.Cut(strings.Fields(action)[0], "@")
			if !ok || len(ref) != 40 || strings.Trim(ref, "0123456789abcdef") != "" {
				t.Errorf("action must have an immutable SHA: %s", line)
			}
		}
	}
}

func TestPagesGateOrderingRejectsRegressions(t *testing.T) {
	w := pagesWorkflow(t)
	_, jobs, _ := strings.Cut(w, "\njobs:\n")
	build, deploy, _ := strings.Cut(jobs, "\n  deploy:\n")
	for name, job := range map[string]string{"build": build, "deploy": deploy} {
		t.Run(name, func(t *testing.T) {
			if pagesGatePrecedesEffects(strings.ReplaceAll(job, "run: sh scripts/approve-publication.sh", "run: true")) {
				t.Error("missing approval gate accepted")
			}
			if pagesGatePrecedesEffects(strings.Replace(job, "    steps:\n", "    steps:\n      - run: make docs\n", 1)) {
				t.Error("build before approval accepted")
			}
			if pagesGatePrecedesEffects(strings.Replace(job, "    steps:\n", "    steps:\n      - uses: actions/upload-pages-artifact@unapproved\n", 1)) {
				t.Error("artifact action before approval accepted")
			}
		})
	}
}

func pagesWorkflow(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../.github/workflows/pages.yml")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func pagesGatePrecedesEffects(job string) bool {
	gate := strings.Index(job, "run: sh scripts/approve-publication.sh")
	if gate < 0 || strings.Count(job, "run: sh scripts/approve-publication.sh") != 1 {
		return false
	}
	// Only immutable checkout may run before the gate; every other action or
	// shell command may create artifacts or perform deployment side effects.
	for _, line := range strings.Split(job[:gate], "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "run:") || (strings.Contains(line, "uses:") && !strings.Contains(line, "uses: actions/checkout@")) {
			return false
		}
	}
	return true
}
