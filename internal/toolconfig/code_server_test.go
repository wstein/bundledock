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
	sums := map[string]string{"amd64": "65bcd20b524d03eb1b5adb7f360244599ef0e5b74089a46a982d32c074b97a29", "arm64": "e5cc78da1d631c6cf6162f9b18eeaa8ee9d3239e1a88bb99d0602cdc5fa1852d"}
	for arch, sum := range sums {
		if c.Lock.Platforms[arch]["code-server"].SHA256 != sum {
			t.Fatalf("wrong %s official checksum", arch)
		}
	}
}
