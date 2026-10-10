package scripts

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDocsBuild(t *testing.T) {
	if os.Getenv("BUNDLEDOCK_DOCS_TEST") != "1" {
		t.Skip("host Hugo integration check; run make docs-test to enable")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()
	cmd := exec.Command("bash", "scripts/hugo.sh", "--destination", output, "--baseURL", "https://example.invalid/bundledock/", "--panicOnWarning")
	cmd.Dir = root
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Hugo build: %v\n%s", err, data)
	}
	for page, text := range map[string]string{
		"index.html":                     "Build once. Share your tools.",
		"cli/index.html":                 "Quick start",
		"guides/tools/index.html":        "gh==2.102.0",
		"guides/architecture/index.html": "Consumer compatibility and build layout",
		"guides/publication/index.html":  "REPOSITORY_USERNAME",
		"guides/development/index.html":  "Verification evidence",
	} {
		data, err := os.ReadFile(filepath.Join(output, page))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), text) {
			t.Errorf("%s missing %q", page, text)
		}
	}
	for _, asset := range []string{"logo.svg", "banner.svg", "favicon.svg"} {
		source, err := os.ReadFile(filepath.Join(root, "assets", asset))
		if err != nil {
			t.Fatal(err)
		}
		rendered, err := os.ReadFile(filepath.Join(output, "assets", asset))
		if err != nil {
			t.Fatal(err)
		}
		if string(source) != string(rendered) {
			t.Errorf("brand asset %s changed", asset)
		}
	}
	if err := checkLocalLinks(output, "/bundledock/"); err != nil {
		t.Fatal(err)
	}
}

// Check the actual rendered site, including fragments and project subpath URLs.
func checkLocalLinks(root, prefix string) error {
	links := regexp.MustCompile(`(?:href|src)="([^"]+)"`)
	return filepath.WalkDir(root, func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(file) != ".html" {
			return nil
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		for _, match := range links.FindAllSubmatch(data, -1) {
			destination, err := url.Parse(string(match[1]))
			if err != nil {
				return err
			}
			if destination.IsAbs() || destination.Host != "" {
				continue
			}
			target := file
			if destination.Path != "" {
				if strings.HasPrefix(destination.Path, "/") {
					if !strings.HasPrefix(destination.Path, prefix) {
						return fmt.Errorf("%s: link escapes base URL: %s", file, destination)
					}
					target = filepath.Join(root, strings.TrimPrefix(destination.Path, prefix))
				} else {
					target = filepath.Join(filepath.Dir(file), destination.Path)
				}
			}
			info, err := os.Stat(target)
			if err != nil {
				return fmt.Errorf("%s: broken local link %s: %w", file, destination, err)
			}
			if info.IsDir() {
				target = filepath.Join(target, "index.html")
			}
			content, err := os.ReadFile(target)
			if err != nil {
				return fmt.Errorf("%s: broken local link %s: %w", file, destination, err)
			}
			if destination.Fragment != "" && !strings.Contains(string(content), `id="`+destination.Fragment+`"`) {
				return fmt.Errorf("%s: missing fragment in %s", file, destination)
			}
		}
		return nil
	})
}
