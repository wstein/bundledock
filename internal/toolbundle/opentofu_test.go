package toolbundle

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenTofuArchivesKeepCLIAndNotices(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/opentofu")
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages["opentofu"]
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			files := []member{{"tofu", "official binary stand-in", 0755, 0}}
			for _, name := range []string{"LICENSE", "README.md", "CHANGELOG.md"} {
				files = append(files, member{name, "unchanged " + name, 0644, 0})
			}
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := Extract(root, "opentofu", p, bytes.NewReader(archive(t, files...))); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"LICENSE", "README.md", "CHANGELOG.md"} {
				data, err := os.ReadFile(filepath.Join(dir, "licenses/opentofu", name))
				if err != nil || string(data) != "unchanged "+name {
					t.Fatal(name, err)
				}
			}
			if err := WriteManifest(root, arch, c.Lock.Platforms[arch]); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "bin/tofu")); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "bin/terraform")); !os.IsNotExist(err) {
				t.Fatal("unexpected renamed binary")
			}
		})
	}
	for _, bad := range []member{{"../tofu", "", 0755, 0}, {"tofu", "", 0755, tar.TypeSymlink}, {"tofu", "", 0644, 0}, {"tofu", "", 0755, tar.TypeFifo}} {
		dir := t.TempDir()
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		err = Extract(root, "opentofu", p, bytes.NewReader(archive(t, bad)))
		root.Close()
		if err == nil {
			t.Fatal("unsafe archive accepted", bad)
		}
	}
}

// Exercise the full installer with the real catalog, including supplemental
// notices copied after extraction. Extract-only coverage misses that boundary.
func TestOpenTofuInstallRetainsArchiveAndSupplementalNotices(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/opentofu")
	if err != nil {
		t.Fatal(err)
	}
	license, err := os.ReadFile("../../bundles/opentofu/licenses/opentofu/LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../bundles/opentofu/licenses/opentofu/SOURCE")
	if err != nil {
		t.Fatal(err)
	}
	payload := archive(t, member{"tofu", "synthetic CLI", 0755, 0}, member{"LICENSE", string(license), 0644, 0}, member{"README.md", "official readme stand-in", 0644, 0}, member{"CHANGELOG.md", "official changelog stand-in", 0644, 0})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payload) }))
	defer server.Close()
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			spec := c.Lock.Platforms[arch]["opentofu"]
			spec.URL = server.URL
			spec.SHA256 = fmt.Sprintf("%x", sha256.Sum256(payload))
			dir := t.TempDir()
			if err := Build(context.Background(), server.Client(), map[string]Spec{"opentofu": spec}, c.Catalog.Packages, os.DirFS("../../bundles/opentofu"), arch, dir, io.Discard); err != nil {
				t.Fatal(err)
			}
			for name, want := range map[string]string{"bin/tofu": "synthetic CLI", "licenses/opentofu/LICENSE": string(license), "licenses/opentofu/SOURCE": string(source), "licenses/opentofu/README.md": "official readme stand-in", "licenses/opentofu/CHANGELOG.md": "official changelog stand-in"} {
				got, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || string(got) != want {
					t.Fatalf("retained %s differs: %v", name, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "SHA256SUMS")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOpenTofuInstallRejectsDuplicateSupplementalLicense(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/opentofu")
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages["opentofu"]
	p.Notices = append(append([]string{}, p.Notices...), "licenses/opentofu/LICENSE")
	payload := archive(t, member{"tofu", "synthetic CLI", 0755, 0}, member{"LICENSE", "archive license must survive", 0644, 0})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payload) }))
	defer server.Close()
	spec := c.Lock.Platforms["amd64"]["opentofu"]
	spec.URL = server.URL
	spec.SHA256 = fmt.Sprintf("%x", sha256.Sum256(payload))
	dir := t.TempDir()
	err = Build(context.Background(), server.Client(), map[string]Spec{"opentofu": spec}, map[string]toolconfig.Package{"opentofu": p}, os.DirFS("../../bundles/opentofu"), "amd64", dir, io.Discard)
	if !os.IsExist(err) {
		t.Fatalf("expected exclusive-create rejection, got %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "licenses/opentofu/LICENSE"))
	if err != nil || string(got) != "archive license must survive" {
		t.Fatal("archive license overwritten", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); !os.IsNotExist(err) {
		t.Fatal("failed install wrote manifest", err)
	}
}
