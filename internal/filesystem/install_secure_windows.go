//go:build windows

package filesystem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowsTemporaryRandomBytes = 12
	windowsTemporaryMarker      = ".sluice-marker"
)

type secureWindowsParent struct {
	handles    []windows.Handle
	components []string
}

type windowsTemporary struct {
	file      *os.File
	marker    *os.File
	name      string
	installed bool
}

type windowsFileRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func (installer *AtomicInstaller) installPlatform(
	ctx context.Context,
	request InstallRequest,
) (InstallResult, error) {
	parent, destination, err := installer.openSecureWindowsParent(request.Path)
	if err != nil {
		return InstallResult{}, err
	}
	defer parent.close()
	if destinationErr := requireAbsentWindowsDestination(parent.final(), destination); destinationErr != nil {
		return InstallResult{}, destinationErr
	}
	temporary, err := installer.createWindowsTemporary(parent)
	if err != nil {
		return InstallResult{}, err
	}
	defer temporary.close()
	result, err := writeAndVerify(ctx, temporary.file, request)
	if err != nil {
		return InstallResult{}, err
	}
	if err := prepareWindowsTemporary(temporary.file, requestedMode(request.Mode)); err != nil {
		return InstallResult{}, err
	}
	if err := commitWindowsTemporary(ctx, parent.final(), temporary, destination); err != nil {
		return InstallResult{}, err
	}
	return result, nil
}

func (installer *AtomicInstaller) openSecureWindowsParent(path LogicalPath) (*secureWindowsParent, string, error) {
	components := strings.Split(path.value, "/")
	destination := components[len(components)-1]
	parents := components[:len(components)-1]
	root, err := openWindowsDirectory(0, windowsNTPath(installer.root.path))
	if err != nil {
		return nil, "", err
	}
	secure := &secureWindowsParent{handles: []windows.Handle{root}, components: parents}
	for _, component := range parents {
		next, openErr := openWindowsDirectory(secure.final(), component)
		if openErr != nil {
			secure.close()
			return nil, "", openErr
		}
		secure.handles = append(secure.handles, next)
	}
	return secure, destination, nil
}

