package toolconfig

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Check direct upstream notice retention for every discovered bundle. This
// deliberately does not claim coverage of every transitive binary dependency.
func TestEveryCatalogPackageRetainsDirectUpstreamNotices(t *testing.T) {
	catalogs, err := filepath.Glob("../../bundles/*/packages.catalog.json")
	if err != nil || len(catalogs) == 0 {
		t.Fatalf("catalog discovery: %v", err)
	}
	for _, catalog := range catalogs {
		directory := filepath.Dir(catalog)
		c, err := Load(directory)
		if err != nil {
			t.Fatal(err)
		}
		for name, p := range c.Catalog.Packages {
			t.Run(filepath.Base(directory)+"/"+name, func(t *testing.T) {
				// Generic tar extraction and supplemental copying both create
				// licenses/<tool>/<basename> exclusively. Avoid a recipe that
				// asks those two phases to create the same destination.
				if !p.Install.Preserve && (p.Install.Kind == "tar.gz" || p.Install.Kind == "tar.xz") {
					archiveNames := map[string]bool{}
					for _, notice := range p.Install.ArchiveNotices {
						archiveNames[filepath.Base(notice)] = true
					}
					for _, notice := range p.Notices {
						if archiveNames[filepath.Base(notice)] {
							t.Fatalf("archive and supplemental notices collide at licenses/%s/%s", name, filepath.Base(notice))
						}
					}
				}
				hasLicense := false
				candidates := append(append([]string{}, p.Install.ArchiveNotices...), p.Notices...)
				for _, notice := range candidates {
					base := strings.ToUpper(filepath.Base(notice))
					if strings.Contains(base, "LICENSE") || strings.Contains(base, "COPYING") || strings.Contains(base, "NOTICE") || strings.Contains(base, "COPYRIGHT") {
						hasLicense = true
					}
				}
				if !hasLicense {
					// The reviewed proprietary Antigravity carrier is the sole
					// terms-only exception: upstream supplies no standalone
					// license, and its complete embedded notices stay intact.
					if filepath.Base(directory) != "agy" || name != "agy" || p.Provider != "antigravity" || p.Repository != "google-antigravity/antigravity-cli" || len(p.Notices) != 1 || p.Notices[0] != "licenses/agy/SOURCE" {
						t.Fatal("no direct upstream license/notice retention recipe")
					}
					source, err := os.ReadFile(filepath.Join(directory, p.Notices[0]))
					if err != nil {
						t.Fatal(err)
					}
					for _, evidence := range []string{"https://antigravity.google/terms", "https://policies.google.com/privacy", "including\nany embedded notices, is retained unchanged as bin/agy", "No standalone license file\nwas present in the inspected repository root or archives."} {
						if !strings.Contains(string(source), evidence) {
							t.Fatalf("missing reviewed terms-only exception evidence: %q", evidence)
						}
					}
				}
				for _, notice := range p.Notices {
					info, err := os.Stat(filepath.Join(directory, notice))
					if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
						t.Fatalf("notice %s is missing, empty or not regular: %v", notice, err)
					}
				}
			})
		}
	}
}

func TestGitHubCLINoticesBoundToLockedRelease(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages["gh"]
	if p.Repository != "cli/cli" || len(p.Notices) != 2 || p.Notices[0] != "licenses/gh/LICENSE" || p.Notices[1] != "licenses/gh/SOURCE" {
		t.Fatal("official gh notice recipe missing")
	}
	source, err := os.ReadFile("../../bundles/toolbox/licenses/gh/SOURCE")
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		version := c.Lock.Platforms[arch]["gh"].Version
		if !strings.Contains(string(source), "Pinned release: "+version+"\n") || !strings.Contains(string(source), "Source archive: https://api.github.com/repos/cli/cli/tarball/"+version+"\n") {
			t.Fatal("gh notice provenance differs from locked release")
		}
	}
	if !regexp.MustCompile(`(?m)^Source SHA256: [0-9a-f]{64}$`).Match(source) {
		t.Fatal("missing exact source archive hash")
	}
	license, err := os.ReadFile("../../bundles/toolbox/licenses/gh/LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	// Golden digest of unchanged LICENSE from the official v2.102.0 source archive.
	if fmt.Sprintf("%x", sha256.Sum256(license)) != "6da4adc42392c8485e40b4251c7e332fc3352df1947c9ffade71dd60b14a7a4f" {
		t.Fatal("gh full official license changed; inspect upstream notice on version updates")
	}
}
