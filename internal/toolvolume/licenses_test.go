package toolvolume

import (
	"bundledock/internal/toolbundle"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLicenseArchiveSurvivesVolumeInitialization(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	root, err := os.OpenRoot(source)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	name := "licenses/fixture/dependencies/vendor-1/COPYING"
	notice := "Complete original vendored notice\nincluding attribution\n"
	if err := toolbundle.WriteFile(root, name, strings.NewReader(notice), 0644); err != nil {
		t.Fatal(err)
	}
	if err := toolbundle.WriteLicenseArchive(root); err != nil {
		t.Fatal(err)
	}
	if err := toolbundle.WriteManifest(root, "amd64", map[string]toolbundle.Spec{}); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := Initialize(source, target, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	mounted, err := os.OpenRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	defer mounted.Close()
	var got bytes.Buffer
	if err := toolbundle.ReadLicenses(mounted, name, &got); err != nil || got.String() != notice {
		t.Fatalf("initialized complete notice: %v %q", err, got.String())
	}
	if _, err := os.Stat(filepath.Join(target, "licenses")); !os.IsNotExist(err) {
		t.Fatal("initializer expanded notices", err)
	}
	if err := os.WriteFile(filepath.Join(target, "licenses.index.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(source, target, io.Discard); err == nil {
		t.Fatal("accepted modified notice index in ready volume")
	}
}
