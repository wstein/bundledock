package toolmanager

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha3"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const zigPin = "0.17.0"

func hexOf(c string) string { return strings.Repeat(c, 64) }

func sha3Hex(b []byte) string { s := sha3.Sum256(b); return fmt.Sprintf("%x", s) }

// publisherHeader is the header line exactly as https://www.sqlite.org/download.html
// publishes it (checked 2026-10-29). It is a literal, not productHeader, so a
// header the parser expects but the publisher does not write fails here.
const publisherHeader = "PRODUCT,VERSION,RELATIVE-URL,SIZE-IN-BYTES,SHA3-HASH"

func productPage(rows ...string) []byte {
	return []byte("<html><body>\n<!-- Download product data for scripts to read\n" + publisherHeader + "\n" + strings.Join(rows, "\n") + "\n-->\n</body></html>\n")
}

// TestParseSQLitePublisherExcerpt parses the product data block of the
// publisher's download page as served on 2026-10-29 for 3.54.1.
func TestParseSQLitePublisherExcerpt(t *testing.T) {
	page := []byte(`<p>Other content</p>
<!-- Download product data for scripts to read
PRODUCT,VERSION,RELATIVE-URL,SIZE-IN-BYTES,SHA3-HASH
PRODUCT,3.54.1,2026/sqlite-src-3540100.zip,14899528,877c6f4996ce7adb368686bce21638de9e3d91534832ad3d9d119037d1bc22ad
PRODUCT,3.54.1,2026/sqlite-amalgamation-3540100.zip,2990966,92281c86e2957f9b9305890f9ac04204caa7b677cc868dc326d942689df28e30
PRODUCT,3.54.1,2026/sqlite-autoconf-3540100.tar.gz,3332984,b6ced585c750916b1052667239a12c25a7ff34e7f06e1e6e01ecb0427657d9c4
PRODUCT,3.54.1,2026/sqlite-preprocessed-3540100.zip,3139216,77320e1ed40b7e655f27b97bff9010b3f159800eddfc4f28b9d96254d8a5bd17
PRODUCT,3.54.1,2026/sqlite-doc-3540100.zip,11961740,6ea5c0eff56c7fb305ca85e820070eacd96da44ae9edfb04d4c82d909540297b
PRODUCT,3.54.1,2026/sqlite-android-3540100.aar,3688530,3afb0568a107d753b640e4d65cdb1d9a2247e9833c25e72ca7d1086b78365de1
PRODUCT,3.54.1,2026/sqlite-tools-linux-x64-3540100.zip,4324601,555635de98534d87a67ee2e6e1b0d79f494b1b33b223477d3e7a8ea0c66e416f
PRODUCT,3.54.1,2026/sqlite-tools-osx-arm64-3540100.zip,4538997,a3106c6db3b50ec9ef4fde4edfafe7504179e9bfd7e0816a280b29c4d56a2e7b
-->
`)
	c := toolconfig.Constraint{Lower: toolconfig.Version{3, 54, 1}, Upper: toolconfig.Version{4, 0, 0}}
	row, err := parseSQLiteProducts(page, c)
	if err != nil {
		t.Fatal(err)
	}
	want := sqliteProduct{Version: "3.54.1", Path: "2026/sqlite-amalgamation-3540100.zip", SHA3256: "92281c86e2957f9b9305890f9ac04204caa7b677cc868dc326d942689df28e30"}
	if row != want {
		t.Fatalf("got %+v, want %+v", row, want)
	}
}

func amalgamationRow(version, rel string, size int, hash string) string {
	return fmt.Sprintf("PRODUCT,%s,%s,%d,%s", version, rel, size, hash)
}

