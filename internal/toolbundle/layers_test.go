package toolbundle

import (
	"bundledock/internal/toolconfig"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func pkg(members map[string]string, aliases map[string]string) toolconfig.Package {
	return toolconfig.Package{Install: toolconfig.Install{Kind: "tar.gz", Members: members, Aliases: aliases}}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

const bundleRel = "opt/bdock/toolbox"

func TestParseLayerPlanRejectsBadPlans(t *testing.T) {
	for name, text := range map[string]string{
		"unknown field":  `{"schema":1,"layers":[{"name":"a","tools":["x"]}],"extra":1}`,
		"wrong schema":   `{"schema":2,"layers":[{"name":"a","tools":["x"]}]}`,
		"no layers":      `{"schema":1,"layers":[]}`,
		"duplicate name": `{"schema":1,"layers":[{"name":"a","tools":["x"]},{"name":"a","tools":["y"]}]}`,
		"bad name":       `{"schema":1,"layers":[{"name":"A b","tools":["x"]}]}`,
		"reserved meta":  `{"schema":1,"layers":[{"name":"meta","tools":["x"]}]}`,
		"duplicate tool": `{"schema":1,"layers":[{"name":"a","tools":["x"]},{"name":"b","tools":["x"]}]}`,
		"empty layer":    `{"schema":1,"layers":[{"name":"a","tools":[]}]}`,
		"trailing data":  `{"schema":1,"layers":[{"name":"a","tools":["x"]}]} {}`,
	} {
		if _, err := ParseLayerPlan(strings.NewReader(text)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	p, err := ParseLayerPlan(strings.NewReader(`{"schema":1,"layers":[{"name":"a","tools":["x"]},{"name":"b","tools":["y"]}]}`))
	if err != nil || len(p.Layers) != 2 {
		t.Fatalf("valid plan: %v %v", p, err)
	}
}

func TestLayerPlanMustCoverCatalogExactly(t *testing.T) {
	pkgs := map[string]toolconfig.Package{"x": pkg(map[string]string{"x": "bin/x"}, nil), "y": pkg(map[string]string{"y": "bin/y"}, nil)}
	plan := &LayerPlan{Schema: 1, Layers: []Layer{{Name: "a", Tools: []string{"x"}}}}
	if err := plan.CheckCovers(pkgs); err == nil || !strings.Contains(err.Error(), "y") {
		t.Errorf("tool y missing from the plan was accepted: %v", err)
	}
	plan.Layers[0].Tools = []string{"x", "y", "z"}
	if err := plan.CheckCovers(pkgs); err == nil || !strings.Contains(err.Error(), "z") {
		t.Errorf("tool z not in the catalog was accepted: %v", err)
	}
	plan.Layers[0].Tools = []string{"x", "y"}
	if err := plan.CheckCovers(pkgs); err != nil {
		t.Error(err)
	}
}

func TestSplitLayersMovesEachToolToItsLayerAndTheRestToMeta(t *testing.T) {
	root, out := t.TempDir(), filepath.Join(t.TempDir(), "layers")
	files := map[string]string{
		bundleRel + "/bin/big":          "1",
		bundleRel + "/bin/fd":           "2",
		bundleRel + "/bin/fdfind":       "2",
		bundleRel + "/libexec/gh-stack": "3",
		bundleRel + "/bin/small":        "4",
		bundleRel + "/manifest.json":    "m",
		bundleRel + "/SHA256SUMS":       "s",
		bundleRel + "/share/shell/a":    "a",
		"usr/local/bin/init-volume":     "i",
	}
	writeTree(t, root, files)
	pkgs := map[string]toolconfig.Package{
		"big":      pkg(map[string]string{"big": "bin/big"}, nil),
		"fd":       pkg(map[string]string{"fd": "bin/fd"}, map[string]string{"bin/fdfind": "bin/fd"}),
		"gh-stack": pkg(map[string]string{"gh-stack": "libexec/gh-stack"}, nil),
		"small":    pkg(map[string]string{"small": "bin/small"}, nil),
	}
	plan := &LayerPlan{Schema: 1, Layers: []Layer{{Name: "base", Tools: []string{"big", "fd"}}, {Name: "top", Tools: []string{"gh-stack", "small"}}}}
	names, err := SplitLayers(root, bundleRel, out, plan, pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "01-base,02-top,03-meta" {
		t.Fatalf("layer directories %v", names)
	}
	want := map[string][]string{
		"01-base": {bundleRel + "/bin/big", bundleRel + "/bin/fd", bundleRel + "/bin/fdfind"},
		"02-top":  {bundleRel + "/bin/small", bundleRel + "/libexec/gh-stack"},
		"03-meta": {bundleRel + "/SHA256SUMS", bundleRel + "/manifest.json", bundleRel + "/share/shell/a", "usr/local/bin/init-volume"},
	}
	var all []string
	for layer, w := range want {
		got := listFiles(t, filepath.Join(out, layer))
		if strings.Join(got, "\n") != strings.Join(w, "\n") {
			t.Errorf("%s: %v, want %v", layer, got, w)
		}
		all = append(all, got...)
	}
	if len(all) != len(files) {
		t.Errorf("%d files after the split, %d before", len(all), len(files))
	}
	data, _ := os.ReadFile(filepath.Join(out, "01-base", bundleRel, "bin/fd"))
	if string(data) != "2" {
		t.Errorf("content changed: %q", data)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Error("the source root still exists; the remainder must move to the meta layer")
	}
}

func TestSplitLayersKeepsDirectoryModes(t *testing.T) {
	root, out := t.TempDir(), filepath.Join(t.TempDir(), "layers")
	writeTree(t, root, map[string]string{bundleRel + "/bin/x": "x"})
	if err := os.Chmod(filepath.Join(root, bundleRel, "bin"), 0o750); err != nil {
		t.Fatal(err)
	}
	plan := &LayerPlan{Schema: 1, Layers: []Layer{{Name: "a", Tools: []string{"x"}}}}
	if _, err := SplitLayers(root, bundleRel, out, plan, map[string]toolconfig.Package{"x": pkg(map[string]string{"x": "bin/x"}, nil)}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(out, "01-a", bundleRel, "bin"))
	if err != nil || info.Mode().Perm() != 0o750 {
		t.Fatalf("mode %v err %v", info, err)
	}
}

func TestSplitLayersFailsClosed(t *testing.T) {
	plan := &LayerPlan{Schema: 1, Layers: []Layer{{Name: "a", Tools: []string{"x"}}}}
	for name, tc := range map[string]struct {
		files map[string]string
		pkgs  map[string]toolconfig.Package
	}{
		"missing file": {map[string]string{bundleRel + "/manifest.json": "m"}, map[string]toolconfig.Package{"x": pkg(map[string]string{"x": "bin/x"}, nil)}},
		"tool not in plan": {map[string]string{bundleRel + "/bin/x": "x", bundleRel + "/bin/y": "y"},
			map[string]toolconfig.Package{"x": pkg(map[string]string{"x": "bin/x"}, nil), "y": pkg(map[string]string{"y": "bin/y"}, nil)}},
		"preserved tree": {map[string]string{bundleRel + "/bin/x": "x"},
			map[string]toolconfig.Package{"x": {Install: toolconfig.Install{Kind: "tar.gz", Preserve: true, Members: map[string]string{"x": "bin/x"}}}}},
		"escaping path": {map[string]string{bundleRel + "/bin/x": "x"}, map[string]toolconfig.Package{"x": pkg(map[string]string{"x": "../../x"}, nil)}},
	} {
		root := t.TempDir()
		writeTree(t, root, tc.files)
		if _, err := SplitLayers(root, bundleRel, filepath.Join(t.TempDir(), "o"), plan, tc.pkgs); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{bundleRel + "/bin/x": "x"})
	out := t.TempDir()
	if _, err := SplitLayers(root, bundleRel, out, plan, map[string]toolconfig.Package{"x": pkg(map[string]string{"x": "bin/x"}, nil)}); err == nil {
		t.Error("existing output directory accepted")
	}
}

// The committed plan must name exactly the committed catalog's tools.
func TestToolboxLayerPlanCoversTheCatalog(t *testing.T) {
	dir := filepath.Join("..", "..", "bundles", "toolbox")
	c, err := toolconfig.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(dir, "layers.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	plan, err := ParseLayerPlan(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.CheckCovers(c.Catalog.Packages); err != nil {
		t.Fatal(err)
	}
	for name, p := range c.Catalog.Packages {
		if _, err := OwnedPaths(p); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
