package main

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	for _, bundle := range []string{"toolbox", "code-server", "node"} {
		var output bytes.Buffer
		cmd := newRootCommand()
		cmd.SetOut(&output)
		cmd.SetErr(&output)
		args := []string{"tools", "volume", "--image", "local:fixture"}
		if bundle != "toolbox" {
			args = append(args, "--bundle", bundle)
		}
		cmd.SetArgs(args)
		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		if want := "Tool volume initialized\nVolume: bundledock-" + bundle + "-" + strings.Repeat("a", 64) + "\n"; output.String() != want {
			t.Fatalf("output %q", output.String())
		}
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

func TestToolsUpgradeCommandHTTP(t *testing.T)                  { testToolsUpgradeCommandHTTP(t, false) }
func TestToolsUpgradeCommandHTTPDirectoryOverride(t *testing.T) { testToolsUpgradeCommandHTTP(t, true) }
func testToolsUpgradeCommandHTTP(t *testing.T, override bool) {
	root := t.TempDir()
	dir := filepath.Join(root, "bundles", "toolbox")
	if override {
		dir = filepath.Join(root, "custom-bundle")
	}
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
	args := []string{"tools", "upgrade", "gh"}
	if override {
		args = append(args, "--directory", dir)
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "gh v2.103.0\n" {
		t.Fatalf("output %q", out.String())
	}
	if _, err := toolconfig.Load(dir); err != nil {
		t.Fatal(err)
	}
}

type upgradeTestTransport struct {
	base      *url.URL
	transport http.RoundTripper
}

func (f upgradeTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = f.base.Scheme
	u.Host = f.base.Host
	clone.URL = &u
	return f.transport.RoundTrip(clone)
}
func TestToolsCommandSurface(t *testing.T) {
	cmd := newToolsCommand()
	var names []string
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
		if len(c.Aliases) > 0 {
			t.Fatalf("unexpected aliases %v", c.Aliases)
		}
	}
	if strings.Join(names, ",") != "upgrade,volume" {
		t.Fatalf("unexpected commands %v", names)
	}
	for _, args := range [][]string{{"tools", "upgrade", "gh", "codex"}, {"tools", "update"}, {"tools", "list"}, {"tools", "check"}, {"tools", "build"}, {"tools", "use"}, {"tools", "upgrade", "--unknown"}} {
		c := newRootCommand()
		c.SetOut(&bytes.Buffer{})
		c.SetErr(&bytes.Buffer{})
		c.SetArgs(args)
		if err := c.Execute(); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestToolsUpgradeDirectoryOverride(t *testing.T) {
	cmd := newToolsCommand()
	upgrade, _, err := cmd.Find([]string{"upgrade"})
	if err != nil {
		t.Fatal(err)
	}
	flag := upgrade.Flags().Lookup("directory")
	if flag == nil || flag.DefValue != "bundles/toolbox" {
		t.Fatal("upgrade must default to bundles/toolbox and retain an explicit directory override")
	}
}
