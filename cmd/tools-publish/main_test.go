package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"bundledock/internal/toolconfig"
	"bundledock/internal/toolpublish"
)

func TestResolveOnceAndKeepExactArtifactBytes(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(toolconfig.ToolboxDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	initial := []byte("{\"old\":true}\n")
	resolved := []byte("{\"resolved\":true}\n")
	if err := os.WriteFile(toolconfig.ToolboxLock, initial, 0600); err != nil {
		t.Fatal(err)
	}
	meta, err := toolpublish.NewMetadata(toolconfig.ToolboxLock, "quay.io/example/toolbox", "2026.10.10-b12.1", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "38069722044")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runner := func(args ...string) ([]byte, error) {
		calls++
		if !reflect.DeepEqual(args, []string{"go", "run", "./cmd/bundledock", "tools", "upgrade"}) {
			t.Fatalf("unexpected resolver command %v", args)
		}
		return nil, os.WriteFile(toolconfig.ToolboxLock, resolved, 0600)
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

func TestPublicationDecisionAgainstPublishedTools(t *testing.T) {
	lock, err := os.ReadFile("../../bundles/toolbox/tools.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.lock.json")
	if err = os.WriteFile(path, lock, 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := toolpublish.ToolDigest(lock)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		event, baseline string
		baselineErr     error
		want            bool
		wantErr         bool
	}{
		{"schedule", digest, nil, false, false},
		{"schedule", strings.Repeat("0", 64), nil, true, false},
		{"workflow_dispatch", digest, nil, true, false},
		{"workflow_dispatch", "", fmt.Errorf("registry unavailable"), false, true},
		{"schedule", "", fmt.Errorf("artifact expired"), false, true},
	} {
		calls := 0
		got, err := publicationDecision(path, test.event, func() (string, error) { calls++; return test.baseline, test.baselineErr })
		if got != test.want || (err != nil) != test.wantErr || calls != 1 {
			t.Fatalf("event=%s got=%t err=%v calls=%d", test.event, got, err, calls)
		}
	}
}

func TestFailedResolveCreatesNoArtifact(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(toolconfig.ToolboxDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(toolconfig.ToolboxLock, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	meta, err := toolpublish.NewMetadata(toolconfig.ToolboxLock, "quay.io/example/toolbox", "2026.10.10-b12.1", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "38069722044")
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
	meta := toolpublish.Metadata{Tag: "2026.10.10-b12.4", RunID: "38069722044", Commit: strings.Repeat("a", 40)}
	if err := artifactIdentity(meta, meta.RunID, "12", "4", meta.Commit); err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct{ id, number, attempt, commit string }{
		{meta.RunID, "12", "5", meta.Commit}, {meta.RunID, "13", "4", meta.Commit},
		{"38069722045", "12", "4", meta.Commit}, {meta.RunID, "12", "4", strings.Repeat("b", 40)},
		{"", "12", "4", meta.Commit}, {meta.RunID, "", "4", meta.Commit}, {meta.RunID, "12", "", meta.Commit},
	} {
		if err := artifactIdentity(meta, run.id, run.number, run.attempt, run.commit); err == nil {
			t.Fatalf("accepted foreign identity: %+v", run)
		}
	}
}

func TestResolveRejectsInvalidRunCountersBeforeResolving(t *testing.T) {
	for _, name := range []string{"GITHUB_RUN_NUMBER", "GITHUB_RUN_ATTEMPT"} {
		for _, value := range []string{"", "0", "-1", "01", "+1", "1.2", "abc", " 1"} {
			t.Run(name+"="+value, func(t *testing.T) {
				t.Chdir(t.TempDir())
				if err := os.MkdirAll(toolconfig.ToolboxDirectory, 0700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("REPOSITORY_URL", "quay.io/example")
				t.Setenv("GITHUB_RUN_ID", "38069722044")
				t.Setenv("GITHUB_RUN_NUMBER", "12")
				t.Setenv("GITHUB_RUN_ATTEMPT", "4")
				t.Setenv("GITHUB_SHA", strings.Repeat("a", 40))
				t.Setenv("GITHUB_SERVER_URL", "https://github.com")
				t.Setenv("GITHUB_REPOSITORY", "example/repo")
				t.Setenv(name, value)
				if err := execute([]string{"resolve"}); err == nil || !strings.Contains(err.Error(), "tag") {
					t.Fatalf("expected tag rejection before resolver, got %v", err)
				}
				if _, err := os.Stat("resolved"); !os.IsNotExist(err) {
					t.Fatalf("created invalid artifact: %v", err)
				}
			})
		}
	}
}

func TestResolveUsesRepositoryURL(t *testing.T) {
	for _, prefix := range []string{"quay.io/bundledock", "quay.io/bundledock/"} {
		t.Run(prefix, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			if err := os.MkdirAll(toolconfig.ToolboxDirectory, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(toolconfig.ToolboxLock, []byte("{}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			// Exercise execute's real environment adapter without resolving upstreams.
			if err := os.WriteFile("go", []byte("#!/bin/sh\n[ \"$*\" = 'run ./cmd/bundledock tools upgrade' ]\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			t.Setenv("REPOSITORY_URL", prefix)
			t.Setenv("QUAY_IMAGE", "quay.io/obsolete/bundledock-toolbox")
			t.Setenv("GITHUB_RUN_ID", "38069722044")
			t.Setenv("GITHUB_RUN_NUMBER", "12")
			t.Setenv("GITHUB_RUN_ATTEMPT", "1")
			t.Setenv("GITHUB_SHA", strings.Repeat("a", 40))
			t.Setenv("GITHUB_SERVER_URL", "https://github.com")
			t.Setenv("GITHUB_REPOSITORY", "wstein/bundledock")
			if err := execute([]string{"resolve"}); err != nil {
				t.Fatal(err)
			}
			var meta toolpublish.Metadata
			if err := readJSON("resolved/provenance.json", &meta); err != nil {
				t.Fatal(err)
			}
			if meta.Tag != time.Now().UTC().Format("2006.01.02")+"-b12.1" || meta.RunID != "38069722044" {
				t.Fatalf("wrong run provenance: %+v", meta)
			}
			if meta.Image != "quay.io/bundledock/toolbox" {
				t.Fatalf("resolved wrong publication destination: %s", meta.Image)
			}
		})
	}
}

func TestValidateCodeServerLockWithoutPublicationEnvironment(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if err := execute([]string{"validate", "--bundle", "code-server"}); err != nil {
		t.Fatal(err)
	}
	if err := execute([]string{"validate", "--bundle", "../toolbox"}); err == nil {
		t.Fatal("accepted unknown bundle")
	}
}

func TestCodeServerBootstrapOnlyOnAuthenticatedMissingStable(t *testing.T) {
	lock := "../../bundles/code-server/tools.lock.json"
	for _, tc := range []struct {
		name, bundle, event string
		baselineErr         error
		want, fail          bool
	}{
		{"initial code-server", "code-server", "workflow_dispatch", toolpublish.ErrBaselineAbsent, true, false},
		{"toolbox absent", "toolbox", "workflow_dispatch", toolpublish.ErrBaselineAbsent, false, true},
		{"scheduled absent", "code-server", "schedule", toolpublish.ErrBaselineAbsent, false, true},
		{"lookup failure", "code-server", "workflow_dispatch", fmt.Errorf("registry unavailable"), false, true},
		{"existing baseline", "code-server", "workflow_dispatch", nil, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := publicationDecisionForBundle(lock, tc.event, tc.bundle, func() (string, error) { return strings.Repeat("a", 64), tc.baselineErr })
			if got != tc.want || (err != nil) != tc.fail {
				t.Fatalf("got %v %v", got, err)
			}
		})
	}
}

func TestValidateNodeLockWithoutPublicationEnvironment(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if err := execute([]string{"validate", "--bundle", "node"}); err != nil {
		t.Fatal(err)
	}
	if err := execute([]string{"validate", "--bundle", "../toolbox"}); err == nil {
		t.Fatal("accepted unknown bundle")
	}
}

func TestNodeBootstrapOnlyOnAuthenticatedMissingStable(t *testing.T) {
	lock := "../../bundles/node/tools.lock.json"
	for _, tc := range []struct {
		name, bundle, event string
		baselineErr         error
		want, fail          bool
	}{
		{"initial node", "node", "workflow_dispatch", toolpublish.ErrBaselineAbsent, true, false},
		{"toolbox absent", "toolbox", "workflow_dispatch", toolpublish.ErrBaselineAbsent, false, true},
		{"scheduled absent", "node", "schedule", toolpublish.ErrBaselineAbsent, false, true},
		{"lookup failure", "node", "workflow_dispatch", fmt.Errorf("registry unavailable"), false, true},
		{"existing baseline", "node", "workflow_dispatch", nil, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := publicationDecisionForBundle(lock, tc.event, tc.bundle, func() (string, error) { return strings.Repeat("a", 64), tc.baselineErr })
			if got != tc.want || (err != nil) != tc.fail {
				t.Fatalf("got %v %v", got, err)
			}
		})
	}
}

func TestValidateBunLockWithoutPublicationEnvironment(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if err := execute([]string{"validate", "--bundle", "bun"}); err != nil {
		t.Fatal(err)
	}
	if err := execute([]string{"validate", "--bundle", "../toolbox"}); err == nil {
		t.Fatal("accepted unknown bundle")
	}
}

func TestBunBootstrapOnlyOnAuthenticatedMissingStable(t *testing.T) {
	lock := "../../bundles/bun/tools.lock.json"
	for _, tc := range []struct {
		name, bundle, event string
		baselineErr         error
		want, fail          bool
	}{
		{"initial bun", "bun", "workflow_dispatch", toolpublish.ErrBaselineAbsent, true, false},
		{"toolbox absent", "toolbox", "workflow_dispatch", toolpublish.ErrBaselineAbsent, false, true},
		{"scheduled absent", "bun", "schedule", toolpublish.ErrBaselineAbsent, false, true},
		{"lookup failure", "bun", "workflow_dispatch", fmt.Errorf("registry unavailable"), false, true},
		{"existing baseline", "bun", "workflow_dispatch", nil, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := publicationDecisionForBundle(lock, tc.event, tc.bundle, func() (string, error) { return strings.Repeat("a", 64), tc.baselineErr })
			if got != tc.want || (err != nil) != tc.fail {
				t.Fatalf("got %v %v", got, err)
			}
		})
	}
}
