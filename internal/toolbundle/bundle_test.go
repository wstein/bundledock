package toolbundle

import (
	"anchorage/internal/toolconfig"
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"

	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type member struct {
	name, data string
	mode       int64
	kind       byte
}

func archive(t *testing.T, members ...member) []byte {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		kind := m.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		h := &tar.Header{Name: m.name, Mode: m.mode, Typeflag: kind, Linkname: "outside"}
		if kind == tar.TypeReg {
			h.Size = int64(len(m.data))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if kind == tar.TypeReg {
			if _, err := io.WriteString(tw, m.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func openRoot(t *testing.T) (*os.Root, string) {
	t.Helper()
	name := t.TempDir()
	r, err := os.OpenRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r, name
}
func fixture(t *testing.T) (map[string][]byte, map[string]Spec, *httptest.Server) {
	t.Helper()
	payloads := map[string][]byte{
		"uv":       archive(t, member{"uv-1/uv", "uv executable", 0755, 0}, member{"uv-1/uvx", "uvx executable", 0755, 0}),
		"ast-grep": sqliteArchive(t, member{"ast-grep", "ast-grep executable", 0755, 0}, member{"sg", "unwanted alias", 0755, 0}),
		"sqlite3":  sqliteArchive(t, member{"sqlite/sqlite3.c", "sqlite source", 0644, 0}, member{"sqlite/shell.c", "shell source", 0644, 0}, member{"sqlite/sqlite3.h", "sqlite header", 0644, 0}, member{"sqlite/sqlite3ext.h", "extension header", 0644, 0}),
		"claude":   []byte("claude executable"),
		"yq":       []byte("yq executable"),
		"jq":       []byte("jq executable"),
		"rg":       archive(t, member{"ripgrep-1/rg", "ripgrep executable", 0755, 0}, member{"ripgrep-1/LICENSE-MIT", "rg license", 0644, 0}, member{"ripgrep-1/UNLICENSE", "rg public domain", 0644, 0}, member{"ripgrep-1/COPYING", "rg copying", 0644, 0}),
		"fd":       archive(t, member{"fd-1/fd", "fd executable", 0755, 0}, member{"fd-1/LICENSE-MIT", "fd mit", 0644, 0}, member{"fd-1/LICENSE-APACHE", "fd apache", 0644, 0}),
		"gh":       archive(t, member{"gh_1/bin/gh", "github cli", 0755, 0}, member{"gh_1/share/man/gh.1", "manual", 0644, 0}),
		"agy":      archive(t, member{"bundle/antigravity", "antigravity cli", 0755, 0}),
		"codex":    archive(t, member{"bin/codex", "codex cli", 0755, 0}, member{"codex-package.json", `{"entrypoint":"bin/codex"}`, 0644, 0}, member{"bin/codex-code-mode-host", "host", 0755, 0}, member{"codex-path/rg", "rg", 0755, 0}, member{"codex-resources/bwrap", "bwrap", 0755, 0}, member{"codex-resources/voice/lib/example.so", "shared library", 0644, 0}, member{"empty/", "", 0755, tar.TypeDir}),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if data, ok := payloads[strings.TrimPrefix(r.URL.Path, "/")]; ok {
			w.Write(data)
		} else {
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	tools := map[string]Spec{}
	for name, data := range payloads {
		s := Spec{Version: "fixture", URL: server.URL + "/" + name}
		if name == "agy" {
			digest := sha512.Sum512(data)
			s.SHA512 = hex.EncodeToString(digest[:])
		} else {
			digest := sha256.Sum256(data)
			s.SHA256 = hex.EncodeToString(digest[:])
		}
		tools[name] = s
	}
	return payloads, tools, server
}
func TestVendorBuildBothArchitectures(t *testing.T) {
	_, tools, server := fixture(t)
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			specs := tools
			first := t.TempDir()
			second := t.TempDir()
			for _, dst := range []string{first, second} {
				root, err := os.OpenRoot(dst)
				if err != nil {
					t.Fatal(err)
				}
				if err := WriteFile(root, "bin/sqlite3", strings.NewReader("compiled sqlite"), 0755); err != nil {
					t.Fatal(err)
				}
				root.Close()
				if err := Build(context.Background(), server.Client(), specs, testPackages(t), os.DirFS("../.."), arch, dst, io.Discard); err != nil {
					t.Fatal(err)
				}
			}
			want := map[string]struct {
				data string
				mode os.FileMode
			}{
				"bin/uv": {"uv executable", 0755}, "bin/uvx": {"uvx executable", 0755},
				"bin/ast-grep": {"ast-grep executable", 0755},
				"bin/sqlite3":  {"compiled sqlite", 0755},
				"bin/yq":       {"yq executable", 0755},
				"bin/jq":       {"jq executable", 0755}, "bin/rg": {"ripgrep executable", 0755}, "bin/fd": {"fd executable", 0755}, "bin/fdfind": {"fd executable", 0755},
				"licenses/rg/LICENSE-MIT": {"rg license", 0644}, "licenses/rg/UNLICENSE": {"rg public domain", 0644}, "licenses/rg/COPYING": {"rg copying", 0644},
				"licenses/fd/LICENSE-MIT": {"fd mit", 0644}, "licenses/fd/LICENSE-APACHE": {"fd apache", 0644},
				"bin/claude": {"claude executable", 0755}, "bin/gh": {"github cli", 0755}, "bin/agy": {"antigravity cli", 0755}, "bin/codex": {"codex cli", 0755},
				"bin/codex-code-mode-host": {"host", 0755}, "codex-path/rg": {"rg", 0755}, "codex-resources/bwrap": {"bwrap", 0755}, "codex-resources/voice/lib/example.so": {"shared library", 0644}, "codex-package.json": {`{"entrypoint":"bin/codex"}`, 0644},
			}
			for name, w := range want {
				p := filepath.Join(first, name)
				b, err := os.ReadFile(p)
				if err != nil || string(b) != w.data {
					t.Fatalf("%s data=%q err=%v", name, b, err)
				}
				info, err := os.Stat(p)
				if err != nil || info.Mode().Perm() != w.mode {
					t.Fatalf("%s mode=%v err=%v", name, info, err)
				}
			}
			if _, err := os.Stat(filepath.Join(first, "bin/sg")); !os.IsNotExist(err) {
				t.Fatalf("unexpected sg alias: %v", err)
			}
			for _, name := range []string{"manifest.json", "SHA256SUMS"} {
				a, _ := os.ReadFile(filepath.Join(first, name))
				b, _ := os.ReadFile(filepath.Join(second, name))
				if !bytes.Equal(a, b) {
					t.Fatalf("nondeterministic %s", name)
				}
			}
			sums, _ := os.ReadFile(filepath.Join(first, "SHA256SUMS"))
			var names []string
			for _, line := range strings.Split(strings.TrimSpace(string(sums)), "\n") {
				names = append(names, line[66:])
				data, err := os.ReadFile(filepath.Join(first, line[66:]))
				if err != nil {
					t.Fatal(err)
				}
				if fmt.Sprintf("%x", sha256.Sum256(data)) != line[:64] {
					t.Fatalf("bad checksum %q", line)
				}
			}
			if !sort.StringsAreSorted(names) || !strings.Contains(string(sums), "  manifest.json\n") || len(names) < len(want)+1 {
				t.Fatalf("unexpected sums %q", sums)
			}
			if _, err := os.Stat(filepath.Join(first, "empty")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestDownloadFailuresBeforeExtraction(t *testing.T) {
	for _, name := range []string{"checksum", "status", "truncated", "timeout"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch name {
				case "status":
					http.Error(w, "unavailable", 503)
				case "truncated":
					w.Header().Set("Content-Length", "100")
					io.WriteString(w, "short")
				case "timeout":
					<-r.Context().Done()
				default:
					io.WriteString(w, "wrong")
				}
			}))
			defer server.Close()
			root, _ := openRoot(t)
			digest := sha256.Sum256([]byte("expected"))
			client := server.Client()
			client.Timeout = 30 * time.Millisecond
			err := install(context.Background(), client, root, "claude", testPackages(t)["claude"], Spec{URL: server.URL, SHA256: fmt.Sprintf("%x", digest)})
			if err == nil {
				t.Fatal("expected download failure")
			}
			if _, err := root.Stat("bin/claude"); !os.IsNotExist(err) {
				t.Fatalf("unverified artifact installed: %v", err)
			}
		})
	}
}
func TestArchiveSafetyAndEntrypoints(t *testing.T) {
	cases := []struct {
		name    string
		members []member
	}{
		{"absolute", []member{{"/escape", "x", 0755, 0}}}, {"parent", []member{{"bin/../escape", "x", 0755, 0}}},
		{"symlink", []member{{"bin/codex", "", 0755, tar.TypeSymlink}}}, {"hardlink", []member{{"bin/codex", "", 0755, tar.TypeLink}}},
		{"device", []member{{"device", "", 0755, tar.TypeChar}}}, {"fifo", []member{{"fifo", "", 0755, tar.TypeFifo}}},
		{"missing", []member{{"bin/helper", "helper", 0755, 0}}}, {"non executable", []member{{"bin/codex", "cli", 0644, 0}}},
		{"duplicate", []member{{"bin/codex", "cli", 0755, 0}, {"bin/codex", "other", 0755, 0}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, _ := openRoot(t)
			if err := Extract(root, "codex", testPackages(t)["codex"], bytes.NewReader(archive(t, c.members...))); err == nil {
				t.Fatal("accepted unsafe/incomplete archive")
			}
		})
	}
	for _, tool := range []string{"gh", "agy", "rg", "fd"} {
		t.Run(tool, func(t *testing.T) {
			exe := tool
			if tool == "agy" {
				exe = "antigravity"
			}
			for _, members := range [][]member{{{"unrelated", "x", 0644, 0}}, {{"a/" + exe, "one", 0755, 0}, {"b/" + exe, "two", 0755, 0}}, {{"a/" + exe, "cli", 0755, 0}, {"../unselected", "x", 0644, 0}}, {{"a/" + exe, "cli", 0755, 0}, {"unselected", "", 0644, tar.TypeSymlink}}} {
				root, _ := openRoot(t)
				if err := Extract(root, tool, testPackages(t)[tool], bytes.NewReader(archive(t, members...))); err == nil {
					t.Fatal("accepted missing/ambiguous/unsafe archive")
				}
			}
		})
	}
	root, _ := openRoot(t)
	data := archive(t, member{"bin/codex", "cli", 0755, 0})
	if err := Extract(root, "codex", testPackages(t)["codex"], bytes.NewReader(data[:len(data)-6])); err == nil {
		t.Fatal("accepted truncated gzip trailer")
	}
}
func TestDestinationLinksCannotEscape(t *testing.T) {
	for _, leaf := range []bool{false, true} {
		t.Run(fmt.Sprint(leaf), func(t *testing.T) {
			root, dir := openRoot(t)
			outside := t.TempDir()
			target := filepath.Join(dir, "bin")
			if leaf {
				if err := os.Mkdir(target, 0755); err != nil {
					t.Fatal(err)
				}
				target = filepath.Join(target, "codex")
			}
			if err := os.Symlink(outside, target); err != nil {
				t.Fatal(err)
			}
			if err := Extract(root, "codex", testPackages(t)["codex"], bytes.NewReader(archive(t, member{"bin/codex", "cli", 0755, 0}))); err == nil {
				t.Fatal("accepted destination symlink")
			}
			entries, _ := os.ReadDir(outside)
			if len(entries) != 0 {
				t.Fatal("wrote outside root")
			}
		})
	}
}

func sqliteArchive(t *testing.T, members ...member) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, m := range members {
		h := &zip.FileHeader{Name: m.name, Method: zip.Deflate}
		mode := os.FileMode(m.mode)
		if m.kind == tar.TypeSymlink {
			mode |= os.ModeSymlink
		}
		h.SetMode(mode)
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, m.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSQLiteSourceVerifiedExtraction(t *testing.T) {
	payloads, tools, server := fixture(t)
	root, sourceDir := openRoot(t)
	if err := PrepareSQLiteSource(context.Background(), server.Client(), tools["sqlite3"], testPackages(t)["sqlite3"], sourceDir); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"sqlite3.c": "sqlite source", "shell.c": "shell source", "sqlite3.h": "sqlite header", "sqlite3ext.h": "extension header"} {
		f, err := OpenFile(root, name)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(f)
		f.Close()
		if err != nil || string(got) != want {
			t.Fatalf("%s=%q err=%v", name, got, err)
		}
	}
	// ZIP parsing must never happen before the source checksum has been verified.
	payloads["sqlite3"] = []byte("unverified invalid zip")
	root, _ = openRoot(t)
	if err := install(context.Background(), server.Client(), root, "sqlite3", testPackages(t)["sqlite3"], tools["sqlite3"]); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSQLiteSourceSafety(t *testing.T) {
	good := []member{{"sqlite/sqlite3.c", "source", 0644, 0}, {"sqlite/shell.c", "shell", 0644, 0}, {"sqlite/sqlite3.h", "header", 0644, 0}, {"sqlite/sqlite3ext.h", "header", 0644, 0}}
	for _, test := range []struct {
		name  string
		extra member
	}{
		{"parent", member{"../escape", "x", 0644, 0}},
		{"absolute", member{"/escape", "x", 0644, 0}},
		{"symlink", member{"unselected", "x", 0644, tar.TypeSymlink}},
		{"duplicate path", good[0]},
		{"duplicate basename", member{"other/sqlite3.c", "x", 0644, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := sqliteArchive(t, append(good, test.extra)...)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payload) }))
			defer server.Close()
			sum := sha256.Sum256(payload)
			root, _ := openRoot(t)
			if err := install(context.Background(), server.Client(), root, "sqlite3", testPackages(t)["sqlite3"], Spec{URL: server.URL, SHA256: fmt.Sprintf("%x", sum)}); err == nil {
				t.Fatal("accepted unsafe SQLite source archive")
			}
		})
	}
}

func TestBuildRequiresCompiledSQLite(t *testing.T) {
	_, tools, server := fixture(t)
	for _, test := range []struct {
		name string
		mode os.FileMode
		data string
	}{
		{"missing", 0, ""}, {"nonexecutable", 0644, "binary"}, {"empty", 0755, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, dst := openRoot(t)
			if test.mode != 0 {
				if err := WriteFile(root, "bin/sqlite3", strings.NewReader(test.data), test.mode); err != nil {
					t.Fatal(err)
				}
			}
			if err := Build(context.Background(), server.Client(), tools, testPackages(t), os.DirFS("../.."), "arm64", dst, io.Discard); err == nil {
				t.Fatal("accepted absent or unusable compiled SQLite")
			}
			if _, err := root.Stat("manifest.json"); !os.IsNotExist(err) {
				t.Fatalf("manifest published before SQLite compilation: %v", err)
			}
		})
	}
}

