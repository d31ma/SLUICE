//go:build windows

package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestClassifyWindowsRenameError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		want error
		name string
		code ErrorCode
	}{
		{
			name: "file exists",
			err:  windows.ERROR_FILE_EXISTS,
			code: CodeDestinationExists,
			want: ErrDestinationExists,
		},
		{
			name: "already exists",
			err:  windows.ERROR_ALREADY_EXISTS,
			code: CodeDestinationExists,
			want: ErrDestinationExists,
		},
		{
			name: "not supported",
			err:  windows.ERROR_NOT_SUPPORTED,
			code: CodeAtomicRenameUnsupported,
			want: ErrAtomicRenameUnsupported,
		},
		{
			name: "not implemented",
			err:  windows.ERROR_CALL_NOT_IMPLEMENTED,
			code: CodeAtomicRenameUnsupported,
			want: ErrAtomicRenameUnsupported,
		},
		{
			name: "other IO failure",
			err:  windows.ERROR_ACCESS_DENIED,
			code: CodeIO,
			want: windows.ERROR_ACCESS_DENIED,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := classifyWindowsRenameError(test.err)
			if got := CodeOf(err); got != test.code {
				t.Fatalf("CodeOf(classifyWindowsRenameError()) = %q, want %q", got, test.code)
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("classifyWindowsRenameError() = %v, want errors.Is(_, %v)", err, test.want)
			}
		})
	}
}

func TestCleanupInterruptedWindowsFileIgnoresLiveTemporary(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	name := ".sluice-install-0123456789abcdef01234567.tmp"
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("active"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	parent, err := openWindowsDirectory(0, windowsNTPath(directory))
	if err != nil {
		t.Fatalf("openWindowsDirectory() error = %v", err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(parent) })
	held := openWindowsFileWithoutDeleteSharing(t, path)
	t.Cleanup(func() { _ = windows.CloseHandle(held) })

	if err := cleanupInterruptedWindowsFile(parent, name); err != nil {
		t.Fatalf("cleanupInterruptedWindowsFile() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(active temporary) error = %v", err)
	}
}

func openWindowsFileWithoutDeleteSharing(t *testing.T, path string) windows.Handle {
	t.Helper()

	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("UTF16PtrFromString() error = %v", err)
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatalf("CreateFile() error = %v", err)
	}
	return handle
}