func TestParseSQLiteProductData(t *testing.T) {
	c := toolconfig.Constraint{Lower: toolconfig.Version{3, 54, 0}, Upper: toolconfig.Version{4, 0, 0}}
	good := amalgamationRow("3.54.1", "2026/sqlite-amalgamation-3540100.zip", 2990966, hexOf("a"))
	other := amalgamationRow("3.54.1", "2026/sqlite-autoconf-3540100.tar.gz", 100, hexOf("b"))
	ok, err := parseSQLiteProducts(productPage(other, good), c)
	if err != nil || ok.Version != "3.54.1" || ok.Path != "2026/sqlite-amalgamation-3540100.zip" || ok.SHA3256 != hexOf("a") {
		t.Fatalf("%+v %v", ok, err)
	}
	for name, tc := range map[string]struct {
		page []byte
		want string
	}{
		"no header":              {[]byte("<html></html>"), "header"},
		"two headers":            {append(productPage(good), productPage(good)...), "header"},
		"no amalgamation":        {productPage(other), "amalgamation"},
		"duplicate row":          {productPage(good, good), "more than one"},
		"row after a blank line": {[]byte(string(productPage(good)[:bytes.Index(productPage(good), []byte("-->"))]) + "\n" + good + "\n-->\n"), "more than one"},
		"malformed row":          {productPage(good, "PRODUCT,3.54.1,only-three"), "malformed"},
		"short hash":             {productPage(amalgamationRow("3.54.1", "2026/sqlite-amalgamation-3540100.zip", 10, "abc")), "hash"},
		"upper-case hash":        {productPage(amalgamationRow("3.54.1", "2026/sqlite-amalgamation-3540100.zip", 10, strings.Repeat("A", 64))), "hash"},
		"zero size":              {productPage(amalgamationRow("3.54.1", "2026/sqlite-amalgamation-3540100.zip", 0, hexOf("a"))), "size"},
		"size not a number":      {productPage("PRODUCT,3.54.1,2026/sqlite-amalgamation-3540100.zip,big," + hexOf("a")), "size"},
		"bad year":               {productPage(amalgamationRow("3.54.1", "26/sqlite-amalgamation-3540100.zip", 10, hexOf("a"))), "path"},
		"nested path":            {productPage(amalgamationRow("3.54.1", "2026/x/sqlite-amalgamation-3540100.zip", 10, hexOf("a"))), "path"},
		"dot-dot path":           {productPage(amalgamationRow("3.54.1", "../sqlite-amalgamation-3540100.zip", 10, hexOf("a"))), "path"},
		"numeric mismatch":       {productPage(amalgamationRow("3.54.1", "2026/sqlite-amalgamation-3540200.zip", 10, hexOf("a"))), "numeric"},
		"version not stable":     {productPage(amalgamationRow("3.54.1-rc1", "2026/sqlite-amalgamation-3540100.zip", 10, hexOf("a"))), "version"},
		"outside range":          {productPage(amalgamationRow("4.0.0", "2026/sqlite-amalgamation-4000000.zip", 10, hexOf("a"))), "satisfy"},
		"below range":            {productPage(amalgamationRow("3.53.0", "2026/sqlite-amalgamation-3530000.zip", 10, hexOf("a"))), "satisfy"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseSQLiteProducts(tc.page, c)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v does not mention %q", err, tc.want)
			}
		})
	}
}

