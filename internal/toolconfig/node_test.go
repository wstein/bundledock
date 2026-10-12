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
	sums := map[string]string{"amd64": "6e1db87ef58b8819e5d5402eff1536491b18edd8eb7bee5ef7897876e88dc5ff", "arm64": "724282c3b43aec998aa9527380465b45d229e021b58035f5f4f63095eabfe5d5"}
	for arch, sum := range sums {
		if c.Lock.Platforms[arch]["node"].SHA256 != sum {
			t.Fatalf("wrong %s official checksum", arch)
		}
	}
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
	for arch, sum := range map[string]string{"amd64": "15fc043a51a5783f2f4b634561091658f5a7f0e3db03f997dca3a8033a33e45b", "arm64": "170454fb022f5e9b5f6cc76c25fa32ef5100895be44eb04e92f9780591491bbf"} {
		if c.Lock.Platforms[arch]["node"].SHA256 != sum {
			t.Fatal("wrong Current checksum")
		}
	}
	for _, channel := range []string{"nightly", "Current", "false"} {
		p.Channel = channel
		c.Catalog.Packages["node"] = p
		c.Bind()
		if c.Validate() == nil {
			t.Fatalf("accepted channel %q", channel)
		}
	}
}
