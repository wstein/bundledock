package toolbundle

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha3"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const linenoiseCommit = "a473823d74b93eab2ba83480df16ed37617493f2"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// sourceBuildFixture is the design's sqlite3 recipe with real hashes of synthetic
// payloads, and a client that serves them without any network access.
type sourceBuildFixture struct {
	config  *toolconfig.Config
	served  map[string][]byte // URL to body
	client  *http.Client
	fetched []string
}

func hex256(b []byte) string { s := sha256.Sum256(b); return fmt.Sprintf("%x", s) }
func hex3(b []byte) string   { s := sha3.Sum256(b); return fmt.Sprintf("%x", s) }

func newSourceBuildFixture(t *testing.T) *sourceBuildFixture {
	t.Helper()
	f := &sourceBuildFixture{served: map[string][]byte{}}
	zigTop := "zig-x86_64-linux-0.17.0"
	zigURL := "https://ziglang.org/download/0.17.0/" + zigTop + ".tar.xz"
	zig := xzArchive(t,
		member{zigTop + "/", "", 0755, tar.TypeDir},
		member{zigTop + "/zig", "zig compiler", 0755, 0},
		member{zigTop + "/lib/", "", 0755, tar.TypeDir},
		member{zigTop + "/lib/include/stdio.h", "header", 0644, 0},
		member{zigTop + "/LICENSE", "mit", 0644, 0},
	)
	source := zipArchive(t,
		member{amalgamation, "", 0755, 0},
		member{amalgamation + "shell.c", "shell source", 0644, 0},
		member{amalgamation + "sqlite3.c", "sqlite source", 0644, 0},
		member{amalgamation + "sqlite3.h", "sqlite header", 0644, 0},
	)
	sourceURL := "https://www.sqlite.org/2026/sqlite-amalgamation-3540100.zip"
	extra := map[string]string{"linenoise.c": "linenoise source", "linenoise.h": "linenoise header", "LICENSE": "bsd-2-clause"}
	files := map[string]toolconfig.LockedFile{}
	for name, body := range extra {
		u := "https://raw.githubusercontent.com/antirez/linenoise/" + linenoiseCommit + "/" + name
		f.served[u] = []byte(body)
		files[name] = toolconfig.LockedFile{URL: u, SHA256: hex256([]byte(body)), Verification: "tls-download-sha256"}
	}
	f.served[zigURL], f.served[sourceURL] = zig, source
	pkg := toolconfig.Package{
		Provider: "source-build", Normalization: "semver",
		VersionSource: &toolconfig.VersionSource{Kind: "sqlite-product-data", URL: "https://www.sqlite.org/download.html"},
		Architectures: map[string]string{"amd64": "x86_64", "arm64": "aarch64"},
		Asset:         "sqlite-amalgamation-{numeric}.zip", BaseURL: "https://www.sqlite.org",
		AllowedHosts: []string{"www.sqlite.org", "raw.githubusercontent.com"}, Verification: "official-or-download",
		Build: &toolconfig.Build{Kind: "c-amalgamation", Toolchain: "zig-cc", ArchivePrefix: "sqlite-amalgamation-{numeric}/",
			Sources: []string{"shell.c", "sqlite3.c", "linenoise.c"}, ExtraSources: []string{"linenoise"},
			Defines: []string{"HAVE_LINENOISE=1", "SQLITE_THREADSAFE=1"}, Cflags: []string{"-Os"}, Libs: []string{"m", "pthread"},
			Static: true, OutputForm: "exec", Output: "sqlite3"},
		ExtraSources: map[string]toolconfig.ExtraSource{"linenoise": {Repository: "antirez/linenoise", Commit: linenoiseCommit,
			Files: map[string]string{"linenoise.c": "linenoise.c", "linenoise.h": "linenoise.h", "LICENSE": "LICENSE"},
			URL:   "https://raw.githubusercontent.com/antirez/linenoise/{commit}/{file}", AllowedHosts: []string{"raw.githubusercontent.com"}}},
		Install: toolconfig.Install{Kind: "build-output", Members: map[string]string{"sqlite3": "bin/sqlite3"}},
	}
	tc := toolconfig.Spec{Version: "0.17.0", URL: zigURL, SHA256: hex256(zig), Verification: "publisher-sha256"}
	archive := toolconfig.Spec{Version: "3.54.1", URL: sourceURL, SHA256: hex256(source), SHA3256: hex3(source), Verification: "publisher-sha3-256", Toolchain: "zig-cc"}
	f.config = &toolconfig.Config{
		Catalog: toolconfig.Catalog{Schema: 1, Packages: map[string]toolconfig.Package{"sqlite3": pkg}},
		Lock: toolconfig.Lock{
			Toolchains:   map[string]map[string]toolconfig.Spec{"zig-cc": {"amd64": tc}},
			Platforms:    map[string]map[string]toolconfig.Spec{"amd64": {"sqlite3": archive}},
			ExtraSources: map[string]toolconfig.LockedSource{"sqlite3/linenoise": {Commit: linenoiseCommit, Files: files}},
		},
	}
	f.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		f.fetched = append(f.fetched, r.URL.String())
		body, ok := f.served[r.URL.String()]
		rec := httptest.NewRecorder()
		if !ok {
			http.NotFound(rec, r)
		} else {
			rec.Write(body)
		}
		return rec.Result(), nil
	})}
	return f
}

