//go:build darwin

package filesystem

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

func renameNoReplace(source, destination string) error {
	return unix.RenamexNp(source, destination, unix.RENAME_EXCL)
}

func isUnsupportedAtomicRename(err error) bool {
	return errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.ENOTSUP)
}

func syncDirectory(path string) error {
	directory, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	return errors.Join(unix.Fsync(directory), unix.Close(directory))
}
