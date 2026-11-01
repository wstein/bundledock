package toolbundle

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// NormalizeMtimes sets the modification time of root and of every entry below
// it to the Unix time epoch. Symlinks are changed themselves, never their
// target (on Linux). Image layers are tar archives that carry these times, so
// fixed times are a precondition for identical layer digests from identical
// inputs. File content, mode and ownership stay unchanged.
func NormalizeMtimes(root string, epoch int64) error {
	if epoch < 0 {
		return fmt.Errorf("negative epoch %d", epoch)
	}
	if _, err := os.Lstat(root); err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return setMtime(path, epoch)
	})
}
