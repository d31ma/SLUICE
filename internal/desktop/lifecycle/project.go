package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
)

const maximumRouteBytes = 4096

func materializeTachyonProject(
	runRoot string,
	bundle runtimeexec.MacOSBundle,
	sessionFile string,
) (string, error) {
	project, err := os.MkdirTemp(runRoot, "tachyon-project-")
	if err != nil {
		return "", fmt.Errorf("create isolated Tachyon project: %w", err)
	}
	if err := os.Chmod(project, 0o700); err != nil { //nolint:gosec // Project is a private directory, not a file.
		_ = os.RemoveAll(project)
		return "", fmt.Errorf("protect isolated Tachyon project: %w", err)
	}
	if err := writeTachyonProject(project, bundle, sessionFile); err != nil {
		_ = os.RemoveAll(project)
		return "", err
	}
	return project, nil
}

func writeTachyonProject(project string, bundle runtimeexec.MacOSBundle, sessionFile string) error {
	configuration, err := json.Marshal(map[string]any{
		"interpreters": map[string][]string{
			"sluice": {bundle.YonBinary, "--session-file", sessionFile},
		},
	})
	if err != nil {
		return fmt.Errorf("encode Tachyon project configuration: %w", err)
	}
	if writeErr := os.WriteFile(filepath.Join(project, ".tachyonrc"), configuration, 0o600); writeErr != nil {
		return fmt.Errorf("write Tachyon project configuration: %w", writeErr)
	}
	route, err := readRouteAsset(bundle.HealthRoute)
	if err != nil {
		return err
	}
	destination := filepath.Join(project, "server", "routes", "v1", "health", "yon.sluice")
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return fmt.Errorf("create Tachyon health route: %w", err)
	}
	if err := os.WriteFile(destination, route, 0o600); err != nil {
		return fmt.Errorf("write Tachyon health route: %w", err)
	}
	return nil
}

func readRouteAsset(path string) ([]byte, error) {
	// #nosec G304 -- the path came from the already validated application bundle.
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open bundled Tachyon route: %w", err)
	}
	defer func() { _ = file.Close() }()
	route, err := io.ReadAll(io.LimitReader(file, maximumRouteBytes+1))
	if err != nil || len(route) > maximumRouteBytes {
		return nil, fmt.Errorf("read bundled Tachyon route")
	}
	return route, nil
}

func removeProject(path string) error {
	if path == "" {
		return nil
	}
	if err := os.RemoveAll(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove isolated Tachyon project: %w", err)
	}
	return nil
}
