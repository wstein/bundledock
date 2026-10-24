package toolconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	if len(c.Catalog.Packages) != 63 {
		t.Fatal("catalog incomplete")
	}
	// A tool is an official prebuilt release unless it is a reviewed static
	// source build that meets the AGENTS.md conditions.
	if _, ok := c.Catalog.Packages["git-trim"]; ok {
		t.Fatal("catalog still lists git-trim")
	}
	for name, p := range c.Catalog.Packages {
		if p.Install.Profile == "git-trim-source" {
			t.Fatalf("%s uses the removed git-trim source profile", name)
		}
	}
	rawCatalog, err := os.ReadFile("../../bundles/toolbox/packages.catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Packages map[string]map[string]any `json:"packages"`
	}
	if err := json.Unmarshal(rawCatalog, &catalog); err != nil {
		t.Fatal(err)
	}
	var lock map[string]any
	rawLock, err := os.ReadFile("../../bundles/toolbox/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawLock, &lock); err != nil {
		t.Fatal(err)
	}
	for name, pkg := range catalog.Packages {
		for _, v := range sourceBuildViolations(name, pkg, lock) {
			t.Errorf("%s: %s", name, v)
		}
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
	for _, tc := range []struct{ name, kind string }{{"fish", "tar.xz"}, {"starship", "tar.gz"}} {
		p := c.Catalog.Packages[tc.name]
		if p.Install.Kind != tc.kind || p.Install.Members[tc.name] != "bin/"+tc.name {
			t.Fatalf("missing %s binary recipe", tc.name)
		}
		assertRollingStablePins(t, c, tc.name)
	}
}

