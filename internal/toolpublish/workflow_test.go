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

func TestDailyPublicationSkipsAllNativeAndPromotionWorkOnNoop(t *testing.T) {
	b, err := os.ReadFile("../../.github/workflows/bundledock-toolbox.yml")
	if err != nil {
		t.Fatal(err)
	}
	w := string(b)
	for _, required := range []string{"cron: '0 6 * * *'", "publish: ${{ steps.plan.outputs.publish }}", "id: plan", `"$RUNNER_TEMP/tools-publish" plan`} {
		if !strings.Contains(w, required) {
			t.Fatalf("missing daily publication contract %s", required)
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
		// and promotion together when the scheduled resolver reports no change.
		if !strings.Contains(job, "if: needs.resolve.outputs.publish == 'true'") {
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
