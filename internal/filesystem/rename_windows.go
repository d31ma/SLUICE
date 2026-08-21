//go:build windows

package filesystem

import (
	"errors"

	"golang.org/x/sys/windows"
)

func renameNoReplace(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
}

func isUnsupportedAtomicRename(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SUPPORTED) || errors.Is(err, windows.ERROR_CALL_NOT_IMPLEMENTED)
}

func syncDirectory(string) error {
	// MOVEFILE_WRITE_THROUGH makes the rename call wait for durable completion.
	return nil
}