func TestAstGrepVerifiedZIP(t *testing.T) {
	_, tools, server := fixture(t)
	root, _ := openRoot(t)
	if err := install(context.Background(), server.Client(), root, "ast-grep", testPackages(t)["ast-grep"], tools["ast-grep"]); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Stat("bin/sg"); !os.IsNotExist(err) {
		t.Fatalf("unexpected sg alias: %v", err)
	}
	for _, members := range [][]member{
		{{"sg", "missing ast-grep", 0755, 0}},
		{{"ast-grep", "binary", 0755, 0}, {"../unselected", "bad", 0644, 0}},
		{{"ast-grep", "binary", 0755, 0}, {"unselected", "link", 0644, tar.TypeSymlink}},
		{{"ast-grep", "binary", 0755, 0}, {"nested/ast-grep", "ambiguous", 0755, 0}},
		{{"ast-grep", "not executable", 0644, 0}},
	} {
		payload := sqliteArchive(t, members...)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payload) }))
		sum := sha256.Sum256(payload)
		root, _ := openRoot(t)
		err := install(context.Background(), server.Client(), root, "ast-grep", testPackages(t)["ast-grep"], Spec{URL: server.URL, SHA256: fmt.Sprintf("%x", sum)})
		server.Close()
		if err == nil {
			t.Fatal("accepted missing/unsafe/ambiguous/nonexecutable ast-grep archive")
		}
	}
}

