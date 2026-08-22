package filesystem_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/d31ma/sluice/internal/filesystem"
)

const requireNativeLinks = "SLUICE_FS_REQUIRE_NATIVE_LINKS"

func TestAtomicInstallerInstallsVerifiedBytes(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	parent := filepath.Join(rootPath, "folder")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	installer := newInstaller(t, rootPath)
	path := mustLogicalPath(t, "folder/file.txt")
	content := []byte("verified content")
	digest := sha256.Sum256(content)

	result, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           path,
		Source:         bytes.NewReader(content),
		ExpectedSHA256: digest,
		Mode:           0o640,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.BytesWritten != int64(len(content)) || result.SHA256 != digest {
		t.Fatalf("Install() result = %+v", result)
	}
	installed, err := os.ReadFile(filepath.Join(parent, "file.txt")) //nolint:gosec // Test-owned temporary path.
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(installed, content) {
		t.Fatalf("installed content = %q, want %q", installed, content)
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(filepath.Join(parent, "file.txt"))
		if statErr != nil {
			t.Fatalf("Stat() error = %v", statErr)
		}
		if got := info.Mode().Perm(); got != 0o640 {
			t.Fatalf("installed mode = %04o, want 0640", got)
		}
	}
	assertNoTemporaryFiles(t, parent)
}

func TestAtomicInstallerCreatesTemporaryFileBesideDestination(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	parent := filepath.Join(rootPath, "folder")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	installer := newInstaller(t, rootPath)
	content := []byte("same filesystem")
	source := &observingReader{
		directory: parent,
		content:   bytes.NewReader(content),
	}

	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "folder/file.txt"),
		Source:         source,
		ExpectedSHA256: sha256.Sum256(content),
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !source.observedSiblingTemporary {
		t.Fatal("source was read without a sibling temporary file present")
	}
}

func TestAtomicInstallerNeverReplacesAnExistingDestination(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	destination := filepath.Join(rootPath, "file.txt")
	if err := os.WriteFile(destination, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	installer := newInstaller(t, rootPath)
	content := []byte("replacement")

	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.txt"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: sha256.Sum256(content),
	})
	if !errors.Is(err, filesystem.ErrDestinationExists) {
		t.Fatalf("Install() error = %v, want ErrDestinationExists", err)
	}
	installed, readErr := os.ReadFile(destination) //nolint:gosec // Test-owned temporary path.
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if string(installed) != "original" {
		t.Fatalf("existing destination changed to %q", installed)
	}
	assertNoTemporaryFiles(t, rootPath)
}

func TestAtomicInstallerNoReplaceIsRaceSafe(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	installer := newInstaller(t, rootPath)
	const contenders = 12
	path := mustLogicalPath(t, "file.txt")
	start := make(chan struct{})
	errorsByContender := make(chan error, contenders)
	contents := make([][]byte, contenders)
	var wait sync.WaitGroup

	for index := range contenders {
		content := []byte(fmt.Sprintf("contender-%d", index))
		contents[index] = content
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := installer.Install(context.Background(), filesystem.InstallRequest{
				Path:           path,
				Source:         bytes.NewReader(content),
				ExpectedSHA256: sha256.Sum256(content),
			})
			errorsByContender <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByContender)

	successes := 0
	for err := range errorsByContender {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, filesystem.ErrDestinationExists) {
			t.Fatalf("Install() contender error = %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful installs = %d, want 1", successes)
	}
	installed, err := os.ReadFile(filepath.Join(rootPath, "file.txt")) //nolint:gosec // Test-owned temporary path.
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	matched := false
	for _, content := range contents {
		matched = matched || bytes.Equal(installed, content)
	}
	if !matched {
		t.Fatalf("installed unexpected bytes %q", installed)
	}
	assertNoTemporaryFiles(t, rootPath)
}

func TestAtomicInstallerRejectsMismatchedContentAndCleansUp(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	installer := newInstaller(t, rootPath)
	content := []byte("corrupt")

	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.txt"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: sha256.Sum256([]byte("expected")),
	})
	if !errors.Is(err, filesystem.ErrDigestMismatch) {
		t.Fatalf("Install() error = %v, want ErrDigestMismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(rootPath, "file.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("destination stat error = %v, want not-exist", statErr)
	}
	assertNoTemporaryFiles(t, rootPath)
}

func TestAtomicInstallerCleansUpAfterReaderFailure(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	installer := newInstaller(t, rootPath)
	readFailure := errors.New("download failed")

	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.txt"),
		Source:         io.MultiReader(strings.NewReader("partial"), failingReader{err: readFailure}),
		ExpectedSHA256: sha256.Sum256([]byte("irrelevant")),
	})
	if !errors.Is(err, readFailure) {
		t.Fatalf("Install() error = %v, want reader failure", err)
	}
	if got := filesystem.CodeOf(err); got != filesystem.CodeIO {
		t.Fatalf("CodeOf(reader failure) = %q, want %q", got, filesystem.CodeIO)
	}
	assertNoTemporaryFiles(t, rootPath)
}

func TestAtomicInstallerHonorsCancellationAndCleansUp(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	installer := newInstaller(t, rootPath)
	ctx, cancel := context.WithCancel(context.Background())
	source := &cancelingReader{cancel: cancel}

	_, err := installer.Install(ctx, filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "file.txt"),
		Source:         source,
		ExpectedSHA256: sha256.Sum256([]byte("irrelevant")),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Install() error = %v, want context.Canceled", err)
	}
	if got := filesystem.CodeOf(err); got != filesystem.CodeCanceled {
		t.Fatalf("CodeOf(cancellation) = %q, want %q", got, filesystem.CodeCanceled)
	}
	assertNoTemporaryFiles(t, rootPath)
}

