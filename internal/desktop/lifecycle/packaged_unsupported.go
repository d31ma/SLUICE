//go:build !darwin && !linux && !windows

package lifecycle

import runtimeexec "github.com/d31ma/sluice/internal/runtime"

func newNativePackaged(string, string, string) (*Coordinator, error) {
	return nil, runtimeexec.ErrNotPackaged
}
