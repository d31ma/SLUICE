//go:build !darwin

package filesystem

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const temporaryFilePattern = ".sluice-install-*.tmp"

func (installer *AtomicInstaller) installPlatform(
	ctx context.Context,
	request InstallRequest,
) (InstallResult, error) {
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
