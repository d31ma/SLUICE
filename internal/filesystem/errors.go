package filesystem

import "errors"

// ErrorCode is a stable, machine-readable filesystem failure classification.
type ErrorCode string

const (
	CodeEmptyPath               ErrorCode = "filesystem.path.empty"
	CodeAbsolutePath            ErrorCode = "filesystem.path.absolute"
	CodeInvalidSeparator        ErrorCode = "filesystem.path.invalid_separator"
	CodeEmptySegment            ErrorCode = "filesystem.path.empty_segment"
	CodeDotSegment              ErrorCode = "filesystem.path.dot_segment"
	CodeControlCharacter        ErrorCode = "filesystem.path.control_character"
	CodeInvalidUTF8             ErrorCode = "filesystem.path.invalid_utf8"
	CodeForbiddenCharacter      ErrorCode = "filesystem.path.forbidden_character"
	CodeTrailingDotOrSpace      ErrorCode = "filesystem.path.trailing_dot_or_space"
	CodeReservedName            ErrorCode = "filesystem.path.reserved_name"
	CodeInvalidRoot             ErrorCode = "filesystem.root.invalid"
	CodeInvalidRequest          ErrorCode = "filesystem.install.invalid_request"
	CodeUnsafeParent            ErrorCode = "filesystem.install.unsafe_parent"
	CodeUnsafeLink              ErrorCode = "filesystem.install.unsafe_link"
	CodeDestinationExists       ErrorCode = "filesystem.install.destination_exists"
	CodeDigestMismatch          ErrorCode = "filesystem.install.digest_mismatch"
	CodeCanceled                ErrorCode = "filesystem.install.canceled"
	CodeIO                      ErrorCode = "filesystem.install.io"
	CodeAtomicRenameUnsupported ErrorCode = "filesystem.install.atomic_rename_unsupported"
)

var (
	ErrInvalidRoot             = errors.New("invalid filesystem root")
	ErrInvalidRequest          = errors.New("invalid installation request")
	ErrUnsafeParent            = errors.New("unsafe destination parent")
	ErrUnsafeLink              = errors.New("unsafe filesystem link")
	ErrDestinationExists       = errors.New("destination exists")
	ErrDigestMismatch          = errors.New("content digest mismatch")
	ErrAtomicRenameUnsupported = errors.New("atomic no-replace rename unsupported")
)

// Error carries a stable code without exposing a sensitive absolute path.
type Error struct {
	cause error
	code  ErrorCode
}

func (err *Error) Error() string {
	return string(err.code)
}

func (err *Error) Unwrap() error {
	return err.cause
}

// Code returns the stable failure classification.
func (err *Error) Code() ErrorCode {
	return err.code
}

// CodeOf returns the filesystem code carried by err, or an empty code.
func CodeOf(err error) ErrorCode {
	var filesystemError *Error
	if errors.As(err, &filesystemError) {
		return filesystemError.Code()
	}
	return ""
}

func failure(code ErrorCode, cause error) error {
	return &Error{cause: cause, code: code}
}
