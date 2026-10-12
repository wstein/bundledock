package toolvolume

import (
	"bundledock/internal/toolbundle"
	"bundledock/internal/toolconfig"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenTofuRegularVolumeRepeatsAndRejectsTampering(t *testing.T) {
	c, err := toolconfig.Load("../../bundles/opentofu")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"contents", "mode", "extra provider"} {
		t.Run(kind, func(t *testing.T) {
			src := t.TempDir()
			root, err := os.OpenRoot(src)
			if err != nil {
				t.Fatal(err)
			}
			if err := toolbundle.WriteFile(root, "bin/tofu", strings.NewReader("binary stand-in"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := toolbundle.WriteManifest(root, "amd64", c.Lock.Platforms["amd64"]); err != nil {
				t.Fatal(err)
			}
			root.Close()
			dst := t.TempDir()
			for i := 0; i < 2; i++ {
				if err := Initialize(src, dst, io.Discard); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "contents":
				if err := os.WriteFile(filepath.Join(dst, "bin/tofu"), []byte("changed"), 0755); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(filepath.Join(dst, "bin/tofu"), 0644); err != nil {
					t.Fatal(err)
				}
			case "extra provider":
				if err := os.WriteFile(filepath.Join(dst, "provider-plugin"), []byte("plugin"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := Initialize(src, dst, io.Discard); err == nil {
				t.Fatal("accepted tampering", kind)
			}
		})
	}
}
