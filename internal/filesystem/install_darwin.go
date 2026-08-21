//go:build darwin

package filesystem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	temporaryRandomBytes = 12
	temporaryXattr       = "com.delma.sluice.install-temporary"
)

var temporaryXattrValue = []byte("v1")

func isUnsupportedAtomicRename(err error) bool {
	return errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.ENOTSUP)
}

type secureParent struct {
	components []string
	rootFD     int
	parentFD   int
}

func (installer *AtomicInstaller) installPlatform(
	ctx context.Context,
	request InstallRequest,
) (InstallResult, error) {
	parent, destination, err := installer.openSecureParent(request.Path)
	if err != nil {
		return InstallResult{}, err
	}
	defer parent.close()
	if destinationErr := requireAbsentDestinationAt(parent.parentFD, destination); destinationErr != nil {
		return InstallResult{}, destinationErr
	}
	temporary, temporaryName, err := createTemporaryAt(parent.parentFD)
	if err != nil {
		return InstallResult{}, err
	}
	return installer.installSecureTemporary(ctx, parent, temporary, temporaryName, destination, request)
}

func (installer *AtomicInstaller) openSecureParent(path LogicalPath) (*secureParent, string, error) {
	components := strings.Split(path.value, "/")
	destination := components[len(components)-1]
	parents := components[:len(components)-1]
	rootFD, err := unix.Open(installer.root.path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", classifyParentOpenError(err)
	}
	parentFD, err := descendDirectories(rootFD, parents)
	if err != nil {
		_ = unix.Close(rootFD)
		return nil, "", err
	}
	return &secureParent{rootFD: rootFD, parentFD: parentFD, components: parents}, destination, nil
}

func descendDirectories(rootFD int, components []string) (int, error) {
	current := rootFD
	for _, component := range components {
		next, err := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			err = classifyParentComponentError(current, component, err)
		}
		if current != rootFD {
			_ = unix.Close(current)
		}
		if err != nil {
			return -1, err
		}
		current = next
	}
	return current, nil
}

func classifyParentComponentError(parentFD int, component string, openErr error) error {
	var info unix.Stat_t
	if err := unix.Fstatat(parentFD, component, &info, unix.AT_SYMLINK_NOFOLLOW); err == nil && info.Mode&unix.S_IFMT == unix.S_IFLNK {
		return failure(CodeUnsafeLink, errorsJoin(ErrUnsafeLink, openErr))
	}
	return classifyParentOpenError(openErr)
}

func classifyParentOpenError(err error) error {
	if errors.Is(err, syscall.ELOOP) {
		return failure(CodeUnsafeLink, errorsJoin(ErrUnsafeLink, err))
	}
	return failure(CodeUnsafeParent, errorsJoin(ErrUnsafeParent, err))
}

func (parent *secureParent) close() {
	if parent.parentFD != parent.rootFD {
		_ = unix.Close(parent.parentFD)
	}
	_ = unix.Close(parent.rootFD)
}

func createTemporaryAt(parentFD int) (*os.File, string, error) {
	if err := unix.Flock(parentFD, unix.LOCK_EX); err != nil {
		return nil, "", failure(CodeIO, err)
	}
	defer func() { _ = unix.Flock(parentFD, unix.LOCK_UN) }()
	if err := cleanupInterruptedAt(parentFD); err != nil {
		return nil, "", err
	}
	return openNewTemporaryAt(parentFD)
}

func openNewTemporaryAt(parentFD int) (*os.File, string, error) {
	for range 64 {
		name, err := randomTemporaryName()
		if err != nil {
			return nil, "", failure(CodeIO, err)
		}
		fd, err := unix.Openat(
			parentFD,
			name,
			unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC,
			0o600,
		)
		if errors.Is(err, syscall.EEXIST) {
			continue
		}
		if err != nil {
			return nil, "", failure(CodeIO, err)
		}
		if err := unix.Flock(fd, unix.LOCK_EX); err != nil {
			_ = unix.Close(fd)
			_ = unix.Unlinkat(parentFD, name, 0)
			return nil, "", failure(CodeIO, err)
		}
		if err := unix.Fsetxattr(fd, temporaryXattr, temporaryXattrValue, 0); err != nil {
			_ = unix.Close(fd)
			_ = unix.Unlinkat(parentFD, name, 0)
			return nil, "", failure(CodeIO, err)
		}
		return os.NewFile(uintptr(fd), name), name, nil
	}
	return nil, "", failure(CodeIO, syscall.EEXIST)
}

func randomTemporaryName() (string, error) {
	value := make([]byte, temporaryRandomBytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return ".sluice-install-" + hex.EncodeToString(value) + ".tmp", nil
}

func cleanupInterruptedAt(parentFD int) error {
	names, err := directoryNames(parentFD)
	if err != nil {
		return failure(CodeIO, err)
	}
	for _, name := range names {
		if !isTemporaryName(name) {
			continue
		}
		if err := cleanupInterruptedFileAt(parentFD, name); err != nil {
			return err
		}
	}
	return nil
}

func directoryNames(parentFD int) ([]string, error) {
	duplicate, err := unix.Dup(parentFD)
	if err != nil {
		return nil, err
	}
	directory := os.NewFile(uintptr(duplicate), "sluice-install-parent")
	defer func() { _ = directory.Close() }()
	return directory.Readdirnames(-1)
}

func isTemporaryName(name string) bool {
	return strings.HasPrefix(name, ".sluice-install-") && strings.HasSuffix(name, ".tmp")
}

func cleanupInterruptedFileAt(parentFD int, name string) error {
	exists, err := requireRegularEntryAt(parentFD, name)
	if err != nil || !exists {
		return err
	}
	fd, exists, err := openExistingTemporaryAt(parentFD, name)
	if err != nil || !exists {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	locked, err := tryLockTemporary(fd)
	if err != nil || !locked {
		return err
	}
	return removeMarkedTemporaryAt(parentFD, fd, name)
}

func requireRegularEntryAt(parentFD int, name string) (bool, error) {
	var info unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &info, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return false, nil
		}
		return false, failure(CodeIO, err)
	}
	if info.Mode&unix.S_IFMT != unix.S_IFREG {
		return false, failure(CodeUnsafeLink, ErrUnsafeLink)
	}
	return true, nil
}

