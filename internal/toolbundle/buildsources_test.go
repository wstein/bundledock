package toolbundle

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const amalgamation = "sqlite-amalgamation-3540100/"

func buildSourceArchive(t *testing.T, extra ...member) []byte {
	t.Helper()
	members := []member{
		{amalgamation, "", 0755, 0},
		{amalgamation + "shell.c", "shell", 0644, 0},
		{amalgamation + "sqlite3.c", "sqlite3", 0644, 0},
		{amalgamation + "sqlite3.h", "header", 0644, 0},
		{amalgamation + "sqlite3ext.h", "ext", 0644, 0},
	}
	return zipArchive(t, append(members, extra...)...)
}

func extractSources(t *testing.T, data []byte, sources, extraNames []string) (string, error) {
	t.Helper()
	dst := t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	err = ExtractBuildSources(root, bytes.NewReader(data), int64(len(data)), amalgamation, sources, extraNames)
	return dst, err
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestExtractBuildSourcesInstallsSourcesAndHeadersByBaseName(t *testing.T) {
	dst, err := extractSources(t, buildSourceArchive(t, member{amalgamation + "unlisted.c", "x", 0644, 0}), []string{"shell.c", "sqlite3.c", "linenoise.c"}, []string{"linenoise.c", "linenoise.h", "LICENSE"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(names(t, dst), " "); got != "shell.c sqlite3.c sqlite3.h sqlite3ext.h" {
		t.Fatalf("extracted %q", got)
	}
	data, _ := os.ReadFile(filepath.Join(dst, "shell.c"))
	info, _ := os.Stat(filepath.Join(dst, "shell.c"))
	if string(data) != "shell" || info.Mode().Perm() != 0644 {
		t.Fatalf("content %q mode %v", data, info.Mode())
	}
}

func TestExtractBuildSourcesFailsClosed(t *testing.T) {
	sources, extra := []string{"shell.c", "sqlite3.c", "linenoise.c"}, []string{"linenoise.c", "linenoise.h", "LICENSE"}
	for name, tc := range map[string]struct {
		archive []byte
		sources []string
		want    string
	}{
		"missing member":             {buildSourceArchive(t), append([]string{"missing.c"}, sources...), "missing"},
		"member at the wrong depth":  {zipArchive(t, member{amalgamation + "sub/shell.c", "x", 0644, 0}, member{amalgamation + "sqlite3.c", "x", 0644, 0}), sources, "missing"},
		"member outside the prefix":  {zipArchive(t, member{"other/shell.c", "x", 0644, 0}, member{amalgamation + "sqlite3.c", "x", 0644, 0}), sources, "missing"},
		"same name at two depths":    {buildSourceArchive(t, member{amalgamation + "sub/shell.c", "x", 0644, 0}), sources, "ambiguous"},
		"collision with extra":       {buildSourceArchive(t, member{amalgamation + "linenoise.c", "evil", 0644, 0}), sources, "collides"},
		"collision case-insensitive": {buildSourceArchive(t, member{amalgamation + "LineNoise.H", "evil", 0644, 0}), sources, "collides"},
		"collision with license":     {buildSourceArchive(t, member{amalgamation + "license", "evil", 0644, 0}), sources, "collides"},
		"collision elsewhere":        {buildSourceArchive(t, member{"docs/linenoise.h", "evil", 0644, 0}), sources, "collides"},
		"zip slip":                   {buildSourceArchive(t, member{"../evil.c", "x", 0644, 0}), sources, "unsafe"},
		"absolute path":              {buildSourceArchive(t, member{"/evil.c", "x", 0644, 0}), sources, "unsafe"},
		"duplicate path":             {buildSourceArchive(t, member{amalgamation + "shell.c", "again", 0644, 0}), sources, "duplicate"},
		"symlink":                    {buildSourceArchive(t, member{amalgamation + "link.h", "shell.c", 0777, tar.TypeSymlink}), sources, "unsupported"},
		"case-variant headers":       {buildSourceArchive(t, member{amalgamation + "SQLITE3.H", "x", 0644, 0}), sources, "case"},
		"unsafe header name":         {buildSourceArchive(t, member{amalgamation + "-x.h", "x", 0644, 0}), sources, "unsafe"},
		"listed source not .c":       {buildSourceArchive(t), []string{"shell.c", "sqlite3.h"}, "source"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := extractSources(t, tc.archive, tc.sources, extra)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v does not mention %q", err, tc.want)
			}
		})
	}
}

func TestExtractBuildSourcesRejectsOversizedMember(t *testing.T) {
	big := strings.Repeat("a", maxBuildSourceSize+1)
	data := zipArchive(t, member{amalgamation + "shell.c", big, 0644, 0}, member{amalgamation + "sqlite3.c", "x", 0644, 0})
	if _, err := extractSources(t, data, []string{"shell.c", "sqlite3.c"}, nil); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized member: %v", err)
	}
}
