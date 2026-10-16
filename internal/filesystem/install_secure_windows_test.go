//go:build windows

package filesystem_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/d31ma/sluice/internal/filesystem"
	"golang.org/x/sys/windows"
)

const (
	windowsInterruptionRoot  = "SLUICE_FS_WINDOWS_INTERRUPTION_ROOT"
	windowsNativeRequired    = "SLUICE_FS_REQUIRE_WINDOWS_NATIVE"
	windowsJunctionRoot      = "SLUICE_FS_WINDOWS_JUNCTION_ROOT"
	windowsENOSPCRoot        = "SLUICE_FS_WINDOWS_ENOSPC_ROOT"
	windowsRenameBoundaryEnv = "SLUICE_FS_WINDOWS_RENAME_BOUNDARY_ROOT"
	windowsRenameBytes       = 16 * 1024 * 1024
)

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

func TestAtomicInstallerRejectsWindowsJunction(t *testing.T) {
	rootPath := os.Getenv(windowsJunctionRoot)
	if rootPath == "" {
		return
	}
	outside := filepath.Join(filepath.Dir(rootPath), "outside")
	installer := newInstaller(t, rootPath)
	content := []byte("must stay contained")
	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "linked/file.txt"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: sha256.Sum256(content),
	})
	if !errors.Is(err, filesystem.ErrUnsafeLink) {
		t.Fatalf("Install() through junction error = %v, want ErrUnsafeLink", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "file.txt")); !os.IsNotExist(err) { //nolint:gosec // Exact sibling of the explicitly supplied native fixture root.
		t.Fatalf("outside destination error = %v, want not-exist", err)
	}
}

func TestAtomicInstallerFailsClosedAfterWindowsACLDenial(t *testing.T) {
	if os.Getenv(windowsNativeRequired) == "" {
		return
	}
	rootPath := t.TempDir()
	parent := filepath.Join(rootPath, "folder")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	installer := newInstaller(t, rootPath)
	denyCurrentWindowsUserWrite(t, parent)
	content := []byte("blocked")

	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "folder/file.txt"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: sha256.Sum256(content),
	})
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("Install() error = %v, want Windows access denial", err)
	}
	if got := filesystem.CodeOf(err); got != filesystem.CodeUnsafeParent {
		t.Fatalf("CodeOf(permission denial) = %q, want %q", got, filesystem.CodeUnsafeParent)
	}
}

func TestAtomicInstallerCleansUpAfterWindowsDiskFull(t *testing.T) {
	volumeRoot := os.Getenv(windowsENOSPCRoot)
	if volumeRoot == "" {
		return
	}
	rootPath, err := os.MkdirTemp(volumeRoot, ".sluice-enospc-")
	if err != nil {
		t.Fatalf("MkdirTemp(qualification volume) error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(rootPath) }) //nolint:gosec // Child directory of the explicitly isolated qualification VHD.
	installer := newInstaller(t, rootPath)
	reserve := filepath.Join(rootPath, "space-reserve")
	writeWindowsFile(t, reserve, 8*1024*1024)
	fillWindowsVolume(t, filepath.Join(rootPath, "filler"))
	if removeErr := os.Remove(reserve); removeErr != nil { //nolint:gosec // Test-owned path beneath the isolated qualification VHD.
		t.Fatalf("Remove(space reserve) error = %v", removeErr)
	}

	content := bytes.Repeat([]byte("w"), 16*1024*1024)
	source := bytes.NewReader(content)
	_, err = installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.bin"),
		Source:         source,
		ExpectedSHA256: sha256.Sum256(content),
	})
	if !isWindowsDiskFull(err) {
		t.Fatalf("Install() error = %v, want Windows disk full", err)
	}
	if got := filesystem.CodeOf(err); got != filesystem.CodeIO {
		t.Fatalf("CodeOf(disk full) = %q, want %q", got, filesystem.CodeIO)
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

func TestAtomicInstallerRecoversAfterWindowsPreRenameTermination(t *testing.T) {
	if os.Getenv(windowsNativeRequired) == "" {
		return
	}
	rootPath := t.TempDir()
	marker := filepath.Join(rootPath, "pre-rename-ready")
	command := exec.Command(os.Args[0], "-test.run=^TestAtomicInstallerWindowsPreRenameChild$") //nolint:gosec // Exact current test binary; no shell.
	command.Env = append(os.Environ(), windowsRenameBoundaryEnv+"="+rootPath)
	if err := command.Start(); err != nil {
		t.Fatalf("Start(pre-rename child) error = %v", err)
	}
	waitForWindowsFile(t, marker)
	if _, err := os.Stat(filepath.Join(rootPath, "file.bin")); !os.IsNotExist(err) {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("destination reached rename before kill: %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("Kill(pre-rename child) error = %v", err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("pre-rename child exited successfully")
	}
	assertWindowsTemporarySize(t, rootPath, windowsRenameBytes)

	installer := newInstaller(t, rootPath)
	content := []byte("recovered after pre-rename termination")
	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.bin"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: sha256.Sum256(content),
	})
	if err != nil {
		t.Fatalf("Install() after pre-rename termination error = %v", err)
	}
	assertNoTemporaryFiles(t, rootPath)
}

func TestAtomicInstallerWindowsPreRenameChild(t *testing.T) {
	rootPath := os.Getenv(windowsRenameBoundaryEnv)
	if rootPath == "" {
		return
	}
	content := bytes.Repeat([]byte("r"), windowsRenameBytes)
	installer := newInstaller(t, rootPath)
	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path: mustLogicalPath(t, "file.bin"),
		Source: &windowsPreRenameReader{
			remaining: windowsRenameBytes,
			marker:    filepath.Join(rootPath, "pre-rename-ready"),
		},
		ExpectedSHA256: sha256.Sum256(content),
	})
	t.Fatalf("Install() crossed the pre-rename observation window: %v", err)
}

