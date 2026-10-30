package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bundledock/internal/toolpublish"
)

func TestNixboxFrozenResolveKeepsTheExactLock(t *testing.T) {
	original, err := os.ReadFile("../../bundles/nixbox/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	if err = os.MkdirAll("bundles/nixbox", 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("bundles/nixbox/packages.lock.json", original, 0600); err != nil {
		t.Fatal(err)
	}
	meta, err := toolpublish.NewMetadataForBundle("nixbox", "bundles/nixbox/packages.lock.json", "quay.io/example/nixbox", "2026.10.10-b1", strings.Repeat("a", 40), "https://github.com/example/repo", "2026-10-10T03:17:00Z", "123", "1")
	if err != nil {
		t.Fatal(err)
	}
	// The allocator hands out the date tag of a date-tagged bundle; resolveVersion
	// never runs the tool upgrader when frozen.
	runner := func(args ...string) ([]byte, error) { t.Fatal("frozen publication resolved upstream"); return nil, nil }
	if err = resolveVersion("resolved", meta, true, "1", runner, func(string, []byte) (string, error) { return "2026.10.10-b1", nil }); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("resolved/packages.lock.json")
	if err != nil || string(got) != string(original) {
		t.Fatal("frozen bytes changed", err)
	}
}

func TestNixboxPublishesOnlyByManualDispatch(t *testing.T) {
	path := "../../bundles/nixbox/packages.lock.json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := toolpublish.LockDigest("nixbox", data)
	if err != nil {
		t.Fatal(err)
	}
	absent := func() (string, error) { return "", toolpublish.ErrBaselineAbsent }
	// Selecting nixbox in the dispatch form is the owner's approval of its first publication.
	if publish, err := publicationDecisionForBundle(path, "workflow_dispatch", "nixbox", absent); err != nil || !publish {
		t.Fatalf("first publication by dispatch: %v %v", publish, err)
	}
	if publish, err := publicationDecisionForBundle(path, "workflow_dispatch", "nixbox", func() (string, error) { return digest, nil }); err != nil || !publish {
		t.Fatalf("republication by dispatch: %v %v", publish, err)
	}
	// Nothing else publishes it, even for a changed or absent baseline.
	for _, event := range []string{"workflow_run", "schedule"} {
		for _, baseline := range []func() (string, error){absent, func() (string, error) { return digest, nil }, func() (string, error) { return strings.Repeat("0", 64), nil }} {
			if publish, err := publicationDecisionForBundle(path, event, "nixbox", baseline); err == nil || publish {
				t.Fatalf("%s published nixbox: %v %v", event, publish, err)
			}
		}
	}
}

func TestNixboxIsValidatedAsAStoreLockNotAToolLock(t *testing.T) {
	t.Chdir("../..")
	if err := execute([]string{"validate", "--bundle", "nixbox"}); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "nixbox")
	if err := os.MkdirAll(bad, 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateBundleSource(toolpublish.Bundle{Name: "nixbox", Directory: bad}); err == nil {
		t.Fatal("empty nixbox directory accepted")
	}
}

func TestNixboxHasNoUpstreamResolution(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GITHUB_EVENT_NAME", "workflow_dispatch")
	t.Setenv("GITHUB_SHA", strings.Repeat("a", 40))
	t.Setenv("REPOSITORY_URL", "quay.io/example")
	err := execute([]string{"resolve", "--bundle", "nixbox"})
	if err == nil || !strings.Contains(err.Error(), "nixbox has no upstream resolution") {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat("resolved"); !os.IsNotExist(err) {
		t.Fatalf("wrote an artifact: %v", err)
	}
}
