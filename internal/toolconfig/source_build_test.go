package toolconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sqliteConfig is the design's sqlite3 example (docs/design/generic-tool-builds)
// reduced to the single main archive; the linenoise extra source belongs to the
// multi-source work package. The hashes are placeholders.
func sqliteConfig(t *testing.T) Config {
	t.Helper()
	desired, err := ParseDesired(strings.NewReader("sqlite3>=3.54.1,<4.0.0\n"))
	if err != nil {
		t.Fatal(err)
	}
	pkg := Package{
		Provider:      "source-build",
		Normalization: "semver",
		VersionSource: &VersionSource{Kind: "sqlite-product-data", URL: "https://www.sqlite.org/download.html"},
		Architectures: map[string]string{"amd64": "x86_64", "arm64": "aarch64"},
		Asset:         "sqlite-amalgamation-{numeric}.zip",
		BaseURL:       "https://www.sqlite.org",
		AllowedHosts:  []string{"www.sqlite.org"},
		Verification:  "official-or-download",
		Build: &Build{
			Kind:          "c-amalgamation",
			Toolchain:     "zig-cc",
			ArchivePrefix: "sqlite-amalgamation-{numeric}/",
			Sources:       []string{"shell.c", "sqlite3.c"},
			Defines:       []string{"SQLITE_THREADSAFE=1", "SQLITE_OMIT_LOAD_EXTENSION=1", "SQLITE_ENABLE_FTS5=1", "SQLITE_ENABLE_RTREE=1", "SQLITE_ENABLE_DBSTAT_VTAB=1", "SQLITE_ENABLE_MATH_FUNCTIONS=1"},
			Cflags:        []string{"-Os"},
			Libs:          []string{"m", "pthread"},
			Static:        true,
			OutputForm:    "exec",
			Output:        "sqlite3",
		},
		Install: Install{Kind: "build-output", Members: map[string]string{"sqlite3": "bin/sqlite3"}},
		Notices: []string{"licenses/sqlite3/NOTICE", "licenses/sqlite3/SOURCE"},
	}
	tool := func(arch string) Spec {
		return Spec{Version: "0.17.0", URL: "https://ziglang.org/download/0.17.0/zig-" + pkg.Architectures[arch] + "-linux-0.17.0.tar.xz", SHA256: h("1"), Verification: "publisher-sha256"}
	}
	spec := Spec{Version: "3.54.1", URL: "https://www.sqlite.org/2026/sqlite-amalgamation-3540100.zip", SHA256: h("2"), SHA3256: h("3"), Verification: "publisher-sha3-256", Toolchain: "zig-cc"}
	c := Config{
		Desired: desired,
		Catalog: Catalog{Schema: 1, Packages: map[string]Package{"sqlite3": pkg}},
		Lock: Lock{
			Toolchains: map[string]map[string]Spec{"zig-cc": {"amd64": tool("amd64"), "arm64": tool("arm64")}},
			Platforms:  map[string]map[string]Spec{"amd64": {"sqlite3": spec}, "arm64": {"sqlite3": spec}},
		},
	}
	c.Bind()
	return c
}

