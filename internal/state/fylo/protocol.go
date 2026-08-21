package fylo

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	defaultMaxRequestBytes  = 1024 * 1024
	defaultMaxResponseBytes = 8 * 1024 * 1024
)

var (
	// ErrCompatibility means the runtime cannot safely open Sluice state.
	ErrCompatibility = errors.New("fylo runtime is incompatible")
	// ErrRootOwned means another live process holds the Fylo root lease.
	ErrRootOwned = errors.New("fylo root already has a live owner")
	// ErrNotRunning means an operation requires a started supervisor.
	ErrNotRunning = errors.New("fylo supervisor is not running")
)

// Requirements is the exact Fylo contract Sluice accepts.
type Requirements struct {
	RuntimeVersion          string
	ProtocolVersion         int
	QueryPaginationVersion  int
	DocumentBucketsVersion  int
	ServerlessQueueVersion  int
	MachineAccessVersion    int
	MinimumMaxRequestBytes  int
	MinimumMaxResponseBytes int
}

// SluiceRequirements returns the Phase 1 runtime contract.
func SluiceRequirements(runtimeVersion string) Requirements {
	return Requirements{
		RuntimeVersion:          runtimeVersion,
		ProtocolVersion:         1,
		QueryPaginationVersion:  1,
		DocumentBucketsVersion:  1,
		ServerlessQueueVersion:  1,
		MachineAccessVersion:    1,
		MinimumMaxRequestBytes:  defaultMaxRequestBytes,
		MinimumMaxResponseBytes: defaultMaxResponseBytes,
	}
}

// Handshake is the side-effect-free identity and capability report returned by
// the live machine loop.
type Handshake struct {
	RuntimeVersion  string             `json:"runtimeVersion"`
	Commit          string             `json:"commit"`
	BuildKind       string             `json:"buildKind"`
	BuildTarget     string             `json:"buildTarget"`
	Machine         MachineContract    `json:"machine"`
	ProtocolVersion int                `json:"protocolVersion"`
	Capabilities    CapabilityContract `json:"capabilities"`
}

// MachineContract records negotiated framing bounds.
type MachineContract struct {
	Framing          string `json:"framing"`
	Encoding         string `json:"encoding"`
	Delimiter        string `json:"delimiter"`
	MaxRequestBytes  int    `json:"maxRequestBytes"`
	MaxResponseBytes int    `json:"maxResponseBytes"`
}

// CapabilityContract records the capability versions required by Sluice.
type CapabilityContract struct {
	Handshake       bool              `json:"handshake"`
	ExclusiveRoot   bool              `json:"exclusiveRoot"`
	QueryPagination VersionCapability `json:"queryPagination"`
	DocumentBuckets VersionCapability `json:"documentBuckets"`
	ServerlessQueue VersionCapability `json:"serverlessQueue"`
	MachineAccess   VersionCapability `json:"machineAccess"`
}

// VersionCapability is a versioned Fylo feature family.
type VersionCapability struct {
	Version int `json:"version"`
}

type response struct { //nolint:govet // Private wire order mirrors machine envelopes.
	Result          json.RawMessage `json:"result"`
	Error           *machineError   `json:"error"`
	Op              string          `json:"op"`
	ProtocolVersion int             `json:"protocolVersion"`
	OK              bool            `json:"ok"`
}

type machineError struct {
	Code string `json:"code"`
}

func (requirements Requirements) validate(handshake Handshake) error {
	checks := []struct { //nolint:govet // Short-lived validation table prioritizes readability.
		name string
		got  any
		want any
	}{
		{"runtime version", handshake.RuntimeVersion, requirements.RuntimeVersion},
		{"protocol version", handshake.ProtocolVersion, requirements.ProtocolVersion},
		{"framing", handshake.Machine.Framing, "ndjson"},
		{"encoding", handshake.Machine.Encoding, "utf-8"},
		{"delimiter", handshake.Machine.Delimiter, "LF"},
		{"query pagination", handshake.Capabilities.QueryPagination.Version, requirements.QueryPaginationVersion},
		{"document buckets", handshake.Capabilities.DocumentBuckets.Version, requirements.DocumentBucketsVersion},
		{"serverless queue", handshake.Capabilities.ServerlessQueue.Version, requirements.ServerlessQueueVersion},
		{"machine access", handshake.Capabilities.MachineAccess.Version, requirements.MachineAccessVersion},
	}
	for _, check := range checks {
		if check.got != check.want {
			return fmt.Errorf("%w: %s is %v, require %v", ErrCompatibility, check.name, check.got, check.want)
		}
	}
	if !handshake.Capabilities.Handshake || !handshake.Capabilities.ExclusiveRoot {
		return fmt.Errorf("%w: required handshake or exclusive-root capability is absent", ErrCompatibility)
	}
	if handshake.Machine.MaxRequestBytes < requirements.MinimumMaxRequestBytes ||
		handshake.Machine.MaxResponseBytes < requirements.MinimumMaxResponseBytes {
		return fmt.Errorf("%w: machine frame limits are below the Sluice minimum", ErrCompatibility)
	}
	return nil
}
