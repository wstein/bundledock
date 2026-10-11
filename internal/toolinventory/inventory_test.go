package toolinventory

import (
	"encoding/json"
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
	for _, name := range []string{"bundles/toolbox/tools.lock.json", "bundles/toolbox/tools.packages.json", "docs/tools.md"} {
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
func changeVersion(t *testing.T, root, arch, version string) {
	t.Helper()
	name := filepath.Join(root, "bundles/toolbox/tools.lock.json")
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
	name := filepath.Join(root, "docs/tools.md")
	before := read(t, name)
	changeVersion(t, root, "amd64", "v2.103.0")
	changeVersion(t, root, "arm64", "v2.103.0")
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
	if !strings.Contains(after, "| GitHub CLI | 2.103.0 |") {
		t.Fatal("version missing")
	}
	if strings.Replace(after, "| GitHub CLI | 2.103.0 |", "| GitHub CLI | 2.102.0 |", 1) != before {
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
		changeVersion(t, root, "amd64", "v2.103.0")
		out, err := invoke(t, root, script)
		if err == nil || !strings.Contains(out, "architecture versions differ") {
			t.Fatalf("%v %s", err, out)
		}
	})
	t.Run("missing marker", func(t *testing.T) {
		root, script := fixture(t)
		name := filepath.Join(root, "docs/tools.md")
		save(t, name, strings.Replace(read(t, name), "<!-- BEGIN GENERATED TOOL INVENTORY -->", "", 1))
		out, err := invoke(t, root, script)
		if err == nil || !strings.Contains(out, "inventory markers") {
			t.Fatalf("%v %s", err, out)
		}
	})
	t.Run("catalog coverage", func(t *testing.T) {
		root, script := fixture(t)
		name := filepath.Join(root, "bundles/toolbox/tools.packages.json")
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
