package toolbundle

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// extractLayout installs a synthetic archive that mirrors a real upstream file
// layout and returns the relative paths found under licenses/<tool>.
func extractLayout(t *testing.T, tool string, members ...member) (string, []string, error) {
	t.Helper()
	c, err := toolconfig.Load("../../bundles/toolbox")
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	err = Extract(root, tool, c.Catalog.Packages[tool], bytes.NewReader(archive(t, members...)))
	var found []string
	base := filepath.Join(dst, "licenses", tool)
	filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(base, p)
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(found)
	return dst, found, err
}

func TestRealArchiveLayoutsKeepNoticesWithoutBasenameCollisions(t *testing.T) {
	t.Run("git-cliff nested root", func(t *testing.T) {
		const r = "git-cliff-2.14.2/"
		dst, found, err := extractLayout(t, "git-cliff",
			member{r, "", 0755, tar.TypeDir},
			member{r + "LICENSE-APACHE", "apache", 0644, 0},
			member{r + "LICENSE-MIT", "mit", 0644, 0},
			member{r + "git-cliff", "exe", 0755, 0},
			member{r + "CHANGELOG.md", "changelog", 0644, 0},
			member{r + "git-cliff-completions", "gen", 0755, 0},
			member{r + "man/", "", 0755, tar.TypeDir},
			member{r + "man/git-cliff.1", "man", 0644, 0},
			member{r + "README.md", "readme", 0644, 0},
			member{r + "git-cliff-mangen", "gen", 0755, 0},
			member{r + "completions/", "", 0755, tar.TypeDir},
			member{r + "completions/git-cliff.bash", "bash", 0644, 0},
		)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(found, ",") != "LICENSE-APACHE,LICENSE-MIT,README.md,SOURCE" && strings.Join(found, ",") != "LICENSE-APACHE,LICENSE-MIT,README.md" {
			t.Fatalf("git-cliff notices: %v", found)
		}
		if b, _ := os.ReadFile(filepath.Join(dst, "licenses/git-cliff/README.md")); string(b) != "readme" {
			t.Fatalf("README: %q", b)
		}
		if _, err := os.Stat(filepath.Join(dst, "bin/git-cliff")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("zoxide nested completions README", func(t *testing.T) {
		dst, found, err := extractLayout(t, "zoxide",
			member{"CHANGELOG.md", "changelog", 0644, 0},
			member{"LICENSE", "mit", 0644, 0},
			member{"README.md", "top readme", 0644, 0},
			member{"completions/", "", 0755, tar.TypeDir},
			member{"completions/README.md", "completions readme", 0644, 0},
			member{"completions/zoxide.bash", "bash", 0644, 0},
			member{"zoxide", "exe", 0755, 0},
		)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(found, ",") != "CHANGELOG.md,LICENSE" {
			t.Fatalf("zoxide notices: %v", found)
		}
		if b, _ := os.ReadFile(filepath.Join(dst, "licenses/zoxide/LICENSE")); string(b) != "mit" {
			t.Fatalf("LICENSE: %q", b)
		}
	})
	t.Run("pinact third_party_licenses do not shadow the project license", func(t *testing.T) {
		_, found, err := extractLayout(t, "pinact",
			member{"LICENSE", "pinact license", 0644, 0},
			member{"README.md", "readme", 0644, 0},
			member{"third_party_licenses/github.com/spf13/cobra/LICENSE.txt", "apache", 0444, 0},
			member{"third_party_licenses/github.com/hashicorp/go-version/LICENSE", "mpl", 0444, 0},
			member{"third_party_licenses/go/LICENSE", "bsd", 0444, 0},
			member{"pinact", "exe", 0755, 0},
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range found {
			if f == "LICENSE" || f == "README.md" {
				t.Fatalf("pinact top-level file leaked unexpectedly: %v", found)
			}
		}
	})
}

// A basename-matched archive notice must never silently pick one of several
// same-named archive files.
func TestArchiveNoticeBasenameCollisionFails(t *testing.T) {
	_, _, err := extractLayout(t, "zoxide",
		member{"LICENSE", "top", 0644, 0},
		member{"vendor/LICENSE", "nested", 0644, 0},
		member{"zoxide", "exe", 0755, 0},
	)
	if err == nil {
		t.Fatal("colliding LICENSE basenames were accepted")
	}
}

func TestPinactRetainsThirdPartyLicenseTree(t *testing.T) {
	dst, found, err := extractLayout(t, "pinact",
		member{"LICENSE", "pinact license", 0644, 0},
		member{"README.md", "readme", 0644, 0},
		member{"third_party_licenses/", "", 0755, tar.TypeDir},
		member{"third_party_licenses/github.com/spf13/cobra/LICENSE.txt", "apache", 0444, 0},
		member{"third_party_licenses/github.com/hashicorp/go-version/LICENSE", "mpl", 0444, 0},
		member{"third_party_licenses/go/LICENSE", "bsd", 0444, 0},
		member{"other/LICENSE", "outside the tree", 0644, 0},
		member{"pinact", "exe", 0755, 0},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "third_party_licenses/github.com/hashicorp/go-version/LICENSE,third_party_licenses/github.com/spf13/cobra/LICENSE.txt,third_party_licenses/go/LICENSE"
	if strings.Join(found, ",") != want {
		t.Fatalf("pinact notices: %v", found)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "licenses/pinact/third_party_licenses/go/LICENSE")); string(b) != "bsd" {
		t.Fatalf("content: %q", b)
	}
	info, err := os.Stat(filepath.Join(dst, "licenses/pinact/third_party_licenses/go/LICENSE"))
	if err != nil || info.Mode().Perm() != 0644 {
		t.Fatalf("mode: %v %v", info, err)
	}
}