func TestUVCompanionSafety(t *testing.T) {
	root, _ := openRoot(t)
	if err := Extract(root, "uv", testPackages(t)["uv"], bytes.NewReader(archive(t, member{"uv/uv", "uv", 0755, 0}, member{"uv/uvx", "uvx", 0755, 0}))); err != nil {
		t.Fatal(err)
	}
	for _, members := range [][]member{
		{{"uv/uv", "missing uvx", 0755, 0}},
		{{"uv/uv", "uv", 0755, 0}, {"uv/uvx", "not executable", 0644, 0}},
		{{"uv/uv", "uv", 0755, 0}, {"uv/uvx", "uvx", 0755, 0}, {"other/uvx", "ambiguous", 0755, 0}},
		{{"uv/uv", "uv", 0755, 0}, {"uv/uvx", "uvx", 0755, 0}, {"unselected", "link", 0644, tar.TypeSymlink}},
	} {
		root, _ := openRoot(t)
		if err := Extract(root, "uv", testPackages(t)["uv"], bytes.NewReader(archive(t, members...))); err == nil {
			t.Fatal("accepted missing/unsafe/ambiguous/nonexecutable uv companion")
		}
	}
}

func TestDeclarativeTarRecipe(t *testing.T) {
	root, dir := openRoot(t)
	p := toolconfig.Package{Install: toolconfig.Install{Kind: "tar.gz", Members: map[string]string{"vendor-bin": "bin/custom"}, Aliases: map[string]string{"bin/alias": "bin/custom"}}}
	if err := Extract(root, "custom", p, bytes.NewReader(archive(t, member{"release/vendor-bin", "custom binary", 0755, 0}))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "bin/alias"))
	if err != nil || string(got) != "custom binary" {
		t.Fatalf("alias %q %v", got, err)
	}
}

func testPackages(t *testing.T) map[string]toolconfig.Package {
	t.Helper()
	c, err := toolconfig.Load("../..")
	if err != nil {
		t.Fatal(err)
	}
	return c.Catalog.Packages
}
