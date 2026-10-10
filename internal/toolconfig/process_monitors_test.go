package toolconfig

import "testing"

func TestProcessMonitorInventory(t *testing.T) {
	c, err := Load("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, executable string }{{"bottom", "btm"}, {"btop", "btop"}} {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := c.Catalog.Packages[tc.name]
			if !ok {
				t.Fatal("process monitor missing from catalog")
			}
			if p.Install.Members[tc.executable] != "bin/"+tc.executable {
				t.Fatal("missing executable recipe")
			}
			if len(p.Notices)+len(p.Install.ArchiveNotices) == 0 {
				t.Fatal("missing license notices")
			}
			if _, ok := c.Desired[tc.name]; !ok {
				t.Fatal("missing bounded requirement")
			}
			for _, arch := range []string{"amd64", "arm64"} {
				pin, ok := c.Lock.Platforms[arch][tc.name]
				if !ok || len(pin.SHA256) != 64 {
					t.Fatalf("missing verified %s pin", arch)
				}
			}
		})
	}
}