func denyCurrentWindowsUserWrite(t *testing.T, path string) {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo() error = %v", err)
	}
	original, _, err := descriptor.DACL()
	if err != nil {
		t.Fatalf("DACL() error = %v", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser() error = %v", err)
	}
	var pinner runtime.Pinner
	pinner.Pin(user.User.Sid)
	defer pinner.Unpin()
	blocked, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_WRITE,
		AccessMode:        windows.DENY_ACCESS,
		Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, original)
	if err != nil {
		t.Fatalf("ACLFromEntries() error = %v", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
		nil,
		nil,
		blocked,
		nil,
	); err != nil {
		t.Fatalf("SetNamedSecurityInfo(deny) error = %v", err)
	}
	t.Cleanup(func() {
		if err := windows.SetNamedSecurityInfo(
			path,
			windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION,
			nil,
			nil,
			original,
			nil,
		); err != nil {
			t.Errorf("SetNamedSecurityInfo(restore) error = %v", err)
		}
		runtime.KeepAlive(descriptor)
	})
}

func fillWindowsVolume(t *testing.T, path string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // Test-owned isolated VHD.
	if err != nil {
		t.Fatalf("OpenFile(filler) error = %v", err)
	}
	defer func() { _ = file.Close() }()
	block := bytes.Repeat([]byte("f"), 1024*1024)
	for range 256 {
		if _, err := file.Write(block); isWindowsDiskFull(err) {
			return
		} else if err != nil {
			t.Fatalf("Write(filler) error = %v, want Windows disk full", err)
		}
	}
	t.Fatal("isolated VHD accepted 256 MiB without reporting disk full")
}

func writeWindowsFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, bytes.Repeat([]byte("s"), size), 0o600); err != nil { //nolint:gosec // Test-owned isolated VHD.
		t.Fatalf("WriteFile(space reserve) error = %v", err)
	}
}

func isWindowsDiskFull(err error) bool {
	return errors.Is(err, windows.ERROR_DISK_FULL) || errors.Is(err, windows.ERROR_HANDLE_DISK_FULL)
}

func assertWindowsTemporarySize(t *testing.T, directory string, want int64) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, ".sluice-install-*.tmp"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("temporary file count = %d, want 1", len(matches))
	}
	info, err := os.Stat(matches[0])
	if err != nil {
		t.Fatalf("Stat(temporary) error = %v", err)
	}
	if info.Size() != want {
		t.Fatalf("temporary size = %d, want %d", info.Size(), want)
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

type windowsPreRenameReader struct {
	marker    string
	remaining int
	blocked   bool
}

func (reader *windowsPreRenameReader) Read(buffer []byte) (int, error) {
	if reader.remaining > 0 {
		written := min(len(buffer), reader.remaining)
		for index := range written {
			buffer[index] = 'r'
		}
		reader.remaining -= written
		return written, nil
	}
	if !reader.blocked {
		reader.blocked = true
		if err := os.WriteFile(reader.marker, []byte("ready"), 0o600); err != nil {
			return 0, err
		}
	}
	select {}
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

var (
	_ io.Reader = (*windowsInterruptingReader)(nil)
	_ io.Reader = (*windowsPreRenameReader)(nil)
)
