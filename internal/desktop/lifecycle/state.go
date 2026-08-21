package lifecycle

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/d31ma/sluice/internal/transport/ipc"
)

const tokenBytes = 32

type privatePaths struct {
	runRoot     string
	tokenFile   string
	sessionFile string
}

func preparePrivateState(stateRoot string) (privatePaths, string, error) {
	if err := ensurePrivateDirectory(stateRoot); err != nil {
		return privatePaths{}, "", fmt.Errorf("prepare Sluice state root: %w", err)
	}
	runRoot := filepath.Join(stateRoot, "run")
	if err := ensurePrivateDirectory(runRoot); err != nil {
		return privatePaths{}, "", fmt.Errorf("prepare Sluice runtime state: %w", err)
	}
	paths := privatePaths{
		runRoot:     runRoot,
		tokenFile:   filepath.Join(runRoot, "api-token"),
		sessionFile: filepath.Join(runRoot, "agent-session.json"),
	}
	token, err := loadOrCreateToken(paths.tokenFile)
	return paths, token, err
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	metadata, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !metadata.IsDir() || metadata.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q is not a real directory", path)
	}
	if runtime.GOOS != "windows" && metadata.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%q is writable by another user", path)
	}
	return nil
}

func loadOrCreateToken(path string) (string, error) {
	if err := ensurePrivateDirectory(filepath.Dir(path)); err != nil {
		return "", fmt.Errorf("prepare local API token directory: %w", err)
	}
	token, err := readPrivateToken(path)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return createPrivateToken(path)
}

func readPrivateToken(path string) (string, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if err := validatePrivateTokenMetadata(pathInfo); err != nil {
		return "", err
	}
	return readVerifiedToken(path, pathInfo)
}

func validatePrivateTokenMetadata(metadata os.FileInfo) error {
	if !metadata.Mode().IsRegular() || metadata.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("local API token file is not private")
	}
	if runtime.GOOS != "windows" && metadata.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("local API token file is not private")
	}
	return nil
}

func readVerifiedToken(path string, pathInfo os.FileInfo) (string, error) {
	// #nosec G304 -- path is fixed beneath the validated private state root and
	// verified by identity immediately after opening.
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(pathInfo, opened) {
		return "", fmt.Errorf("local API token changed while opening")
	}
	return readTokenContents(file)
}

func readTokenContents(file *os.File) (string, error) {
	data, err := io.ReadAll(io.LimitReader(file, 257))
	if err != nil || len(data) > 256 {
		return "", fmt.Errorf("read local API token")
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 32 {
		return "", fmt.Errorf("local API token is invalid")
	}
	return token, nil
}

func createPrivateToken(path string) (string, error) {
	random := make([]byte, tokenBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("create local API token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- validated private state path.
	if errors.Is(err, os.ErrExist) {
		return readPrivateToken(path)
	}
	if err != nil {
		return "", fmt.Errorf("create local API token: %w", err)
	}
	if err := persistToken(file, token); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return token, nil
}

func persistToken(file *os.File, token string) error {
	if _, err := io.WriteString(file, token+"\n"); err != nil {
		_ = file.Close()
		return fmt.Errorf("write local API token: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync local API token: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close local API token: %w", err)
	}
	return nil
}

func descriptorCapability(path string) string {
	descriptor, err := ipc.ReadDescriptor(path)
	if err != nil {
		return ""
	}
	return descriptor.Capability
}