func TestAtomicInstallerRejectsLinkComponentsAndDestinations(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	outside := t.TempDir()
	linkParent := filepath.Join(rootPath, "linked")
	if err := os.Symlink(outside, linkParent); err != nil {
		if os.Getenv(requireNativeLinks) != "" {
			t.Fatalf("required native symlink fixture unavailable: %v", err)
		}
		t.Skipf("symlink unavailable: %v", err)
	}
	installer := newInstaller(t, rootPath)
	content := []byte("content")
	digest := sha256.Sum256(content)

	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "linked/file.txt"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: digest,
	})
	if !errors.Is(err, filesystem.ErrUnsafeLink) {
		t.Fatalf("Install() through linked parent error = %v, want ErrUnsafeLink", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "file.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("outside destination stat error = %v, want not-exist", statErr)
	}

	destination := filepath.Join(rootPath, "destination")
	if linkErr := os.Symlink(filepath.Join(outside, "target"), destination); linkErr != nil {
		t.Fatalf("Symlink() destination error = %v", linkErr)
	}
	_, err = installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "destination"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: digest,
	})
	if !errors.Is(err, filesystem.ErrUnsafeLink) {
		t.Fatalf("Install() to linked destination error = %v, want ErrUnsafeLink", err)
	}
	assertNoTemporaryFiles(t, rootPath)
}

func TestAtomicInstallerRequiresAnExistingRealParentDirectory(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	installer := newInstaller(t, rootPath)
	content := []byte("content")

	_, err := installer.Install(context.Background(), filesystem.InstallRequest{
		Path:           mustLogicalPath(t, "missing/file.txt"),
		Source:         bytes.NewReader(content),
		ExpectedSHA256: sha256.Sum256(content),
	})
	if !errors.Is(err, filesystem.ErrUnsafeParent) {
		t.Fatalf("Install() error = %v, want ErrUnsafeParent", err)
	}
	assertNoTemporaryFiles(t, rootPath)
}

func TestAtomicInstallerRejectsInvalidConstructionAndRequests(t *testing.T) {
	t.Parallel()

	if _, err := filesystem.NewAtomicInstaller(filesystem.Root{}); !errors.Is(err, filesystem.ErrInvalidRoot) {
		t.Fatalf("NewAtomicInstaller(zero root) error = %v, want ErrInvalidRoot", err)
	}

	rootPath := t.TempDir()
	installer := newInstaller(t, rootPath)
	path := mustLogicalPath(t, "file.txt")
	requests := []filesystem.InstallRequest{
		{Path: path, ExpectedSHA256: sha256.Sum256(nil)},
		{Path: path, Source: bytes.NewReader(nil), ExpectedSHA256: sha256.Sum256(nil), Mode: os.ModeSetuid},
	}
	for _, request := range requests {
		if _, err := installer.Install(context.Background(), request); !errors.Is(err, filesystem.ErrInvalidRequest) {
			t.Fatalf("Install(invalid request) error = %v, want ErrInvalidRequest", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := installer.Install(ctx, filesystem.InstallRequest{
		Path:           path,
		Source:         bytes.NewReader(nil),
		ExpectedSHA256: sha256.Sum256(nil),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Install(canceled) error = %v, want context.Canceled", err)
	}
	if got := filesystem.CodeOf(err); got != filesystem.CodeCanceled {
		t.Fatalf("CodeOf(canceled request) = %q, want %q", got, filesystem.CodeCanceled)
	}
}

func newInstaller(t *testing.T, rootPath string) *filesystem.AtomicInstaller {
	t.Helper()

	root, err := filesystem.CanonicalizeRoot(rootPath)
	if err != nil {
		t.Fatalf("CanonicalizeRoot() error = %v", err)
	}
	installer, err := filesystem.NewAtomicInstaller(root)
	if err != nil {
		t.Fatalf("NewAtomicInstaller() error = %v", err)
	}
	return installer
}

func mustLogicalPath(t *testing.T, value string) filesystem.LogicalPath {
	t.Helper()

	path, err := filesystem.ParseLogicalPath(value)
	if err != nil {
		t.Fatalf("ParseLogicalPath(%q) error = %v", value, err)
	}
	return path
}

func assertNoTemporaryFiles(t *testing.T, directory string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(directory, ".sluice-install-*.tmp"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

type failingReader struct {
	err error
}

func (reader failingReader) Read([]byte) (int, error) {
	return 0, reader.err
}

type cancelingReader struct {
	cancel func()
	done   bool
}

type observingReader struct {
	content                  *bytes.Reader
	directory                string
	observedSiblingTemporary bool
}

func (reader *observingReader) Read(buffer []byte) (int, error) {
	matches, err := filepath.Glob(filepath.Join(reader.directory, ".sluice-install-*.tmp"))
	if err != nil {
		return 0, err
	}
	if len(matches) == 0 {
		return 0, errors.New("sibling temporary file is missing")
	}
	reader.observedSiblingTemporary = true
	return reader.content.Read(buffer)
}

func (reader *cancelingReader) Read(buffer []byte) (int, error) {
	if reader.done {
		return 0, io.EOF
	}
	reader.done = true
	reader.cancel()
	return copy(buffer, "partial"), nil
}
