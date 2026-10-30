package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"bundledock/internal/toolpublish"
)

func TestNixboxFrozenResolveKeepsTheExactLock(t *testing.T) {
	original, err := os.ReadFile("../../bundles/nixbox/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	tools, err := os.ReadFile("../../bundles/nixbox/tools/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	if err = os.MkdirAll("bundles/nixbox/tools", 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("bundles/nixbox/packages.lock.json", original, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("bundles/nixbox/tools/packages.lock.json", tools, 0600); err != nil {
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
	// The lock of the official binaries travels with it, and the recorded hash covers both.
	got, err = os.ReadFile("resolved/tools/packages.lock.json")
	if err != nil || string(got) != string(tools) {
		t.Fatal("frozen tools lock missing or changed", err)
	}
	var recorded toolpublish.Metadata
	data, err := os.ReadFile("resolved/provenance.json")
	if err != nil || json.Unmarshal(data, &recorded) != nil {
		t.Fatal("provenance", err)
	}
	if want, err := toolpublish.LockSHA256("nixbox", "resolved/packages.lock.json"); err != nil || recorded.LockSHA256 != want {
		t.Fatalf("recorded %s, want %s (%v)", recorded.LockSHA256, want, err)
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

func TestNixboxValidationCoversTheToolsLock(t *testing.T) {
	repo, err := filepath.Abs("../../bundles/nixbox")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	for _, name := range []string{"packages.catalog.json", "packages.lock.json", "tools/packages.catalog.json", "tools/packages.lock.json", "tools/packages.txt"} {
		data, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.MkdirAll(filepath.Join("bundles/nixbox", filepath.Dir(name)), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join("bundles/nixbox", name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	bundle := toolpublish.Bundle{Name: "nixbox", Directory: "bundles/nixbox"}
	if err := validateBundleSource(bundle); err != nil {
		t.Fatal(err)
	}
	lock, err := os.ReadFile("bundles/nixbox/tools/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(lock), `"sha256": "`, `"sha256": "00`, 1)
	if err = os.WriteFile("bundles/nixbox/tools/packages.lock.json", []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateBundleSource(bundle); err == nil {
		t.Fatal("a corrupted tools lock passed nixbox validation")
	}
}

// A shim naming a file no recipe installs passes every other check (the Nix lock and
// the tools lock are untouched and valid); only CheckOfficial catches it.
func TestNixboxValidationRefusesAShimWithoutItsRecipe(t *testing.T) {
	repo, err := filepath.Abs("../../bundles/nixbox")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	for _, name := range []string{"packages.catalog.json", "packages.lock.json", "tools/packages.catalog.json", "tools/packages.lock.json", "tools/packages.txt"} {
		data, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "packages.catalog.json" {
			data = []byte(strings.Replace(string(data), `"binary": "bin/zk"`, `"binary": "bin/not-installed"`, 1))
		}
		if err = os.MkdirAll(filepath.Join("bundles/nixbox", filepath.Dir(name)), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join("bundles/nixbox", name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// The catalog digest in the lock no longer matches, so rebuild a consistent pair first.
	lock, _ := os.ReadFile("bundles/nixbox/packages.lock.json")
	catalog, _ := os.ReadFile("bundles/nixbox/packages.catalog.json")
	sum := fmt.Sprintf("%x", sha256.Sum256(catalog))
	lock = regexp.MustCompile(`("catalog_digest": ")[0-9a-f]{64}`).ReplaceAll(lock, []byte("${1}"+sum))
	if err = os.WriteFile("bundles/nixbox/packages.lock.json", lock, 0600); err != nil {
		t.Fatal(err)
	}
	err = validateBundleSource(toolpublish.Bundle{Name: "nixbox", Directory: "bundles/nixbox"})
	if err == nil || !strings.Contains(err.Error(), "no tools recipe installs") {
		t.Fatalf("%v", err)
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

func TestBuildRestoresTheResolvedToolsLockForNixbox(t *testing.T) {
	t.Chdir(t.TempDir())
	for path, body := range map[string]string{
		"resolved/packages.lock.json": "resolved nix", "resolved/tools/packages.lock.json": "resolved tools",
		"bundles/nixbox/packages.lock.json": "old nix", "bundles/nixbox/tools/packages.lock.json": "old tools",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	bundle := toolpublish.Bundle{Name: "nixbox", Directory: "bundles/nixbox"}
	if err := restoreResolvedLocks(bundle, "resolved/packages.lock.json"); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"bundles/nixbox/packages.lock.json": "resolved nix", "bundles/nixbox/tools/packages.lock.json": "resolved tools"} {
		if got, _ := os.ReadFile(path); string(got) != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
	// Another bundle never touches a tools lock.
	if err := os.MkdirAll("bundles/node", 0700); err != nil {
		t.Fatal(err)
	}
	if err := restoreResolvedLocks(toolpublish.Bundle{Name: "node", Directory: "bundles/node"}, "resolved/packages.lock.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("bundles/node/tools"); err == nil {
		t.Error("a non-nixbox bundle got a tools lock")
	}
	// A missing resolved tools lock fails.
	if err := os.Remove("resolved/tools/packages.lock.json"); err != nil {
		t.Fatal(err)
	}
	if err := restoreResolvedLocks(bundle, "resolved/packages.lock.json"); err == nil {
		t.Error("restored without the resolved tools lock")
	}
}