func (f *sourceBuildFixture) set(url string, body []byte) { f.served[url] = body }

func fetchTo(t *testing.T, f *sourceBuildFixture) (string, error) {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "work")
	var out bytes.Buffer
	err := FetchSourceBuild(context.Background(), f.client, f.config, "sqlite3", "amd64", dst, &out)
	return dst, err
}

func TestFetchSourceBuildUnpacksVerifiedInputs(t *testing.T) {
	f := newSourceBuildFixture(t)
	dst, err := fetchTo(t, f)
	if err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{
		"toolchain/zig":                 "zig compiler",
		"toolchain/lib/include/stdio.h": "header",
		"src/shell.c":                   "shell source",
		"src/sqlite3.c":                 "sqlite source",
		"src/sqlite3.h":                 "sqlite header",
		"src/linenoise.c":               "linenoise source",
		"src/linenoise.h":               "linenoise header",
		"src/LICENSE":                   "bsd-2-clause",
	} {
		got, err := os.ReadFile(filepath.Join(dst, file))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q %v", file, got, err)
		}
	}
	if info, _ := os.Stat(filepath.Join(dst, "toolchain/zig")); info.Mode().Perm() != 0755 {
		t.Fatalf("zig mode %v", info.Mode())
	}
}

// Each input is rejected on its checksum before it is unpacked, so a tampered
// download leaves no extracted file of that input behind.
func TestFetchSourceBuildRejectsTamperedInputsBeforeExtraction(t *testing.T) {
	for name, tc := range map[string]struct {
		url    func(*sourceBuildFixture) string
		body   []byte
		want   string
		absent string
	}{
		"toolchain":      {func(f *sourceBuildFixture) string { return f.config.Lock.Toolchains["zig-cc"]["amd64"].URL }, []byte("not xz at all"), "sha256 checksum mismatch", "toolchain"},
		"source archive": {func(f *sourceBuildFixture) string { return f.config.Lock.Platforms["amd64"]["sqlite3"].URL }, []byte("not a zip"), "checksum mismatch", "src"},
		"extra file": {func(f *sourceBuildFixture) string {
			return f.config.Lock.ExtraSources["sqlite3/linenoise"].Files["linenoise.c"].URL
		}, []byte("tampered"), "sha256 checksum mismatch", "src/linenoise.c"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newSourceBuildFixture(t)
			f.set(tc.url(f), tc.body)
			dst, err := fetchTo(t, f)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v does not mention %q", err, tc.want)
			}
			if _, statErr := os.Stat(filepath.Join(dst, tc.absent)); statErr == nil {
				t.Fatalf("%s was extracted before verification", tc.absent)
			}
		})
	}
}

func TestFetchSourceBuildChecksBothSourceHashes(t *testing.T) {
	for name, mutate := range map[string]func(s *toolconfig.Spec){
		"publisher sha3-256": func(s *toolconfig.Spec) { s.SHA3256 = hex3([]byte("other")) },
		"builder sha256":     func(s *toolconfig.Spec) { s.SHA256 = hex256([]byte("other")) },
	} {
		t.Run(name, func(t *testing.T) {
			f := newSourceBuildFixture(t)
			s := f.config.Lock.Platforms["amd64"]["sqlite3"]
			mutate(&s)
			f.config.Lock.Platforms["amd64"]["sqlite3"] = s
			dst, err := fetchTo(t, f)
			if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("%v", err)
			}
			if _, statErr := os.Stat(filepath.Join(dst, "src/shell.c")); statErr == nil {
				t.Fatal("source extracted despite a hash mismatch")
			}
		})
	}
}

