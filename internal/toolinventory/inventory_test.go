package toolinventory

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("requires python3: ", err)
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, name := range []string{"internal/toolbundle/python/specify/specify.requirements.txt", "internal/toolbundle/python/tldr/tldr.requirements.txt", "bundles/toolbox/packages.lock.json", "bundles/toolbox/packages.catalog.json", "docs/toolbox.md"} {
		data, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root, filepath.Join(repo, "scripts/tool-inventory.py")
}
func invoke(t *testing.T, root, script string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("python3", append([]string{script, "--root", root}, args...)...)
	data, err := cmd.CombinedOutput()
	return string(data), err
}
func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func save(t *testing.T, name, data string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
func pinnedVersion(t *testing.T, root, tool string) string {
	t.Helper()
	var lock struct {
		Platforms map[string]map[string]struct {
			Version string `json:"version"`
		} `json:"platforms"`
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(root, "bundles/toolbox/packages.lock.json"))), &lock); err != nil {
		t.Fatal(err)
	}
	version := lock.Platforms["amd64"][tool].Version
	if version == "" {
		t.Fatalf("missing fixture version for %s", tool)
	}
	return version
}

func nextGitHubVersion(t *testing.T, root string) string {
	t.Helper()
	var major, minor, patch int
	version := pinnedVersion(t, root, "gh")
	if n, err := fmt.Sscanf(version, "v%d.%d.%d", &major, &minor, &patch); err != nil || n != 3 {
		t.Fatalf("unexpected stable GitHub CLI version %q", version)
	}
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch+1)
}

func changeVersion(t *testing.T, root, arch, version string) {
	t.Helper()
	name := filepath.Join(root, "bundles/toolbox/packages.lock.json")
	var lock map[string]any
	if err := json.Unmarshal([]byte(read(t, name)), &lock); err != nil {
		t.Fatal(err)
	}
	lock["platforms"].(map[string]any)[arch].(map[string]any)["gh"].(map[string]any)["version"] = version
	b, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	save(t, name, string(b))
}
func TestVersionChangeRequiresInventoryUpdate(t *testing.T) {
	root, script := fixture(t)
	name := filepath.Join(root, "docs/toolbox.md")
	before := read(t, name)
	oldVersion := strings.TrimPrefix(pinnedVersion(t, root, "gh"), "v")
	newPin := nextGitHubVersion(t, root)
	newVersion := strings.TrimPrefix(newPin, "v")
	changeVersion(t, root, "amd64", newPin)
	changeVersion(t, root, "arm64", newPin)
	out, err := invoke(t, root, script, "--check")
	if err == nil || !strings.Contains(out, "inventory is stale") {
		t.Fatalf("expected drift failure, err=%v output=%s", err, out)
	}
	if got := read(t, name); got != before {
		t.Fatal("check mode rewrote documentation")
	}
	if out, err := invoke(t, root, script); err != nil {
		t.Fatalf("generate: %v %s", err, out)
	}
	after := read(t, name)
	if !strings.Contains(after, "| GitHub CLI | "+newVersion+" |") {
		t.Fatal("version missing")
	}
	if strings.Replace(after, "| GitHub CLI | "+newVersion+" |", "| GitHub CLI | "+oldVersion+" |", 1) != before {
		t.Fatal("generator changed more than the managed version")
	}
	if out, err := invoke(t, root, script, "--check"); err != nil {
		t.Fatalf("check: %v %s", err, out)
	}
	if out, err := invoke(t, root, script); err != nil {
		t.Fatalf("regenerate: %v %s", err, out)
	}
	if read(t, name) != after {
		t.Fatal("generation was not deterministic")
	}
}
func TestCheckedInInventoryMatchesLock(t *testing.T) {
	root, script := fixture(t)
	if out, err := invoke(t, root, script, "--check"); err != nil {
		t.Fatalf("check: %v %s", err, out)
	}
}
func TestRefuseArchitectureMismatchAndMissingMarkers(t *testing.T) {
	t.Run("architecture mismatch", func(t *testing.T) {
		root, script := fixture(t)
		changeVersion(t, root, "amd64", nextGitHubVersion(t, root))
		out, err := invoke(t, root, script)
		if err == nil || !strings.Contains(out, "architecture versions differ") {
			t.Fatalf("%v %s", err, out)
		}
	})
	t.Run("missing marker", func(t *testing.T) {
		root, script := fixture(t)
		name := filepath.Join(root, "docs/toolbox.md")
		save(t, name, strings.Replace(read(t, name), "<!-- BEGIN GENERATED TOOL INVENTORY -->", "", 1))
		out, err := invoke(t, root, script)
		if err == nil || !strings.Contains(out, "inventory markers") {
			t.Fatalf("%v %s", err, out)
		}
	})
	t.Run("catalog coverage", func(t *testing.T) {
		root, script := fixture(t)
		name := filepath.Join(root, "bundles/toolbox/packages.catalog.json")
		var catalog map[string]any
		if err := json.Unmarshal([]byte(read(t, name)), &catalog); err != nil {
			t.Fatal(err)
		}
		catalog["packages"].(map[string]any)["unexpected-tool"] = map[string]any{}
		b, err := json.Marshal(catalog)
		if err != nil {
			t.Fatal(err)
		}
		save(t, name, string(b))
		out, err := invoke(t, root, script)
		if err == nil || !strings.Contains(out, "catalog IDs") {
			t.Fatalf("%v %s", err, out)
		}
	})
}

func TestInventoryUsesDisplayedLabelAlphabeticalOrder(t *testing.T) {
	root, script := fixture(t)
	if out, err := invoke(t, root, script); err != nil {
		t.Fatalf("generate: %v %s", err, out)
	}
	source := read(t, filepath.Join(root, "docs/toolbox.md"))
	region := strings.Split(strings.Split(source, "<!-- BEGIN GENERATED TOOL INVENTORY -->")[1], "<!-- END GENERATED TOOL INVENTORY -->")[0]
	var labels []string
	for _, line := range strings.Split(region, "\n") {
		if strings.HasPrefix(line, "| ") && !strings.HasPrefix(line, "| Tool |") && !strings.HasPrefix(line, "| ---") {
			labels = append(labels, strings.TrimSpace(strings.Split(line, "|")[1]))
		}
	}
	if len(labels) != 57 {
		t.Fatalf("expected 55 catalog tools and two launchers, got %d rows", len(labels))
	}
	for i := 1; i < len(labels); i++ {
		if strings.ToLower(labels[i-1]) > strings.ToLower(labels[i]) {
			t.Fatalf("display labels not alphabetical: %q before %q", labels[i-1], labels[i])
		}
	}
	if !strings.Contains(region, "| Mike Farah yq (`yq`) |") || !strings.Contains(region, "| tmux (`tmux`) | "+strings.TrimPrefix(pinnedVersion(t, root, "tmux"), "v")+" (official static builds) |") {
		t.Fatal("display labels or versions changed")
	}
}
