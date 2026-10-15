package toolbundle

import (
	"go/build"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// internalDeps returns the module's internal packages that the given commands
// import, directly or through each other, as paths relative to the module root
// (for example "internal/toolbundle"). Test-only imports are not followed.
func internalDeps(t *testing.T, root string, commands ...string) []string {
	t.Helper()
	const module = "bundledock/"
	seen := map[string]bool{}
	var visit func(dir string)
	visit = func(dir string) {
		pkg, err := build.ImportDir(filepath.Join(root, dir), 0)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		for _, imp := range pkg.Imports {
			rel, ok := strings.CutPrefix(imp, module)
			if !ok || !strings.HasPrefix(rel, "internal/") || seen[rel] {
				continue
			}
			seen[rel] = true
			visit(rel)
		}
	}
	for _, c := range commands {
		visit(c)
	}
	deps := make([]string, 0, len(seen))
	for d := range seen {
		deps = append(deps, d)
	}
	sort.Strings(deps)
	return deps
}

// TestBundleDockerfilesCopyEveryInternalDependency keeps each bundle image able
// to compile its builder and initializer: every internal package they import
// must be copied into the build stage and, where a bundle filters its build
// context, allowed by its Dockerfile.dockerignore. A missing package only shows
// up in an image build, which source CI does not run.
func TestBundleDockerfilesCopyEveryInternalDependency(t *testing.T) {
	root := filepath.Join("..", "..")
	deps := internalDeps(t, root, "cmd/tools-bundle", "cmd/tools-init")
	if len(deps) == 0 {
		t.Fatal("no internal dependencies found; the import walk is broken")
	}
	dockerfiles, err := filepath.Glob(filepath.Join(root, "bundles", "*", "Dockerfile"))
	if err != nil || len(dockerfiles) == 0 {
		t.Fatalf("no bundle Dockerfiles: %v", err)
	}
	for _, file := range dockerfiles {
		bundle := filepath.Base(filepath.Dir(file))
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "COPY cmd/tools-bundle ./cmd/tools-bundle") {
			continue
		}
		ignore, err := os.ReadFile(file + ".dockerignore")
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		allowed := map[string]bool{}
		for _, line := range strings.Split(string(ignore), "\n") {
			allowed[strings.TrimSpace(line)] = true
		}
		for _, dep := range deps {
			if !strings.Contains(text, "COPY "+dep+" ./"+dep+"\n") {
				t.Errorf("%s: Dockerfile does not copy %s, which the bundle builder imports", bundle, dep)
			}
			if ignore != nil && (!allowed["!"+dep] || !allowed["!"+dep+"/**"]) {
				t.Errorf("%s: Dockerfile.dockerignore does not allow %s", bundle, dep)
			}
		}
	}
}
