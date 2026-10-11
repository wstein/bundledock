package toolvolume

import (
	"io"
	"os"
)

// InitializeWithProfile explicitly opts into a supported bundle profile.
func InitializeWithProfile(source, target, profile string, output io.Writer) error {
	return initializeProfile(source, target, profile, output, func(src, dst *os.Root) error { return copyBundleProfile(src, dst, profile) })
}
