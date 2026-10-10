package toolpublish

import (
	"os"
	"os/exec"
	"path/filepath"
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
		if strings.Count(workflow, mapping) != 2 {
			t.Fatalf("both native and promotion logins must use %s", mapping)
		}
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
