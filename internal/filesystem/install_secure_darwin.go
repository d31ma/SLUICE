//go:build darwin

package filesystem

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

const temporaryXattr = "com.delma.sluice.install-temporary"

func renameSecureNoReplace(parentFD int, source, destination string) error {
	return unix.RenameatxNp(parentFD, source, parentFD, destination, unix.RENAME_EXCL)
}

func isMissingTemporaryXattr(err error) bool {
	return errors.Is(err, unix.ENOATTR)
}

func isUnsupportedAtomicRename(err error) bool {
	return errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.ENOTSUP)
}
