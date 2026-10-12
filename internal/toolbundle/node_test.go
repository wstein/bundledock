package toolbundle

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func nodeFixture(t *testing.T, arch, link, target string, kind byte) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	prefix := "node-v24.21.0-linux-" + arch + "/"
	for n, d := range map[string]string{"bin/node": "node", "lib/node_modules/npm/bin/npm-cli.js": "npm", "lib/node_modules/npm/bin/npx-cli.js": "npx", "LICENSE": "notices", "include/node/node.h": "header"} {
		if err := tw.WriteHeader(&tar.Header{Name: prefix + n, Mode: 0755, Size: int64(len(d))}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(d))
	}
	for n, v := range map[string]string{"bin/npm": "../lib/node_modules/npm/bin/npm-cli.js", "bin/npx": "../lib/node_modules/npm/bin/npx-cli.js"} {
		if n == link {
			v = target
		}
		k := byte(tar.TypeSymlink)
		if n == link {
			k = kind
		}
		tw.WriteHeader(&tar.Header{Name: prefix + n, Typeflag: k, Linkname: v})
	}
	tw.Close()
	gz.Close()
	return b.Bytes()
}
func TestNodeFaithfulRuntimeAndLinks(t *testing.T) {
	for _, arch := range []string{"x64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			r, dir := openRoot(t)
			p := toolconfig.Package{Install: toolconfig.Install{Kind: "tar.gz", Profile: "node-runtime", Preserve: true, Members: map[string]string{"bin/node": "payload/bin/node"}}}
			if err := Extract(r, "node", p, bytes.NewReader(nodeFixture(t, arch, "", "", 0))); err != nil {
				t.Fatal(err)
			}
			for n, v := range map[string]string{"npm": "../lib/node_modules/npm/bin/npm-cli.js", "npx": "../lib/node_modules/npm/bin/npx-cli.js"} {
				got, err := os.Readlink(filepath.Join(dir, "payload/bin", n))
				if err != nil || got != v {
					t.Fatalf("%s %q %v", n, got, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "payload/include/node/node.h")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestNodeRejectsHostileGraph(t *testing.T) {
	for _, v := range []string{"/etc/passwd", "../../outside", "../lib/node_modules/npm/bin/missing", "npx", "../lib/node_modules/npm"} {
		t.Run(v, func(t *testing.T) {
			r, _ := openRoot(t)
			p := toolconfig.Package{Install: toolconfig.Install{Kind: "tar.gz", Profile: "node-runtime", Preserve: true}}
			if err := Extract(r, "node", p, bytes.NewReader(nodeFixture(t, "x64", "bin/npm", v, tar.TypeSymlink))); err == nil {
				t.Fatal("unsafe Node link accepted")
			}
		})
	}
}

func TestNodeLaunchersSelectBundledRuntimeAndPreserveConsumerState(t *testing.T) {
	r, dir := openRoot(t)
	if err := writeNodeLaunchers(r); err != nil {
		t.Fatal(err)
	}
	// This shell stand-in records invocation; no Node binary is executed.
	fake := "#!/bin/sh\nprintf '%s\\n' \"$HOME\" \"$PATH\" \"${npm_config_prefix-unset}\" \"$@\"\n"
	if err := WriteFile(r, "payload/bin/node", strings.NewReader(fake), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"node", "npm", "npx"} {
		cmd := exec.Command(filepath.Join(dir, "bin", name), "one two", "literal$argument")
		cmd.Env = []string{"HOME=/tmp/node-fixture-home", "PATH=/usr/bin:/bin", "npm_config_prefix=/tmp/custom-prefix"}
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		wantPath := "/usr/bin:/bin"
		args := "one two\nliteral$argument\n"
		if name != "node" {
			wantPath = dir + "/bin:" + wantPath
			args = dir + "/payload/lib/node_modules/npm/bin/" + name + "-cli.js\n" + args
		}
		want := "/tmp/node-fixture-home\n" + wantPath + "\n/tmp/custom-prefix\n" + args
		if string(out) != want {
			t.Fatalf("%s: %q want %q", name, out, want)
		}
	}
}
