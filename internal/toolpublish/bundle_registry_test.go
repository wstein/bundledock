package toolpublish

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestPrecompilerRegistryMatchesNativeSelection(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("requires python3: ", err)
	}
	cmd := exec.Command("python3", "-B", "-c", `import importlib.util,json; s=importlib.util.spec_from_file_location('b','../../scripts/bundle-config.py'); m=importlib.util.module_from_spec(s); s.loader.exec_module(m); print(json.dumps(m.BUNDLES))`)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var registry map[string]map[string]string
	if err = json.Unmarshal(output, &registry); err != nil {
		t.Fatal(err)
	}
	if len(registry) != 4 {
		t.Fatal("unexpected bundle choices")
	}
	for name, settings := range registry {
		bundle, err := SelectBundle(name)
		if err != nil {
			t.Fatal(err)
		}
		if settings["name"] != bundle.Name || settings["directory"] != bundle.Directory || settings["workflow"] != bundle.Workflow || settings["artifact"] != bundle.Artifact || settings["lock"] != bundle.Directory+"/tools.lock.json" {
			t.Fatal("precompiler registry differs for", name)
		}
	}
}
