package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseOptionsBoundaries(t *testing.T) {
	required := []string{
		"--fylo-binary", "/opt/fylo",
		"--fylo-root", "/var/lib/sluice/fylo",
		"--fylo-version", "1.2.3",
		"--api-token-file", "/run/sluice/token",
		"--session-file", "/run/sluice/session.json",
	}
	tests := []struct {
		name      string
		arguments []string
	}{
		{name: "unknown option", arguments: []string{"--unknown"}},
		{name: "positional argument", arguments: append(append([]string{}, required...), "extra")},
		{name: "missing required option", arguments: required[:len(required)-2]},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseOptions(test.arguments); err == nil {
				t.Fatal("parseOptions() error = nil, want rejection")
			}
		})
	}

	config, err := parseOptions(append(required, "--listen", "[::1]:0"))
	if err != nil {
		t.Fatalf("parseOptions() valid arguments: %v", err)
	}
	if config.listen != "[::1]:0" {
		t.Fatalf("listen = %q, want [::1]:0", config.listen)
	}
}

func TestReadTokenBoundaries(t *testing.T) {
	if _, err := readToken("relative-token"); err == nil {
		t.Fatal("readToken() accepted a relative path")
	}

	directory := t.TempDir()
	if _, err := readToken(directory); err == nil {
		t.Fatal("readToken() accepted a directory")
	}

	shortPath := writeTokenFile(t, directory, "short", 0o600)
	if _, err := readToken(shortPath); err == nil {
		t.Fatal("readToken() accepted a short token")
	}

	oversizedPath := writeTokenFile(t, directory, strings.Repeat("x", 4097), 0o600)
	if _, err := readToken(oversizedPath); err == nil {
		t.Fatal("readToken() accepted an oversized token")
	}

	validPath := writeTokenFile(t, directory, "  "+strings.Repeat("a", 32)+"\n", 0o600)
	got, err := readToken(validPath)
	if err != nil {
		t.Fatalf("readToken() valid token: %v", err)
	}
	if got != strings.Repeat("a", 32) {
		t.Fatalf("readToken() = %q, want trimmed token", got)
	}
}

func TestReadTokenRejectsBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use Unix permission bits")
	}
	path := writeTokenFile(t, t.TempDir(), strings.Repeat("a", 32), 0o644)
	if _, err := readToken(path); err == nil {
		t.Fatal("readToken() accepted broad permissions")
	}
}

func writeTokenFile(t *testing.T, directory, contents string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(directory, strings.ReplaceAll(t.Name(), "/", "-")+".token")
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatalf("write token fixture: %v", err)
	}
	return path
}
