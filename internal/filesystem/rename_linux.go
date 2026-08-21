//go:build linux

package filesystem

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

func renameNoReplace(source, destination string) error {
	return unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
}

func isUnsupportedAtomicRename(err error) bool {
	return errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EINVAL)
}

func syncDirectory(path string) error {
	directory, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	return errors.Join(unix.Fsync(directory), unix.Close(directory))
}
