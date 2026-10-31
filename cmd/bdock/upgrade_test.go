package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bundledock/internal/toolpublish"
)

func TestUpgradeHelp(t *testing.T) {
	for _, args := range [][]string{{"upgrade", "--help"}, {"tools", "upgrade", "--help"}} {
		var output bytes.Buffer
		cmd := newRootCommand()
		cmd.SetOut(&output)
		cmd.SetErr(&output)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		text := output.String()
		for _, want := range []string{"--bundle", "--all", "--directory"} {
			if !strings.Contains(text, want) {
				t.Fatalf("help output for %v missing %q: %s", args, want, text)
			}
		}
	}
}

func TestUpgradeFlagValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown bundle", []string{"upgrade", "--bundle", "nonexistent"}, "unknown canonical publication bundle"},
		{"all with tool", []string{"upgrade", "gh", "--all"}, "cannot specify a tool with --all"},
		{"all with custom directory", []string{"upgrade", "--all", "--directory", "custom"}, "cannot combine --directory and --all"},
		{"all with bundle", []string{"upgrade", "--all", "--bundle", "node"}, "cannot combine --bundle and --all"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCommand()
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for args %v, got nil", tc.args)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestRootUpgradeCommandExecution(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "bundles", "toolbox")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"packages.txt", "packages.catalog.json", "packages.lock.json"} {
		b, err := os.ReadFile(filepath.Join("../../bundles/toolbox", n))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Error(err)
		}
	})
	payload := []byte("CLI fixture")
	sum := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			fmt.Fprintf(w, `[{"tag_name":"v2.103.0","assets":[{"name":"gh_2.103.0_linux_amd64.tar.gz","browser_download_url":"https://github.com/cli/amd64","digest":"%s"},{"name":"gh_2.103.0_linux_arm64.tar.gz","browser_download_url":"https://github.com/cli/arm64","digest":"%s"}]}]`, sum, sum)
			return
		}
		w.Write(payload)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	client := &http.Client{Transport: upgradeTestTransport{base, server.Client().Transport}}
	cmd := newRootCommandWithClient(client)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"upgrade", "gh"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if out.String() != "gh v2.103.0\n" {
		t.Fatalf("output %q", out.String())
	}
}

func TestUpgradeWithBundleFlag(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "bundles", "code-server")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"packages.txt", "packages.catalog.json", "packages.lock.json"} {
		b, err := os.ReadFile(filepath.Join("../../bundles/code-server", n))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Error(err)
		}
	})
	payload := []byte("CLI fixture")
	sum := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			fmt.Fprintf(w, `[{"tag_name":"v4.142.0","assets":[{"name":"code-server-4.142.0-linux-amd64.tar.gz","browser_download_url":"https://github.com/coder/amd64","digest":"%s"},{"name":"code-server-4.142.0-linux-arm64.tar.gz","browser_download_url":"https://github.com/coder/arm64","digest":"%s"}]}]`, sum, sum)
			return
		}
		w.Write(payload)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	client := &http.Client{Transport: upgradeTestTransport{base, server.Client().Transport}}
	cmd := newRootCommandWithClient(client)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"upgrade", "--bundle", "code-server"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if out.String() != "code-server v4.142.0\n" {
		t.Fatalf("output %q", out.String())
	}
}

func recordUpgrades(t *testing.T) *[]string {
	t.Helper()
	var dirs []string
	original := upgradeBundle
	t.Cleanup(func() { upgradeBundle = original })
	upgradeBundle = func(_ context.Context, _ *http.Client, directory, _ string, _ io.Writer) error {
		dirs = append(dirs, directory)
		return nil
	}
	return &dirs
}

func TestUpgradeAllSkipsRetiredBundles(t *testing.T) {
	dirs := recordUpgrades(t)
	cmd := newRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"upgrade", "--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(*dirs) != len(toolpublish.ActiveBundles()) || len(*dirs) == 0 {
		t.Fatalf("upgraded %v", *dirs)
	}
	for _, dir := range *dirs {
		if dir == "bundles/opentofu" {
			t.Fatal("--all upgraded the retired opentofu bundle")
		}
	}
}

func TestUpgradeOfARetiredBundleWarnsButStillWorks(t *testing.T) {
	dirs := recordUpgrades(t)
	var out, errOut bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"upgrade", "--bundle", "opentofu"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(*dirs) != 1 || (*dirs)[0] != "bundles/opentofu" {
		t.Fatalf("upgraded %v", *dirs)
	}
	if !strings.Contains(errOut.String(), "opentofu is retired") || strings.Contains(out.String(), "retired") {
		t.Fatalf("warning belongs on stderr: out=%q err=%q", out.String(), errOut.String())
	}
	errOut.Reset()
	cmd = newRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"upgrade", "--bundle", "agy"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(errOut.String(), "retired") {
		t.Fatalf("unexpected warning %q", errOut.String())
	}
}

func TestUpgradeAllMissingBundle(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Error(err)
		}
	})
	cmd := newRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"upgrade", "--all"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected error when bundle directory is missing")
	}
	if !strings.Contains(err.Error(), "upgrade toolbox:") {
		t.Fatalf("unexpected error %v", err)
	}
}
