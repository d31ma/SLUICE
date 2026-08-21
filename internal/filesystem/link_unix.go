//go:build !windows

package filesystem

import (
	"io/fs"
)

func isUnsafeLink(_ string, info fs.FileInfo) (bool, error) {
	return info.Mode()&fs.ModeSymlink != 0, nil
}