// sqliteBundle writes a source-build bundle directory whose lock pins
// version 3.54.0 and returns it with the archive bytes of 3.54.1.
func sqliteBundle(t *testing.T) (dir string, archive []byte) {
	t.Helper()
	dir = t.TempDir()
	linenoise := "a473823d74b93eab2ba83480df16ed37617493f2"
	catalog := toolconfig.Catalog{Schema: 1, Packages: map[string]toolconfig.Package{"sqlite3": {
		Provider:      "source-build",
		Normalization: "semver",
		VersionSource: &toolconfig.VersionSource{Kind: "sqlite-product-data", URL: "https://www.sqlite.org/download.html"},
		Architectures: map[string]string{"amd64": "x86_64", "arm64": "aarch64"},
		Asset:         "sqlite-amalgamation-{numeric}.zip",
		BaseURL:       "https://www.sqlite.org",
		AllowedHosts:  []string{"www.sqlite.org", "raw.githubusercontent.com"},
		Verification:  "official-or-download",
		Build: &toolconfig.Build{Kind: "c-amalgamation", Toolchain: "zig-cc", ArchivePrefix: "sqlite-amalgamation-{numeric}/",
			Sources: []string{"shell.c", "sqlite3.c", "linenoise.c"}, ExtraSources: []string{"linenoise"},
			Defines: []string{"HAVE_LINENOISE=1"}, Cflags: []string{"-Os"}, Libs: []string{"m", "pthread"}, Static: true, OutputForm: "exec", Output: "sqlite3"},
		ExtraSources: map[string]toolconfig.ExtraSource{"linenoise": {Repository: "antirez/linenoise", Commit: linenoise,
			Files: map[string]string{"linenoise.c": "linenoise.c", "linenoise.h": "linenoise.h", "LICENSE": "LICENSE"},
			URL:   "https://raw.githubusercontent.com/antirez/linenoise/{commit}/{file}", AllowedHosts: []string{"raw.githubusercontent.com"}}},
		Install: toolconfig.Install{Kind: "build-output", Members: map[string]string{"sqlite3": "bin/sqlite3"}},
	}}}
	zig := func(arch string) toolconfig.Spec {
		return toolconfig.Spec{Version: zigPin, URL: "https://ziglang.org/download/" + zigPin + "/zig-" + catalog.Packages["sqlite3"].Architectures[arch] + "-linux-" + zigPin + ".tar.xz", SHA256: hexOf("1"), Verification: "publisher-sha256"}
	}
	old := toolconfig.Spec{Version: "3.54.0", URL: "https://www.sqlite.org/2026/sqlite-amalgamation-3540000.zip", SHA256: hexOf("2"), SHA3256: hexOf("3"), Verification: "publisher-sha3-256", Toolchain: "zig-cc"}
	files := map[string]toolconfig.LockedFile{}
	for _, f := range []string{"linenoise.c", "linenoise.h", "LICENSE"} {
		files[f] = toolconfig.LockedFile{URL: "https://raw.githubusercontent.com/antirez/linenoise/" + linenoise + "/" + f, SHA256: hexOf("5"), Verification: "tls-download-sha256"}
	}
	c := toolconfig.Config{
		Catalog: catalog,
		Lock: toolconfig.Lock{
			Toolchains:   map[string]map[string]toolconfig.Spec{"zig-cc": {"amd64": zig("amd64"), "arm64": zig("arm64")}},
			Platforms:    map[string]map[string]toolconfig.Spec{"amd64": {"sqlite3": old}, "arm64": {"sqlite3": old}},
			ExtraSources: map[string]toolconfig.LockedSource{"sqlite3/linenoise": {Commit: linenoise, Files: files}},
		},
	}
	var err error
	c.Desired, err = toolconfig.ParseDesired(strings.NewReader("sqlite3>=3.50.0,<4.0.0\n"))
	if err != nil {
		t.Fatal(err)
	}
	c.Bind()
	if err := c.Validate(); err != nil {
		t.Fatalf("fixture invalid: %v", err)
	}
	write := func(name string, v any) {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "packages.txt"), []byte("sqlite3>=3.50.0,<4.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	write("packages.catalog.json", c.Catalog)
	write("packages.lock.json", c.Lock)
	return dir, []byte("pretend amalgamation zip of 3.54.1")
}

func sqliteServer(t *testing.T, archive []byte, hash string, version, rel string) *http.Client {
	t.Helper()
	return fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/download.html":
			w.Write(productPage(amalgamationRow(version, rel, len(archive), hash)))
		case "/" + rel:
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	})
}

func lockSections(t *testing.T, dir string) (toolchains, extra, platforms []byte) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(readLock(t, dir), &raw); err != nil {
		t.Fatal(err)
	}
	return raw["toolchains"], raw["extra_sources"], raw["platforms"]
}

func TestSourceBuildUpgradeBumpsOnlyTheSource(t *testing.T) {
	const rel = "2026/sqlite-amalgamation-3540100.zip"
	for _, tool := range []string{"", "sqlite3"} {
		t.Run("tool="+tool, func(t *testing.T) {
			dir, archive := sqliteBundle(t)
			tc0, ex0, _ := lockSections(t, dir)
			client := sqliteServer(t, archive, sha3Hex(archive), "3.54.1", rel)
			var out bytes.Buffer
			if err := Upgrade(context.Background(), client, dir, tool, &out); err != nil {
				t.Fatal(err)
			}
			tc1, ex1, _ := lockSections(t, dir)
			if !bytes.Equal(tc0, tc1) || !bytes.Equal(ex0, ex1) {
				t.Fatalf("toolchains or extra sources changed:\n%s\n%s", tc1, ex1)
			}
			c, err := toolconfig.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := toolconfig.Spec{Version: "3.54.1", URL: "https://www.sqlite.org/" + rel, SHA256: fmt.Sprintf("%x", sha256.Sum256(archive)), SHA3256: sha3Hex(archive), Verification: "publisher-sha3-256", Toolchain: "zig-cc"}
			for _, arch := range []string{"amd64", "arm64"} {
				if got := c.Lock.Platforms[arch]["sqlite3"]; got != want {
					t.Fatalf("%s: %+v, want %+v", arch, got, want)
				}
			}
			if !strings.Contains(out.String(), "sqlite3 3.54.1") {
				t.Fatalf("report %q", out.String())
			}
		})
	}
}

