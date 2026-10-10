package main

import (
	"anchorage/internal/toolbundle"
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceFlagHelp(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"--help"}, t.TempDir(), &output)
	if err != flag.ErrHelp || !strings.Contains(output.String(), "-source") || !strings.Contains(output.String(), "/opt/anchorage") {
		t.Fatalf("initializer must describe optional source with existing default: %v\n%s", err, &output)
	}

}

func TestCustomSourceInitializesAndVerifiesVolume(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	root, err := os.OpenRoot(source)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := toolbundle.WriteFile(root, "bin/python3", strings.NewReader("runtime"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := toolbundle.WriteManifest(root, "arm64", map[string]toolbundle.Spec{"python3": {Version: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := run([]string{"--source", source}, target, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(target, "bin/python3"))
	if err != nil || string(data) != "runtime" {
		t.Fatalf("custom source not copied: %q %v", data, err)
	}
	for _, args := range [][]string{{"--unknown"}, {"unexpected"}} {
		if err := run(args, t.TempDir(), io.Discard); err == nil {
			t.Fatalf("accepted invalid flags %v", args)
		}
	}
}
