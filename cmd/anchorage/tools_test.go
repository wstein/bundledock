package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolsVolumeCommand(t *testing.T) {
	dir := t.TempDir()
	id := "sha256:" + strings.Repeat("a", 64)
	script := "#!/bin/sh\ncase \"$1\" in\nimage) printf '%s\\n' '" + id + "' ;;\nvolume) printf 'discarded create output\\n' ;;\nrun) printf 'Tool volume initialized\\n' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"tools", "volume", "--image", "local:fixture"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := "Tool volume initialized\nVolume: anchorage-tools-" + strings.Repeat("a", 64) + "\n"; output.String() != want {
		t.Fatalf("output %q", output.String())
	}
}
func TestToolsRejectsUnexpectedArguments(t *testing.T) {
	for _, args := range [][]string{{"tools", "volume", "extra"}, {"tools", "volume", "--unknown"}} {
		cmd := newRootCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted args %#v", args)
		}
	}
}
