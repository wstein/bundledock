package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bundledock/internal/toolpublish"
)

func TestResolveOnceAndKeepExactArtifactBytes(t *testing.T) {
	t.Chdir(t.TempDir())
	initial := []byte("{\"old\":true}\n")
	resolved := []byte("{\"resolved\":true}\n")
	if err := os.WriteFile("tools.lock.json", initial, 0600); err != nil {
		t.Fatal(err)
	}
	meta, err := toolpublish.NewMetadata("tools.lock.json", "quay.io/example/bundledock-tools", "2026.10.10-123-1", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runner := func(args ...string) ([]byte, error) {
		calls++
		if !reflect.DeepEqual(args, []string{"go", "run", "./cmd/bundledock", "tools", "upgrade"}) {
			t.Fatalf("unexpected resolver command %v", args)
		}
		return nil, os.WriteFile("tools.lock.json", resolved, 0600)
	}
	if err = resolve("resolved", meta, runner); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("resolved %d times", calls)
	}
	artifact, err := os.ReadFile(filepath.Join("resolved", "tools.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(artifact) != string(resolved) {
		t.Fatal("artifact lock bytes changed")
	}
	var provenance toolpublish.Metadata
	if err = readJSON("resolved/provenance.json", &provenance); err != nil {
		t.Fatal(err)
	}
	sum, err := toolpublish.HashFile("resolved/tools.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	meta.LockSHA256 = sum
	if meta != provenance {
		t.Fatalf("resolver provenance changed: %+v", provenance)
	}
}

func TestFailedResolveCreatesNoArtifact(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("tools.lock.json", []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	meta, err := toolpublish.NewMetadata("tools.lock.json", "quay.io/example/bundledock-tools", "2026.10.10-123-1", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if err = resolve("resolved", meta, func(...string) ([]byte, error) { return nil, os.ErrNotExist }); err == nil {
		t.Fatal("failed resolver returned success")
	}
	if _, err = os.Stat("resolved"); !os.IsNotExist(err) {
		t.Fatalf("failed resolver created artifact: %v", err)
	}
}

func TestJSONOutputEndsWithNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	if err := writeJSON(path, map[string]string{"sha": "abc"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) || b[len(b)-1] != '\n' {
		t.Fatal("invalid JSON artifact")
	}
}

func TestArtifactRunIdentityRejectsPartialReruns(t *testing.T) {
	meta := toolpublish.Metadata{Tag: "2026.10.10-123-1", Commit: strings.Repeat("a", 40)}
	if err := artifactIdentity(meta, "123", "1", meta.Commit); err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct{ id, attempt, commit string }{{"123", "2", meta.Commit}, {"124", "1", meta.Commit}, {"123", "1", strings.Repeat("b", 40)}} {
		if err := artifactIdentity(meta, run.id, run.attempt, run.commit); err == nil {
			t.Fatal("accepted artifact from another run, attempt or revision")
		}
	}
}
