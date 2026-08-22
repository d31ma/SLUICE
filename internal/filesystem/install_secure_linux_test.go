//go:build linux

package filesystem_test

import "golang.org/x/sys/unix"

func syncFilesystem() error {
	unix.Sync()
	return nil
}
