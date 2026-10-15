package toolconfig

import "testing"

func TestOptionalCodeServerBundlePinsBothOfficialPayloads(t *testing.T) {
	c, err := Load("../../bundles/code-server")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Catalog.Packages) != 1 {
		t.Fatal("code-server must remain a separate optional bundle")
	}
	p := c.Catalog.Packages["code-server"]
	if p.Install.Profile != "code-server-standalone" || !p.Install.Preserve {
		t.Fatal("full faithful standalone profile required")
	}
	assertRollingStablePins(t, c, "code-server")
}