func TestFetchSourceBuildFailsClosed(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(f *sourceBuildFixture)
		want   string
	}{
		"http error": {func(f *sourceBuildFixture) { delete(f.served, f.config.Lock.Platforms["amd64"]["sqlite3"].URL) }, "404"},
		"archive lacks a listed source": {func(f *sourceBuildFixture) {
			p := f.config.Catalog.Packages["sqlite3"]
			p.Build.Sources = append(p.Build.Sources, "missing.c")
			f.config.Catalog.Packages["sqlite3"] = p
		}, "missing"},
		"archive member collides with an extra source": {func(f *sourceBuildFixture) {
			z := zipArchive(t,
				member{amalgamation + "shell.c", "s", 0644, 0}, member{amalgamation + "sqlite3.c", "s", 0644, 0},
				member{amalgamation + "Linenoise.c", "evil", 0644, 0})
			s := f.config.Lock.Platforms["amd64"]["sqlite3"]
			s.SHA256, s.SHA3256 = hex256(z), hex3(z)
			f.config.Lock.Platforms["amd64"]["sqlite3"] = s
			f.set(s.URL, z)
		}, "collides"},
		"toolchain with a symlink": {func(f *sourceBuildFixture) {
			top := "zig-x86_64-linux-0.17.0"
			x := xzArchive(t, member{top + "/zig", "z", 0755, 0}, member{top + "/lib", "", 0777, tar.TypeSymlink})
			s := f.config.Lock.Toolchains["zig-cc"]["amd64"]
			s.SHA256 = hex256(x)
			f.config.Lock.Toolchains["zig-cc"]["amd64"] = s
			f.set(s.URL, x)
		}, "unsupported"},
		"toolchain member outside its root": {func(f *sourceBuildFixture) {
			x := xzArchive(t, member{"zig-x86_64-linux-0.17.0/zig", "z", 0755, 0}, member{"other/evil", "x", 0644, 0})
			s := f.config.Lock.Toolchains["zig-cc"]["amd64"]
			s.SHA256 = hex256(x)
			f.config.Lock.Toolchains["zig-cc"]["amd64"] = s
			f.set(s.URL, x)
		}, "outside"},
		"toolchain without zig": {func(f *sourceBuildFixture) {
			x := xzArchive(t, member{"zig-x86_64-linux-0.17.0/README", "r", 0644, 0})
			s := f.config.Lock.Toolchains["zig-cc"]["amd64"]
			s.SHA256 = hex256(x)
			f.config.Lock.Toolchains["zig-cc"]["amd64"] = s
			f.set(s.URL, x)
		}, "zig"},
		"toolchain zig not executable": {func(f *sourceBuildFixture) {
			x := xzArchive(t, member{"zig-x86_64-linux-0.17.0/zig", "z", 0644, 0})
			s := f.config.Lock.Toolchains["zig-cc"]["amd64"]
			s.SHA256 = hex256(x)
			f.config.Lock.Toolchains["zig-cc"]["amd64"] = s
			f.set(s.URL, x)
		}, "zig"},
		"unknown tool": {func(f *sourceBuildFixture) { delete(f.config.Catalog.Packages, "sqlite3") }, "not a source-build"},
		"no toolchain": {func(f *sourceBuildFixture) { f.config.Lock.Toolchains = nil }, "lacks"},
		"no sha256 in extra lock": {func(f *sourceBuildFixture) {
			l := f.config.Lock.ExtraSources["sqlite3/linenoise"]
			lf := l.Files["LICENSE"]
			lf.SHA256 = ""
			l.Files["LICENSE"] = lf
		}, "invalid sha256"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newSourceBuildFixture(t)
			tc.mutate(f)
			_, err := fetchTo(t, f)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v does not mention %q", err, tc.want)
			}
		})
	}
}

func TestFetchSourceBuildNeedsAFreshDestination(t *testing.T) {
	f := newSourceBuildFixture(t)
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "old"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := FetchSourceBuild(context.Background(), f.client, f.config, "sqlite3", "amd64", dst, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("%v", err)
	}
	if len(f.fetched) != 0 {
		t.Fatalf("downloaded into a used destination: %v", f.fetched)
	}
}

