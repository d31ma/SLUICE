//go:build darwin

package filesystem_test

import "golang.org/x/sys/unix"

func syncFilesystem() error {
	return unix.Sync()
}
