package toolpublish

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPublicationWorkflowRepositoryConfiguration(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/bundledock-toolbox.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	if !strings.HasPrefix(workflow, "name: Publish toolbox\n") || !strings.Contains(workflow, "REPOSITORY_URL: ${{ vars.REPOSITORY_URL }}") {
		t.Fatal("workflow must expose Publish toolbox and use the repository namespace variable")
	}
	for _, obsolete := range []string{"QUAY_IMAGE", "QUAY_USERNAME", "QUAY_PASSWORD"} {
		if strings.Contains(workflow, obsolete) {
			t.Fatalf("workflow still uses obsolete setting %s", obsolete)
		}
	}
	for _, mapping := range []string{"REPOSITORY_USERNAME: ${{ vars.REPOSITORY_USERNAME }}", "REPOSITORY_TOKEN: ${{ secrets.REPOSITORY_TOKEN }}"} {
		if strings.Count(workflow, mapping) != 5 {
			t.Fatalf("native and promotion logins and absence checks must use %s", mapping)
		}
	}

	native := workflow[strings.Index(workflow, "\n  native:"):]
	smoke := strings.Index(native, "- name: Build once and smoke on the native architecture")
	firstSecret := strings.Index(native, "REPOSITORY_TOKEN: ${{ secrets.REPOSITORY_TOKEN }}")
	if smoke < 0 || firstSecret < smoke {
		t.Fatal("registry credentials supplied before native smoke")
	}
	logins := 0
	for _, line := range strings.Split(workflow, "\n") {
		if !strings.Contains(line, "docker login ") {
			continue
		}
		logins++
		dir := t.TempDir()
		// Capture dummy credentials from the workflow's actual shell command.
		stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$LOGIN_ARGS\"\ncat > \"$LOGIN_INPUT\"\n"
		if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(stub), 0700); err != nil {
			t.Fatal(err)
		}
		args, input := filepath.Join(dir, "args"), filepath.Join(dir, "input")
		cmd := exec.Command("sh", "-c", strings.TrimPrefix(strings.TrimSpace(line), "run: "))
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "REPOSITORY_USERNAME=bundledock+github-actions", "REPOSITORY_TOKEN=dummy-token", "LOGIN_ARGS="+args, "LOGIN_INPUT="+input)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("workflow login: %v %s", err, out)
		}
		argv, err := os.ReadFile(args)
		if err != nil {
			t.Fatal(err)
		}
		stdin, err := os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
		if string(argv) != "login quay.io --username bundledock+github-actions --password-stdin\n" || string(stdin) != "dummy-token" {
			t.Fatalf("workflow login did not pass configured username and token through stdin: %q %q", argv, stdin)
		}
	}
	if logins != 2 {
		t.Fatalf("expected native and promotion logins, got %d", logins)
	}
}

func TestPublicationKeepsNativeAndPromotionGates(t *testing.T) {
	b, err := os.ReadFile("../../.github/workflows/bundledock-toolbox.yml")
	if err != nil {
		t.Fatal(err)
	}
	w := string(b)
	for _, required := range []string{"publish: ${{ steps.plan.outputs.publish }}", "id: plan", `"$RUNNER_TEMP/tools-publish" plan`} {
		if !strings.Contains(w, required) {
			t.Fatalf("missing publication contract %s", required)
		}
	}
	for _, name := range []string{"native", "promote"} {
		start := strings.Index(w, "\n  "+name+":\n")
		if start < 0 {
			t.Fatal("missing job " + name)
		}
		job := w[start:]
		if keys := regexp.MustCompile(`(?m)^  [a-z]+:\n`).FindAllStringIndex(job, -1); len(keys) > 1 {
			job = job[:keys[1][0]]
		}
		// A job-level condition excludes native runners, builds, logins, pushes
		// and promotion together when the resolver does not approve publication.
		if !strings.Contains(job, "needs.resolve.outputs.publish == 'true'") {
			t.Fatalf("%s does not skip all work on no-op", name)
		}
	}
}

func TestWorkflowsUsePatchedBuildToolchain(t *testing.T) {
	version, err := os.ReadFile("../../.go-version")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(version)) != "1.27.2" {
		t.Fatal("build toolchain must use the verified patched Go release")
	}
	dockerfile, err := os.ReadFile("../../bundles/toolbox/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockerfile), "golang:"+strings.TrimSpace(string(version))+"-alpine@sha256:") {
		t.Fatal("OCI compiler must match the CI toolchain")
	}
	for _, name := range []string{"ci.yml", "bundledock-toolbox.yml"} {
		data, err := os.ReadFile("../../.github/workflows/" + name)
		if err != nil {
			t.Fatal(err)
		}
		workflow := string(data)
		if strings.Count(workflow, "uses: actions/setup-go@") != strings.Count(workflow, "go-version-file: .go-version") || strings.Contains(workflow, "go-version-file: go.mod") {
			t.Errorf("%s must use the shared patched compiler version for every Go setup", name)
		}
	}
}

