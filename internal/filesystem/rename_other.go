//go:build !darwin && !linux && !windows

package filesystem

func renameNoReplace(string, string) error {
	return ErrAtomicRenameUnsupported
}

func isUnsupportedAtomicRename(error) bool {
	return true
}

func syncDirectory(string) error {
	return ErrAtomicRenameUnsupported
}
