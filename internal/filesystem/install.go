package filesystem

import (
	"context"
	"crypto/sha256"
	"errors"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const temporaryFilePattern = ".sluice-install-*.tmp"

// InstallRequest describes one verified, no-replace local installation.
type InstallRequest struct {
	Source         io.Reader
	Path           LogicalPath
	ExpectedSHA256 [sha256.Size]byte
	Mode           fs.FileMode
}

// InstallResult describes bytes durably written before installation returned.
type InstallResult struct {
	BytesWritten int64
	SHA256       [sha256.Size]byte
}

// AtomicInstaller installs content beneath one canonical root.
type AtomicInstaller struct {
	root Root
}

// NewAtomicInstaller creates an installer bound to an existing canonical root.
func NewAtomicInstaller(root Root) (*AtomicInstaller, error) {
	if root.path == "" {
		return nil, failure(CodeInvalidRoot, ErrInvalidRoot)
	}
	if err := requireRealDirectory(root.path); err != nil {
		return nil, failure(CodeInvalidRoot, errorsJoin(ErrInvalidRoot, err))
	}
	return &AtomicInstaller{root: root}, nil
}

// Install writes to a sibling temporary file, verifies SHA-256, flushes the
// file, and atomically renames it without replacing an existing destination.
func (installer *AtomicInstaller) Install(ctx context.Context, request InstallRequest) (InstallResult, error) {
	if err := validateInstallRequest(ctx, request); err != nil {
		return InstallResult{}, err
	}
	destination, parent, err := installer.prepareDestination(request.Path)
	if err != nil {
		return InstallResult{}, err
	}
	temporary, err := os.CreateTemp(parent, temporaryFilePattern)
	if err != nil {
		return InstallResult{}, failure(CodeIO, err)
	}
	return installer.installTemporary(ctx, temporary, destination, parent, request)
}

func (installer *AtomicInstaller) prepareDestination(path LogicalPath) (string, string, error) {
	destination := installer.root.Resolve(path)
	parent := filepath.Dir(destination)
	if err := installer.requireSafeParent(parent); err != nil {
		return "", "", err
	}
	if err := requireAbsentDestination(destination); err != nil {
		return "", "", err
	}
	return destination, parent, nil
}

func (installer *AtomicInstaller) installTemporary(
	ctx context.Context,
	temporary *os.File,
	destination string,
	parent string,
	request InstallRequest,
) (InstallResult, error) {
	temporaryPath := temporary.Name()
	installed := false
	defer func() {
		if !installed {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
	}()

	result, err := writeAndVerify(ctx, temporary, request)
	if err != nil {
		return InstallResult{}, err
	}
	if err := prepareForRename(temporary, requestedMode(request.Mode)); err != nil {
		return InstallResult{}, err
	}
	if err := installer.commitTemporary(ctx, temporaryPath, destination, parent); err != nil {
		return InstallResult{}, err
	}
	installed = true
	return result, nil
}

func (installer *AtomicInstaller) commitTemporary(
	ctx context.Context,
	temporaryPath string,
	destination string,
	parent string,
) error {
	if err := ctx.Err(); err != nil {
		return failure(CodeCanceled, err)
	}
	if err := installer.requireSafeParent(parent); err != nil {
		return err
	}
	if err := requireAbsentDestination(destination); err != nil {
		return err
	}
	if err := renameNoReplace(temporaryPath, destination); err != nil {
		return classifyRenameError(err)
	}
	if err := syncDirectory(parent); err != nil {
		return failure(CodeIO, err)
	}
	return nil
}

func validateInstallRequest(ctx context.Context, request InstallRequest) error {
	if err := ctx.Err(); err != nil {
		return failure(CodeCanceled, err)
	}
	if request.Path.value == "" || request.Source == nil {
		return failure(CodeInvalidRequest, ErrInvalidRequest)
	}
	if request.Mode&^fs.ModePerm != 0 {
		return failure(CodeInvalidRequest, ErrInvalidRequest)
	}
	return nil
}

func requestedMode(mode fs.FileMode) fs.FileMode {
	if mode == 0 {
		return 0o600
	}
	return mode.Perm()
}

func writeAndVerify(ctx context.Context, destination *os.File, request InstallRequest) (InstallResult, error) {
	digest := sha256.New()
	written, err := copyWithContext(ctx, destination, digest, request.Source)
	if err != nil {
		return InstallResult{}, err
	}
	actual := digestArray(digest)
	if actual != request.ExpectedSHA256 {
		return InstallResult{}, failure(CodeDigestMismatch, ErrDigestMismatch)
	}
	return InstallResult{BytesWritten: written, SHA256: actual}, nil
}

func copyWithContext(ctx context.Context, file io.Writer, digest hash.Hash, source io.Reader) (int64, error) {
	reader := &contextReader{ctx: ctx, source: source}
	written, err := io.CopyBuffer(io.MultiWriter(file, digest), reader, make([]byte, 128*1024))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 0, failure(CodeCanceled, err)
		}
		return 0, failure(CodeIO, err)
	}
	return written, nil
}

func digestArray(digest hash.Hash) [sha256.Size]byte {
	var result [sha256.Size]byte
	copy(result[:], digest.Sum(nil))
	return result
}

func prepareForRename(file *os.File, mode fs.FileMode) error {
	if err := file.Chmod(mode); err != nil {
		return failure(CodeIO, err)
	}
	if err := file.Sync(); err != nil {
		return failure(CodeIO, err)
	}
	if err := file.Close(); err != nil {
		return failure(CodeIO, err)
	}
	return nil
}

func (installer *AtomicInstaller) requireSafeParent(parent string) error {
	relative, err := filepath.Rel(installer.root.path, parent)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return failure(CodeUnsafeParent, errorsJoin(ErrUnsafeParent, err))
	}
	current := installer.root.path
	if err := requireRealDirectory(current); err != nil {
		return err
	}
	if relative == "." {
		return nil
	}
	for _, segment := range splitHostPath(relative) {
		current = filepath.Join(current, segment)
		if err := requireRealDirectory(current); err != nil {
			return err
		}
	}
	return nil
}

func splitHostPath(path string) []string {
	return strings.Split(filepath.ToSlash(path), "/")
}

func requireRealDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return failure(CodeUnsafeParent, errorsJoin(ErrUnsafeParent, err))
	}
	unsafe, err := isUnsafeLink(path, info)
	if err != nil {
		return failure(CodeIO, err)
	}
	if unsafe {
		return failure(CodeUnsafeLink, ErrUnsafeLink)
	}
	if !info.IsDir() {
		return failure(CodeUnsafeParent, ErrUnsafeParent)
	}
	return nil
}

func requireAbsentDestination(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		unsafe, inspectErr := isUnsafeLink(path, info)
		if inspectErr != nil {
			return failure(CodeIO, inspectErr)
		}
		if unsafe {
			return failure(CodeUnsafeLink, ErrUnsafeLink)
		}
		return failure(CodeDestinationExists, ErrDestinationExists)
	}
	if os.IsNotExist(err) {
		return nil
	}
	return failure(CodeIO, err)
}

func classifyRenameError(err error) error {
	if os.IsExist(err) {
		return failure(CodeDestinationExists, errorsJoin(ErrDestinationExists, err))
	}
	if isUnsupportedAtomicRename(err) {
		return failure(CodeAtomicRenameUnsupported, errorsJoin(ErrAtomicRenameUnsupported, err))
	}
	return failure(CodeIO, err)
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	count, err := reader.source.Read(buffer)
	if contextErr := reader.ctx.Err(); contextErr != nil {
		return count, contextErr
	}
	return count, err
}
