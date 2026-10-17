package toolbundle

import (
	"archive/tar"
	"archive/zip"
	"bundledock/internal/shelllayer"
	"bundledock/internal/toolconfig"
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

	"github.com/ulikunitz/xz"
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
		"helix":    helixArchive(t, "helix-25.07.1-x86_64-linux", "", 0),
		"fzf":      archive(t, member{"fzf", "fzf executable", 0755, 0}),
		"bat":      archive(t, member{"bat-v0.26.1-x86_64-unknown-linux-musl/bat", "bat executable", 0755, 0}),
		"regctl":   []byte("regctl executable"),
		"mise":     []byte("mise executable"),
		"hugo":     archive(t, member{"hugo", "hugo executable", 0755, 0}, member{"LICENSE", "hugo license", 0644, 0}),
		"typos":    archive(t, member{"./typos", "typos executable", 0755, 0}, member{"./LICENSE-MIT", "typos mit", 0644, 0}, member{"./LICENSE-APACHE", "typos apache", 0644, 0}),
		"lychee":   archive(t, member{"lychee-x86_64-unknown-linux-musl/lychee", "lychee executable", 0755, 0}),
		"gitleaks": archive(t, member{"gitleaks", "gitleaks executable", 0755, 0}, member{"LICENSE", "gitleaks license", 0644, 0}),
		"trivy":    archive(t, member{"trivy", "trivy executable", 0755, 0}, member{"LICENSE", "trivy license", 0644, 0}),
		"bottom":   archive(t, member{"btm", "bottom executable", 0755, 0}),
		"btop":     archive(t, member{"./btop/bin/btop", "btop executable", 0755, 0}, member{"./btop/LICENSE", "btop license", 0644, 0}),
		"syft":     archive(t, member{"syft", "syft executable", 0755, 0}, member{"LICENSE", "syft license", 0644, 0}),
		"nu":       archive(t, member{"nu-0.116.1/nu", "nu executable", 0755, 0}),
		"tmux":     archive(t, member{"tmux", "tmux executable", 0755, 0}),
		"fish":     xzArchive(t, member{"fish", "fish executable", 0755, 0}),
		"starship": archive(t, member{"starship", "starship executable", 0755, 0}),
		"uv":       archive(t, member{"uv-1/uv", "uv executable", 0755, 0}, member{"uv-1/uvx", "uvx executable", 0755, 0}),
		"ast-grep": zipArchive(t, member{"ast-grep", "ast-grep executable", 0755, 0}, member{"sg", "unwanted alias", 0755, 0}),
		"claude":   []byte("claude executable"),
		"herdr":    []byte("herdr executable"),
		"yq":       []byte("yq executable"),
		"jq":       []byte("jq executable"),
		"rg":       archive(t, member{"ripgrep-1/rg", "ripgrep executable", 0755, 0}, member{"ripgrep-1/LICENSE-MIT", "rg license", 0644, 0}, member{"ripgrep-1/UNLICENSE", "rg public domain", 0644, 0}, member{"ripgrep-1/COPYING", "rg copying", 0644, 0}),
		"fd":       archive(t, member{"fd-1/fd", "fd executable", 0755, 0}, member{"fd-1/LICENSE-MIT", "fd mit", 0644, 0}, member{"fd-1/LICENSE-APACHE", "fd apache", 0644, 0}),
		"gh":       archive(t, member{"gh_1/bin/gh", "github cli", 0755, 0}, member{"gh_1/share/man/gh.1", "manual", 0644, 0}),
		"agy":      archive(t, member{"bundle/antigravity", "antigravity cli", 0755, 0}),
		"codex":    archive(t, member{"bin/codex", "codex cli", 0755, 0}, member{"codex-package.json", `{"entrypoint":"bin/codex"}`, 0644, 0}, member{"bin/codex-code-mode-host", "host", 0755, 0}, member{"codex-path/rg", "rg", 0755, 0}, member{"codex-resources/bwrap", "bwrap", 0755, 0}, member{"codex-resources/zsh/bin/zsh", "zsh", 0755, 0}, member{"codex-resources/voice/", "", 0755, tar.TypeDir}, member{"./codex-resources/voice/lib/example.so", "shared library", 0644, 0}, member{"codex-resources/voice-other/keep", "other resource", 0644, 0}, member{"empty/", "", 0755, tar.TypeDir}),
	}
	for _, name := range []string{"shellcheck", "shfmt", "just", "ruff", "delta", "watchexec", "hyperfine", "age", "leaf", "jj", "lazygit", "git-lfs", "navi", "typst", "trufflehog", "lsd", "actionlint", "hadolint", "pinact"} {
		p := testPackages(t)[name]
		var files []member
		for executable := range p.Install.Members {
			files = append(files, member{"upstream/" + executable, executable + " executable", 0755, 0})
		}
		for _, notice := range p.Install.ArchiveNotices {
			files = append(files, member{"upstream/" + notice, name + " license", 0644, 0})
		}
		switch p.Install.Kind {
		case "raw":
			payloads[name] = []byte(name + " executable")
		case "tar.xz":
			payloads[name] = xzArchive(t, files...)
		default:
			payloads[name] = archive(t, files...)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasPrefix(key, "helix-25.07.1-") {
			key = "helix"
		}
		if data, ok := payloads[key]; ok {
			w.Write(data)
		} else {
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	tools := map[string]Spec{}
	for name, data := range payloads {
		s := Spec{Version: "fixture", URL: server.URL + "/" + name}
		if name == "helix" {
			s.Version = "25.07.1"
			s.URL = server.URL + "/helix-25.07.1-x86_64-linux.tar.xz"
		}
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
				root.Close()
				if err := Build(context.Background(), server.Client(), specs, testPackages(t), os.DirFS("../../bundles/toolbox"), arch, dst, io.Discard, WithShellLayer(os.DirFS("../shelllayer"))); err != nil {
					t.Fatal(err)
				}
			}
			want := map[string]struct {
				data string
				mode os.FileMode
			}{
				"bin/hugo": {"hugo executable", 0755}, "licenses/hugo/LICENSE": {"hugo license", 0644},
				"bin/btm": {"bottom executable", 0755}, "bin/btop": {"btop executable", 0755},
				"licenses/btop/LICENSE": {"btop license", 0644},
				"bin/nu":                {"nu executable", 0755}, "bin/tmux": {"tmux executable", 0755},
				"bin/fish": {"fish executable", 0755}, "bin/starship": {"starship executable", 0755},
				"bin/uv": {"uv executable", 0755}, "bin/uvx": {"uvx executable", 0755},
				"bin/ast-grep": {"ast-grep executable", 0755},
				"bin/herdr":    {"herdr executable", 0755},
				"bin/mise":     {"mise executable", 0755},
				"bin/yq":       {"yq executable", 0755},
				"bin/jq":       {"jq executable", 0755}, "bin/rg": {"ripgrep executable", 0755}, "bin/fd": {"fd executable", 0755}, "bin/fdfind": {"fd executable", 0755},
				"licenses/rg/LICENSE-MIT": {"rg license", 0644}, "licenses/rg/UNLICENSE": {"rg public domain", 0644}, "licenses/rg/COPYING": {"rg copying", 0644},
				"licenses/herdr/LICENSE":  {herdrLicense(t), 0644},
				"licenses/herdr/SOURCE":   {"Upstream: https://github.com/herdrdev/herdr\nLicense: Apache-2.0\nPinned release: v0.9.3\nLicense source: https://raw.githubusercontent.com/herdrdev/herdr/v0.9.3/LICENSE\n", 0644},
				"licenses/fd/LICENSE-MIT": {"fd mit", 0644}, "licenses/fd/LICENSE-APACHE": {"fd apache", 0644},
				"bin/claude": {"claude executable", 0755}, "bin/gh": {"github cli", 0755}, "bin/agy": {"antigravity cli", 0755}, "bin/codex": {"codex cli", 0755},
				"bin/codex-code-mode-host": {"host", 0755}, "codex-path/rg": {"rg", 0755}, "codex-resources/bwrap": {"bwrap", 0755}, "codex-resources/zsh/bin/zsh": {"zsh", 0755}, "codex-resources/voice-other/keep": {"other resource", 0644}, "codex-package.json": {`{"entrypoint":"bin/codex"}`, 0644},
			}

			for _, name := range []string{"typos", "lychee", "gitleaks", "trivy", "syft"} {
				want["bin/"+name] = struct {
					data string
					mode os.FileMode
				}{name + " executable", 0755}
			}
			for file, content := range map[string]string{"typos/LICENSE-MIT": "typos mit", "typos/LICENSE-APACHE": "typos apache", "gitleaks/LICENSE": "gitleaks license", "trivy/LICENSE": "trivy license", "syft/LICENSE": "syft license"} {
				want["licenses/"+file] = struct {
					data string
					mode os.FileMode
				}{content, 0644}
			}
			for _, file := range []string{"FMT-LICENSE.rst", "WIDECHAR-LICENSE", "UNICODE-LICENSE", "SOURCE"} {
				content, err := os.ReadFile("../../bundles/toolbox/licenses/btop/" + file)
				if err != nil {
					t.Fatal(err)
				}
				want["licenses/btop/"+file] = struct {
					data string
					mode os.FileMode
				}{string(content), 0644}
			}
			for _, file := range []string{"LICENSE", "SOURCE"} {
				content, err := os.ReadFile("../../bundles/toolbox/licenses/bottom/" + file)
				if err != nil {
					t.Fatal(err)
				}
				want["licenses/bottom/"+file] = struct {
					data string
					mode os.FileMode
				}{string(content), 0644}
			}
			for _, file := range []string{"LICENSE-MIT", "LICENSE-APACHE"} {
				content, err := os.ReadFile("../../bundles/toolbox/licenses/lychee/" + file)
				if err != nil {
					t.Fatal(err)
				}
				want["licenses/lychee/"+file] = struct {
					data string
					mode os.FileMode
				}{string(content), 0644}
			}
			layer, err := shelllayer.Files()
			if err != nil {
				t.Fatal(err)
			}
			for file, content := range layer {
				want[file] = struct {
					data string
					mode os.FileMode
				}{string(content), 0644}
			}
			for name, w := range want {
				p := filepath.Join(first, name)
				b, err := readInstalledFile(t, first, name)
				if err != nil || string(b) != w.data {
					t.Fatalf("%s data=%q err=%v", name, b, err)
				}
				if strings.HasPrefix(name, "licenses/") {
					continue
				}
				info, err := os.Stat(p)
				if err != nil || info.Mode().Perm() != w.mode {
					t.Fatalf("%s mode=%v err=%v", name, info, err)
				}
			}
			index, err := os.ReadFile(filepath.Join(first, "licenses.index.json"))
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Count(string(index), `"package": "shell-layer"`) + strings.Count(string(index), `"package":"shell-layer"`); got != 2 {
				t.Fatalf("license index lists %d shell-layer notices, want 2", got)
			}
			if _, err := os.Stat(filepath.Join(first, "codex-resources/voice")); !os.IsNotExist(err) {
				t.Fatalf("optional Codex voice resources installed: %v", err)
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
			if !sort.StringsAreSorted(names) || !strings.Contains(string(sums), "  manifest.json\n") || len(names) < 4 {
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

func zipArchive(t *testing.T, members ...member) []byte {
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
		payload := zipArchive(t, members...)
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
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	// Generic extraction fixtures retain every provider, independent of toolbox membership.
	for _, name := range []string{"codex", "agy", "claude"} {
		standalone, err := toolconfig.Load("../../bundles/" + name)
		if err != nil {
			t.Fatal(err)
		}
		p := standalone.Catalog.Packages[name]
		p.Notices = nil // Dedicated carrier tests verify their own source notices.
		c.Catalog.Packages[name] = p
	}
	return c.Catalog.Packages
}

func herdrLicense(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../bundles/toolbox/licenses/herdr/LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestExtractTarXZ(t *testing.T) {
	data, err := os.ReadFile("testdata/fish.tar.xz")
	if err != nil {
		t.Fatal(err)
	}
	p := toolconfig.Package{Install: toolconfig.Install{Kind: "tar.xz", Members: map[string]string{"fish": "bin/fish"}}}
	root, dir := openRoot(t)
	if err := Extract(root, "fish", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "bin/fish"))
	if err != nil || string(got) != "fish executable" {
		t.Fatalf("data=%q err=%v", got, err)
	}
}

func xzArchive(t *testing.T, members ...member) []byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(archive(t, members...)))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	var data bytes.Buffer
	writer, err := xz.NewWriter(&data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(writer, gz); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestTarXZSafety(t *testing.T) {
	p := toolconfig.Package{Install: toolconfig.Install{Kind: "tar.xz", Members: map[string]string{"fish": "bin/fish"}}}
	for _, members := range [][]member{
		{{"../escape", "bad", 0755, 0}}, {{"/escape", "bad", 0755, 0}},
		{{"fish", "", 0755, tar.TypeSymlink}}, {{"fish", "", 0755, tar.TypeLink}},
		{{"fish", "", 0755, tar.TypeChar}}, {{"fish", "", 0755, tar.TypeFifo}},
		{{"fish", "bad", 0644, 0}}, {{"other", "bad", 0755, 0}},
		{{"a/fish", "one", 0755, 0}, {"b/fish", "two", 0755, 0}},
		{{"fish", "one", 0755, 0}, {"fish", "two", 0755, 0}},
		{{"fish", "one", 0755, 0}, {"../unselected", "bad", 0644, 0}},
	} {
		root, _ := openRoot(t)
		if err := Extract(root, "fish", p, bytes.NewReader(xzArchive(t, members...))); err == nil {
			t.Fatal("accepted unsafe or incomplete XZ archive")
		}
	}
	good := xzArchive(t, member{"fish", "one", 0755, 0})
	corrupt := bytes.Clone(good)
	corrupt[len(corrupt)-8] ^= 1
	for _, data := range [][]byte{good[:len(good)-6], corrupt, []byte("not XZ")} {
		root, _ := openRoot(t)
		if err := Extract(root, "fish", p, bytes.NewReader(data)); err == nil {
			t.Fatal("accepted malformed/truncated XZ")
		}
	}
	root, _ := openRoot(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(good) }))
	defer server.Close()
	if err := install(context.Background(), server.Client(), root, "fish", p, Spec{URL: server.URL, SHA256: strings.Repeat("0", 64)}); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unverified extraction: %v", err)
	}
	if _, err := root.Stat("bin/fish"); !os.IsNotExist(err) {
		t.Fatal("installed unverified XZ")
	}
}

func TestTarXZMemberLimit(t *testing.T) {
	p := toolconfig.Package{Install: toolconfig.Install{Kind: "tar.xz", Members: map[string]string{"fish": "bin/fish"}}}
	members := []member{{"fish", "one", 0755, 0}}
	for i := 0; i < 4096; i++ {
		members = append(members, member{fmt.Sprintf("dir-%d/", i), "", 0755, tar.TypeDir})
	}
	root, _ := openRoot(t)
	if err := Extract(root, "fish", p, bytes.NewReader(xzArchive(t, members...))); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("member limit: %v", err)
	}
}

func TestExcludedArchiveEntriesStillValidated(t *testing.T) {
	for _, kind := range []string{"tar.gz", "tar.xz"} {
		t.Run(kind, func(t *testing.T) {
			p := testPackages(t)["codex"]
			p.Install.Kind = kind
			makeArchive := archive
			if kind == "tar.xz" {
				makeArchive = xzArchive
			}
			for _, bad := range []member{
				{"codex-resources/voice/../escape", "bad", 0644, 0},
				{"codex-resources/voice/absolute", "", 0644, tar.TypeSymlink},
				{"codex-resources/voice/hardlink", "", 0644, tar.TypeLink},
				{"codex-resources/voice/device", "", 0644, tar.TypeChar},
				{"codex-resources/voice/fifo", "", 0644, tar.TypeFifo},
			} {
				t.Run(bad.name, func(t *testing.T) {
					root, _ := openRoot(t)
					data := makeArchive(t, member{"bin/codex", "cli", 0755, 0}, bad)
					if err := Extract(root, "codex", p, bytes.NewReader(data)); err == nil {
						t.Fatal("accepted unsafe excluded entry")
					}
				})
			}
			t.Run("duplicate", func(t *testing.T) {
				root, _ := openRoot(t)
				data := makeArchive(t, member{"bin/codex", "cli", 0755, 0}, member{"codex-resources/voice/lib.so", "one", 0644, 0}, member{"./codex-resources/voice/lib.so", "two", 0644, 0})
				if err := Extract(root, "codex", p, bytes.NewReader(data)); err == nil {
					t.Fatal("accepted duplicate excluded entry")
				}
			})
			for _, damage := range []string{"truncated payload", "truncated trailer", "corrupt trailer"} {
				t.Run(damage, func(t *testing.T) {
					data := makeArchive(t, member{"bin/codex", "cli", 0755, 0}, member{"codex-resources/voice/lib.so", "library", 0644, 0})
					if damage == "truncated payload" {
						var raw bytes.Buffer
						tw := tar.NewWriter(&raw)
						if err := tw.WriteHeader(&tar.Header{Name: "bin/codex", Size: 3, Mode: 0755}); err != nil {
							t.Fatal(err)
						}
						if _, err := io.WriteString(tw, "cli"); err != nil {
							t.Fatal(err)
						}
						if err := tw.WriteHeader(&tar.Header{Name: "codex-resources/voice/lib.so", Size: 100, Mode: 0644}); err != nil {
							t.Fatal(err)
						}
						if _, err := io.WriteString(tw, "partial"); err != nil {
							t.Fatal(err)
						}
						// Keep compression valid while truncating the excluded tar member.
						var compressed bytes.Buffer
						var writer io.WriteCloser = gzip.NewWriter(&compressed)
						if kind == "tar.xz" {
							var err error
							writer, err = xz.NewWriter(&compressed)
							if err != nil {
								t.Fatal(err)
							}
						}
						if _, err := writer.Write(raw.Bytes()); err != nil {
							t.Fatal(err)
						}
						if err := writer.Close(); err != nil {
							t.Fatal(err)
						}
						data = compressed.Bytes()
					} else if damage == "truncated trailer" {
						data = data[:len(data)-6]
					} else {
						data[len(data)-8] ^= 1
					}
					root, _ := openRoot(t)
					if err := Extract(root, "codex", p, bytes.NewReader(data)); err == nil {
						t.Fatal("accepted damaged excluded archive")
					}
				})
			}
		})
	}
}

func TestExcludedContentsStillRequirePinnedChecksum(t *testing.T) {
	original := archive(t, member{"bin/codex", "cli", 0755, 0}, member{"codex-resources/voice/lib.so", "original", 0644, 0})
	changed := archive(t, member{"bin/codex", "cli", 0755, 0}, member{"codex-resources/voice/lib.so", "changed", 0644, 0})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(changed) }))
	defer server.Close()
	root, _ := openRoot(t)
	err := install(context.Background(), server.Client(), root, "codex", testPackages(t)["codex"], Spec{URL: server.URL, SHA256: fmt.Sprintf("%x", sha256.Sum256(original))})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected original archive checksum rejection, got %v", err)
	}
	if _, err := root.Stat("bin/codex"); !os.IsNotExist(err) {
		t.Fatalf("decoded before checksum verification: %v", err)
	}
}
