package toolbundle

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"bundledock/internal/toolconfig"
)

// Archives that nest everything below one wrapper directory (tool_1.0_linux_amd64/)
// keep their notice trees: the wrapper is dropped, the tree path below it is retained.
func TestNoticeTreeBelowArchiveRootIsRetained(t *testing.T) {
	p := toolconfig.Package{Install: toolconfig.Install{
		Kind:               "tar.gz",
		Members:            map[string]string{"tool": "bin/tool"},
		ArchiveNotices:     []string{"LICENSE"},
		ArchiveNoticeTrees: []string{"third_party"},
	}}
	data := archive(t,
		member{"tool_1.0_linux_amd64/tool", "exe", 0755, 0},
		member{"tool_1.0_linux_amd64/LICENSE", "root license", 0644, 0},
		member{"tool_1.0_linux_amd64/third_party/dep/LICENSE", "dep license", 0644, 0},
		member{"tool_1.0_linux_amd64/other/third_party/x", "deeper", 0644, 0},
	)
	dst := t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := Extract(root, "tool", p, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{"licenses/tool/third_party/dep/LICENSE": "dep license", "licenses/tool/LICENSE": "root license"} {
		if got, err := os.ReadFile(filepath.Join(dst, file)); err != nil || string(got) != want {
			t.Fatalf("%s: %q %v", file, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "licenses/tool/third_party/x")); !os.IsNotExist(err) {
		t.Fatalf("a tree deeper than the archive root must not match: %v", err)
	}
}
