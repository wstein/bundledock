package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
		if !reflect.DeepEqual(args, []string{"go", "run", "./cmd/bdock", "tools", "upgrade"}) {
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
	artifact, err := os.ReadFile(filepath.Join("resolved", "packages.lock.json"))
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
	sum, err := toolpublish.HashFile("resolved/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	meta.LockSHA256 = sum
	if meta != provenance {
		t.Fatalf("resolver provenance changed: %+v", provenance)
	}
}

func TestPublicationDecisionAgainstPublishedTools(t *testing.T) {
	lock, err := os.ReadFile("../../bundles/toolbox/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "packages.lock.json")
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
	t.Setenv("GITHUB_EVENT_NAME", "")
	t.Setenv("PUBLICATION_SHA", "")
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

func TestResolverFixturesIgnorePublicationEnvironment(t *testing.T) {
	// Publication runs execute make check with these variables in scope. Keep
	// the resolver fixture tests independent of the job's real authorization.
	cmd := exec.Command("go", "test", ".", "-run", "^(TestResolveRejectsInvalidRunCountersBeforeResolving|TestResolveUsesRepositoryURL)$", "-count=1")
	cmd.Env = append(os.Environ(),
		"GITHUB_EVENT_NAME=workflow_run",
		"PUBLICATION_SHA="+strings.Repeat("a", 40),
	)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("resolver fixtures inherited publication authority: %v\n%s", err, data)
	}
}

func TestResolveUsesRepositoryURL(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "")
	t.Setenv("PUBLICATION_SHA", "")
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
			if err := os.WriteFile("go", []byte("#!/bin/sh\n[ \"$*\" = 'run ./cmd/bdock tools upgrade' ]\n"), 0700); err != nil {
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
	if err := execute([]string{"validate", "--bundle", "code-server", "--frozen-lock"}); err == nil {
		t.Fatal("accepted frozen lock outside publication resolution")
	}
	if err := execute([]string{"validate", "--bundle", "../toolbox"}); err == nil {
		t.Fatal("accepted unknown bundle")
	}
}

func TestCodeServerBootstrapOnlyOnAuthenticatedMissingStable(t *testing.T) {
	lock := "../../bundles/code-server/packages.lock.json"
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
	lock := "../../bundles/node/packages.lock.json"
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
	lock := "../../bundles/bun/packages.lock.json"
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

func TestFrozenToolboxLockNeverResolves(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(toolconfig.ToolboxDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	lock := []byte("committed merged lock\n")
	if err := os.WriteFile(toolconfig.ToolboxLock, lock, 0600); err != nil {
		t.Fatal(err)
	}
	meta, err := toolpublish.NewMetadata(toolconfig.ToolboxLock, "quay.io/example/toolbox", "2026.10.10-b12.1", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "123")
	if err != nil {
		t.Fatal(err)
	}
	if err = resolveFrozen("resolved", meta); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("resolved/packages.lock.json")
	if err != nil || string(got) != string(lock) {
		t.Fatalf("frozen lock altered: %q %v", got, err)
	}
	var provenance toolpublish.Metadata
	if err = readJSON("resolved/provenance.json", &provenance); err != nil {
		t.Fatal(err)
	}
	if provenance.Commit != meta.Commit || provenance.LockSHA256 != meta.LockSHA256 {
		t.Fatal("frozen provenance changed")
	}
}

func TestAutomaticPublicationSourceIsExplicitForAllBundles(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "workflow_run")
	t.Setenv("GITHUB_SHA", strings.Repeat("b", 40))
	t.Setenv("PUBLICATION_SHA", strings.Repeat("a", 40))
	got, err := publicationCommit("toolbox")
	if err != nil || got != strings.Repeat("a", 40) {
		t.Fatalf("source=%s error=%v", got, err)
	}
	for _, bundle := range []string{"node", "node-current", "bun", "code-server"} {
		if got, err = publicationCommit(bundle); err != nil || got != strings.Repeat("a", 40) {
			t.Fatal("explicit bundle source rejected", bundle, err)
		}
	}
	if _, err = publicationCommit("unknown"); err == nil {
		t.Fatal("unknown bundle accepted")
	}
	t.Setenv("PUBLICATION_SHA", "")
	if _, err = publicationCommit("toolbox"); err == nil {
		t.Fatal("implicit run SHA used as automatic source")
	}
	t.Setenv("GITHUB_EVENT_NAME", "workflow_dispatch")
	got, err = publicationCommit("toolbox")
	if err != nil || got != strings.Repeat("b", 40) {
		t.Fatal("manual source semantics changed")
	}
}

func TestMergedBundlePlanUsesBaseline(t *testing.T) {
	lock, err := os.ReadFile("../../bundles/toolbox/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "lock.json")
	if err = os.WriteFile(path, lock, 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := toolpublish.ToolDigest(lock)
	if err != nil {
		t.Fatal(err)
	}
	for _, bundle := range []string{"toolbox", "node", "node-current", "bun", "code-server", "codex", "agy", "claude", "opentofu"} {
		publish, err := publicationDecisionForBundle(path, "workflow_run", bundle, func() (string, error) { return digest, nil })
		if err != nil || publish {
			t.Fatalf("unchanged merged lock publication=%v error=%v", publish, err)
		}
	}
	publish, err := publicationDecisionForBundle(path, "workflow_run", "toolbox", func() (string, error) { return "changed", nil })
	if err != nil || !publish {
		t.Fatalf("changed merged lock publication=%v error=%v", publish, err)
	}
	if _, err = publicationDecisionForBundle(path, "workflow_run", "toolbox", func() (string, error) { return "", fmt.Errorf("unavailable") }); err == nil {
		t.Fatal("missing baseline accepted")
	}
}

func TestFrozenPublicationUsesExactLockForEveryBundle(t *testing.T) {
	for _, name := range []string{"toolbox", "code-server", "node", "node-current", "bun", "codex", "agy", "claude", "opentofu"} {
		t.Run(name, func(t *testing.T) {
			bundle, err := toolpublish.SelectBundle(name)
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile("../../" + bundle.Directory + "/packages.lock.json")
			if err != nil {
				t.Fatal(err)
			}
			t.Chdir(t.TempDir())
			if err = os.MkdirAll(bundle.Directory, 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(bundle.Directory+"/packages.lock.json", original, 0600); err != nil {
				t.Fatal(err)
			}
			meta, err := toolpublish.NewMetadataForBundle(name, bundle.Directory+"/packages.lock.json", "quay.io/example/"+bundle.Repository, "2026.10.10-b12.1", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "123")
			if err != nil {
				t.Fatal(err)
			}
			if err = resolveFrozen("resolved", meta); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile("resolved/packages.lock.json")
			if err != nil || string(got) != string(original) {
				t.Fatal("frozen bytes changed", err)
			}
		})
	}
}

func TestAgentInitialPublicationRequiresManualDispatch(t *testing.T) {
	for _, bundle := range []string{"codex", "agy", "claude", "opentofu"} {
		for _, event := range []string{"workflow_dispatch", "workflow_run", "schedule"} {
			publish, err := publicationDecisionForBundle("../../bundles/toolbox/packages.lock.json", event, bundle, func() (string, error) { return "", toolpublish.ErrBaselineAbsent })
			manual := event == "workflow_dispatch"
			if publish != manual || (err == nil) != manual {
				t.Fatalf("%s %s absent baseline: publish=%v err=%v", bundle, event, publish, err)
			}
		}
	}
}

func TestResolveSingletonNamesTheResolvedLockOnce(t *testing.T) {
	original, err := os.ReadFile("../../bundles/codex/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("bundles/codex", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("bundles/codex/packages.lock.json", original, 0600); err != nil {
		t.Fatal(err)
	}
	meta, err := toolpublish.NewMetadata("bundles/codex/packages.lock.json", "quay.io/example/codex", "2026.10.12-b999.7", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-12T00:00:00Z", "123")
	if err != nil {
		t.Fatal(err)
	}
	changed := []byte(strings.ReplaceAll(string(original), "rust-v0.162.1", "rust-v0.163.0"))
	calls := 0
	runner := func(args ...string) ([]byte, error) {
		calls++
		return nil, os.WriteFile("bundles/codex/packages.lock.json", changed, 0600)
	}
	allocate := func(image string, data []byte) (string, error) {
		if string(data) != string(changed) {
			t.Fatal("allocated before upstream resolution")
		}
		return "0.163.0-b1", nil
	}
	if err := resolveSingleton("resolved", meta, false, "7", runner, allocate); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("resolved %d times", calls)
	}
	var got toolpublish.Metadata
	if err := readJSON("resolved/provenance.json", &got); err != nil {
		t.Fatal(err)
	}
	if got.Tag != "0.163.0-b1" || got.RunAttempt != "7" {
		t.Fatalf("wrong artifact: %+v", got)
	}
	if err := artifactIdentity(got, "123", "2345", "7", meta.Commit); err != nil {
		t.Fatal(err)
	}
	if err := artifactIdentity(got, "123", "2345", "8", meta.Commit); err == nil {
		t.Fatal("accepted another attempt")
	}
	data, err := os.ReadFile("resolved/packages.lock.json")
	if err != nil || string(data) != string(changed) {
		t.Fatal("artifact lock changed")
	}
	calls = 0
	if err := resolveSingleton("frozen", got, true, "8", runner, allocate); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("frozen publication resolved upstream")
	}
}

// Exercise the real CLI and approval script with no resolver or registry access.
func TestExecuteFrozenPublication(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("host shell unavailable")
	}
	gate, err := os.ReadFile("../../scripts/approve-publication.sh")
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("a", 40)
	cases := []struct {
		name, key, value string
		wantErr          bool
	}{
		{"approved manual", "", "", false},
		{"approved merged", "GITHUB_EVENT_NAME", "workflow_run", false},
		{"missing approval", "APPROVED_SHA", "", true},
		{"short approval", "APPROVED_SHA", "aaaaaaa", true},
		{"uppercase approval", "APPROVED_SHA", strings.Repeat("A", 40), true},
		{"nonhex approval", "APPROVED_SHA", strings.Repeat("g", 40), true},
		{"different approval", "APPROVED_SHA", strings.Repeat("b", 40), true},
		{"different source", "GITHUB_SHA", strings.Repeat("b", 40), true},
		{"different default", "DEFAULT_BRANCH", "develop", true},
		{"different ref", "GITHUB_REF", "refs/heads/topic", true},
		{"bot sender", "SENDER_TYPE", "Bot", true},
		{"missing sender", "SENDER_TYPE", "", true},
		{"missing actor", "GITHUB_ACTOR", "", true},
		{"bot actor", "GITHUB_ACTOR", "renovate[bot]", true},
		{"actions actor", "GITHUB_ACTOR", "github-actions", true},
		{"missing triggering actor", "TRIGGERING_ACTOR", "", true},
		{"automated rerun", "TRIGGERING_ACTOR", "github-actions[bot]", true},
		{"schedule", "GITHUB_EVENT_NAME", "schedule", true},
		{"push", "GITHUB_EVENT_NAME", "push", true},
		{"PR", "GITHUB_EVENT_NAME", "pull_request", true},
		{"missing event", "GITHUB_EVENT_NAME", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			for _, path := range []string{toolconfig.ToolboxDirectory, "scripts"} {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			lock := []byte("{\"reviewed\":\"exact lock bytes\"}\n")
			if err := os.WriteFile(toolconfig.ToolboxLock, lock, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("scripts/approve-publication.sh", gate, 0600); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"go", "git", "docker", "curl"} {
				if err := os.WriteFile(name, []byte("#!/bin/sh\necho unexpected >> external-calls\nexit 99\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			for key, value := range map[string]string{
				"GITHUB_EVENT_NAME": "workflow_dispatch", "APPROVED_SHA": sha,
				"GITHUB_SHA": sha, "PUBLICATION_SHA": sha, "DEFAULT_BRANCH": "main",
				"GITHUB_REF": "refs/heads/main", "SENDER_TYPE": "User",
				"GITHUB_ACTOR": "wstein", "TRIGGERING_ACTOR": "wstein",
				"REPOSITORY_URL": "quay.io/bundledock", "GITHUB_SERVER_URL": "https://github.com",
				"GITHUB_REPOSITORY": "wstein/bundledock", "GITHUB_RUN_ID": "38419885093",
				"GITHUB_RUN_NUMBER": "12", "GITHUB_RUN_ATTEMPT": "1",
			} {
				t.Setenv(key, value)
			}
			if tc.key != "" {
				t.Setenv(tc.key, tc.value)
			}
			err := execute([]string{"resolve", "--frozen-lock"})
			if (err != nil) != tc.wantErr {
				t.Fatalf("resolve error=%v, wantErr=%t", err, tc.wantErr)
			}
			if _, err := os.Stat("external-calls"); !os.IsNotExist(err) {
				t.Fatalf("unexpected upstream command: %v", err)
			}
			if tc.wantErr {
				if _, err := os.Stat("resolved"); !os.IsNotExist(err) {
					t.Fatalf("rejected approval wrote artifact: %v", err)
				}
				return
			}
			for _, path := range []string{toolconfig.ToolboxLock, "resolved/packages.lock.json"} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != string(lock) {
					t.Fatalf("lock changed at %s: %q %v", path, got, err)
				}
			}
			var meta toolpublish.Metadata
			if err := readJSON("resolved/provenance.json", &meta); err != nil {
				t.Fatal(err)
			}
			sum, err := toolpublish.HashFile(toolconfig.ToolboxLock)
			if err != nil {
				t.Fatal(err)
			}
			if meta.Commit != sha || meta.LockSHA256 != sum || meta.Source != "https://github.com/wstein/bundledock" || meta.Image != "quay.io/bundledock/toolbox" {
				t.Fatalf("wrong frozen provenance: %+v", meta)
			}
		})
	}
}

func TestExecuteFrozenModeRemainsResolveOnlyAndRequiredForMerged(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GITHUB_EVENT_NAME", "workflow_run")
	t.Setenv("PUBLICATION_SHA", strings.Repeat("a", 40))
	for _, args := range [][]string{{"resolve"}, {"plan", "--frozen-lock"}, {"build", "--frozen-lock"}, {"promote", "--frozen-lock"}} {
		if err := execute(args); err == nil {
			t.Fatalf("accepted unsupported mode %v", args)
		}
	}
	if _, err := os.Stat("resolved"); !os.IsNotExist(err) {
		t.Fatalf("invalid mode wrote artifact: %v", err)
	}
}
