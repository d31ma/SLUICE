//go:build windows

package filesystem_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/d31ma/sluice/internal/filesystem"
)

const windowsInterruptionRoot = "SLUICE_FS_WINDOWS_INTERRUPTION_ROOT"

func TestAtomicInstallerPreventsWindowsParentSwapDuringInstall(t *testing.T) {
	rootPath := t.TempDir()
	parent := filepath.Join(rootPath, "folder")
	displaced := filepath.Join(rootPath, "displaced")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	installer := newInstaller(t, rootPath)
	content := []byte("contained content")
	ready := make(chan struct{})
	proceed := make(chan struct{})
	result := make(chan error, 1)
	path := mustLogicalPath(t, "folder/file.txt")

	go func() {
		_, err := installer.Install(context.Background(), filesystem.InstallRequest{
			Path:           path,
			Source:         &windowsGatedReader{content: bytes.NewReader(content), ready: ready, proceed: proceed},
			ExpectedSHA256: sha256.Sum256(content),
		})
		result <- err
	}()

	<-ready
	if err := os.Rename(parent, displaced); err == nil {
		close(proceed)
		<-result
		t.Fatal("parent rename succeeded while secure installer held the directory")
	}
	close(proceed)
	if err := <-result; err != nil {
		t.Fatalf("Install() after blocked parent swap error = %v", err)
	}
	installed, err := os.ReadFile(filepath.Join(parent, "file.txt")) //nolint:gosec // Test-owned temporary path.
	if err != nil {
		t.Fatalf("ReadFile(installed) error = %v", err)
	}
	if !bytes.Equal(installed, content) {
		t.Fatalf("installed content = %q, want %q", installed, content)
	}
}

func TestAtomicInstallerRecoversWindowsTemporaryAfterForcedInterruption(t *testing.T) {
	rootPath := t.TempDir()
	marker := filepath.Join(rootPath, "child-ready")
	command := exec.Command(os.Args[0], "-test.run=^TestAtomicInstallerWindowsForcedInterruptionChild$") //nolint:gosec // Exact current test binary; no shell.
	command.Env = append(os.Environ(), windowsInterruptionRoot+"="+rootPath)
	if err := command.Start(); err != nil {
		t.Fatalf("Start(child) error = %v", err)
	}
	waitForWindowsFile(t, marker)
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("Kill(child) error = %v", err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("forced-interruption child exited successfully")
	}
	assertWindowsTemporaryFileCount(t, rootPath, 1)

	installer := newInstaller(t, rootPath)
	content := []byte("recovered")
	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.txt"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: sha256.Sum256(content),
	})
	if err != nil {
		t.Fatalf("Install() after forced interruption error = %v", err)
	}
	assertNoTemporaryFiles(t, rootPath)
}

func TestAtomicInstallerWindowsForcedInterruptionChild(t *testing.T) {
	rootPath := os.Getenv(windowsInterruptionRoot)
	if rootPath == "" {
		return
	}
	installer := newInstaller(t, rootPath)
	marker := filepath.Join(rootPath, "child-ready")
	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.txt"),
		Source:         &windowsInterruptingReader{marker: marker},
		ExpectedSHA256: sha256.Sum256([]byte("never completes")),
	})
	t.Fatalf("Install() unexpectedly returned: %v", err)
}

func TestAtomicInstallerDoesNotDeleteUnmarkedWindowsReservedLookingFile(t *testing.T) {
	rootPath := t.TempDir()
	reservedLooking := filepath.Join(rootPath, ".sluice-install-0123456789abcdef01234567.tmp")
	if err := os.WriteFile(reservedLooking, []byte("user content"), 0o600); err != nil {
		t.Fatalf("WriteFile(reserved-looking) error = %v", err)
	}
	installer := newInstaller(t, rootPath)
	content := []byte("installed")
	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.txt"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: sha256.Sum256(content),
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	got, err := os.ReadFile(reservedLooking) //nolint:gosec // Test-owned temporary path.
	if err != nil {
		t.Fatalf("ReadFile(reserved-looking) error = %v", err)
	}
	if string(got) != "user content" {
		t.Fatalf("reserved-looking file content = %q", got)
	}
}

func waitForWindowsFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", filepath.Base(path))
}

func assertWindowsTemporaryFileCount(t *testing.T, directory string, want int) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, ".sluice-install-*.tmp"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != want {
		t.Fatalf("temporary file count = %d, want %d", len(matches), want)
	}
}

type windowsGatedReader struct {
	content *bytes.Reader
	ready   chan<- struct{}
	proceed <-chan struct{}
	opened  bool
}

func (reader *windowsGatedReader) Read(buffer []byte) (int, error) {
	if !reader.opened {
		reader.opened = true
		close(reader.ready)
		<-reader.proceed
	}
	return reader.content.Read(buffer)
}

type windowsInterruptingReader struct {
	marker string
	wrote  bool
}

func (reader *windowsInterruptingReader) Read(buffer []byte) (int, error) {
	if reader.wrote {
		select {}
	}
	reader.wrote = true
	if err := os.WriteFile(reader.marker, []byte("ready"), 0o600); err != nil {
		return 0, err
	}
	return copy(buffer, "partial"), nil
}

var _ io.Reader = (*windowsInterruptingReader)(nil)
