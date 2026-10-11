package toolconfig

import "testing"

func TestHelixCalendarRelease(t *testing.T) {
	p := Package{Provider: "github", Repository: "helix-editor/helix", Normalization: "helix-calendar"}
	for _, tag := range []string{"25.07.1", "25.07", "25.7.1"} {
		got, err := p.Version(tag)
		if err != nil || got != "25.7.1" && tag != "25.07" || tag == "25.07" && got != "25.7.0" {
			t.Fatalf("%s => %s: %v", tag, got, err)
		}
	}
	for _, tag := range []string{"25.07.1-rc1", "25.13.1", "25.00.1", "25.07.01"} {
		if _, err := p.Version(tag); err == nil {
			t.Fatalf("accepted %s", tag)
		}
	}
	if _, err := ParseVersion("25.07.1"); err == nil {
		t.Fatal("generic SemVer must stay strict")
	}
}

func TestHelixCatalogPins(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["helix"]
	if !ok || p.Repository != "helix-editor/helix" || p.Install.Profile != "helix-runtime" || !p.Install.Preserve {
		t.Fatal("missing complete official Helix runtime recipe")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		s := c.Lock.Platforms[arch]["helix"]
		if s.Version != "25.07.1" || s.Verification != "publisher-sha256" {
			t.Fatalf("missing verified Helix %s pin", arch)
		}
	}
}

func TestHelixRejectsMismatchedReleasePin(t *testing.T) {
	c, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	s := c.Lock.Platforms["amd64"]["helix"]
	s.URL = c.Lock.Platforms["arm64"]["helix"].URL
	c.Lock.Platforms["amd64"]["helix"] = s
	if err := c.Validate(); err == nil {
		t.Fatal("accepted Helix archive for wrong architecture")
	}
}