func openExistingTemporaryAt(parentFD int, name string) (int, bool, error) {
	fd, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, syscall.ENOENT) {
		return -1, false, nil
	}
	if err != nil {
		return -1, false, failure(CodeIO, err)
	}
	return fd, true, nil
}

func tryLockTemporary(fd int) (bool, error) {
	if lockErr := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); lockErr != nil {
		if errors.Is(lockErr, syscall.EWOULDBLOCK) {
			return false, nil
		}
		return false, failure(CodeIO, lockErr)
	}
	return true, nil
}

func removeMarkedTemporaryAt(parentFD int, fd int, name string) error {
	marked, err := isMarkedTemporary(fd)
	if err != nil {
		return failure(CodeIO, err)
	}
	if !marked {
		return nil
	}
	if unlinkErr := unix.Unlinkat(parentFD, name, 0); unlinkErr != nil && !errors.Is(unlinkErr, syscall.ENOENT) {
		return failure(CodeIO, unlinkErr)
	}
	return nil
}

func isMarkedTemporary(fd int) (bool, error) {
	value := make([]byte, len(temporaryXattrValue))
	size, err := unix.Fgetxattr(fd, temporaryXattr, value)
	if errors.Is(err, unix.ENOATTR) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return size == len(temporaryXattrValue) && string(value) == string(temporaryXattrValue), nil
}

func requireAbsentDestinationAt(parentFD int, name string) error {
	var info unix.Stat_t
	err := unix.Fstatat(parentFD, name, &info, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, syscall.ENOENT) {
		return nil
	}
	if err != nil {
		return failure(CodeIO, err)
	}
	if info.Mode&unix.S_IFMT == unix.S_IFLNK {
		return failure(CodeUnsafeLink, ErrUnsafeLink)
	}
	return failure(CodeDestinationExists, ErrDestinationExists)
}

func (installer *AtomicInstaller) installSecureTemporary(
	ctx context.Context,
	parent *secureParent,
	temporary *os.File,
	temporaryName string,
	destination string,
	request InstallRequest,
) (InstallResult, error) {
	installed := false
	defer func() {
		_ = temporary.Close()
		if !installed {
			_ = unix.Unlinkat(parent.parentFD, temporaryName, 0)
		}
	}()
	result, err := writeAndVerify(ctx, temporary, request)
	if err != nil {
		return InstallResult{}, err
	}
	if err := prepareSecureFile(temporary, requestedMode(request.Mode)); err != nil {
		return InstallResult{}, err
	}
	if err := installer.commitSecureTemporary(ctx, parent, temporary, temporaryName, destination); err != nil {
		return InstallResult{}, err
	}
	installed = true
	return result, nil
}

func prepareSecureFile(file *os.File, mode os.FileMode) error {
	if err := file.Chmod(mode); err != nil {
		return failure(CodeIO, err)
	}
	if err := file.Sync(); err != nil {
		return failure(CodeIO, err)
	}
	return nil
}

func (installer *AtomicInstaller) commitSecureTemporary(
	ctx context.Context,
	parent *secureParent,
	temporary *os.File,
	temporaryName string,
	destination string,
) error {
	if err := ctx.Err(); err != nil {
		return failure(CodeCanceled, err)
	}
	if err := installer.verifySecureParent(parent); err != nil {
		return err
	}
	if err := requireAbsentDestinationAt(parent.parentFD, destination); err != nil {
		return err
	}
	if err := unix.RenameatxNp(parent.parentFD, temporaryName, parent.parentFD, destination, unix.RENAME_EXCL); err != nil {
		return classifyRenameError(err)
	}
	if err := unix.Fremovexattr(int(temporary.Fd()), temporaryXattr); err != nil {
		return failure(CodeIO, err)
	}
	if err := unix.Fsync(parent.parentFD); err != nil {
		return failure(CodeIO, err)
	}
	return nil
}

func (installer *AtomicInstaller) verifySecureParent(parent *secureParent) error {
	root, err := unix.Open(installer.root.path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return classifyParentOpenError(err)
	}
	defer func() { _ = unix.Close(root) }()
	if !sameDirectory(root, parent.rootFD) {
		return failure(CodeUnsafeParent, ErrUnsafeParent)
	}
	current, err := descendDirectories(root, parent.components)
	if err != nil {
		return err
	}
	if current != root {
		defer func() { _ = unix.Close(current) }()
	}
	if !sameDirectory(current, parent.parentFD) {
		return failure(CodeUnsafeParent, ErrUnsafeParent)
	}
	return nil
}

func sameDirectory(first int, second int) bool {
	var firstInfo unix.Stat_t
	var secondInfo unix.Stat_t
	if unix.Fstat(first, &firstInfo) != nil || unix.Fstat(second, &secondInfo) != nil {
		return false
	}
	return firstInfo.Dev == secondInfo.Dev && firstInfo.Ino == secondInfo.Ino
}
