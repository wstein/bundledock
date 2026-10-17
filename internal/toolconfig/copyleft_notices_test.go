package toolconfig

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Statically linked GPL binaries need a corresponding-source record (tarball
// URL and SHA256) and the upstream third-party notices.
func TestHadolintRetainsGPLSourceRecordAndThirdPartyNotices(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range c.Catalog.Packages["hadolint"].Notices {
		if n == "licenses/hadolint/ThirdPartyNotices.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("hadolint catalog must retain ThirdPartyNotices.txt")
	}
	b, err := os.ReadFile("../../bundles/toolbox/licenses/hadolint/SOURCE")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "License: GPL-3.0-only") {
		t.Fatal("SPDX identifier must be GPL-3.0-only")
	}
	if !regexp.MustCompile(`(?m)^Source archive: https://api\.github\.com/repos/hadolint/hadolint/tarball/v\d+\.\d+\.\d+$`).MatchString(s) ||
		!regexp.MustCompile(`(?m)^Source SHA256: [0-9a-f]{64}$`).MatchString(s) {
		t.Fatal("missing source tarball URL and SHA256")
	}
	n, err := os.ReadFile("../../bundles/toolbox/licenses/hadolint/ThirdPartyNotices.txt")
	if err != nil || !strings.Contains(string(n), "ShellCheck") {
		t.Fatalf("third-party notices missing or truncated: %v", err)
	}
}

func TestArchiveNoticeTreesValidation(t *testing.T) {
	for _, tree := range []string{"third_party_licenses", "a/b", "", ".", "..", "../x", "/abs", "a/../b", "a/", "a\\b"} {
		t.Run(tree, func(t *testing.T) {
			c, err := Load("../../bundles/toolbox")
			if err != nil {
				t.Fatal(err)
			}
			p := c.Catalog.Packages["pinact"]
			p.Install.ArchiveNoticeTrees = []string{tree}
			c.Catalog.Packages["pinact"] = p
			c.Bind()
			err = c.Validate()
			ok := tree == "third_party_licenses" || tree == "a/b"
			if ok && err != nil || !ok && err == nil {
				t.Fatalf("tree %q: %v", tree, err)
			}
			if ok {
				p.Install.Preserve = true
				c.Catalog.Packages["pinact"] = p
				c.Bind()
				if c.Validate() == nil {
					t.Fatal("notice trees accepted with preserve")
				}
			}
		})
	}
}

func TestPinactCatalogRetainsThirdPartyLicenses(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	trees := c.Catalog.Packages["pinact"].Install.ArchiveNoticeTrees
	if len(trees) != 1 || trees[0] != "third_party_licenses" {
		t.Fatalf("pinact notice trees: %v", trees)
	}
}
