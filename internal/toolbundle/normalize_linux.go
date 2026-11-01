//go:build linux

package toolbundle

import (
	"syscall"
	"unsafe"
)

const atSymlinkNoFollow = 0x100

// setMtime calls utimensat(AT_FDCWD, path, {epoch, epoch}, AT_SYMLINK_NOFOLLOW).
func setMtime(path string, epoch int64) error {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	ts := [2]syscall.Timespec{{Sec: epoch}, {Sec: epoch}}
	atFdcwd := -100
	_, _, errno := syscall.Syscall6(syscall.SYS_UTIMENSAT, uintptr(atFdcwd), uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&ts[0])), atSymlinkNoFollow, 0, 0)
	if errno != 0 {
		return &pathError{path, errno}
	}
	return nil
}

type pathError struct {
	path string
	err  error
}

func (e *pathError) Error() string { return "utimensat " + e.path + ": " + e.err.Error() }
func (e *pathError) Unwrap() error { return e.err }
