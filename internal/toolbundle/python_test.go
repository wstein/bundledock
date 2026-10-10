package toolbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type pythonMember struct {
	name, data, link string
	mode             int64
	kind             byte
}

func pythonArchive(t *testing.T, members ...pythonMember) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		kind := m.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		h := &tar.Header{Name: m.name, Linkname: m.link, Mode: m.mode, Typeflag: kind}
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
	return b.Bytes()
}
func TestPythonFullTreeAndBoundedAliases(t *testing.T) {
	root, dst := openRoot(t)
	payload := pythonArchive(t,
		pythonMember{name: "python/", kind: tar.TypeDir, mode: 0777},
		pythonMember{name: "python/bin/python3.15", data: "interpreter", mode: 04755},
		pythonMember{name: "python/bin/python3", link: "python3.15", kind: tar.TypeSymlink},
		pythonMember{name: "python/bin/python", link: "python3", kind: tar.TypeSymlink},
		pythonMember{name: "python/lib/python3.15/os.py", data: "stdlib", mode: 0666},
		pythonMember{name: "python/lib/python3.15/lib-dynload/_ssl.so", data: "native", mode: 0755},
		pythonMember{name: "python/lib/libpython3.15.so.1.0", data: "library", mode: 0755},
		pythonMember{name: "python/lib/libpython3.15.so", link: "python/lib/libpython3.15.so.1.0", kind: tar.TypeLink},
		pythonMember{name: "python/licenses/LICENSE.txt", data: "license", mode: 0644},
		pythonMember{name: "python/share/man/man1/python3.15.1", data: "manual", mode: 0644},
		pythonMember{name: "python/share/man/man1/python3.1", link: "python3.15.1", kind: tar.TypeSymlink},
	)
	if err := Extract(root, "python3", bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"bin/python3.15": "interpreter", "bin/python3": "interpreter", "bin/python": "interpreter",
		"lib/python3.15/os.py": "stdlib", "lib/python3.15/lib-dynload/_ssl.so": "native",
		"lib/libpython3.15.so": "library", "lib/libpython3.15.so.1.0": "library",
		"licenses/LICENSE.txt": "license", "share/man/man1/python3.1": "manual",
	} {
		p := filepath.Join(dst, name)
		data, err := os.ReadFile(p)
		if err != nil || string(data) != want {
			t.Fatalf("%s: %q %v", name, data, err)
		}
		info, err := os.Lstat(p)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("%s not regular: %v %v", name, info, err)
		}
		mode := os.FileMode(0644)
		if name == "bin/python" || name == "bin/python3" || name == "bin/python3.15" || name == "lib/libpython3.15.so" || name == "lib/libpython3.15.so.1.0" || name == "lib/python3.15/lib-dynload/_ssl.so" {
			mode = 0755
		}
		if info.Mode().Perm() != mode || info.Mode()&os.ModeSetuid != 0 {
			t.Fatalf("%s unsafe mode %v", name, info.Mode())
		}
	}
}