func TestToolchainExtractionBounds(t *testing.T) {
	top := "zig-x86_64-linux-0.17.0"
	for name, members := range map[string][]member{
		"duplicate path": {{top + "/zig", "a", 0755, 0}, {top + "/zig", "b", 0755, 0}},
		"zip slip":       {{top + "/../evil", "x", 0644, 0}},
		"hard link":      {{top + "/zig", "", 0755, tar.TypeLink}},
		"root is a file": {{top, "x", 0644, 0}},
	} {
		t.Run(name, func(t *testing.T) {
			root, _ := openRoot(t)
			if err := extractToolchain(root, bytes.NewReader(xzArchive(t, members...)), top, defaultToolchainLimits); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func buildWith(t *testing.T, f *sourceBuildFixture, outputs string, opts ...BuildOption) (string, error) {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "bundle")
	if outputs != "" {
		opts = append(opts, WithBuildOutputs(outputs))
	}
	tools := f.config.Lock.Platforms["amd64"]
	err := Build(context.Background(), f.client, tools, f.config.Catalog.Packages, os.DirFS(t.TempDir()), "amd64", dst, &bytes.Buffer{}, opts...)
	return dst, err
}

func writeOutput(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sqlite3"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sqlite3", "sqlite3"), data, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBuildInstallsACheckedCompiledOutput(t *testing.T) {
	f := newSourceBuildFixture(t)
	elfBytes := staticExec(0x3e).bytes()
	dst, err := buildWith(t, f, writeOutput(t, elfBytes))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "bin/sqlite3"))
	info, _ := os.Stat(filepath.Join(dst, "bin/sqlite3"))
	if err != nil || !bytes.Equal(got, elfBytes) || info.Mode().Perm() != 0755 {
		t.Fatalf("installed output: %v %v", err, info)
	}
	manifest, _ := os.ReadFile(filepath.Join(dst, "manifest.json"))
	for _, want := range []string{`"sqlite3"`, `"sha3_256": "` + f.config.Lock.Platforms["amd64"]["sqlite3"].SHA3256, `"toolchain": "zig-cc"`} {
		if !strings.Contains(string(manifest), want) {
			t.Errorf("manifest lacks %s:\n%s", want, manifest)
		}
	}
	if len(f.fetched) != 0 {
		t.Fatalf("the bundle build downloaded %v; compiled outputs come from the compile stage", f.fetched)
	}
}

func TestBuildRejectsUnverifiableCompiledOutput(t *testing.T) {
	dynamic := staticExec(0x3e)
	dynamic.progs = append(dynamic.progs, fixtureProg{3, 4}) // PT_INTERP
	for name, tc := range map[string]struct {
		outputs func(t *testing.T) string
		want    string
	}{
		"dynamic output":   {func(t *testing.T) string { return writeOutput(t, dynamic.bytes()) }, "PT_INTERP"},
		"wrong machine":    {func(t *testing.T) string { return writeOutput(t, staticExec(0xb7).bytes()) }, "machine"},
		"not an ELF":       {func(t *testing.T) string { return writeOutput(t, []byte("#!/bin/sh\n")) }, "not an ELF"},
		"no outputs given": {func(t *testing.T) string { return "" }, "compiled"},
		"missing output":   {func(t *testing.T) string { return t.TempDir() }, "sqlite3"},
		"output is a link": {func(t *testing.T) string {
			dir := writeOutput(t, staticExec(0x3e).bytes())
			if err := os.Rename(filepath.Join(dir, "sqlite3", "sqlite3"), filepath.Join(dir, "real")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("../real", filepath.Join(dir, "sqlite3", "sqlite3")); err != nil {
				t.Fatal(err)
			}
			return dir
		}, "sqlite3"},
	} {
		t.Run(name, func(t *testing.T) {
			dst, err := buildWith(t, newSourceBuildFixture(t), tc.outputs(t))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v does not mention %q", err, tc.want)
			}
			if _, statErr := os.Stat(filepath.Join(dst, "bin/sqlite3")); statErr == nil {
				t.Fatal("rejected output was installed")
			}
			if _, statErr := os.Stat(filepath.Join(dst, "manifest.json")); statErr == nil {
				t.Fatal("manifest written for a rejected build")
			}
		})
	}
}

func TestToolchainExtractionEnforcesEachBound(t *testing.T) {
	top := "zig-x86_64-linux-0.17.0"
	zig := member{top + "/zig", "0123456789", 0755, 0}
	lib := member{top + "/lib/a.h", "0123456789", 0644, 0}
	for name, tc := range map[string]struct {
		lim     toolchainLimits
		members []member
		want    string // empty: must be accepted
	}{
		"members at the limit": {toolchainLimits{3, 10, 1 << 20}, []member{{top + "/", "", 0755, tar.TypeDir}, zig, lib}, ""},
		"one member too many":  {toolchainLimits{3, 10, 1 << 20}, []member{{top + "/", "", 0755, tar.TypeDir}, zig, lib, {top + "/b.h", "x", 0644, 0}}, "too many"},
		"member at the limit":  {toolchainLimits{10, 10, 1 << 20}, []member{zig}, ""},
		"member one byte over": {toolchainLimits{10, 10, 1 << 20}, []member{zig, {top + "/lib/big.h", "0123456789a", 0644, 0}}, "too large"},
		"total at the limit":   {toolchainLimits{10, 10, 3072}, []member{zig, lib}, ""},
		"total over the limit": {toolchainLimits{10, 10, 3072}, []member{zig, lib, {top + "/c.h", "0123456789", 0644, 0}}, "exceeds"},
	} {
		t.Run(name, func(t *testing.T) {
			root, _ := openRoot(t)
			err := extractToolchain(root, bytes.NewReader(xzArchive(t, tc.members...)), top, tc.lim)
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error %v does not mention %q", err, tc.want)
			}
		})
	}
}
