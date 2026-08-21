// Package buildinfo publishes the independently versioned Sluice contracts.
package buildinfo

const (
	localAPIVersion         = 1
	agentProtocolVersion    = 1
	fyloSchemaVersion       = 1
	remoteProtocolVersion   = 1
	diagnosticBundleVersion = 1
)

// These values are replaced through -ldflags in release builds.
var (
	version = "0.0.0-dev"
	commit  = "unknown"
	builtAt = "unknown"
)

// Info identifies one Sluice artifact and every public compatibility boundary
// it speaks.
type Info struct {
	ApplicationVersion      string `json:"applicationVersion"`
	Commit                  string `json:"commit"`
	BuiltAt                 string `json:"builtAt"`
	LocalAPIVersion         int    `json:"localApiVersion"`
	AgentProtocolVersion    int    `json:"agentProtocolVersion"`
	FyloSchemaVersion       int    `json:"fyloSchemaVersion"`
	RemoteProtocolVersion   int    `json:"remoteProtocolVersion"`
	DiagnosticBundleVersion int    `json:"diagnosticBundleVersion"`
}

// Current returns the build identity without reading the environment or
// filesystem, keeping version inspection side-effect free.
func Current() Info {
	return Info{
		ApplicationVersion:      version,
		Commit:                  commit,
		BuiltAt:                 builtAt,
		LocalAPIVersion:         localAPIVersion,
		AgentProtocolVersion:    agentProtocolVersion,
		FyloSchemaVersion:       fyloSchemaVersion,
		RemoteProtocolVersion:   remoteProtocolVersion,
		DiagnosticBundleVersion: diagnosticBundleVersion,
	}
}
