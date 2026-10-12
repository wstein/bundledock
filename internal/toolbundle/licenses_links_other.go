//go:build !unix

package toolbundle

import (
	"fmt"
	"os"
)

func checkLicenseLinks(info os.FileInfo) error {
	return fmt.Errorf("license packaging requires Unix file link metadata")
}
