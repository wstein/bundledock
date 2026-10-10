package main

import (
	"bundledock/internal/toolbundle"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializerDefaultsAndRelocatedBundle(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "init-volume")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build initializer: %v\n%s", err, output)
	}
	t.Run("defaults", func(t *testing.T) {
		output, err := exec.Command(binary, "--help").CombinedOutput()
		if err != nil || !strings.Contains(string(output), `default "/opt/workharbor"`) || !strings.Contains(string(output), `default "/bundle"`) {
			t.Fatalf("initializer defaults: %v\n%s", err, output)
		}
	})
	t.Run("relative bundle", func(t *testing.T) {
		if _, err := os.Stat("/bin/sh"); err != nil {
			t.Skip("requires /bin/sh: ", err)
		}
		parent := t.TempDir()
		source := filepath.Join(parent, "opt", "bundledock")
		target := filepath.Join(parent, "bundle")
		mounted := filepath.Join(parent, "opt", "workharbor")
		for _, dir := range []string{source, target} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
		}
		root, err := os.OpenRoot(source)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		// The executable resolves its asset relative to its installed location,
		// like the native package assets preserved by the bundle builder.
		for name, data := range map[string]string{
			"bin/fixture":    "#!/bin/sh\nIFS= read -r message < \"${0%/*}/../assets/message\"\nprintf '%s\\n' \"$message\"\n",
			"assets/message": "relocated tool\n",
		} {
			mode := os.FileMode(0644)
			if strings.HasPrefix(name, "bin/") {
				mode = 0755
			}
			if err := toolbundle.WriteFile(root, name, strings.NewReader(data), mode); err != nil {
				t.Fatal(err)
			}
		}
		if err := toolbundle.WriteManifest(root, "amd64", map[string]toolbundle.Spec{"fixture": {Version: "test"}}); err != nil {
			t.Fatal(err)
		}
		initialize := func(destination string) []byte {
			t.Helper()
			output, err := exec.Command(binary, "--source", source, "--target", destination).CombinedOutput()
			if err != nil {
				t.Fatalf("initialize %s: %v\n%s", destination, err, output)
			}
			return output
		}
		initialize(target)
		if err := os.Rename(target, mounted); err != nil {
			t.Fatal(err)
		}
		if output := initialize(mounted); len(output) != 0 {
			t.Fatalf("relocation should verify existing volume: %s", output)
		}
		output, err := exec.Command(filepath.Join(mounted, "bin", "fixture")).CombinedOutput()
		if err != nil || !bytes.Equal(output, []byte("relocated tool\n")) {
			t.Fatalf("relocated executable: %v %q", err, output)
		}
	})
}
