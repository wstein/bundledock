package toolbundle

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"bundledock/internal/toolconfig"
)

func TestToolboxBuildRequiresShellLayer(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	toolbox := map[string]toolconfig.Package{"starship": {}}
	if err := shellLayerStep(root, toolbox, buildOptions{}); err == nil || !strings.Contains(err.Error(), "requires the shared shell layer") {
		t.Fatalf("toolbox built without its shell layer: %v", err)
	}
}

func TestShellLayerSourceMustBeComplete(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	partial := fstest.MapFS{"generated/init.bash": {Data: []byte("x")}}
	err = shellLayerStep(root, map[string]toolconfig.Package{"starship": {}}, buildOptions{shellLayer: partial})
	if err == nil {
		t.Fatal("incomplete shell layer source accepted")
	}
}