// clone deep-copies a Config through JSON so mutations never leak between cases.
func clone(t *testing.T, c Config) Config {
	t.Helper()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var out Config
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSourceBuildDesignExampleValidates(t *testing.T) {
	c := sqliteConfig(t)
	if err := c.Validate(); err != nil {
		t.Fatalf("the design's sqlite3 example is rejected: %v", err)
	}
}

func TestSourceBuildRejectedShapes(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		want   string
	}{
		"missing static": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Static = false
			c.Catalog.Packages["sqlite3"] = p
		}, "static"},
		"unknown kind": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Kind = "make"
			c.Catalog.Packages["sqlite3"] = p
		}, "kind"},
		"no output form": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.OutputForm = ""
			c.Catalog.Packages["sqlite3"] = p
		}, "output_form"},
		"unknown output form": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.OutputForm = "pie"
			c.Catalog.Packages["sqlite3"] = p
		}, "output_form"},
		"unknown toolchain name": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Toolchain = "gcc"
			c.Catalog.Packages["sqlite3"] = p
		}, "toolchain"},
		"unpinned toolchain":    {func(c *Config) { c.Lock.Toolchains = nil }, "toolchain"},
		"toolchain on one arch": {func(c *Config) { delete(c.Lock.Toolchains["zig-cc"], "arm64") }, "toolchain"},
		"toolchain without sha256": {func(c *Config) {
			s := c.Lock.Toolchains["zig-cc"]["amd64"]
			s.SHA256 = ""
			c.Lock.Toolchains["zig-cc"]["amd64"] = s
		}, "toolchain"},
		"toolchain builder-hashed": {func(c *Config) {
			s := c.Lock.Toolchains["zig-cc"]["amd64"]
			s.Verification = "tls-download-sha256"
			c.Lock.Toolchains["zig-cc"]["amd64"] = s
		}, "toolchain"},
		"toolchain wrong host": {func(c *Config) {
			s := c.Lock.Toolchains["zig-cc"]["amd64"]
			s.URL = "https://example.com/zig.tar.xz"
			c.Lock.Toolchains["zig-cc"]["amd64"] = s
		}, "host"},
		"toolchain versions differ": {func(c *Config) {
			s := c.Lock.Toolchains["zig-cc"]["arm64"]
			s.Version = "0.16.0"
			c.Lock.Toolchains["zig-cc"]["arm64"] = s
		}, "toolchain"},
		"unknown lock toolchain": {func(c *Config) { c.Lock.Toolchains["other"] = c.Lock.Toolchains["zig-cc"] }, "toolchain"},
		"third toolchain arch":   {func(c *Config) { c.Lock.Toolchains["zig-cc"]["riscv64"] = c.Lock.Toolchains["zig-cc"]["amd64"] }, "toolchain"},
		"unused toolchain": {func(c *Config) {
			c.Catalog.Packages["sqlite3"] = prebuiltPackage()
			s := Spec{Version: "3.54.1", URL: "https://github.com/a/b/x", SHA256: h("2"), Verification: "publisher-sha256"}
			c.Lock.Platforms["amd64"]["sqlite3"] = s
			c.Lock.Platforms["arm64"]["sqlite3"] = s
		}, "used by no source-build"},
		"two install members": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Install.Members = map[string]string{"sqlite3": "bin/sqlite3", "extra": "bin/extra"}
			c.Catalog.Packages["sqlite3"] = p
		}, "output"},
		"missing source checksum": {func(c *Config) {
			s := c.Lock.Platforms["arm64"]["sqlite3"]
			s.SHA256 = ""
			c.Lock.Platforms["arm64"]["sqlite3"] = s
		}, "checksum"},
		"main archive tls class": {func(c *Config) {
			s := c.Lock.Platforms["amd64"]["sqlite3"]
			s.Verification, s.SHA3256 = "tls-download-sha256", ""
			c.Lock.Platforms["amd64"]["sqlite3"] = s
		}, "publisher"},
		"sha3 class without sha3_256": {func(c *Config) {
			s := c.Lock.Platforms["amd64"]["sqlite3"]
			s.SHA3256 = ""
			c.Lock.Platforms["amd64"]["sqlite3"] = s
		}, "sha3"},
		"sha3 class without sha256": {func(c *Config) {
			s := c.Lock.Platforms["amd64"]["sqlite3"]
			s.SHA256 = ""
			c.Lock.Platforms["amd64"]["sqlite3"] = s
		}, "checksum"},
		"sha3 wrong length": {func(c *Config) {
			s := c.Lock.Platforms["amd64"]["sqlite3"]
			s.SHA3256 = "abc"
			c.Lock.Platforms["amd64"]["sqlite3"] = s
		}, "sha3"},
		"sha3 with sha512": {func(c *Config) {
			s := c.Lock.Platforms["amd64"]["sqlite3"]
			s.SHA512 = strings.Repeat("4", 128)
			c.Lock.Platforms["amd64"]["sqlite3"] = s
		}, "checksum"},
		"sha3 value on other class": {func(c *Config) {
			s := c.Lock.Platforms["amd64"]["sqlite3"]
			s.Verification = "publisher-sha256"
			c.Lock.Platforms["amd64"]["sqlite3"] = s
		}, "sha3"},
		"lock toolchain differs": {func(c *Config) {
			s := c.Lock.Platforms["amd64"]["sqlite3"]
			s.Toolchain = "other"
			c.Lock.Platforms["amd64"]["sqlite3"] = s
		}, "toolchain"},
		"lock toolchain missing": {func(c *Config) {
			s := c.Lock.Platforms["amd64"]["sqlite3"]
			s.Toolchain = ""
			c.Lock.Platforms["amd64"]["sqlite3"] = s
		}, "toolchain"},
		"architectures differ": {func(c *Config) {
			s := c.Lock.Platforms["arm64"]["sqlite3"]
			s.Version = "3.54.2"
			c.Lock.Platforms["arm64"]["sqlite3"] = s
		}, "different versions"},
		"no build block": {func(c *Config) { p := c.Catalog.Packages["sqlite3"]; p.Build = nil; c.Catalog.Packages["sqlite3"] = p }, "build"},
		"no version source": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.VersionSource = nil
			c.Catalog.Packages["sqlite3"] = p
		}, "version_source"},
		"version source host": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.VersionSource.URL = "https://example.com/x"
			c.Catalog.Packages["sqlite3"] = p
		}, "host"},
		"unknown version source kind": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.VersionSource.Kind = "html"
			c.Catalog.Packages["sqlite3"] = p
		}, "version_source"},
		"install kind raw": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Install.Kind = "raw"
			c.Catalog.Packages["sqlite3"] = p
		}, "build-output"},
		"install profile": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Install.Profile = "bun-runtime"
			c.Catalog.Packages["sqlite3"] = p
		}, "profile"},
		"member is not the output": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Install.Members = map[string]string{"other": "bin/other"}
			c.Catalog.Packages["sqlite3"] = p
		}, "output"},
		"unsafe output": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Output = "../sqlite3"
			p.Install.Members = map[string]string{"../sqlite3": "bin/sqlite3"}
			c.Catalog.Packages["sqlite3"] = p
		}, "output"},
		"no sources": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Sources = nil
			c.Catalog.Packages["sqlite3"] = p
		}, "sources"},
		"source with path": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Sources = []string{"../x.c"}
			c.Catalog.Packages["sqlite3"] = p
		}, "source"},
		"source not C": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Sources = []string{"shell.s"}
			c.Catalog.Packages["sqlite3"] = p
		}, "source"},
		"duplicate source": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Sources = []string{"shell.c", "shell.c"}
			c.Catalog.Packages["sqlite3"] = p
		}, "source"},
		"bad archive prefix": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.ArchivePrefix = "../x/"
			c.Catalog.Packages["sqlite3"] = p
		}, "archive_prefix"},
		"archive prefix without slash": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.ArchivePrefix = "x"
			c.Catalog.Packages["sqlite3"] = p
		}, "archive_prefix"},
		"unlisted define": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Defines = []string{"EVIL=1"}
			c.Catalog.Packages["sqlite3"] = p
		}, "define"},
		"define with space": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Defines = []string{"SQLITE_THREADSAFE=1 -include x.h"}
			c.Catalog.Packages["sqlite3"] = p
		}, "define"},
		"cflag include": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Cflags = []string{"-include", "x.h"}
			c.Catalog.Packages["sqlite3"] = p
		}, "cflag"},
		"cflag include dir": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Cflags = []string{"-I/etc"}
			c.Catalog.Packages["sqlite3"] = p
		}, "cflag"},
		"cflag linker passthrough": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Cflags = []string{"-Wl,-rpath,/x"}
			c.Catalog.Packages["sqlite3"] = p
		}, "cflag"},
		"cflag shared": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Cflags = []string{"-shared"}
			c.Catalog.Packages["sqlite3"] = p
		}, "cflag"},
		"unlisted lib": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Libs = []string{"ssl"}
			c.Catalog.Packages["sqlite3"] = p
		}, "lib"},
		"lib path": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Libs = []string{"/usr/lib/libc.so"}
			c.Catalog.Packages["sqlite3"] = p
		}, "lib"},
		"source option -Wl": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Sources = []string{"-Wl,--wrap=x.c"}
			c.Catalog.Packages["sqlite3"] = p
		}, "source"},
		"source option -o": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Sources = []string{"-ofoo.c"}
			c.Catalog.Packages["sqlite3"] = p
		}, "source"},
		"source option -include": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Sources = []string{"-include.c"}
			c.Catalog.Packages["sqlite3"] = p
		}, "source"},
		"source with space": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Sources = []string{"a b.c"}
			c.Catalog.Packages["sqlite3"] = p
		}, "source"},
		"source with comma": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Sources = []string{"a,b.c"}
			c.Catalog.Packages["sqlite3"] = p
		}, "source"},
		"source in directory": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Sources = []string{"dir/x.c"}
			c.Catalog.Packages["sqlite3"] = p
		}, "source"},
		"output option -o": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Output = "-o"
			p.Install.Members = map[string]string{"-o": "bin/sqlite3"}
			c.Catalog.Packages["sqlite3"] = p
		}, "output"},
		"output with space": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Output = "a b"
			p.Install.Members = map[string]string{"a b": "bin/sqlite3"}
			c.Catalog.Packages["sqlite3"] = p
		}, "output"},
		"output with comma": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Output = "a,b"
			p.Install.Members = map[string]string{"a,b": "bin/sqlite3"}
			c.Catalog.Packages["sqlite3"] = p
		}, "output"},
		"output in directory": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.Output = "bin/sqlite3"
			p.Install.Members = map[string]string{"bin/sqlite3": "bin/sqlite3"}
			c.Catalog.Packages["sqlite3"] = p
		}, "output"},
		"source-build repository": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Repository = "a/b"
			c.Catalog.Packages["sqlite3"] = p
		}, "semver normalization and no repository"},
		"source-build sidecar": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Sidecar = "SUMS"
			c.Catalog.Packages["sqlite3"] = p
		}, "semver normalization and no repository"},
		"source-build metadata_url": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.MetadataURL = "https://www.sqlite.org/m"
			c.Catalog.Packages["sqlite3"] = p
		}, "semver normalization and no repository"},
		"source-build tag prefix": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.TagPrefix = "v"
			c.Catalog.Packages["sqlite3"] = p
		}, "semver normalization and no repository"},
		"source-build alias": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Install.Aliases = map[string]string{"bin/sq": "bin/sqlite3"}
			c.Catalog.Packages["sqlite3"] = p
		}, "no install profile"},
		"source-build archive notices": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Install.ArchiveNotices = []string{"LICENSE"}
			c.Catalog.Packages["sqlite3"] = p
		}, "no install profile"},
		"source-build missing base_url": {func(c *Config) { p := c.Catalog.Packages["sqlite3"]; p.BaseURL = ""; c.Catalog.Packages["sqlite3"] = p }, "base_url"},
		"archive prefix placeholder": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.ArchivePrefix = "{version}/"
			c.Catalog.Packages["sqlite3"] = p
		}, "archive_prefix"},
		"version_source on prebuilt": {func(c *Config) {
			p := prebuiltPackage()
			p.VersionSource = &VersionSource{Kind: "sqlite-product-data", URL: "https://github.com/x"}
			c.Catalog.Packages["sqlite3"] = p
			c.Lock.Toolchains = nil
			s := Spec{Version: "3.54.1", URL: "https://github.com/a/b/x", SHA256: h("2"), Verification: "publisher-sha256"}
			c.Lock.Platforms["amd64"]["sqlite3"], c.Lock.Platforms["arm64"]["sqlite3"] = s, s
		}, "version_source"},
		"nested toolchain field": {func(c *Config) {
			s := c.Lock.Toolchains["zig-cc"]["amd64"]
			s.Toolchain = "zig-cc"
			c.Lock.Toolchains["zig-cc"]["amd64"] = s
		}, "nested toolchain"},
		"template on prebuilt": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Provider, p.Repository = "github", "a/b"
			p.Build, p.VersionSource = nil, nil
			p.Install = Install{Kind: "zip", Members: map[string]string{"a": "a"}}
			c.Catalog.Packages["sqlite3"] = p
		}, "numeric"},
		"build on prebuilt": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Provider, p.Repository, p.Asset = "github", "a/b", "tool-{arch}.zip"
			p.VersionSource = nil
			p.Install = Install{Kind: "zip", Members: map[string]string{"a": "a"}}
			c.Catalog.Packages["sqlite3"] = p
			s := c.Lock.Platforms["amd64"]["sqlite3"]
			s.SHA3256, s.Toolchain, s.Verification = "", "", "publisher-sha256"
			c.Lock.Platforms["amd64"]["sqlite3"] = s
			c.Lock.Platforms["arm64"]["sqlite3"] = s
			c.Lock.Toolchains = nil
		}, "build"},
		"build-output on prebuilt": {func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Provider, p.Repository, p.Asset = "github", "a/b", "tool-{arch}.zip"
			p.Build, p.VersionSource = nil, nil
			c.Catalog.Packages["sqlite3"] = p
			s := c.Lock.Platforms["amd64"]["sqlite3"]
			s.SHA3256, s.Toolchain, s.Verification = "", "", "publisher-sha256"
			c.Lock.Platforms["amd64"]["sqlite3"] = s
			c.Lock.Platforms["arm64"]["sqlite3"] = s
			c.Lock.Toolchains = nil
		}, "build-output"},
		"sha3 class on prebuilt": {func(c *Config) {
			c.Catalog.Packages["sqlite3"] = prebuiltPackage()
			c.Lock.Toolchains = nil
			s := Spec{Version: "3.54.1", URL: "https://github.com/a/b/x", SHA256: h("2"), SHA3256: h("3"), Verification: "publisher-sha3-256"}
			c.Lock.Platforms["amd64"]["sqlite3"] = s
			c.Lock.Platforms["arm64"]["sqlite3"] = s
		}, "source-build"},
		"toolchain field on prebuilt": {func(c *Config) {
			c.Catalog.Packages["sqlite3"] = prebuiltPackage()
			c.Lock.Toolchains = nil
			s := Spec{Version: "3.54.1", URL: "https://github.com/a/b/x", SHA256: h("2"), Verification: "publisher-sha256", Toolchain: "zig-cc"}
			c.Lock.Platforms["amd64"]["sqlite3"] = s
			c.Lock.Platforms["arm64"]["sqlite3"] = s
		}, "toolchain"},
	} {
		t.Run(name, func(t *testing.T) {
			c := clone(t, sqliteConfig(t))
			tc.mutate(&c)
			c.Bind()
			err := c.Validate()
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func prebuiltPackage() Package {
	return Package{Provider: "github", Repository: "a/b", Normalization: "semver", Architectures: map[string]string{"amd64": "x", "arm64": "y"},
		Asset: "tool-{arch}.zip", AllowedHosts: []string{"github.com"}, Verification: "official-or-download",
		Install: Install{Kind: "zip", Members: map[string]string{"a": "bin/a"}}}
}

func TestSourceBuildAcceptedVariants(t *testing.T) {
	for name, mutate := range map[string]func(c *Config){
		"static-pie": func(c *Config) {
			p := c.Catalog.Packages["sqlite3"]
			p.Build.OutputForm = "static-pie"
			c.Catalog.Packages["sqlite3"] = p
		},
		"sha256 publisher": func(c *Config) {
			s := c.Lock.Platforms["amd64"]["sqlite3"]
			s.SHA3256, s.Verification = "", "publisher-sha256"
			c.Lock.Platforms["amd64"]["sqlite3"] = s
			c.Lock.Platforms["arm64"]["sqlite3"] = s
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := clone(t, sqliteConfig(t))
			mutate(&c)
			c.Bind()
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// writeBundle writes a Config as the three user files of a bundle directory.
func writeBundle(t *testing.T, dir, packages string, c Config) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "packages.txt"), []byte(packages), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]any{"packages.catalog.json": c.Catalog, "packages.lock.json": c.Lock} {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInputsProbeAcceptsSourceBuildRecipe(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "sqlite3>=3.54.1,<4.0.0\n", sqliteConfig(t))
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Lock.Toolchains["zig-cc"]["amd64"].Version != "0.17.0" {
		t.Fatal("toolchains not loaded")
	}
}

// The WP1 guard heuristics are replaced by Validate: a legacy source profile on
// a prebuilt provider and a bare source-build provider are both rejected.
func TestSourceRecipesOutsideSourceBuildAreRejected(t *testing.T) {
	for name, mutate := range map[string]func(p *Package){
		"legacy source profile": func(p *Package) { p.Install.Profile = "git-trim-source" },
		"bare source-build":     func(p *Package) { p.Provider, p.Repository = "source-build", "" },
	} {
		t.Run(name, func(t *testing.T) {
			c := clone(t, sqliteConfig(t))
			p := prebuiltPackage()
			mutate(&p)
			c.Catalog.Packages["sqlite3"] = p
			s := Spec{Version: "3.54.1", URL: "https://github.com/a/b/x", SHA256: h("2"), Verification: "publisher-sha256"}
			c.Lock.Platforms["amd64"]["sqlite3"], c.Lock.Platforms["arm64"]["sqlite3"] = s, s
			c.Lock.Toolchains = nil
			c.Bind()
			if err := c.Validate(); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
