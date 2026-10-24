package toolconfig

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The notices of the sqlite3 source build: the SQLite public-domain dedication, the
// linenoise BSD-2-Clause text, and the musl and zig runtime licences of the
// statically linked output. The digests identify the retained upstream bytes
// (linenoise at the pinned commit; musl and zig from the pinned zig 0.17.0 archive,
// identical in the x86_64 and aarch64 archives).
func TestSQLite3NoticesRetainTheUpstreamTexts(t *testing.T) {
	const base = "../../bundles/toolbox/licenses/sqlite3"
	for file, want := range map[string]string{
		"LINENOISE-LICENSE": "efe1692aa5b869edaa0ac88f95eb0276c1125304ffc636a299db3ebbab47f62b",
		"MUSL-COPYRIGHT":    "f9bc4423732350eb0b3f7ed7e91d530298476f8fec0c6c427a1c04ade22655af",
		"ZIG-LICENSE":       "5c537d6853e005298a285d508cff9ac7192cea23576c840d485b2b586a7ff177",
	} {
		data, err := os.ReadFile(filepath.Join(base, file))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != want {
			t.Errorf("%s: retained bytes changed: %s", file, got)
		}
	}
	read := func(file string) string {
		data, err := os.ReadFile(filepath.Join(base, file))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	notice := read("NOTICE")
	for _, s := range []string{"public domain", "May you do good and not evil.", "BSD-2-Clause", "LINENOISE-LICENSE", "MIT", "MUSL-COPYRIGHT", "ZIG-LICENSE"} {
		if !strings.Contains(notice, s) {
			t.Errorf("NOTICE lacks %q", s)
		}
	}
	source := read("SOURCE")
	for _, s := range []string{
		"Upstream: https://www.sqlite.org/\n",
		"License: blessing (SQLite",
		"antirez/linenoise",
		"a473823d74b93eab2ba83480df16ed37617493f2",
		"tls-download-sha256",
		"zig 0.17.0",
		"https://ziglang.org/download/index.json",
		"SHA3-256",
	} {
		if !strings.Contains(source, s) {
			t.Errorf("SOURCE lacks %q", s)
		}
	}
}
