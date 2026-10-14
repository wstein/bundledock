package toolconfig

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestDesiredBoundedStableVersions(t *testing.T) {
	desired, err := ParseDesired(strings.NewReader("# tools\ncodex >=0.162.1,<0.163.0 # compatible\ngh==2.102.0\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, version string
		want          bool
	}{
		{"codex", "0.162.1", true}, {"codex", "0.162.9", true}, {"codex", "0.163.0", false}, {"codex", "0.162.2-rc.1", false},
		{"gh", "2.102.0", true}, {"gh", "2.102.1", false},
	} {
		if got := desired[tc.name].Contains(tc.version); got != tc.want {
			t.Errorf("%s %s: %v", tc.name, tc.version, got)
		}
	}
	for _, input := range []string{"gh<3.0.0", "gh>=3.0.0,<2.0.0", "gh~=2.0", "gh==v2.0.0", "gh==02.0.0", "gh==2.0.0\ngh==2.0.0", "gh>=2.0.0, <3.0.0, ==2.1.0", "gh==2.0.0 extra", "gh==2.0.0-beta"} {
		if _, err := ParseDesired(strings.NewReader(input)); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestRepositoryIdentityAndBothArchitectures(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Catalog.Packages) != 41 {
		t.Fatal("catalog incomplete")
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	original := c.Lock.CatalogDigest
	c.Lock.CatalogDigest = strings.Repeat("a", 64)
	if err := c.Validate(); err == nil {
		t.Fatal("accepted changed catalog")
	}
	c.Lock.CatalogDigest = original
	s := c.Lock.Platforms["arm64"]["gh"]
	delete(c.Lock.Platforms["arm64"], "gh")
	if err := c.Validate(); err == nil {
		t.Fatal("accepted missing arm64 tool")
	}
	c.Lock.Platforms["arm64"]["gh"] = s
	s.Version = "v999.0.0"
	c.Lock.Platforms["arm64"]["gh"] = s
	if err := c.Validate(); err == nil {
		t.Fatal("accepted divergent/out of range version")
	}
}

func TestCatalogRejectsIncompleteAndUnsafeRecipes(t *testing.T) {
	for _, kind := range []string{"provider", "platform", "shell", "template", "member", "alias", "claude-base", "normalization"} {
		t.Run(kind, func(t *testing.T) {
			c, err := Load("../../bundles/toolbox")
			if err != nil {
				t.Fatal(err)
			}
			name := "gh"
			if kind == "claude-base" {
				name = "claude"
				c, err = Load("../../bundles/claude")
				if err != nil {
					t.Fatal(err)
				}
			}
			p := c.Catalog.Packages[name]
			switch kind {
			case "provider":
				p.Provider = "shell"
			case "platform":
				delete(p.Architectures, "arm64")
			case "shell":
				p.Install.Kind = "exec"
			case "template":
				p.Asset = "tool-{command}.tar.gz"
			case "member":
				p.Install.Members["../bad"] = "bin/bad"
			case "alias":
				p.Install.Aliases = map[string]string{"bin/alias": "../outside"}
			case "claude-base":
				p.BaseURL = ""
			case "normalization":
				p.Normalization = "sqlite-compact"
			}
			c.Catalog.Packages[name] = p
			c.Bind()
			if err := c.Validate(); err == nil {
				t.Fatalf("accepted invalid %s recipe", kind)
			}
		})
	}
}
func TestStableNumericOrdering(t *testing.T) {
	a, _ := ParseVersion("2.10.0")
	b, _ := ParseVersion("2.9.99")
	if a.Compare(b) <= 0 {
		t.Fatal("lexical version ordering")
	}
	for _, v := range []string{"1.2", "1.2.3.4", "1.2.3+build", "18446744073709551616.0.0", "1.02.3", "v1.2.3"} {
		if _, err := ParseVersion(v); err == nil {
			t.Fatalf("accepted %q", v)
		}
	}
}

func TestCatalogAllowsTarXZ(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages["gh"]
	p.Install.Kind = "tar.xz"
	c.Catalog.Packages["gh"] = p
	c.Bind()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFishAndStarshipCatalogPins(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, kind, version string }{{"fish", "tar.xz", "4.9.3"}, {"starship", "tar.gz", "v1.26.0"}} {
		p := c.Catalog.Packages[tc.name]
		if p.Install.Kind != tc.kind || p.Install.Members[tc.name] != "bin/"+tc.name {
			t.Fatalf("missing %s binary recipe", tc.name)
		}
		for _, arch := range []string{"amd64", "arm64"} {
			if c.Lock.Platforms[arch][tc.name].Version != tc.version {
				t.Fatalf("missing %s/%s pin", tc.name, arch)
			}
		}
	}
}

func TestShellAlternativeCatalogPins(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, kind, version string }{{"nu", "tar.gz", "0.116.1"}, {"tmux", "tar.gz", "v3.7c"}} {
		p, ok := c.Catalog.Packages[tc.name]
		if !ok || p.Install.Kind != tc.kind || p.Install.Members[tc.name] != "bin/"+tc.name {
			t.Fatalf("missing %s binary recipe", tc.name)
		}
		for _, arch := range []string{"amd64", "arm64"} {
			if c.Lock.Platforms[arch][tc.name].Version != tc.version {
				t.Fatalf("missing %s/%s pin", tc.name, arch)
			}
		}
	}
	p := c.Catalog.Packages["tmux"]
	if v, err := p.Version("3.8"); err != nil || v != "3.8.0" {
		t.Fatalf("tmux release normalization: %q %v", v, err)
	}
	for tag, want := range map[string]string{"v3.7a": "3.7.1", "v3.7b": "3.7.2", "v3.7c": "3.7.3", "v3.8": "3.8.0"} {
		if got, err := p.Version(tag); err != nil || got != want {
			t.Fatalf("%s: %s %v", tag, got, err)
		}
	}
	for _, bad := range []string{"v3.7aa", "v3.7-rc", "v3.07c", "v3.7.1", "v3.7C"} {
		if _, err := p.Version(bad); err == nil {
			t.Fatalf("accepted tmux tag %s", bad)
		}
	}
}
func TestLoadInputsAllowsNarrowTmuxRanges(t *testing.T) {
	for _, requirement := range []string{"tmux==3.7.3", "tmux>=3.7.3,<3.8.0"} {
		t.Run(requirement, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{"packages.txt", "packages.catalog.json"} {
				b, err := os.ReadFile(filepath.Join("../../bundles/toolbox", name))
				if err != nil {
					t.Fatal(err)
				}
				if name == "packages.txt" {
					b = []byte(strings.Replace(string(b), "tmux>=3.7.3", requirement, 1))
				}
				if err := os.WriteFile(filepath.Join(dir, name), b, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := LoadInputs(dir); err != nil {
				t.Fatalf("valid desired input rejected: %v", err)
			}
		})
	}
}

func TestQualitySecurityCatalogPins(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, version string }{
		{"typos", "v1.51.1"}, {"lychee", "lychee-v0.24.2"}, {"gitleaks", "v8.30.1"}, {"trivy", "v0.75.0"}, {"syft", "v1.54.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := c.Catalog.Packages[tc.name]
			if p.Install.Kind != "tar.gz" || p.Install.Members[tc.name] != "bin/"+tc.name {
				t.Fatal("missing executable recipe")
			}
			if len(p.Notices)+len(p.Install.ArchiveNotices) == 0 {
				t.Fatal("missing license notices")
			}
			for _, arch := range []string{"amd64", "arm64"} {
				pin := c.Lock.Platforms[arch][tc.name]
				if pin.Version != tc.version || len(pin.SHA256) != 64 {
					t.Fatalf("missing verified %s pin", arch)
				}
			}
		})
	}
}

func TestPreservedArchiveDirectoryExclusions(t *testing.T) {
	for _, dir := range []string{"codex-resources/voice", "", ".", "../voice", "/voice", "codex-resources/../voice", "./voice", "voice/", "voice\\lib", "voice\nlib", "bin"} {
		t.Run(dir, func(t *testing.T) {
			c, err := Load("../../bundles/codex")
			if err != nil {
				t.Fatal(err)
			}
			p := c.Catalog.Packages["codex"]
			p.Install.ExcludeDirectories = []string{dir}
			c.Catalog.Packages["codex"] = p
			c.Bind()
			err = c.Validate()
			if dir == "codex-resources/voice" {
				if err != nil {
					t.Fatal(err)
				}
				p.Install.Preserve = false
				c.Catalog.Packages["codex"] = p
				c.Bind()
				if err := c.Validate(); err == nil {
					t.Fatal("exclusions accepted without preserve")
				}
			} else if err == nil {
				t.Fatal("unsafe or required directory exclusion accepted")
			}
		})
	}
}

func TestRepositoryBundleScriptLayout(t *testing.T) {
	for _, bundle := range []string{"toolbox", "code-server", "node", "node-current", "bun", "codex", "agy", "claude", "opentofu"} {
		t.Run(bundle, func(t *testing.T) {
			root := filepath.Join("../../bundles", bundle)
			if _, err := os.Stat(filepath.Join(root, "scripts/smoke.sh")); err != nil {
				t.Fatal(err)
			}
			obsolete, err := filepath.Glob(filepath.Join(root, "*.sh"))
			if err != nil || len(obsolete) != 0 {
				t.Fatalf("bundle-root shell scripts: %v, %v", obsolete, err)
			}
		})
	}
}

func TestRepositoryToolboxLayout(t *testing.T) {
	for _, name := range []string{"packages.txt", "packages.catalog.json", "packages.lock.json", "Dockerfile", "Dockerfile.dockerignore", "scripts/smoke.sh", "scripts/volume.sh"} {
		if _, err := os.Stat(filepath.Join("../../bundles/toolbox", name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, old := range []string{"../../packages.txt", "../../packages.catalog.json", "../../packages.lock.json", "../../oci"} {
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Fatalf("obsolete duplicate bundle path %s", old)
		}
	}
}

// Package input filenames are shared by strict loading, upgrades and image builders.
func TestPackageFilenamesLoadAllBundleInputs(t *testing.T) {
	for _, bundle := range []string{"toolbox", "code-server", "node", "node-current", "bun", "codex", "agy", "claude", "opentofu"} {
		t.Run(bundle, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{"packages.txt", "packages.catalog.json", "packages.lock.json"} {
				data, err := os.ReadFile(filepath.Join("../../bundles", bundle, name))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			strict, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			upgrade, err := LoadForUpgrade(dir)
			if err != nil {
				t.Fatal(err)
			}
			inputs, err := LoadInputs(dir)
			if err != nil {
				t.Fatal(err)
			}
			if strict.DesiredDigest() != upgrade.DesiredDigest() || strict.DesiredDigest() != inputs.DesiredDigest() || strict.CatalogDigest() != inputs.CatalogDigest() {
				t.Fatal("package input identity differs across loaders")
			}
		})
	}
}

func TestPackageListsHaveMaintenanceHeaderAndAlphabeticKeys(t *testing.T) {
	paths, err := filepath.Glob("../../bundles/*/packages.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 9 {
		t.Fatalf("expected all nine package lists, got %d", len(paths))
	}
	const header = "# package list should be sorted alphabetically for easier maintenance."
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), header+"\n") {
			t.Errorf("%s: missing maintenance header", path)
		}
		var names []string
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			names = append(names, strings.FieldsFunc(line, func(r rune) bool { return strings.ContainsRune("<>= \t", r) })[0])
		}
		if !sort.StringsAreSorted(names) {
			t.Errorf("%s: package keys are not alphabetic: %v", path, names)
		}
	}
}
