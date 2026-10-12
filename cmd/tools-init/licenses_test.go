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

func TestLicenseReaderFlagsDoNotInitializeOrNeedExternalTools(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "init-volume")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("test compiler: %v %s", err, output)
	}
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "must-remain-absent")
	root, err := os.OpenRoot(source)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	name := "licenses/fixture/dependencies/library-1/LICENSE"
	full := "Original entire upstream notice\nwith all attribution\n"
	if err := toolbundle.WriteFile(root, name, strings.NewReader(full), 0644); err != nil {
		t.Fatal(err)
	}
	if err := toolbundle.WriteLicenseArchive(root); err != nil {
		t.Fatal(err)
	}
	if err := toolbundle.WriteManifest(root, "amd64", map[string]toolbundle.Spec{"fixture": {Version: "1"}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(source, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		args []string
		want string
		fail bool
	}{
		{[]string{"--license", name}, full, false},
		{[]string{"--list-licenses"}, `"package": "fixture"`, false},
		{[]string{"--license", "../LICENSE"}, "", true},
		{[]string{"--license", "licenses/fixture/missing"}, "", true},
		{[]string{"--license", name, "--list-licenses"}, "", true},
		{[]string{"--license="}, "", true},
	} {
		command := exec.Command(binary, append([]string{"--source", source, "--target", target}, test.args...)...)
		command.Env = append(os.Environ(), "PATH=")
		output, err := command.CombinedOutput()
		if (err != nil) != test.fail || (!test.fail && !strings.Contains(string(output), test.want)) {
			t.Fatalf("%v: %v %q", test.args, err, output)
		}
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("reader created target", err)
	}
	for _, marker := range []string{".ready", ".initializing"} {
		if _, err := root.Lstat(marker); !os.IsNotExist(err) {
			t.Fatal("reader wrote source marker", marker, err)
		}
	}
	after, err := os.ReadFile(filepath.Join(source, "SHA256SUMS"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("reader changed checksums", err)
	}
}
