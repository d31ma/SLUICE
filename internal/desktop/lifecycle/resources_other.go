//go:build !darwin && !linux

package lifecycle

import "github.com/d31ma/sluice/internal/transport/ipc"

func captureNativeEndpoint(ipc.Descriptor) fileIdentity {
	return fileIdentity{}
}
