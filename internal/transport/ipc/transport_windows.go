//go:build windows

package ipc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"path/filepath"
	"regexp"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

var privatePipeAddress = regexp.MustCompile(`^\\\\\.\\pipe\\sluice-agent-[0-9a-f]{32}$`)

func nativeEndpoint(sessionFile, _ string) (string, string, error) {
	clean, err := descriptorPath(sessionFile)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256([]byte(filepath.Clean(clean)))
	return "npipe", fmt.Sprintf(`\\.\pipe\sluice-agent-%x`, digest[:16]), nil
}

func validateTransportAddress(network, address string) error {
	switch network {
	case "tcp":
		return validateLoopbackTCP(address)
	case "npipe":
		if !privatePipeAddress.MatchString(address) {
			return fmt.Errorf("%w: invalid named-pipe address", ErrUnavailable)
		}
		return nil
	default:
		return unsupportedTransport()
	}
}

func listenTransport(network, address string) (net.Listener, func() error, error) {
	if network == "tcp" {
		listener, err := net.Listen(network, address)
		return listener, noTransportCleanup, err
	}
	if network != "npipe" {
		return nil, nil, unsupportedTransport()
	}
	securityDescriptor, err := currentUserPipeSecurityDescriptor()
	if err != nil {
		return nil, nil, err
	}
	listener, err := winio.ListenPipe(address, &winio.PipeConfig{
		SecurityDescriptor: securityDescriptor,
		InputBufferSize:    MaxRequestBytes,
		OutputBufferSize:   MaxResponseBytes,
	})
	return listener, noTransportCleanup, err
}

func dialTransport(ctx context.Context, network, address string) (net.Conn, error) {
	if network == "npipe" {
		return winio.DialPipeContext(ctx, address)
	}
	return (&net.Dialer{}).DialContext(ctx, network, address)
}

func currentUserPipeSecurityDescriptor() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("resolve current Windows user for agent IPC ACL: %w", err)
	}
	sid := user.User.Sid.String()
	if sid == "" {
		return "", fmt.Errorf("resolve current Windows user for agent IPC ACL")
	}
	return "D:P(A;;GA;;;" + sid + ")", nil
}
