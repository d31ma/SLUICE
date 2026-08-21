//go:build !darwin && !linux && !windows

package ipc

import (
	"context"
	"fmt"
	"net"
)

func nativeEndpoint(string, string) (string, string, error) {
	return "", "", fmt.Errorf("native agent IPC is unsupported on this platform")
}

func validateTransportAddress(network, address string) error {
	if network != "tcp" {
		return unsupportedTransport()
	}
	return validateLoopbackTCP(address)
}

func listenTransport(network, address string) (net.Listener, func() error, error) {
	if network != "tcp" {
		return nil, nil, unsupportedTransport()
	}
	listener, err := net.Listen(network, address)
	return listener, noTransportCleanup, err
}

func dialTransport(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, address)
}
