package gittest

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// commitTrace commits in a fresh repository with the given global config and
// returns Git's trace2 event log of every process it started.
func commitTrace(t *testing.T, config string) string {
	t.Helper()
	dir := t.TempDir()
	events := t.TempDir() + "/trace2.json"
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL="+config, "GIT_CONFIG_NOSYSTEM=1", "GIT_TRACE2_EVENT="+events,
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	for _, args := range [][]string{{"init", "-q"}, {"commit", "-q", "--allow-empty", "-m", "x"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = dir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	b, err := os.ReadFile(events)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hasMaintenanceChild(trace string) bool {
	for _, line := range strings.Split(trace, "\n") {
		if strings.Contains(line, `"event":"child_start"`) && strings.Contains(line, `"maintenance"`) {
			return true
		}
	}
	return false
}

func TestGlobalConfigStartsNoMaintenance(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// Control: an empty config does start maintenance, so the trace check can fail.
	empty := t.TempDir() + "/empty"
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasMaintenanceChild(commitTrace(t, empty)) {
		t.Skip("this Git version starts no maintenance after commit; nothing to prove")
	}
	if hasMaintenanceChild(commitTrace(t, GlobalConfig(t))) {
		t.Fatal("maintenance started despite NoMaintenanceConfig")
	}
}
