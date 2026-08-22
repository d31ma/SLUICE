//go:build darwin || linux

package lifecycle

import (
	"os"

	"github.com/d31ma/sluice/internal/transport/ipc"
)

func captureNativeEndpoint(descriptor ipc.Descriptor) fileIdentity {
	if descriptor.Network != "unix" {
		return fileIdentity{}
	}
	metadata, err := os.Lstat(descriptor.Address)
	if err != nil || metadata.Mode()&os.ModeSocket == 0 {
		return fileIdentity{}
	}
	return fileIdentity{path: descriptor.Address, metadata: metadata}
}
