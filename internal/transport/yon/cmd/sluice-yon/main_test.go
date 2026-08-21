package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d31ma/sluice/internal/transport/yon"
)

func TestRunRejectsInvalidOptions(t *testing.T) {
	tests := [][]string{
		{"--unknown"},
		{"first", "second"},
	}
	for _, arguments := range tests {
		if err := run(arguments, strings.NewReader("{}"), &bytes.Buffer{}); err == nil {
			t.Fatalf("run(%q) error = nil, want option rejection", arguments)
		}
	}
}

func TestRunUsesExplicitSessionFile(t *testing.T) {
	request := `{"protocol_version":1,"kind":"request","request_id":"ABC123","operation":"health.get","route":"/v1/health","method":"GET","headers":{}}`
	var output bytes.Buffer
	missing := filepath.Join(t.TempDir(), "missing-session.json")
	if err := run([]string{"--session-file", missing}, strings.NewReader(request), &output); err != nil {
		t.Fatalf("run() error: %v", err)
	}
	var response yon.Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != http.StatusServiceUnavailable {
		t.Fatalf("response status = %d, want %d", response.Status, http.StatusServiceUnavailable)
	}
}
