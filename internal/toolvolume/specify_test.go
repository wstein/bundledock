package toolvolume

import (
	"archive/tar"
	"bundledock/internal/toolbundle"
	"bundledock/internal/toolconfig"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSpecifyLauncherSurvivesVolumeInitialization(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			src, dst := t.TempDir(), t.TempDir()
			root, err := os.OpenRoot(src)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			var payload bytes.Buffer
			gz := gzip.NewWriter(&payload)
			tw := tar.NewWriter(gz)
			for _, name := range []string{"uv", "uvx"} {
				if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: 7}); err != nil {
					t.Fatal(err)
				}
				if _, err := io.WriteString(tw, "fixture"); err != nil {
					t.Fatal(err)
				}
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			p := toolconfig.Package{Install: toolconfig.Install{Kind: "tar.gz", Members: map[string]string{"uv": "bin/uv", "uvx": "bin/uvx"}}}
			if err := toolbundle.Extract(root, "uv", p, bytes.NewReader(payload.Bytes())); err != nil {
				t.Fatal(err)
			}
			if err := toolbundle.WriteManifest(root, arch, map[string]toolbundle.Spec{"uv": {Version: "fixture"}}); err != nil {
				t.Fatal(err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				if err := Initialize(src, dst, io.Discard); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"bin/specify", "libexec/python-tool.py", "share/specify/specify.requirements.txt", "bin/tldr", "share/tldr/tldr.requirements.txt", "manifest.json", "SHA256SUMS"} {
				before, err := os.ReadFile(filepath.Join(src, name))
				if err != nil {
					t.Fatal(err)
				}
				after, err := os.ReadFile(filepath.Join(dst, name))
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("copy %s: %v", name, err)
				}
			}
		})
	}
}
