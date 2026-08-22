//go:build linux

package filesystem

import (
	"errors"
	"syscall"
	"testing"
)

func TestClassifyRenameErrorReportsUnsupportedLinuxFilesystem(t *testing.T) {
	err := classifyRenameError(syscall.EINVAL)
	if !errors.Is(err, ErrAtomicRenameUnsupported) {
		t.Fatalf("classifyRenameError(EINVAL) = %v, want ErrAtomicRenameUnsupported", err)
	}
	if got := CodeOf(err); got != CodeAtomicRenameUnsupported {
		t.Fatalf("CodeOf(EINVAL) = %q, want %q", got, CodeAtomicRenameUnsupported)
	}
}
