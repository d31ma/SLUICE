//go:build darwin || linux

package filesystem_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/d31ma/sluice/internal/filesystem"
	"golang.org/x/sys/unix"
)

const (
	forcedInterruptionRoot = "SLUICE_FS_INTERRUPTION_ROOT"
	writeLimitRoot         = "SLUICE_FS_WRITE_LIMIT_ROOT"
	enospcRoot             = "SLUICE_FS_ENOSPC_ROOT"
	requireAPFSCollisions  = "SLUICE_FS_REQUIRE_APFS_COLLISIONS"
	requireRenameBoundary  = "SLUICE_FS_REQUIRE_RENAME_BOUNDARY"
	renameBoundaryRoot     = "SLUICE_FS_RENAME_BOUNDARY_ROOT"
	renameBoundaryBytes    = 64 * 1024 * 1024
)

func TestAtomicInstallerContainsHostileParentSwap(t *testing.T) {
	rootPath := t.TempDir()
	parent := filepath.Join(rootPath, "folder")
	displaced := filepath.Join(rootPath, "displaced")
	outside := t.TempDir()
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
			Source:         &gatedReader{content: bytes.NewReader(content), ready: ready, proceed: proceed},
			ExpectedSHA256: sha256.Sum256(content),
		})
		result <- err
	}()

	<-ready
	if err := os.Rename(parent, displaced); err != nil {
		t.Fatalf("Rename(parent) error = %v", err)
	}
	if err := os.Symlink(outside, parent); err != nil {
		t.Fatalf("Symlink(replacement) error = %v", err)
	}
	close(proceed)

	err := <-result
	if !errors.Is(err, filesystem.ErrUnsafeLink) && !errors.Is(err, filesystem.ErrUnsafeParent) {
		t.Fatalf("Install() after parent swap error = %v, want unsafe parent or link", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside destination error = %v, want not-exist", err)
	}
	if _, err := os.Stat(filepath.Join(displaced, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("displaced destination error = %v, want not-exist", err)
	}
	assertNoTemporaryFiles(t, displaced)
	assertNoTemporaryFiles(t, outside)
}

func TestAtomicInstallerFailsClosedAfterPermissionLoss(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses ordinary directory permission checks")
	}
	rootPath := t.TempDir()
	parent := filepath.Join(rootPath, "folder")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	installer := newInstaller(t, rootPath)
	if err := os.Chmod(parent, 0); err != nil {
		t.Fatalf("Chmod(0000) error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) }) //nolint:gosec // Directory needs owner execute permission for cleanup.
	content := []byte("blocked")

	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "folder/file.txt"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: sha256.Sum256(content),
	})
	if !errors.Is(err, syscall.EACCES) {
		t.Fatalf("Install() error = %v, want permission denial", err)
	}
	if got := filesystem.CodeOf(err); got != filesystem.CodeUnsafeParent {
		t.Fatalf("CodeOf(permission denial) = %q, want %q", got, filesystem.CodeUnsafeParent)
	}
}

func TestAtomicInstallerRecoversTemporaryFileAfterForcedInterruption(t *testing.T) {
	rootPath := t.TempDir()
	marker := filepath.Join(rootPath, "child-ready")
	command := exec.Command(os.Args[0], "-test.run=^TestAtomicInstallerForcedInterruptionChild$") //nolint:gosec // Exact current test binary; no shell.
	command.Env = append(os.Environ(), forcedInterruptionRoot+"="+rootPath)
	if err := command.Start(); err != nil {
		t.Fatalf("Start(child) error = %v", err)
	}
	waitForFile(t, marker)
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("Kill(child) error = %v", err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("forced-interruption child exited successfully")
	}
	assertTemporaryFileCount(t, rootPath, 1)

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

func TestAtomicInstallerDoesNotDeleteUnmarkedReservedLookingFile(t *testing.T) {
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

func TestAtomicInstallerForcedInterruptionChild(t *testing.T) {
	rootPath := os.Getenv(forcedInterruptionRoot)
	if rootPath == "" {
		return
	}
	installer := newInstaller(t, rootPath)
	marker := filepath.Join(rootPath, "child-ready")
	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.txt"),
		Source:         &interruptingReader{marker: marker},
		ExpectedSHA256: sha256.Sum256([]byte("never completes")),
	})
	t.Fatalf("Install() unexpectedly returned: %v", err)
}

