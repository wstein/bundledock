package toolbundle

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
)

// maxBuildSourceSize bounds one extracted source or header (sqlite3.c is about 9 MB).
const maxBuildSourceSize = 64 << 20

var buildFileName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+-]*$`)

// ExtractBuildSources prepares the build directory of a source-build tool from its
// verified source ZIP. It installs, by base name and mode 0644, every listed source
// that does not come from an extra source (extraNames are the extra sources' build
// file names) and every header directly below prefix. Nothing is written unless the
// whole archive passes: it fails closed on an unsafe or duplicate path, a non-regular
// entry, a missing or ambiguous listed source, and on any archive member whose base
// name equals an extra-source build file, compared case-insensitively.
func ExtractBuildSources(root *os.Root, source io.ReaderAt, size int64, prefix string, sources, extraNames []string) error {
	if !strings.HasSuffix(prefix, "/") || ValidPath(strings.TrimSuffix(prefix, "/")) != nil {
		return fmt.Errorf("invalid archive prefix %q", prefix)
	}
	extra := map[string]bool{}
	for _, n := range extraNames {
		extra[strings.ToLower(n)] = true
	}
	wanted, wantedLower := map[string]bool{}, map[string]bool{}
	for _, s := range sources {
		if !strings.HasSuffix(s, ".c") || !buildFileName.MatchString(s) {
			return fmt.Errorf("invalid build source %q", s)
		}
		if !extra[strings.ToLower(s)] {
			wanted[s], wantedLower[strings.ToLower(s)] = true, true
		}
	}
	archive, err := zip.NewReader(source, size)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	selected := map[string]*zip.File{}
	stray := map[string]bool{}
	for _, entry := range archive.File {
		if err := ValidPath(entry.Name); err != nil {
			return err
		}
		name := path.Clean(entry.Name)
		if seen[name] {
			return fmt.Errorf("duplicate ZIP archive path %q", name)
		}
		seen[name] = true
		if entry.Mode().IsDir() {
			continue
		}
		if !entry.Mode().IsRegular() {
			return fmt.Errorf("unsupported ZIP archive entry %q", name)
		}
		base := path.Base(name)
		if extra[strings.ToLower(base)] {
			return fmt.Errorf("archive member %q collides with an extra-source build file", name)
		}
		if path.Dir(name)+"/" != prefix {
			if wanted[base] {
				stray[base] = true
			}
			continue
		}
		if !wantedLower[strings.ToLower(base)] && !strings.HasSuffix(strings.ToLower(base), ".h") {
			continue
		}
		if !buildFileName.MatchString(base) {
			return fmt.Errorf("unsafe build file name %q", base)
		}
		if entry.UncompressedSize64 > maxBuildSourceSize {
			return fmt.Errorf("archive member %q too large", name)
		}
		selected[base] = entry
	}
	lower := map[string]string{}
	var order []string
	for base := range selected {
		if other, ok := lower[strings.ToLower(base)]; ok {
			return fmt.Errorf("archive members %q and %q differ only by case", other, base)
		}
		lower[strings.ToLower(base)] = base
		order = append(order, base)
	}
	sort.Strings(order)
	for base := range wanted {
		switch {
		case selected[base] == nil:
			return fmt.Errorf("missing build source %q below %s", base, prefix)
		case stray[base]:
			return fmt.Errorf("ambiguous build source %q: another member has the same name", base)
		}
	}
	for _, base := range order {
		if err := writeZIPMember(root, selected[base], base); err != nil {
			return err
		}
	}
	return nil
}

func writeZIPMember(root *os.Root, entry *zip.File, target string) error {
	f, err := entry.Open()
	if err != nil {
		return err
	}
	writeErr := WriteFile(root, target, &boundedReader{r: f, left: maxBuildSourceSize, name: target}, fs.FileMode(0644))
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// boundedReader fails, instead of truncating, when a member yields more than its limit.
type boundedReader struct {
	r    io.Reader
	left int64
	name string
}

func (b *boundedReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	b.left -= int64(n)
	if b.left < 0 {
		return n, fmt.Errorf("archive member %q too large", b.name)
	}
	return n, err
}
