package ipc

import (
	"fmt"
	"net"
)

func validateLoopbackTCP(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return fmt.Errorf("%w: invalid loopback address", ErrUnavailable)
	}
	parsed := net.ParseIP(host)
	if parsed == nil || !parsed.IsLoopback() {
		return fmt.Errorf("%w: agent address is not loopback", ErrUnavailable)
	}
	return nil
}

func unsupportedTransport() error {
	return fmt.Errorf("%w: unsupported local network", ErrUnavailable)
}

func noTransportCleanup() error {
	return nil
}
