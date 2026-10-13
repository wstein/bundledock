package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestBundleDockCommandIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"help", []string{"--help"}, "bdock [flags]"},
		{"tools help", []string{"tools", "--help"}, "bdock tools [command]"},
		{"version", []string{"--version"}, "bdock version dev\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			cmd := newRootCommand()
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), tc.want) {
				t.Fatalf("output %q does not contain %q", output.String(), tc.want)
			}
		})
	}
}

func TestToolsVolumeHelpDefaultImage(t *testing.T) {
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"tools", "volume", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `default "bundledock-toolbox:poc"`) {
		t.Fatalf("volume help has an unexpected default image: %q", output.String())
	}
}

func TestBundleDockInjectedVersion(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })
	version = "local-test"
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "bdock version local-test\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}
