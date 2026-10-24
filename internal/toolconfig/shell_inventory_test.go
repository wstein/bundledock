package toolconfig

import (
	"crypto/sha256"
	"fmt"
	"os"
	"slices"
	"testing"
)

func TestBundledBashRemovedAndAlternativesPreserved(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Catalog.Packages["bash"]; ok {
		t.Fatal("bundled Bash remains in catalog")
	}
	if _, ok := c.Desired["bash"]; ok {
		t.Fatal("bundled Bash remains desired")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if _, ok := c.Lock.Platforms[arch]["bash"]; ok {
			t.Fatal("bundled Bash remains locked")
		}
	}
	for _, name := range []string{"nu", "tmux", "fish", "herdr", "starship"} {
		if _, ok := c.Catalog.Packages[name]; !ok {
			t.Fatalf("preferred tool or alternative %s removed", name)
		}
	}
}

// SQLite is bundled as a reviewed static source build (AGENTS.md, docs/design/generic-tool-builds).
// The test asserts the allowed shape and provenance, not a version or checksum constant: the
// updater may move the SQLite pin within the desired range.
func TestSQLiteIsBundledAsStaticSourceBuild(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["sqlite3"]
	if !ok || p.Provider != "source-build" || p.Build == nil {
		t.Fatal("sqlite3 must be a source-build package")
	}
	if _, ok := c.Desired["sqlite3"]; !ok {
		t.Fatal("sqlite3 is not desired")
	}
	if !p.Build.Static || p.Build.OutputForm != "exec" || p.Build.Toolchain != "zig-cc" || p.Install.Kind != "build-output" {
		t.Fatalf("sqlite3 must be a static zig-cc build with a build output: %+v", p.Build)
	}
	if !slices.Contains(p.Build.Defines, "HAVE_LINENOISE=1") || !slices.Contains(p.Build.Defines, "SQLITE_OMIT_LOAD_EXTENSION=1") || !slices.Contains(p.Build.Sources, "linenoise.c") {
		t.Fatal("sqlite3 must be built with linenoise and without extension loading")
	}
	assertRollingStablePins(t, c, "sqlite3")
	amd64, arm64 := c.Lock.Platforms["amd64"]["sqlite3"], c.Lock.Platforms["arm64"]["sqlite3"]
	if amd64 != arm64 {
		t.Fatal("both architectures must pin the same source")
	}
	if amd64.Verification != "publisher-sha3-256" || amd64.Toolchain != "zig-cc" || len(amd64.SHA3256) != 64 || len(amd64.SHA256) != 64 {
		t.Fatalf("the source needs the publisher SHA3-256 and the SHA256 of the same bytes: %+v", amd64)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		tc := c.Lock.Toolchains["zig-cc"][arch]
		if tc.Verification != "publisher-sha256" || len(tc.SHA256) != 64 || tc.Version == "" {
			t.Fatalf("zig-cc/%s must be pinned by a publisher checksum: %+v", arch, tc)
		}
	}
	locked := c.Lock.ExtraSources["sqlite3/linenoise"]
	if locked.Commit != "a473823d74b93eab2ba83480df16ed37617493f2" || len(locked.Files) != 3 {
		t.Fatalf("linenoise must be pinned by commit with three files: %+v", locked)
	}
	for file, f := range locked.Files {
		if f.Verification != "tls-download-sha256" || len(f.SHA256) != 64 {
			t.Fatalf("linenoise %s: %+v", file, f)
		}
	}
	// The retained linenoise license text is the very file the lock pins.
	data, err := os.ReadFile("../../bundles/toolbox/licenses/sqlite3/LINENOISE-LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != locked.Files["LICENSE"].SHA256 {
		t.Fatal("the retained linenoise license differs from the locked file")
	}
	for _, notice := range []string{"NOTICE", "SOURCE", "LINENOISE-LICENSE", "MUSL-COPYRIGHT", "ZIG-LICENSE"} {
		if !slices.Contains(p.Notices, "licenses/sqlite3/"+notice) {
			t.Errorf("sqlite3 does not retain %s", notice)
		}
	}
}