func TestSourceBuildUpgradeFailsClosedAndKeepsTheLock(t *testing.T) {
	const rel = "2026/sqlite-amalgamation-3540100.zip"
	for _, tool := range []string{"", "sqlite3"} {
		for name, tc := range map[string]struct {
			hash    string
			version string
			rel     string
			want    string
		}{
			"publisher hash mismatch": {hexOf("0"), "3.54.1", rel, "mismatch"},
			"downgrade":               {"", "3.53.0", "2026/sqlite-amalgamation-3530000.zip", "older"},
			"same version other hash": {"", "3.54.0", "2026/sqlite-amalgamation-3540000.zip", "changed"},
		} {
			t.Run("tool="+tool+"/"+name, func(t *testing.T) {
				dir, archive := sqliteBundle(t)
				before := readLock(t, dir)
				hash := tc.hash
				if hash == "" {
					hash = sha3Hex(archive)
				}
				err := Upgrade(context.Background(), sqliteServer(t, archive, hash, tc.version, tc.rel), dir, tool, &bytes.Buffer{})
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error %v does not mention %q", err, tc.want)
				}
				if !bytes.Equal(before, readLock(t, dir)) {
					t.Fatal("lock modified by a failed upgrade")
				}
			})
		}
	}
}

func TestSourceBuildUpgradeNeedsPinnedToolchains(t *testing.T) {
	dir, archive := sqliteBundle(t)
	if err := os.Remove(filepath.Join(dir, "packages.lock.json")); err != nil {
		t.Fatal(err)
	}
	err := Upgrade(context.Background(), sqliteServer(t, archive, sha3Hex(archive), "3.54.1", "2026/sqlite-amalgamation-3540100.zip"), dir, "", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "toolchain") {
		t.Fatalf("a first lock without toolchain pins must fail by name: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "packages.lock.json")); statErr == nil {
		t.Fatal("lock written")
	}
}

func TestSourceBuildUpgradeRefusesAChangedRecipePin(t *testing.T) {
	dir, archive := sqliteBundle(t)
	b, _ := os.ReadFile(filepath.Join(dir, "packages.catalog.json"))
	b = bytes.ReplaceAll(b, []byte("a473823d74b93eab2ba83480df16ed37617493f2"), []byte("b473823d74b93eab2ba83480df16ed37617493f2"))
	if err := os.WriteFile(filepath.Join(dir, "packages.catalog.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Upgrade(context.Background(), sqliteServer(t, archive, sha3Hex(archive), "3.54.1", "2026/sqlite-amalgamation-3540100.zip"), dir, "", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "extra source") {
		t.Fatalf("a bumped extra-source commit must need an owner-authored lock: %v", err)
	}
}

// The real toolbox entry resolves like the fixture: a newer SQLite release moves only the
// sqlite3 source pin, and the toolchain, the linenoise pin and every other tool stay.
func TestToolboxSQLiteUpgradeBumpsOnlyTheSource(t *testing.T) {
	dir := checkout(t)
	tc0, ex0, _ := lockSections(t, dir)
	before, err := toolconfig.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The next patch release after whatever the lock pins now (no version constant).
	locked, err := toolconfig.ParseVersion(before.Lock.Platforms["amd64"]["sqlite3"].Version)
	if err != nil {
		t.Fatal(err)
	}
	locked[2]++
	next := locked.String()
	numeric, err := toolconfig.NumericVersion(next)
	if err != nil {
		t.Fatal(err)
	}
	rel := "2026/sqlite-amalgamation-" + numeric + ".zip"
	archive := []byte("pretend amalgamation zip of " + next)
	client := sqliteServer(t, archive, sha3Hex(archive), next, rel)
	if err := Upgrade(context.Background(), client, dir, "sqlite3", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	tc1, ex1, _ := lockSections(t, dir)
	compact := func(b []byte) string {
		var out bytes.Buffer
		if err := json.Compact(&out, b); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	if compact(tc0) != compact(tc1) || compact(ex0) != compact(ex1) {
		t.Fatal("toolchains or extra sources changed")
	}
	after, err := toolconfig.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := toolconfig.Spec{Version: next, URL: "https://www.sqlite.org/" + rel, SHA256: fmt.Sprintf("%x", sha256.Sum256(archive)), SHA3256: sha3Hex(archive), Verification: "publisher-sha3-256", Toolchain: "zig-cc"}
	for arch, tools := range after.Lock.Platforms {
		for name, spec := range tools {
			if name == "sqlite3" && spec != want || name != "sqlite3" && spec != before.Lock.Platforms[arch][name] {
				t.Fatalf("%s/%s: %+v", arch, name, spec)
			}
		}
	}
}
