//go:build !linux

package toolbundle

import (
	"os"
	"time"
)

// setMtime is a development fallback; image builds run on Linux. Symlinks are
// skipped here because os.Chtimes would follow them.
func setMtime(path string, epoch int64) error {
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink != 0 {
		return err
	}
	t := time.Unix(epoch, 0)
	return os.Chtimes(path, t, t)
}
