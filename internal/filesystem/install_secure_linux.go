//go:build linux

package filesystem

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

const temporaryXattr = "user.com.delma.sluice.install-temporary"

func renameSecureNoReplace(parentFD int, source, destination string) error {
	return unix.Renameat2(parentFD, source, parentFD, destination, unix.RENAME_NOREPLACE)
}

func isMissingTemporaryXattr(err error) bool {
	return errors.Is(err, syscall.ENODATA)
}

func isUnsupportedAtomicRename(err error) bool {
	return errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EINVAL)
}