func TestShellAlternativeCatalogPins(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, kind string }{{"nu", "tar.gz"}, {"tmux", "tar.gz"}} {
		p, ok := c.Catalog.Packages[tc.name]
		if !ok || p.Install.Kind != tc.kind || p.Install.Members[tc.name] != "bin/"+tc.name {
			t.Fatalf("missing %s binary recipe", tc.name)
		}
		assertRollingStablePins(t, c, tc.name)
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
	// Stable versions roll forward through lock-only updater PRs. The desired
	// constraints own the version floor; this test owns recipe and pin integrity.
	for _, name := range []string{"typos", "lychee", "gitleaks", "trivy", "syft"} {
		t.Run(name, func(t *testing.T) {
			p := c.Catalog.Packages[name]
			if p.Install.Kind != "tar.gz" || p.Install.Members[name] != "bin/"+name {
				t.Fatal("missing executable recipe")
			}
			if len(p.Notices)+len(p.Install.ArchiveNotices) == 0 {
				t.Fatal("missing license notices")
			}
			assertRollingPublisherPins(t, c, name)
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

func TestLockRejectsSourceBuildPins(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"packages.txt", "packages.catalog.json", "packages.lock.json"} {
		data, err := os.ReadFile(filepath.Join("../../bundles/toolbox", name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "packages.lock.json" {
			// A pin carrying a source-build block, as the removed git-trim recipe had.
			data = []byte(strings.Replace(string(data), `"verification": "publisher-sha256"`, `"verification": "publisher-sha256", "source": {"commit": "x"}`, 1))
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("accepted a lock with a source-build pin")
	}
}

var (
	hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }
func str(v any) string         { s, _ := v.(string); return s }

// sourceBuildViolations reports why a raw catalog package is a source build
// that AGENTS.md does not allow. Where each field lives follows the design
// (docs/design/generic-tool-builds): the catalog package has provider
// "source-build", build.static and the build.toolchain name; the lock pins the
// toolchain per architecture (lock.toolchains), the source checksum per
// architecture (lock.platforms) and every extra source by a full 40-hex commit
// SHA with per-file builder-computed hashes (lock.extra_sources, class
// tls-download-sha256). Whether no official prebuilt exists for both
// architectures is judged by review, not by code. A prebuilt package has no
// violations.
func sourceBuildViolations(name string, pkg, lock map[string]any) []string {
	profile := str(obj(pkg["install"])["profile"])
	_, hasBuild := pkg["build"]
	if pkg["provider"] != "source-build" && !hasBuild && !strings.Contains(profile, "source") {
		return nil
	}
	var out []string
	bad := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }
	if pkg["provider"] != "source-build" {
		bad("source recipe outside the source-build provider")
	}
	build := obj(pkg["build"])
	if build["static"] != true {
		bad("source build is not declared static")
	}
	tc := str(build["toolchain"])
	for _, arch := range []string{"amd64", "arm64"} {
		if tc == "" || !hex64.MatchString(str(obj(obj(obj(lock["toolchains"])[tc])[arch])["sha256"])) {
			bad("%s: toolchain %q is not pinned by sha256 in the lock", arch, tc)
		}
		if tc != "" && !strings.HasPrefix(str(obj(obj(obj(lock["toolchains"])[tc])[arch])["verification"]), "publisher-") {
			bad("%s: toolchain %q checksum is not publisher data", arch, tc)
		}
		entry := obj(obj(obj(lock["platforms"])[arch])[name])
		if !hex64.MatchString(str(entry["sha256"])) {
			bad("%s: no source checksum in the lock", arch)
		}
		if !strings.HasPrefix(str(entry["verification"]), "publisher-") {
			bad("%s: the main source archive needs a publisher checksum class, not %q", arch, str(entry["verification"]))
		}
		if str(entry["toolchain"]) != tc {
			bad("%s: lock toolchain differs from the catalog", arch)
		}
	}
	extras, _ := build["extra_sources"].([]any)
	listed := map[string]bool{}
	for _, e := range extras {
		listed[str(e)] = true
	}
	for e := range obj(pkg["extra_sources"]) {
		if !listed[e] {
			bad("extra source %s is declared but not listed in build.extra_sources", e)
		}
	}
	for key := range obj(lock["extra_sources"]) {
		if strings.HasPrefix(key, name+"/") && !listed[strings.TrimPrefix(key, name+"/")] {
			bad("locked extra source %s is not listed in build.extra_sources", key)
		}
	}
	for _, e := range extras {
		key := name + "/" + str(e)
		commit := str(obj(obj(pkg["extra_sources"])[str(e)])["commit"])
		locked := obj(obj(lock["extra_sources"])[key])
		if !hex40.MatchString(commit) || str(locked["commit"]) != commit {
			bad("extra source %s is not pinned by one full 40-hex commit SHA in catalog and lock", key)
		}
		files := obj(locked["files"])
		if len(files) == 0 {
			bad("extra source %s has no locked files", key)
		}
		for f, v := range files {
			if !hex64.MatchString(str(obj(v)["sha256"])) || str(obj(v)["verification"]) != "tls-download-sha256" {
				bad("extra source %s file %s lacks a builder-computed sha256", key, f)
			}
		}
	}
	return out
}

const designSQLite3 = `{
 "catalog": {"provider":"source-build","install":{"kind":"build-output"},
   "build":{"kind":"c-amalgamation","toolchain":"zig-cc","static":true,"extra_sources":["linenoise"]},
   "extra_sources":{"linenoise":{"commit":"a473823d74b93eab2ba83480df16ed37617493f2"}}},
 "lock": {"toolchains":{"zig-cc":{
     "amd64":{"sha256":"` + "0000000000000000000000000000000000000000000000000000000000000001" + `","verification":"publisher-sha256"},
     "arm64":{"sha256":"` + "0000000000000000000000000000000000000000000000000000000000000002" + `","verification":"publisher-sha256"}}},
   "platforms":{
     "amd64":{"sqlite3":{"sha256":"` + "0000000000000000000000000000000000000000000000000000000000000003" + `","verification":"publisher-sha3-256","toolchain":"zig-cc"}},
     "arm64":{"sqlite3":{"sha256":"` + "0000000000000000000000000000000000000000000000000000000000000003" + `","verification":"publisher-sha3-256","toolchain":"zig-cc"}}},
   "extra_sources":{"sqlite3/linenoise":{"commit":"a473823d74b93eab2ba83480df16ed37617493f2","files":{
     "linenoise.c":{"sha256":"` + "0000000000000000000000000000000000000000000000000000000000000004" + `","verification":"tls-download-sha256"}}}}}
}`

func designExample(t *testing.T) (map[string]any, map[string]any) {
	t.Helper()
	var d map[string]any
	if err := json.Unmarshal([]byte(designSQLite3), &d); err != nil {
		t.Fatal(err)
	}
	return obj(d["catalog"]), obj(d["lock"])
}

func TestSourceBuildViolations(t *testing.T) {
	pkg, lock := designExample(t)
	if v := sourceBuildViolations("sqlite3", pkg, lock); len(v) != 0 {
		t.Fatalf("the design's sqlite3 example is rejected: %v", v)
	}
	if v := sourceBuildViolations("gh", map[string]any{"provider": "github"}, lock); len(v) != 0 {
		t.Fatalf("prebuilt rejected: %v", v)
	}
	for name, mutate := range map[string]func(pkg, lock map[string]any){
		"not static":            func(p, l map[string]any) { obj(p["build"])["static"] = false },
		"no static key":         func(p, l map[string]any) { delete(obj(p["build"]), "static") },
		"no toolchain name":     func(p, l map[string]any) { delete(obj(p["build"]), "toolchain") },
		"toolchain not in lock": func(p, l map[string]any) { delete(obj(l["toolchains"]), "zig-cc") },
		"toolchain one arch":    func(p, l map[string]any) { delete(obj(obj(l["toolchains"])["zig-cc"]), "arm64") },
		"toolchain unhashed":    func(p, l map[string]any) { obj(obj(obj(l["toolchains"])["zig-cc"])["amd64"])["sha256"] = "" },
		"no source checksum":    func(p, l map[string]any) { delete(obj(obj(obj(l["platforms"])["arm64"])["sqlite3"]), "sha256") },
		"short source checksum": func(p, l map[string]any) { obj(obj(obj(l["platforms"])["amd64"])["sqlite3"])["sha256"] = "abc" },
		"lock toolchain differs": func(p, l map[string]any) {
			obj(obj(obj(l["platforms"])["amd64"])["sqlite3"])["toolchain"] = "other"
		},
		"commit is a branch": func(p, l map[string]any) { obj(obj(p["extra_sources"])["linenoise"])["commit"] = "main" },
		"commit is a branch everywhere": func(p, l map[string]any) {
			obj(obj(p["extra_sources"])["linenoise"])["commit"] = "main"
			obj(obj(l["extra_sources"])["sqlite3/linenoise"])["commit"] = "main"
		},
		"no locked files": func(p, l map[string]any) { delete(obj(obj(l["extra_sources"])["sqlite3/linenoise"]), "files") },
		"main archive builder-computed only": func(p, l map[string]any) {
			obj(obj(obj(l["platforms"])["amd64"])["sqlite3"])["verification"] = "tls-download-sha256"
		},
		"main archive no class": func(p, l map[string]any) { delete(obj(obj(obj(l["platforms"])["arm64"])["sqlite3"]), "verification") },
		"toolchain builder-computed": func(p, l map[string]any) {
			obj(obj(obj(l["toolchains"])["zig-cc"])["amd64"])["verification"] = "tls-download-sha256"
		},
		"extra source not listed": func(p, l map[string]any) {
			delete(obj(p["build"]), "extra_sources")
			delete(obj(l["extra_sources"]), "sqlite3/linenoise")
		},
		"extra source only locked": func(p, l map[string]any) {
			obj(l["extra_sources"])["sqlite3/other"] = map[string]any{"commit": strings.Repeat("c", 40)}
		},
		"commit is short": func(p, l map[string]any) { obj(obj(p["extra_sources"])["linenoise"])["commit"] = "a473823" },
		"lock commit differs": func(p, l map[string]any) {
			obj(obj(l["extra_sources"])["sqlite3/linenoise"])["commit"] = strings.Repeat("b", 40)
		},
		"extra source unlocked": func(p, l map[string]any) { delete(obj(l["extra_sources"]), "sqlite3/linenoise") },
		"file unhashed": func(p, l map[string]any) {
			delete(obj(obj(obj(obj(l["extra_sources"])["sqlite3/linenoise"])["files"])["linenoise.c"]), "sha256")
		},
		"file wrong class": func(p, l map[string]any) {
			obj(obj(obj(obj(l["extra_sources"])["sqlite3/linenoise"])["files"])["linenoise.c"])["verification"] = "publisher-sha256"
		},
		"other provider with build": func(p, l map[string]any) { p["provider"] = "github" },
	} {
		pkg, lock := designExample(t)
		mutate(pkg, lock)
		if len(sourceBuildViolations("sqlite3", pkg, lock)) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
	// Source recipes recognised without a build block or provider.
	if len(sourceBuildViolations("x", map[string]any{"provider": "github", "install": map[string]any{"profile": "git-trim-source"}}, lock)) == 0 {
		t.Error("legacy source profile on a prebuilt provider: accepted")
	}
	if len(sourceBuildViolations("x", map[string]any{"provider": "github", "build": map[string]any{"static": true}}, lock)) == 0 {
		t.Error("build block on a prebuilt provider: accepted")
	}
	if len(sourceBuildViolations("x", map[string]any{"provider": "source-build"}, lock)) == 0 {
		t.Error("bare source-build provider: accepted")
	}
}