func TestAtomicInstallerCleansUpAfterKernelWriteLimit(t *testing.T) {
	rootPath := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext( //nolint:gosec // Exact current test binary; no shell.
		ctx,
		os.Args[0],
		"-test.run=^TestAtomicInstallerWriteLimitChild$",
		"-test.gocoverdir=",
	)
	command.Env = append(os.Environ(), writeLimitRoot+"="+rootPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("write-limit child error = %v\n%s", err, output)
	}
	assertNoTemporaryFiles(t, rootPath)
}

func TestAtomicInstallerWriteLimitChild(t *testing.T) {
	rootPath := os.Getenv(writeLimitRoot)
	if rootPath == "" {
		return
	}
	signal.Ignore(syscall.SIGXFSZ)
	limit := unix.Rlimit{Cur: 1024, Max: 1024}
	if err := unix.Setrlimit(unix.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatalf("Setrlimit(RLIMIT_FSIZE) error = %v", err)
	}
	installer := newInstaller(t, rootPath)
	content := bytes.Repeat([]byte("x"), 1024*1024)
	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.txt"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: sha256.Sum256(content),
	})
	if got := filesystem.CodeOf(err); got != filesystem.CodeIO {
		t.Fatalf("CodeOf(write-limit error) = %q, want %q (error: %v)", got, filesystem.CodeIO, err)
	}
	destination := filepath.Join(rootPath, "file.txt")
	if _, err := os.Stat(destination); !os.IsNotExist(err) { //nolint:gosec // Root is supplied by the isolated parent test.
		t.Fatalf("destination error = %v, want not-exist", err)
	}
	assertNoTemporaryFiles(t, rootPath)
	os.Exit(0) // The isolated child must exit before its deliberately tiny file limit blocks coverage metadata.
}

func TestAtomicInstallerCleansUpAfterRealENOSPC(t *testing.T) {
	volumeRoot := os.Getenv(enospcRoot)
	if volumeRoot == "" {
		return
	}
	rootPath, err := os.MkdirTemp(volumeRoot, ".sluice-enospc-")
	if err != nil {
		t.Fatalf("MkdirTemp(qualification volume) error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(rootPath) }) //nolint:gosec // Child directory of the explicitly isolated qualification volume.

	reserve := filepath.Join(rootPath, "space-reserve")
	writeRandomFile(t, reserve, 8*1024*1024)
	fillVolume(t, filepath.Join(rootPath, "filler"))
	if removeErr := os.Remove(reserve); removeErr != nil { //nolint:gosec // Test-owned path beneath the isolated qualification volume.
		t.Fatalf("Remove(space reserve) error = %v", removeErr)
	}
	if syncErr := syncFilesystem(); syncErr != nil {
		t.Fatalf("Sync(qualification volume) error = %v", syncErr)
	}

	content := randomBytes(t, 16*1024*1024)
	source := bytes.NewReader(content)
	installer := newInstaller(t, rootPath)
	_, err = installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.bin"),
		Source:         source,
		ExpectedSHA256: sha256.Sum256(content),
	})
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("Install() error = %v, want ENOSPC", err)
	}
	if got := filesystem.CodeOf(err); got != filesystem.CodeIO {
		t.Fatalf("CodeOf(ENOSPC) = %q, want %q", got, filesystem.CodeIO)
	}
	consumed := len(content) - source.Len()
	if consumed == 0 || consumed == len(content) {
		t.Fatalf("source bytes consumed = %d, want a partial write", consumed)
	}
	if _, err := os.Stat(filepath.Join(rootPath, "file.bin")); !os.IsNotExist(err) { //nolint:gosec // Test-owned path.
		t.Fatalf("destination error = %v, want not-exist", err)
	}
	assertNoTemporaryFiles(t, rootPath)
}

func TestAtomicInstallerFailsClosedOnAPFSCollisions(t *testing.T) {
	if os.Getenv(requireAPFSCollisions) == "" {
		return
	}
	tests := []struct {
		name   string
		first  string
		second string
	}{
		{name: "case", first: "Report.txt", second: "report.txt"},
		{name: "normalization", first: "Caf\u00e9.txt", second: "Cafe\u0301.txt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rootPath := t.TempDir()
			installer := newInstaller(t, rootPath)
			installContent(t, installer, test.first, []byte("first version"))
			content := []byte("must not replace")
			_, err := installer.Install(context.Background(), filesystem.InstallRequest{
				Path:           mustLogicalPath(t, test.second),
				Source:         bytes.NewReader(content),
				ExpectedSHA256: sha256.Sum256(content),
			})
			if !errors.Is(err, filesystem.ErrDestinationExists) {
				t.Fatalf("Install(%q after %q) error = %v, want destination collision", test.second, test.first, err)
			}
			got, readErr := os.ReadFile(filepath.Join(rootPath, test.first)) //nolint:gosec // Test-owned APFS path.
			if readErr != nil || string(got) != "first version" {
				t.Fatalf("first version after collision = %q, %v", got, readErr)
			}
		})
	}
}

