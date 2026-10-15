package toolconfig

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseVersionFixtures(t *testing.T) {
	var fixtures []struct {
		Name          string            `json:"name"`
		Normalization string            `json:"normalization"`
		TagPrefix     string            `json:"tag_prefix"`
		Valid         map[string]string `json:"valid"`
		Invalid       []string          `json:"invalid"`
	}
	readReleaseFixture(t, "release-versions.json", &fixtures)
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			// These historical examples never read the mutable production lock.
			p := Package{Normalization: fixture.Normalization, TagPrefix: fixture.TagPrefix}
			for tag, want := range fixture.Valid {
				if got, err := p.Version(tag); err != nil || got != want {
					t.Fatalf("%s: want %s, got %s: %v", tag, want, got, err)
				}
			}
			for _, tag := range fixture.Invalid {
				if _, err := p.Version(tag); err == nil {
					t.Fatalf("accepted invalid tag %s", tag)
				}
			}
		})
	}
}

func TestReviewedLicenseSnapshots(t *testing.T) {
	var fixtures []struct {
		Name          string `json:"name"`
		Bundle        string `json:"bundle"`
		Repository    string `json:"repository"`
		Version       string `json:"version"`
		SourceURL     string `json:"source_url"`
		SourceSHA256  string `json:"source_sha256"`
		LicenseSHA256 string `json:"license_sha256"`
	}
	readReleaseFixture(t, "license-snapshots.json", &fixtures)
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			directory := filepath.Join("../../bundles", fixture.Bundle)
			c, err := Load(directory)
			if err != nil {
				t.Fatal(err)
			}
			p := c.Catalog.Packages[fixture.Name]
			prefix := "licenses/" + fixture.Name + "/"
			if p.Repository != fixture.Repository || len(p.Notices) != 2 || p.Notices[0] != prefix+"LICENSE" || p.Notices[1] != prefix+"SOURCE" {
				t.Fatal("official license snapshot recipe missing")
			}
			source, err := os.ReadFile(filepath.Join(directory, prefix, "SOURCE"))
			if err != nil {
				t.Fatal(err)
			}
			fields := []string{
				"Upstream: https://github.com/" + fixture.Repository,
				"License snapshot release: " + fixture.Version,
				"License snapshot source: " + fixture.SourceURL,
				"LICENSE SHA256: " + fixture.LicenseSHA256,
			}
			if fixture.SourceSHA256 != "" {
				fields = append(fields, "Source archive SHA256: "+fixture.SourceSHA256)
			}
			for _, field := range fields {
				if !strings.Contains(string(source), field+"\n") {
					t.Fatalf("missing reviewed provenance %q", field)
				}
			}
			if strings.Contains(string(source), "Pinned release:") {
				t.Fatal("historical license snapshot must not claim the current binary pin")
			}
			license, err := os.ReadFile(filepath.Join(directory, prefix, "LICENSE"))
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprintf("%x", sha256.Sum256(license)) != fixture.LicenseSHA256 {
				t.Fatal("reviewed license bytes changed; inspect provenance and full notice")
			}
		})
	}
}

func readReleaseFixture(t *testing.T, name string, target any) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := Decode(f, target); err != nil {
		t.Fatal(err)
	}
}