func TestAutomaticCIOnlyChecksSource(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	w := string(data)
	if !strings.Contains(w, "run: make check") {
		t.Fatal("automatic CI must retain source checks")
	}
	for _, forbidden := range []string{"make build", "ci-smoke.sh", "docker ", "tools-build", "make release", "make docs", "  native:"} {
		if strings.Contains(w, forbidden) {
			t.Errorf("automatic CI must not build artifacts: %s", forbidden)
		}
	}
}

func TestPublicationRequiresManualApprovedRevision(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/bundledock-toolbox.yml")
	if err != nil {
		t.Fatal(err)
	}
	w := string(data)
	start, end := strings.Index(w, "\non:\n"), strings.Index(w, "\npermissions:")
	if start < 0 || end < start {
		t.Fatal("missing workflow event contract")
	}
	triggers := w[start:end]
	for _, forbidden := range []string{"schedule:", "push:", "pull_request:", "repository_dispatch:", "workflow_call:"} {
		if strings.Contains(triggers, forbidden) {
			t.Errorf("publication must require user dispatch: %s", forbidden)
		}
	}
	for _, required := range []string{"workflow_dispatch:", "approved_sha:", "required: true"} {
		if !strings.Contains(triggers, required) {
			t.Errorf("missing explicit approval input: %s", required)
		}
	}
	for _, required := range []string{"github.event_name == 'workflow_dispatch'", "github.event.repository.default_branch == 'main'", "github.ref == 'refs/heads/main'", "github.event.sender.type == 'User'", "inputs.approved_sha == github.sha", "run: sh scripts/approve-publication.sh", "APPROVED_SHA: ${{ inputs.approved_sha }}", "SENDER_TYPE: ${{ github.event.sender.type }}", "TRIGGERING_ACTOR: ${{ github.triggering_actor }}"} {
		if !strings.Contains(w, required) {
			t.Errorf("missing approval gate: %s", required)
		}
	}
	if strings.Count(w, "run: sh scripts/approve-publication.sh") != 3 {
		t.Error("resolve, native and promote must each recheck approval, including partial reruns")
	}
	gate := strings.Index(w, "run: sh scripts/approve-publication.sh")
	setup := strings.Index(w, "uses: actions/setup-go@")
	if gate < 0 || setup < gate {
		t.Error("approval must precede compilation and resolution")
	}
}

func TestPublicationApprovalFailsClosed(t *testing.T) {
	sha := strings.Repeat("a", 40)
	base := []string{"GITHUB_EVENT_NAME=workflow_dispatch", "DEFAULT_BRANCH=main", "GITHUB_REF=refs/heads/main", "GITHUB_SHA=" + sha, "APPROVED_SHA=" + sha, "GITHUB_ACTOR=maintainer", "TRIGGERING_ACTOR=maintainer", "SENDER_TYPE=User"}
	cases := []struct {
		name, replacement string
		wantErr           bool
	}{
		{"human approval", "", false},
		{"schedule", "GITHUB_EVENT_NAME=schedule", true},
		{"PR", "GITHUB_EVENT_NAME=pull_request", true},
		{"push", "GITHUB_EVENT_NAME=push", true},
		{"wrong branch", "GITHUB_REF=refs/heads/topic", true},
		{"different default", "DEFAULT_BRANCH=develop", true},
		{"missing approval", "APPROVED_SHA=", true},
		{"wrong revision", "APPROVED_SHA=" + strings.Repeat("b", 40), true},
		{"short revision", "APPROVED_SHA=aaaaaaa", true},
		{"bot sender", "SENDER_TYPE=Bot", true},
		{"bot actor", "GITHUB_ACTOR=renovate[bot]", true},
		{"automated rerun", "TRIGGERING_ACTOR=github-actions[bot]", true},
		{"missing actor", "GITHUB_ACTOR=", true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cmd := exec.Command("sh", "../../scripts/approve-publication.sh")
			cmd.Env = append(os.Environ(), base...)
			if test.replacement != "" {
				cmd.Env = append(cmd.Env, test.replacement)
			}
			out, err := cmd.CombinedOutput()
			if (err != nil) != test.wantErr {
				t.Fatalf("approval error=%v output=%s", err, out)
			}
		})
	}
}
