package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func copyFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "toolbundle", "testdata", "nixdb", name))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "db.sqlite")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNormalizeNixDBCommand(t *testing.T) {
	a, b := copyFixture(t, "nixdb-a.sqlite"), copyFixture(t, "nixdb-b.sqlite")
	t.Setenv("SOURCE_DATE_EPOCH", "1")
	for _, p := range []string{a, b} {
		if err := runNormalizeNixDB([]string{"-db", p}); err != nil {
			t.Fatal(err)
		}
	}
	da, _ := os.ReadFile(a)
	db, _ := os.ReadFile(b)
	if !bytes.Equal(da, db) {
		t.Fatal("databases differ after the command")
	}
	// -epoch wins over SOURCE_DATE_EPOCH.
	c := copyFixture(t, "nixdb-a.sqlite")
	if err := runNormalizeNixDB([]string{"-db", c, "-epoch", "1704067200"}); err != nil {
		t.Fatal(err)
	}
	if dc, _ := os.ReadFile(c); bytes.Equal(dc, da) {
		t.Fatal("-epoch ignored")
	}
}

func TestNormalizeNixDBCommandRejectsBadInput(t *testing.T) {
	p := copyFixture(t, "nixdb-a.sqlite")
	t.Setenv("SOURCE_DATE_EPOCH", "")
	for name, args := range map[string][]string{
		"no db":         {"-epoch", "1"},
		"no epoch":      {"-db", p},
		"bad epoch":     {"-db", p, "-epoch", "soon"},
		"unknown flag":  {"-db", p, "-epoch", "1", "-x"},
		"missing file":  {"-db", p + ".missing", "-epoch", "1"},
		"negative time": {"-db", p, "-epoch", "-5"},
	} {
		if err := runNormalizeNixDB(args); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The subcommand is wired into main: a database path that does not exist fails
// with the command's own message, not with the builder's flag usage.
func TestNormalizeNixDBIsADispatchedSubcommand(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.sqlite")
	out, err := exec.Command("go", "run", ".", "normalize-nix-db", "-db", missing, "-epoch", "1").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "missing.sqlite") || strings.Contains(string(out), "Usage") {
		t.Fatalf("got %v: %s", err, out)
	}
}
