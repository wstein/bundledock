package toolbundle

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func standaloneArchive(t *testing.T, link, target string, kind byte, extra bool) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	prefix := "code-server-4.141.0-linux-amd64/"
	for _, m := range []struct {
		name, data string
		mode       int64
	}{{"bin/code-server", "launcher", 0755}, {"lib/node", "node", 0755}, {"node_modules/semver/bin/semver.js", "script", 0755}, {"LICENSE", "MIT", 0644}} {
		if err := tw.WriteHeader(&tar.Header{Name: prefix + m.name, Mode: m.mode, Size: int64(len(m.data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(m.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.WriteHeader(&tar.Header{Name: prefix + link, Linkname: target, Typeflag: kind, Mode: 0777}); err != nil {
		t.Fatal(err)
	}
	if extra {
		if err := tw.WriteHeader(&tar.Header{Name: prefix + link + "/overwrite", Size: 1, Mode: 0644}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte("x"))
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func standalonePackage() toolconfig.Package {
	return toolconfig.Package{Install: toolconfig.Install{Kind: "tar.gz", Preserve: true, Profile: "code-server-standalone", Members: map[string]string{"bin/code-server": "payload/bin/code-server"}}}
}
func TestStandalonePreservesLauncherNodeAndRelativeLink(t *testing.T) {
	r, dir := openRoot(t)
	data := standaloneArchive(t, "node_modules/.bin/semver", "../semver/bin/semver.js", tar.TypeSymlink, false)
	if err := Extract(r, "code-server", standalonePackage(), bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bin/code-server", "lib/node", "LICENSE"} {
		if _, err := os.Stat(filepath.Join(dir, "payload", name)); err != nil {
			t.Fatal(err)
		}
	}
	target, err := os.Readlink(filepath.Join(dir, "payload/node_modules/.bin/semver"))
	if err != nil || target != "../semver/bin/semver.js" {
		t.Fatalf("link %q: %v", target, err)
	}
}
func TestStandaloneRejectsUnsafeLinkGraph(t *testing.T) {
	for _, tc := range []struct {
		name, link, target string
		kind               byte
		extra              bool
	}{
		{"absolute", "node_modules/.bin/semver", "/etc/passwd", tar.TypeSymlink, false},
		{"escape", "node_modules/.bin/semver", "../../../outside", tar.TypeSymlink, false},
		{"dangling", "node_modules/.bin/semver", "../missing", tar.TypeSymlink, false},
		{"directory", "node_modules/.bin/semver", "../semver", tar.TypeSymlink, false},
		{"self", "node_modules/.bin/semver", "semver", tar.TypeSymlink, false},
		{"hardlink", "node_modules/.bin/semver", "../semver/bin/semver.js", tar.TypeLink, false},
		{"replacement", "lib/node", "../node_modules/semver/bin/semver.js", tar.TypeSymlink, false},
		{"write through link", "node_modules/.bin/semver", "../semver/bin/semver.js", tar.TypeSymlink, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := openRoot(t)
			if err := Extract(r, "code-server", standalonePackage(), bytes.NewReader(standaloneArchive(t, tc.link, tc.target, tc.kind, tc.extra))); err == nil {
				t.Fatal("accepted unsafe graph")
			}
		})
	}
}

func TestStandaloneActualTwelveLinksAllowForwardReferences(t *testing.T) {
	links := map[string]string{
		"node-gyp-build": "../node-gyp-build/bin.js", "semver": "../semver/bin/semver.js", "cross-env-shell": "../cross-env/dist/bin/cross-env-shell.js", "node-which": "../which/bin/node-which", "node-gyp-build-test": "../node-gyp-build/build-test.js", "js-yaml": "../js-yaml/bin/js-yaml.mjs", "escodegen": "../escodegen/bin/escodegen.js", "cross-env": "../cross-env/dist/bin/cross-env.js", "esvalidate": "../esprima/bin/esvalidate.js", "esparse": "../esprima/bin/esparse.js", "node-gyp-build-optional": "../node-gyp-build/optional.js", "esgenerate": "../escodegen/bin/esgenerate.js",
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			var b bytes.Buffer
			gz := gzip.NewWriter(&b)
			tw := tar.NewWriter(gz)
			prefix := "code-server-4.141.0-linux-" + arch + "/"
			// Both verified official archives contain these twelve .bin links. Emit links
			// first here so the fixture also proves safe forward-reference handling.
			for name, target := range links {
				if err := tw.WriteHeader(&tar.Header{Name: prefix + "node_modules/.bin/" + name, Typeflag: tar.TypeSymlink, Linkname: target, Mode: 0777}); err != nil {
					t.Fatal(err)
				}
			}
			files := map[string]string{"bin/code-server": "launcher", "lib/node": "node", "ThirdPartyNotices.txt": "third parties"}
			for _, target := range links {
				files["node_modules/"+strings.TrimPrefix(target, "../")] = "script"
			}
			for name, data := range files {
				if err := tw.WriteHeader(&tar.Header{Name: prefix + name, Mode: 0755, Size: int64(len(data))}); err != nil {
					t.Fatal(err)
				}
				if _, err := tw.Write([]byte(data)); err != nil {
					t.Fatal(err)
				}
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			r, dir := openRoot(t)
			if err := Extract(r, "code-server", standalonePackage(), bytes.NewReader(b.Bytes())); err != nil {
				t.Fatal(err)
			}
			for name, target := range links {
				got, err := os.Readlink(filepath.Join(dir, "payload/node_modules/.bin", name))
				if err != nil || got != target {
					t.Fatalf("%s: %q %v", name, got, err)
				}
			}
			spec := Spec{Version: "v4.141.0", URL: "https://github.com/coder/code-server/releases/download/v4.141.0/code-server-4.141.0-linux-" + arch + ".tar.gz", SHA256: strings.Repeat("a", 64), Verification: "publisher-sha256"}
			if err := WriteStandaloneManifest(r, arch, map[string]Spec{"code-server": spec}); err != nil {
				t.Fatal(err)
			}
			got, err := BundleLinks(r, StandaloneProfile)
			if err != nil || len(got) != 12 {
				t.Fatalf("manifest links %d: %v", len(got), err)
			}
		})
	}
}

func TestStandaloneRejectsWrongPinnedReleaseRoot(t *testing.T) {
	r, _ := openRoot(t)
	data := standaloneArchive(t, "node_modules/.bin/semver", "../semver/bin/semver.js", tar.TypeSymlink, false)
	if err := extractStandalone(r, bytes.NewReader(data), "code-server-4.141.0-linux-arm64"); err == nil {
		t.Fatal("accepted archive from another pinned architecture")
	}
}

func TestDefaultRecipeStillRejectsStandaloneLinks(t *testing.T) {
	r, _ := openRoot(t)
	p := standalonePackage()
	p.Install.Profile = ""
	data := standaloneArchive(t, "node_modules/.bin/semver", "../semver/bin/semver.js", tar.TypeSymlink, false)
	if err := Extract(r, "code-server", p, bytes.NewReader(data)); err == nil {
		t.Fatal("default extraction accepted links")
	}
}
