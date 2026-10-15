package toolpublish

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Inspect imports rather than maintaining a second list of compiler packages.
// Whole-package copies also retain go:embed inputs such as embedded scripts,
// requirement files and third-party notices. This check never invokes Docker.
func TestCompilerSourceClosure(t *testing.T) {
	root := filepath.Join("..", "..")
	packages := compilerPackages(t, root, "cmd/tools-bundle", "cmd/tools-init")
	for _, bundle := range []string{"toolbox", "code-server", "node", "node-current", "bun", "codex", "agy", "claude", "opentofu"} {
		t.Run(bundle, func(t *testing.T) {
			dockerfile := filepath.Join(root, "bundles", bundle, "Dockerfile")
			data, err := os.ReadFile(dockerfile)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			firstStage := strings.Index(text, "FROM ")
			if firstStage < 0 {
				t.Fatal("Dockerfile has no compiler stage")
			}
			compiler := strings.SplitN(text[firstStage:], "\nFROM ", 2)[0]
			ignore, err := os.ReadFile(dockerfile + ".dockerignore")
			if os.IsNotExist(err) {
				ignore, err = os.ReadFile(filepath.Join(root, ".dockerignore"))
			}
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			var rules []string
			for _, line := range strings.Split(string(ignore), "\n") {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					rules = append(rules, line)
				}
			}
			// These deliberately minimal contexts use an exclude-all allowlist.
			// Fail explicitly if its form changes instead of guessing Docker's
			// matching semantics and claiming that embedded files are retained.
			for i, rule := range rules {
				if (i == 0 && rule != "**") || (i > 0 && !strings.HasPrefix(rule, "!")) {
					t.Fatalf("unsupported compiler context allowlist rule %q", rule)
				}
			}
			for _, pkg := range packages {
				if !strings.Contains(compiler, "\nCOPY "+pkg+" ./"+pkg+"\n") {
					t.Errorf("compiler does not copy required package %s (including embedded assets)", pkg)
				}
				if len(rules) == 0 {
					continue // No Docker context exclusions.
				}
				parts := strings.Split(pkg, "/")
				for i := range parts {
					dir := strings.Join(parts[:i+1], "/")
					if !containsContextRule(rules, "!"+dir) {
						t.Errorf("compiler context does not include package directory %s", dir)
					}
				}
				if !containsContextRule(rules, "!"+pkg+"/**") {
					t.Errorf("compiler context does not include all sources and embedded assets under %s", pkg)
				}
			}
		})
	}
}

func containsContextRule(rules []string, want string) bool {
	for _, rule := range rules {
		if rule == want {
			return true
		}
	}
	return false
}

func compilerPackages(t *testing.T, root string, queue ...string) []string {
	t.Helper()
	seen := map[string]bool{}
	var packages []string
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		if seen[pkg] {
			continue
		}
		seen[pkg] = true
		packages = append(packages, pkg)
		files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pkg), "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("compiler package %s: source files missing: %v", pkg, err)
		}
		for _, name := range files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imported := range file.Imports {
				path, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(path, "bundledock/") {
					queue = append(queue, strings.TrimPrefix(path, "bundledock/"))
				}
			}
		}
	}
	sort.Strings(packages)
	return packages
}
