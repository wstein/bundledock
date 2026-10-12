package toolconfig

import (
	"reflect"
	"testing"
)

func TestCodexBundleRetainsPinnedToolboxPackage(t *testing.T) {
	standalone, err := Load("../../bundles/codex")
	if err != nil {
		t.Fatal(err)
	}
	toolbox, err := Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	p := standalone.Catalog.Packages["codex"]
	p.Notices = nil
	if len(standalone.Catalog.Packages) != 1 || !reflect.DeepEqual(p, toolbox.Catalog.Packages["codex"]) {
		t.Fatal("Codex must retain the complete official package recipe and voice exclusion")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if len(standalone.Lock.Platforms[arch]) != 1 || standalone.Lock.Platforms[arch]["codex"] != toolbox.Lock.Platforms[arch]["codex"] {
			t.Fatal("Codex pin differs", arch)
		}
	}
}
