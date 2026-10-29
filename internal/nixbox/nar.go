package nixbox

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// NARHash returns the SHA-256 and byte size of the Nix archive (NAR)
// serialization of a file, symlink or directory tree, like `nix hash path`.
func NARHash(path string) ([]byte, int64, error) {
	h := sha256.New()
	w := &narWriter{w: h}
	w.str("nix-archive-1")
	if err := w.node(path); err != nil {
		return nil, 0, err
	}
	if w.err != nil {
		return nil, 0, w.err
	}
	return h.Sum(nil), w.n, nil
}

type narWriter struct {
	w   io.Writer
	n   int64
	err error
}

func (w *narWriter) write(p []byte) {
	if w.err != nil {
		return
	}
	n, err := w.w.Write(p)
	w.n += int64(n)
	w.err = err
}

func (w *narWriter) pad(size int64) {
	if rem := size % 8; rem != 0 {
		w.write(make([]byte, 8-rem))
	}
}

func (w *narWriter) str(s string) {
	var l [8]byte
	binary.LittleEndian.PutUint64(l[:], uint64(len(s)))
	w.write(l[:])
	w.write([]byte(s))
	w.pad(int64(len(s)))
}

func (w *narWriter) node(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	w.str("(")
	w.str("type")
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		w.str("symlink")
		w.str("target")
		w.str(target)
	case info.Mode().IsRegular():
		w.str("regular")
		if info.Mode()&0o100 != 0 {
			w.str("executable")
			w.str("")
		}
		w.str("contents")
		var l [8]byte
		binary.LittleEndian.PutUint64(l[:], uint64(info.Size()))
		w.write(l[:])
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		n, err := io.Copy(narSink{w}, f)
		f.Close()
		if err != nil {
			return err
		}
		if n != info.Size() {
			return fmt.Errorf("%s changed while hashing", path)
		}
		w.pad(info.Size())
	case info.IsDir():
		w.str("directory")
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		sort.Strings(names)
		for _, name := range names {
			w.str("entry")
			w.str("(")
			w.str("name")
			w.str(name)
			w.str("node")
			if err := w.node(filepath.Join(path, name)); err != nil {
				return err
			}
			w.str(")")
		}
	default:
		return fmt.Errorf("%s: unsupported file type %s", path, info.Mode().Type())
	}
	w.str(")")
	return w.err
}

// narSink forwards raw file contents, which carry no length prefix of their own.
type narSink struct{ w *narWriter }

func (s narSink) Write(p []byte) (int, error) {
	s.w.write(p)
	return len(p), s.w.err
}
