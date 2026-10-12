//go:build unix

package toolbundle

import (
	"fmt"
	"os"
	"syscall"
)

func checkLicenseLinks(info os.FileInfo) error {
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || stat.Nlink != 1 {
		return fmt.Errorf("linked or unsupported license file: %s", info.Name())
	}
	return nil
}
