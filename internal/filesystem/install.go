package filesystem

import (
	"context"
	"crypto/sha256"
	"errors"
	"hash"
	"io"
	"io/fs"
	"os"
)

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
	return installer.installPlatform(ctx, request)
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
