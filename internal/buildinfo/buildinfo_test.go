package buildinfo_test

import (
	"encoding/json"
	"testing"

	"github.com/d31ma/sluice/internal/buildinfo"
)

func TestCurrentPublishesEveryIndependentContractVersion(t *testing.T) {
	t.Parallel()

	info := buildinfo.Current()

	if info.ApplicationVersion == "" {
		t.Fatal("application version must not be empty")
	}
	if info.Commit == "" {
		t.Fatal("commit must not be empty")
	}
	if info.LocalAPIVersion != 1 {
		t.Fatalf("local API version = %d, want 1", info.LocalAPIVersion)
	}
	if info.AgentProtocolVersion != 1 {
		t.Fatalf("agent protocol version = %d, want 1", info.AgentProtocolVersion)
	}
	if info.FyloSchemaVersion != 1 {
		t.Fatalf("Fylo schema version = %d, want 1", info.FyloSchemaVersion)
	}
	if info.RemoteProtocolVersion != 1 {
		t.Fatalf("remote protocol version = %d, want 1", info.RemoteProtocolVersion)
	}
	if info.DiagnosticBundleVersion != 1 {
		t.Fatalf("diagnostic bundle version = %d, want 1", info.DiagnosticBundleVersion)
	}
}

func TestCurrentHasStableJSONFieldNames(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(buildinfo.Current())
	if err != nil {
		t.Fatalf("marshal build info: %v", err)
	}

	const want = `{"applicationVersion":"0.0.0-dev","commit":"unknown","builtAt":"unknown","localApiVersion":1,"agentProtocolVersion":1,"fyloSchemaVersion":1,"remoteProtocolVersion":1,"diagnosticBundleVersion":1}`
	if string(payload) != want {
		t.Fatalf("build info JSON = %s, want %s", payload, want)
	}
}
