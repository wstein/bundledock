package nixbox

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Initialize fills an empty volume with the image tree at source (store,
// var/nix, profile) and verifies it. It mirrors the other bundle carriers: an
// atomic .initializing directory serializes writers and a .ready marker proves a
// complete, verified volume. A second run on a ready volume only verifies. The
// store of the source and of the volume must both match the platform lock by
// NAR hash.
func Initialize(source, target string, p PlatformLock, output io.Writer) (err error) {
	if err := os.Mkdir(filepath.Join(target, ".initializing"), 0o755); err != nil {
		return fmt.Errorf("mount a named volume at %s that no initializer holds: %w", target, err)
	}
	defer func() {
		if cleanup := os.Remove(filepath.Join(target, ".initializing")); err == nil {
			err = cleanup
		}
	}()
	if err := VerifyStore(p, filepath.Join(source, "store")); err != nil {
		return fmt.Errorf("image store: %w", err)
	}
	if _, err := os.Lstat(filepath.Join(source, ".ready")); err == nil {
		return fmt.Errorf("reserved bundle file .ready in the image")
	}
	if _, err := os.Lstat(filepath.Join(source, ".initializing")); err == nil {
		return fmt.Errorf("reserved bundle file .initializing in the image")
	}
	ready, err := os.Lstat(filepath.Join(target, ".ready"))
	switch {
	case err == nil:
		if !ready.Mode().IsRegular() || ready.Size() != 0 {
			return fmt.Errorf("invalid ready marker")
		}
		return verifyVolume(source, target, p)
	case !os.IsNotExist(err):
		return err
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	if len(entries) != 1 || entries[0].Name() != ".initializing" {
		return fmt.Errorf("refusing to initialize a nonempty volume")
	}
	if err := copyTree(source, target); err != nil {
		return err
	}
	if err := verifyVolume(source, target, p); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(target, ".ready"), nil, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(output, "Nix volume initialized")
	return nil
}

func verifyVolume(source, target string, p PlatformLock) error {
	if err := VerifyStore(p, filepath.Join(target, "store")); err != nil {
		return fmt.Errorf("volume store: %w", err)
	}
	return compareTrees(source, target)
}

type node struct {
	kind   fs.FileMode
	mode   fs.FileMode
	link   string
	digest [sha256.Size]byte
}

// snapshot describes a tree by path: type, permission bits, link target and
// content digest. The volume root marker files are skipped.
func snapshot(root string, skipMarkers bool) (map[string]node, error) {
	out := map[string]node{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		if skipMarkers && (rel == ".ready" || rel == ".initializing") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		n := node{kind: info.Mode().Type(), mode: info.Mode().Perm()}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			n.mode = 0
			if n.link, err = os.Readlink(path); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			h := sha256.New()
			_, err = io.Copy(h, f)
			f.Close()
			if err != nil {
				return err
			}
			copy(n.digest[:], h.Sum(nil))
		case info.IsDir():
		default:
			return fmt.Errorf("unsupported file type at %s", rel)
		}
		out[rel] = n
		return nil
	})
	return out, err
}

// compareTrees requires target to hold exactly source's paths with the same
// types, permission bits, link targets and contents (apart from root markers).
func compareTrees(source, target string) error {
	want, err := snapshot(source, false)
	if err != nil {
		return err
	}
	have, err := snapshot(target, true)
	if err != nil {
		return err
	}
	var names []string
	for name := range want {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		got, ok := have[name]
		if !ok {
			return fmt.Errorf("volume lacks %s", name)
		}
		if got != want[name] {
			return fmt.Errorf("volume path %s differs from the image", name)
		}
		delete(have, name)
	}
	for name := range have {
		return fmt.Errorf("volume holds %s, which the image does not", name)
	}
	return nil
}

// copyTree copies regular files, directories and symlinks, keeping permission
// bits. Directories are made writable while filled and get their final mode
// afterwards, so read-only store directories copy as a non-root user too.
func copyTree(source, target string) error {
	type dirMode struct {
		path string
		mode fs.FileMode
	}
	var dirs []dirMode
	err := filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(source, path)
		if rel == "." {
			return nil
		}
		dst := filepath.Join(target, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			if err := os.Mkdir(dst, 0o755); err != nil {
				return err
			}
			dirs = append(dirs, dirMode{dst, info.Mode().Perm()})
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, dst)
		case info.Mode().IsRegular():
			return copyFile(path, dst, info.Mode().Perm())
		default:
			return fmt.Errorf("unsupported file type at %s", rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i].path, dirs[i].mode); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}
