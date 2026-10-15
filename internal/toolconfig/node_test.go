package toolconfig

import "testing"

func TestOptionalNodeBundlePinsBothOfficialPayloads(t *testing.T) {
	c, err := Load("../../bundles/node")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Catalog.Packages) != 1 {
		t.Fatal("node must remain a separate optional bundle")
	}
	p := c.Catalog.Packages["node"]
	if p.Install.Profile != "node-runtime" || !p.Install.Preserve {
		t.Fatal("full faithful standalone profile required")
	}
	assertRollingPublisherPins(t, c, "node")
}

func TestOptionalCurrentUsesStrictSharedNodeRecipe(t *testing.T) {
	c, err := Load("../../bundles/node-current")
	if err != nil {
		t.Fatal(err)
	}
	p := c.Catalog.Packages["node"]
	if len(c.Catalog.Packages) != 1 || p.Channel != "current" || p.Provider != "node" || p.Install.Profile != "node-runtime" {
		t.Fatal("wrong Current recipe")
	}
	assertRollingPublisherPins(t, c, "node")
	for _, channel := range []string{"nightly", "Current", "false"} {
		p.Channel = channel
		c.Catalog.Packages["node"] = p
		c.Bind()
		if c.Validate() == nil {
			t.Fatalf("accepted channel %q", channel)
		}
	}
}
