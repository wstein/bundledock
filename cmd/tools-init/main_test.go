package main

import (
	"bundledock/internal/toolbundle"
	"bytes"
	"fmt"
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
		if err != nil || !strings.Contains(string(output), `default "/opt/bundledock/toolbox"`) || !strings.Contains(string(output), `default "/bundle"`) {
			t.Fatalf("initializer defaults: %v\n%s", err, output)
		}
	})
	t.Run("relative bundle", func(t *testing.T) {
		if _, err := os.Stat("/bin/sh"); err != nil {
			t.Skip("requires /bin/sh: ", err)
		}
		parent := t.TempDir()
		source := filepath.Join(parent, "opt", "workharbor")
		target := filepath.Join(parent, "bundle")
		mounted := filepath.Join(parent, "opt", "bundledock")
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
	t.Run("sibling bundle roots", func(t *testing.T) {
		if _, err := os.Stat("/bin/sh"); err != nil {
			t.Skip("requires /bin/sh: ", err)
		}
		parent := t.TempDir()
		prefix := filepath.Join(parent, "opt", "bundledock")
		sources := make(map[string]string)
		manifests := make(map[string][]byte)
		for _, name := range []string{"toolbox", "fixture-other"} {
			source := filepath.Join(parent, "sources", name)
			mounted := filepath.Join(prefix, name)
			for _, dir := range []string{source, mounted} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			src, err := os.OpenRoot(source)
			if err != nil {
				t.Fatal(err)
			}
			defer src.Close()
			// The same executable name and relative asset occur in each bundle.
			for file, data := range map[string]string{
				"bin/fixture":    "#!/bin/sh\ncat \"${0%/*}/../assets/message\"\n",
				"assets/message": name + "\n",
			} {
				mode := os.FileMode(0644)
				if strings.HasPrefix(file, "bin/") {
					mode = 0755
				}
				if err := toolbundle.WriteFile(src, file, strings.NewReader(data), mode); err != nil {
					t.Fatal(err)
				}
			}
			if err := toolbundle.WriteManifest(src, "amd64", map[string]toolbundle.Spec{"fixture": {Version: name}}); err != nil {
				t.Fatal(err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				output, err := exec.Command(binary, "--source", source, "--target", mounted).CombinedOutput()
				if err != nil {
					t.Fatalf("initialize %s: %v\n%s", name, err, output)
				}
			}
			manifest, err := os.ReadFile(filepath.Join(mounted, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			sources[name], manifests[name] = source, manifest
		}
		entries, err := os.ReadDir(prefix)
		if err != nil || len(entries) != 2 {
			t.Fatalf("expected only two sibling bundle roots: %v %v", entries, err)
		}
		if bytes.Equal(manifests["toolbox"], manifests["fixture-other"]) {
			t.Fatal("bundles must keep distinct manifests")
		}
		for name, manifest := range manifests {
			mounted := filepath.Join(prefix, name)
			other := "toolbox"
			if name == other {
				other = "fixture-other"
			}
			output, err := exec.Command(binary, "--source", sources[other], "--target", mounted).CombinedOutput()
			if err == nil || !strings.Contains(string(output), "differs from bundle") {
				t.Fatalf("accepted another bundle in %s: %v %s", name, err, output)
			}
			got, err := os.ReadFile(filepath.Join(mounted, "manifest.json"))
			if err != nil || !bytes.Equal(got, manifest) {
				t.Fatalf("changed %s manifest: %v", name, err)
			}
			// PATH order chooses a bundle; assets resolve within that same root.
			command := exec.Command("/bin/sh", "-ec", "exec fixture")
			command.Env = append(os.Environ(), fmt.Sprintf("PATH=%s/bin:%s/bin:%s", mounted, filepath.Join(prefix, other), os.Getenv("PATH")))
			output, err = command.CombinedOutput()
			if err != nil || string(output) != name+"\n" {
				t.Fatalf("PATH selected %s: %v %q", name, err, output)
			}
		}
	})
}