func TestPythonRejectsUnsafeNamespaceAndLinks(t *testing.T) {
	good := pythonMember{name: "python/bin/python3", data: "python", mode: 0755}
	cases := map[string][]pythonMember{
		"absolute name":      {{name: "/python/escape", data: "x"}},
		"traversal name":     {{name: "python/bin/../escape", data: "x"}},
		"foreign prefix":     {{name: "other/file", data: "x"}},
		"duplicate":          {good},
		"absolute link":      {{name: "python/a", link: "/etc/passwd", kind: tar.TypeSymlink}},
		"escaping symlink":   {{name: "python/a", link: "../outside", kind: tar.TypeSymlink}},
		"escape and reenter": {{name: "python/a", link: "../python/bin/python3", kind: tar.TypeSymlink}},
		"escaping hardlink":  {{name: "python/a", link: "outside", kind: tar.TypeLink}},
		"missing target":     {{name: "python/a", link: "missing", kind: tar.TypeSymlink}},
		"cycle":              {{name: "python/a", link: "b", kind: tar.TypeSymlink}, {name: "python/b", link: "a", kind: tar.TypeSymlink}},
		"directory target":   {{name: "python/d", kind: tar.TypeDir}, {name: "python/a", link: "d", kind: tar.TypeSymlink}},
		"link parent":        {{name: "python/d", link: "bin/python3", kind: tar.TypeSymlink}, {name: "python/d/file", data: "x"}},
		"regular parent":     {{name: "python/d", data: "x"}, {name: "python/d/file", data: "x"}},
		"device":             {{name: "python/device", kind: tar.TypeChar}},
		"fifo":               {{name: "python/fifo", kind: tar.TypeFifo}},
		"manifest override":  {{name: "python/manifest.json", data: "x"}},
		"checksums override": {{name: "python/SHA256SUMS", data: "x"}},
		"ready override":     {{name: "python/.ready", data: "x"}},
		"lock override":      {{name: "python/.initializing/file", data: "x"}},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			root, _ := openRoot(t)
			entries = append([]pythonMember{good}, entries...)
			if err := Extract(root, "python3", bytes.NewReader(pythonArchive(t, entries...))); err == nil {
				t.Fatal("accepted unsafe Python archive")
			}
			installed, err := root.Open(".")
			if err != nil {
				t.Fatal(err)
			}
			defer installed.Close()
			files, err := installed.ReadDir(-1)
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 0 {
				t.Fatalf("unsafe namespace wrote bundle contents: %v", files)
			}
		})
	}
	for name, entries := range map[string][]pythonMember{
		"missing interpreter":       {{name: "python/lib/os.py", data: "x"}},
		"nonexecutable interpreter": {{name: "python/bin/python3", data: "x", mode: 0644}},
	} {
		t.Run(name, func(t *testing.T) {
			root, _ := openRoot(t)
			if err := Extract(root, "python3", bytes.NewReader(pythonArchive(t, entries...))); err == nil {
				t.Fatal("accepted incomplete runtime")
			}
		})
	}
	root, _ := openRoot(t)
	payload := pythonArchive(t, good)
	if err := Extract(root, "python3", bytes.NewReader(payload[:len(payload)-6])); err == nil {
		t.Fatal("accepted truncated gzip")
	}
}
func TestPythonBoundedParentTargetsAndForwardHardlinks(t *testing.T) {
	root, _ := openRoot(t)
	payload := pythonArchive(t,
		pythonMember{name: "python/bin/python3", link: "python/bin/python3.15", kind: tar.TypeLink},
		pythonMember{name: "python/share/terminfo/x/xterm", link: "../v/vscode", kind: tar.TypeSymlink},
		pythonMember{name: "python/share/terminfo/v/vscode", data: "terminal", mode: 0644},
		pythonMember{name: "python/bin/python3.15", data: "python", mode: 0755},
	)
	if err := Extract(root, "python3", bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	f, err := root.Open("share/terminfo/x/xterm")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil || string(data) != "terminal" {
		t.Fatalf("bounded parent alias: %q %v", data, err)
	}
}

func TestPythonProfileBuildAndIsolation(t *testing.T) {
	payload := pythonArchive(t, pythonMember{name: "python/bin/python3", data: "python", mode: 0755}, pythonMember{name: "python/lib/python3.15/os.py", data: "stdlib", mode: 0644})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payload) }))
	defer server.Close()
	specs := map[string]Spec{"python3": {Version: "3.15.0+fixture", URL: server.URL, SHA256: fmt.Sprintf("%x", sha256.Sum256(payload))}}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			data, _ := json.Marshal(Lock{Schema: 1, Platforms: map[string]map[string]Spec{arch: specs}})
			if _, err := ReadLock(bytes.NewReader(data), arch); err == nil {
				t.Fatal("agent lock accepted Python-only profile")
			}
			tools, err := ReadPythonLock(bytes.NewReader(data), arch)
			if err != nil {
				t.Fatal(err)
			}
			first, second := t.TempDir(), t.TempDir()
			for _, dst := range []string{first, second} {
				if err := BuildPython(context.Background(), server.Client(), tools, arch, dst, io.Discard); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"manifest.json", "SHA256SUMS"} {
				a, err := os.ReadFile(filepath.Join(first, name))
				if err != nil {
					t.Fatal(err)
				}
				b, err := os.ReadFile(filepath.Join(second, name))
				if err != nil || !bytes.Equal(a, b) {
					t.Fatalf("nondeterministic %s %v", name, err)
				}
			}
			sums, _ := os.ReadFile(filepath.Join(first, "SHA256SUMS"))
			if len(strings.Split(strings.TrimSpace(string(sums)), "\n")) != 3 {
				t.Fatalf("incomplete checksums: %s", sums)
			}
			for _, line := range strings.Split(strings.TrimSpace(string(sums)), "\n") {
				data, err := os.ReadFile(filepath.Join(first, line[66:]))
				if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != line[:64] {
					t.Fatalf("bad checksum %s: %v", line, err)
				}
			}
			for _, name := range []string{"codex", "claude", "gh", "agy"} {
				if _, err := os.Stat(filepath.Join(first, "bin", name)); !os.IsNotExist(err) {
					t.Fatal("Python bundle contains agent", name)
				}
			}
		})
	}
	for name, mutate := range map[string]func(map[string]Spec){
		"wrong tool":   func(m map[string]Spec) { m["codex"] = m["python3"]; delete(m, "python3") },
		"extra tool":   func(m map[string]Spec) { m["gh"] = m["python3"] },
		"no version":   func(m map[string]Spec) { v := m["python3"]; v.Version = ""; m["python3"] = v },
		"invalid URL":  func(m map[string]Spec) { v := m["python3"]; v.URL = "file:///tmp/python"; m["python3"] = v },
		"bad checksum": func(m map[string]Spec) { v := m["python3"]; v.SHA256 = "invalid"; m["python3"] = v },
	} {
		t.Run(name, func(t *testing.T) {
			m := map[string]Spec{"python3": specs["python3"]}
			mutate(m)
			data, _ := json.Marshal(Lock{Schema: 1, Platforms: map[string]map[string]Spec{"amd64": m}})
			if _, err := ReadPythonLock(bytes.NewReader(data), "amd64"); err == nil {
				t.Fatal("accepted invalid Python profile")
			}
			if err := BuildPython(context.Background(), server.Client(), m, "amd64", t.TempDir(), io.Discard); err == nil {
				t.Fatal("build accepted invalid profile")
			}
		})
	}
	wrong := map[string]Spec{"python3": specs["python3"]}
	v := wrong["python3"]
	v.SHA256 = strings.Repeat("0", 64)
	wrong["python3"] = v
	dst := t.TempDir()
	if err := BuildPython(context.Background(), server.Client(), wrong, "amd64", dst, io.Discard); err == nil {
		t.Fatal("accepted checksum mismatch")
	}
	if entries, _ := os.ReadDir(dst); len(entries) != 0 {
		t.Fatal("installed unverified Python artifact")
	}
	agentLock, err := os.ReadFile("../../oci/tools.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPythonLock(bytes.NewReader(agentLock), "amd64"); err == nil {
		t.Fatal("Python profile accepted agent lock")
	}
}

func TestPinnedPythonLock(t *testing.T) {
	data, err := os.ReadFile("../../oci/python/python.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		tools, err := ReadPythonLock(bytes.NewReader(data), arch)
		if err != nil {
			t.Fatal(err)
		}
		if len(tools) != 1 || tools["python3"].Version != "3.15.0+20261009" {
			t.Fatalf("wrong Python pins %v", tools)
		}
		if _, err := ReadLock(bytes.NewReader(data), arch); err == nil {
			t.Fatal("agent profile accepted Python lock")
		}
	}
	for _, bad := range []string{"{", string(data) + " {}", strings.Replace(string(data), `"schema": 1`, `"unknown": 1`, 1)} {
		if _, err := ReadPythonLock(strings.NewReader(bad), "amd64"); err == nil {
			t.Fatal("accepted malformed Python lock")
		}
	}
	if _, err := ReadPythonLock(bytes.NewReader(data), "386"); err == nil {
		t.Fatal("accepted unsupported architecture")
	}
}
