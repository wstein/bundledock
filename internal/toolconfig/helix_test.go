package toolconfig

import "testing"

func TestHelixGenericSemverRemainsStrict(t *testing.T) {
	if _, err := ParseVersion("25.07.1"); err == nil {
		t.Fatal("generic SemVer must stay strict")
	}
}

func TestHelixCatalogPins(t *testing.T) {
	c, err := Load("../../bundles/nixbox/tools")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Catalog.Packages["helix"]
	if !ok || p.Repository != "helix-editor/helix" || p.Install.Profile != "helix-runtime" || !p.Install.Preserve {
		t.Fatal("missing complete official Helix runtime recipe")
	}
	assertRollingPublisherPins(t, c, "helix")
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