func windowsNTPath(path string) string {
	cleaned := filepath.Clean(path)
	if strings.HasPrefix(cleaned, `\\?\UNC\`) {
		return `\??\UNC\` + strings.TrimPrefix(cleaned, `\\?\UNC\`)
	}
	if strings.HasPrefix(cleaned, `\\?\`) {
		return `\??\` + strings.TrimPrefix(cleaned, `\\?\`)
	}
	if strings.HasPrefix(cleaned, `\\`) {
		return `\??\UNC\` + strings.TrimPrefix(cleaned, `\\`)
	}
	return `\??\` + cleaned
}

func openWindowsDirectory(root windows.Handle, name string) (windows.Handle, error) {
	handle, err := ntCreateWindowsFile(
		root,
		name,
		windows.FILE_LIST_DIRECTORY|windows.FILE_TRAVERSE|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_OPEN,
		windows.FILE_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_OPEN_FOR_BACKUP_INTENT,
		windows.FILE_ATTRIBUTE_DIRECTORY,
	)
	if err != nil {
		return 0, classifyWindowsParentError(err)
	}
	if err := requireSafeWindowsHandle(handle, true); err != nil {
		_ = windows.CloseHandle(handle)
		return 0, err
	}
	return handle, nil
}

func ntCreateWindowsFile(
	root windows.Handle,
	name string,
	access uint32,
	disposition uint32,
	options uint32,
	attributes uint32,
) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	object := &windows.OBJECT_ATTRIBUTES{
		RootDirectory: root,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	object.Length = uint32(unsafe.Sizeof(*object))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(
		&handle,
		access,
		object,
		&status,
		nil,
		attributes,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		disposition,
		options,
		0,
		0,
	)
	return handle, normalizeWindowsError(err)
}

func requireSafeWindowsHandle(handle windows.Handle, directory bool) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return failure(CodeIO, err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return failure(CodeUnsafeLink, ErrUnsafeLink)
	}
	isDirectory := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if isDirectory != directory {
		return failure(CodeUnsafeParent, ErrUnsafeParent)
	}
	return nil
}

func (parent *secureWindowsParent) final() windows.Handle {
	return parent.handles[len(parent.handles)-1]
}

func (parent *secureWindowsParent) close() {
	for index := len(parent.handles) - 1; index >= 0; index-- {
		_ = windows.CloseHandle(parent.handles[index])
	}
	parent.handles = nil
}

func requireAbsentWindowsDestination(parent windows.Handle, name string) error {
	handle, err := ntCreateWindowsFile(
		parent,
		name,
		windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_OPEN,
		windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT,
		0,
	)
	if isWindowsNotFound(err) {
		return nil
	}
	if err != nil {
		if isWindowsReparseError(err) {
			return failure(CodeUnsafeLink, errorsJoin(ErrUnsafeLink, err))
		}
		return failure(CodeIO, err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return failure(CodeIO, err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return failure(CodeUnsafeLink, ErrUnsafeLink)
	}
	return failure(CodeDestinationExists, ErrDestinationExists)
}

func (installer *AtomicInstaller) createWindowsTemporary(parent *secureWindowsParent) (*windowsTemporary, error) {
	if err := installer.cleanupInterruptedWindows(parent); err != nil {
		return nil, err
	}
	for range 64 {
		name, err := randomWindowsTemporaryName()
		if err != nil {
			return nil, failure(CodeIO, err)
		}
		handle, err := ntCreateWindowsFile(
			parent.final(),
			name,
			windows.FILE_GENERIC_WRITE|windows.FILE_READ_ATTRIBUTES|windows.DELETE,
			windows.FILE_CREATE,
			windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_WRITE_THROUGH,
			windows.FILE_ATTRIBUTE_NORMAL,
		)
		if isWindowsCollision(err) {
			continue
		}
		if err != nil {
			return nil, failure(CodeIO, err)
		}
		temporary := &windowsTemporary{file: os.NewFile(uintptr(handle), name), name: name}
		marker, markerErr := createWindowsMarker(parent.final(), name, handle)
		if markerErr != nil {
			temporary.close()
			return nil, markerErr
		}
		temporary.marker = marker
		return temporary, nil
	}
	return nil, failure(CodeIO, windows.ERROR_FILE_EXISTS)
}

func randomWindowsTemporaryName() (string, error) {
	value := make([]byte, windowsTemporaryRandomBytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return ".sluice-install-" + hex.EncodeToString(value) + ".tmp", nil
}

func createWindowsMarker(parent windows.Handle, temporaryName string, temporary windows.Handle) (*os.File, error) {
	markerValue, err := windowsTemporaryIdentity(temporary)
	if err != nil {
		return nil, failure(CodeIO, err)
	}
	handle, err := ntCreateWindowsFile(
		parent,
		windowsMarkerName(temporaryName),
		windows.FILE_GENERIC_WRITE|windows.DELETE,
		windows.FILE_CREATE,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_WRITE_THROUGH,
		windows.FILE_ATTRIBUTE_HIDDEN,
	)
	if err != nil {
		return nil, failure(CodeIO, err)
	}
	marker := os.NewFile(uintptr(handle), windowsMarkerName(temporaryName))
	written, err := marker.Write(markerValue)
	if err == nil && written != len(markerValue) {
		err = io.ErrShortWrite
	}
	if err != nil {
		_ = deleteWindowsHandle(windows.Handle(marker.Fd()))
		_ = marker.Close()
		return nil, failure(CodeIO, err)
	}
	if err := marker.Sync(); err != nil {
		_ = deleteWindowsHandle(windows.Handle(marker.Fd()))
		_ = marker.Close()
		return nil, failure(CodeIO, err)
	}
	return marker, nil
}

func prepareWindowsTemporary(file *os.File, mode fs.FileMode) error {
	if err := file.Chmod(mode); err != nil {
		return failure(CodeIO, err)
	}
	if err := file.Sync(); err != nil {
		return failure(CodeIO, err)
	}
	return nil
}

func commitWindowsTemporary(
	ctx context.Context,
	parent windows.Handle,
	temporary *windowsTemporary,
	destination string,
) error {
	if err := ctx.Err(); err != nil {
		return failure(CodeCanceled, err)
	}
	if err := requireAbsentWindowsDestination(parent, destination); err != nil {
		return err
	}
	if err := renameWindowsHandleNoReplace(windows.Handle(temporary.file.Fd()), parent, destination); err != nil {
		return classifyWindowsRenameError(err)
	}
	temporary.installed = true
	if err := deleteWindowsHandle(windows.Handle(temporary.marker.Fd())); err != nil {
		return failure(CodeIO, err)
	}
	if err := temporary.marker.Close(); err != nil {
		return failure(CodeIO, err)
	}
	temporary.marker = nil
	if err := temporary.file.Sync(); err != nil {
		return failure(CodeIO, err)
	}
	return nil
}

func renameWindowsHandleNoReplace(file, parent windows.Handle, destination string) error {
	name, err := windows.UTF16FromString(destination)
	if err != nil {
		return err
	}
	nameBytes := (len(name) - 1) * 2
	var layout windowsFileRenameInformation
	buffer := make([]byte, int(unsafe.Offsetof(layout.FileName))+nameBytes)
	information := (*windowsFileRenameInformation)(unsafe.Pointer(&buffer[0])) //nolint:gosec // Buffer uses the documented FILE_RENAME_INFORMATION layout.
	information.RootDirectory = parent
	information.FileNameLength = uint32(nameBytes)                                //nolint:gosec // UTF-16 names are bounded by NewNTUnicodeString well below uint32.
	copy(unsafe.Slice(&information.FileName[0], nameBytes/2), name[:len(name)-1]) //nolint:gosec // Slice is backed by the explicitly sized rename buffer.
	var status windows.IO_STATUS_BLOCK
	return normalizeWindowsError(windows.NtSetInformationFile(
		file,
		&status,
		&buffer[0],
		uint32(len(buffer)), //nolint:gosec // Rename buffer is bounded by the uint16 NT string length.
		windows.FileRenameInformation,
	))
}

func classifyWindowsRenameError(err error) error {
	if isWindowsCollision(err) {
		return failure(CodeDestinationExists, errorsJoin(ErrDestinationExists, err))
	}
	if errors.Is(err, windows.ERROR_NOT_SUPPORTED) || errors.Is(err, windows.ERROR_CALL_NOT_IMPLEMENTED) {
		return failure(CodeAtomicRenameUnsupported, errorsJoin(ErrAtomicRenameUnsupported, err))
	}
	return failure(CodeIO, err)
}

func (installer *AtomicInstaller) cleanupInterruptedWindows(parent *secureWindowsParent) error {
	directory := installer.root.path
	if len(parent.components) > 0 {
		directory = filepath.Join(append([]string{directory}, parent.components...)...)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return failure(CodeIO, err)
	}
	for _, entry := range entries {
		if !isWindowsTemporaryName(entry.Name()) {
			continue
		}
		if err := cleanupInterruptedWindowsFile(parent.final(), entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func isWindowsTemporaryName(name string) bool {
	return strings.HasPrefix(name, ".sluice-install-") && strings.HasSuffix(name, ".tmp")
}

func cleanupInterruptedWindowsFile(parent windows.Handle, name string) error {
	handle, err := ntCreateWindowsFile(
		parent,
		name,
		windows.FILE_READ_ATTRIBUTES|windows.DELETE|windows.SYNCHRONIZE,
		windows.FILE_OPEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT,
		0,
	)
	if isWindowsNotFound(err) || isWindowsSharingViolation(err) {
		return nil
	}
	if err != nil {
		if isWindowsReparseError(err) {
			return failure(CodeUnsafeLink, errorsJoin(ErrUnsafeLink, err))
		}
		return failure(CodeIO, err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	if handleErr := requireSafeWindowsHandle(handle, false); handleErr != nil {
		return handleErr
	}
	marker, marked, err := openMarkedWindowsTemporary(parent, name, handle)
	if marker != nil {
		defer func() { _ = marker.Close() }()
	}
	if err != nil || !marked {
		return err
	}
	if err := deleteWindowsHandle(handle); err != nil {
		return failure(CodeIO, err)
	}
	if err := deleteWindowsHandle(windows.Handle(marker.Fd())); err != nil {
		return failure(CodeIO, err)
	}
	return nil
}

func openMarkedWindowsTemporary(
	parent windows.Handle,
	name string,
	temporary windows.Handle,
) (*os.File, bool, error) {
	expected, err := windowsTemporaryIdentity(temporary)
	if err != nil {
		return nil, false, failure(CodeIO, err)
	}
	handle, err := ntCreateWindowsFile(
		parent,
		windowsMarkerName(name),
		windows.FILE_GENERIC_READ|windows.DELETE,
		windows.FILE_OPEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0,
	)
	if isWindowsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, failure(CodeIO, err)
	}
	marker := os.NewFile(uintptr(handle), windowsMarkerName(name))
	value, readErr := io.ReadAll(io.LimitReader(marker, int64(len(expected)+1)))
	if readErr != nil {
		_ = marker.Close()
		return nil, false, failure(CodeIO, readErr)
	}
	return marker, string(value) == string(expected), nil
}

func windowsMarkerName(temporaryName string) string {
	return temporaryName + windowsTemporaryMarker
}

func windowsTemporaryIdentity(handle windows.Handle) ([]byte, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf(
		"v1:%08x:%08x%08x",
		info.VolumeSerialNumber,
		info.FileIndexHigh,
		info.FileIndexLow,
	)), nil
}

func (temporary *windowsTemporary) close() {
	if temporary.marker != nil {
		if !temporary.installed {
			_ = deleteWindowsHandle(windows.Handle(temporary.marker.Fd()))
		}
		_ = temporary.marker.Close()
		temporary.marker = nil
	}
	if temporary.file == nil {
		return
	}
	if !temporary.installed {
		_ = deleteWindowsHandle(windows.Handle(temporary.file.Fd()))
	}
	_ = temporary.file.Close()
	temporary.file = nil
}

func deleteWindowsHandle(handle windows.Handle) error {
	information := [1]byte{1}
	var status windows.IO_STATUS_BLOCK
	return normalizeWindowsError(windows.NtSetInformationFile(
		handle,
		&status,
		&information[0],
		uint32(len(information)),
		windows.FileDispositionInformation,
	))
}

func classifyWindowsParentError(err error) error {
	if isWindowsReparseError(err) {
		return failure(CodeUnsafeLink, errorsJoin(ErrUnsafeLink, err))
	}
	return failure(CodeUnsafeParent, errorsJoin(ErrUnsafeParent, err))
}

func normalizeWindowsError(err error) error {
	if err == nil {
		return nil
	}
	var status windows.NTStatus
	if errors.As(err, &status) {
		return errors.Join(err, status.Errno())
	}
	return err
}

func isWindowsNotFound(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}

func isWindowsCollision(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS)
}

func isWindowsSharingViolation(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

func isWindowsReparseError(err error) bool {
	return errors.Is(err, windows.ERROR_STOPPED_ON_SYMLINK) ||
		errors.Is(err, windows.ERROR_CANT_ACCESS_FILE) ||
		errors.Is(err, windows.STATUS_REPARSE_POINT_ENCOUNTERED)
}
