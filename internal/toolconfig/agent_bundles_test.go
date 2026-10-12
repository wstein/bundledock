package toolconfig

import "testing"

func TestToolboxExcludesStandaloneAgents(t *testing.T) {
	toolbox, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"codex", "agy", "claude"} {
		if _, ok := toolbox.Desired[name]; ok {
			t.Errorf("toolbox still requests standalone agent %s", name)
		}
		if _, ok := toolbox.Catalog.Packages[name]; ok {
			t.Errorf("toolbox still catalogs standalone agent %s", name)
		}
		for arch, packages := range toolbox.Lock.Platforms {
			if _, ok := packages[name]; ok {
				t.Errorf("toolbox still pins standalone agent %s on %s", name, arch)
			}
		}
		standalone, err := Load("../../bundles/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if len(standalone.Desired) != 1 || len(standalone.Catalog.Packages) != 1 {
			t.Fatalf("%s must remain independently selectable", name)
		}
		for _, arch := range []string{"amd64", "arm64"} {
			if len(standalone.Lock.Platforms[arch]) != 1 || standalone.Lock.Platforms[arch][name].URL == "" {
				t.Fatalf("%s missing standalone %s pin", name, arch)
			}
		}
	}
}