func TestAtomicInstallerRecoversAfterPreRenameTermination(t *testing.T) {
	if os.Getenv(requireRenameBoundary) == "" {
		return
	}
	rootPath := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestAtomicInstallerPreRenameChild$") //nolint:gosec // Exact current test binary; no shell.
	command.Env = append(os.Environ(), renameBoundaryRoot+"="+rootPath)
	if err := command.Start(); err != nil {
		t.Fatalf("Start(pre-rename child) error = %v", err)
	}
	waitForFullTemporary(t, rootPath, renameBoundaryBytes)
	if err := command.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatalf("Stop(pre-rename child) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootPath, "file.bin")); !os.IsNotExist(err) {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("destination reached rename before stop: %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("Kill(pre-rename child) error = %v", err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("pre-rename child exited successfully")
	}
	assertTemporaryFileCount(t, rootPath, 1)

	installer := newInstaller(t, rootPath)
	installContent(t, installer, "file.bin", []byte("recovered after pre-rename termination"))
	assertNoTemporaryFiles(t, rootPath)
}

func TestAtomicInstallerPreRenameChild(t *testing.T) {
	rootPath := os.Getenv(renameBoundaryRoot)
	if rootPath == "" {
		return
	}
	content := bytes.Repeat([]byte("r"), renameBoundaryBytes)
	installer := newInstaller(t, rootPath)
	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.bin"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: sha256.Sum256(content),
	})
	t.Fatalf("Install() crossed the pre-rename observation window: %v", err)
}

func waitForFullTemporary(t *testing.T, rootPath string, size int64) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		matches, err := filepath.Glob(filepath.Join(rootPath, ".sluice-install-*.tmp"))
		if err != nil {
			t.Fatalf("Glob(pre-rename temporary) error = %v", err)
		}
		if len(matches) == 1 {
			info, statErr := os.Stat(matches[0])
			if statErr == nil && info.Size() == size {
				return
			}
		}
	}
	t.Fatal("timed out before the temporary reached the pre-rename boundary")
}

func fillVolume(t *testing.T, path string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // Test-owned isolated image.
	if err != nil {
		t.Fatalf("OpenFile(filler) error = %v", err)
	}
	defer func() { _ = file.Close() }()
	block := randomBytes(t, 1024*1024)
	for range 256 {
		if _, err := file.Write(block); errors.Is(err, syscall.ENOSPC) {
			return
		} else if err != nil {
			t.Fatalf("Write(filler) error = %v, want ENOSPC", err)
		}
	}
	t.Fatal("isolated volume accepted 256 MiB without ENOSPC")
}

func writeRandomFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, randomBytes(t, size), 0o600); err != nil { //nolint:gosec // Test-owned isolated image.
		t.Fatalf("WriteFile(space reserve) error = %v", err)
	}
}

func randomBytes(t *testing.T, size int) []byte {
	t.Helper()
	value := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		t.Fatalf("ReadFull(random) error = %v", err)
	}
	return value
}

func installContent(t *testing.T, installer *filesystem.AtomicInstaller, path string, content []byte) {
	t.Helper()
	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, path),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: sha256.Sum256(content),
	})
	if err != nil {
		t.Fatalf("Install(%q) error = %v", path, err)
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", filepath.Base(path))
}

func assertTemporaryFileCount(t *testing.T, directory string, want int) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, ".sluice-install-*.tmp"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != want {
		t.Fatalf("temporary file count = %d, want %d", len(matches), want)
	}
}

type gatedReader struct {
	content *bytes.Reader
	ready   chan<- struct{}
	proceed <-chan struct{}
	opened  bool
}

func (reader *gatedReader) Read(buffer []byte) (int, error) {
	if !reader.opened {
		reader.opened = true
		close(reader.ready)
		<-reader.proceed
	}
	return reader.content.Read(buffer)
}

type interruptingReader struct {
	marker string
	wrote  bool
}

func (reader *interruptingReader) Read(buffer []byte) (int, error) {
	if reader.wrote {
		select {}
	}
	reader.wrote = true
	if err := os.WriteFile(reader.marker, []byte("ready"), 0o600); err != nil {
		return 0, err
	}
	return copy(buffer, "partial"), nil
}

var _ io.Reader = (*interruptingReader)(nil)
