//go:build !windows

package filesystem

import (
	"errors"
	"io/fs"
)

func classifyRenameError(err error) error {
	if errors.Is(err, fs.ErrExist) {
		return failure(CodeDestinationExists, errorsJoin(ErrDestinationExists, err))
	}
	if isUnsupportedAtomicRename(err) {
		return failure(CodeAtomicRenameUnsupported, errorsJoin(ErrAtomicRenameUnsupported, err))
	}
	return failure(CodeIO, err)
}
