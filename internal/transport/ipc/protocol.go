// Package ipc implements the bounded, authenticated local protocol between
// short-lived Yon handlers and the persistent Sluice agent.
package ipc

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	// ProtocolVersion is independent from the Sluice local HTTP API version.
	ProtocolVersion = 1
	// MaxRequestBytes bounds one serialized agent request frame.
	MaxRequestBytes = 1024 * 1024
	// MaxResponseBytes bounds one serialized agent response frame.
	MaxResponseBytes = 4 * 1024 * 1024
	capabilityBytes  = 32
)

var (
	// ErrUnavailable means no live agent accepted the request.
	ErrUnavailable = errors.New("sluice agent is unavailable")
	// ErrProtocolMismatch means the agent IPC versions cannot interoperate.
	ErrProtocolMismatch = errors.New("sluice agent protocol is incompatible")
	// ErrUnauthenticated means the per-agent session capability was rejected.
	ErrUnauthenticated = errors.New("sluice agent session is unauthenticated")
	// ErrFrameTooLarge means a request or response exceeded its fixed bound.
	ErrFrameTooLarge = errors.New("sluice agent frame exceeds the configured limit")
)

// Descriptor is the mode-0600 connection record published by a live agent.
// Its capability authenticates IPC only; it is distinct from the local API
// bearer credential forwarded in HTTP headers.
type Descriptor struct {
	Capability      string `json:"capability"`
	Network         string `json:"network"`
	Address         string `json:"address"`
	ProtocolVersion int    `json:"protocolVersion"`
}

// HTTPResponse is the transport-neutral HTTP response returned by the agent.
type HTTPResponse struct {
	Header     http.Header
	Body       []byte
	StatusCode int
}

type wireRequest struct { //nolint:govet // Field order keeps the envelope readable on the wire.
	Headers         http.Header `json:"headers,omitempty"`
	Body            []byte      `json:"body,omitempty"`
	Capability      string      `json:"capability"`
	RequestID       string      `json:"requestId"`
	Method          string      `json:"method"`
	Target          string      `json:"target"`
	Kind            string      `json:"kind"`
	ProtocolVersion int         `json:"protocolVersion"`
}

type wireResponse struct { //nolint:govet // Field order keeps the envelope readable on the wire.
	Headers         http.Header `json:"headers,omitempty"`
	Body            []byte      `json:"body,omitempty"`
	RequestID       string      `json:"requestId"`
	Kind            string      `json:"kind"`
	ErrorCode       string      `json:"errorCode,omitempty"`
	StatusCode      int         `json:"statusCode"`
	ProtocolVersion int         `json:"protocolVersion"`
}

// NewCapability returns a 256-bit URL-safe per-agent session secret.
func NewCapability() (string, error) {
	random := make([]byte, capabilityBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("create agent session capability: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}

func validateDescriptor(descriptor Descriptor) error {
	if descriptor.ProtocolVersion != ProtocolVersion {
		return ErrProtocolMismatch
	}
	if descriptor.Network != "tcp" {
		return fmt.Errorf("%w: unsupported local network", ErrUnavailable)
	}
	host, port, err := net.SplitHostPort(descriptor.Address)
	if err != nil || port == "" {
		return fmt.Errorf("%w: invalid local address", ErrUnavailable)
	}
	address := net.ParseIP(host)
	if address == nil || !address.IsLoopback() {
		return fmt.Errorf("%w: agent address is not loopback", ErrUnavailable)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(descriptor.Capability)
	if err != nil || len(decoded) != capabilityBytes {
		return fmt.Errorf("%w: invalid session capability", ErrUnavailable)
	}
	return nil
}

func validRequestTarget(target string) bool {
	parsed, err := url.ParseRequestURI(target)
	return err == nil && strings.HasPrefix(target, "/") && !parsed.IsAbs() && parsed.Host == ""
}

func capabilityMatches(presented, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) == 1
}
