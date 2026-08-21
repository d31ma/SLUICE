package domain

import "fmt"

type Code string

const (
	CodeInvalidArgument        Code = "invalid_argument"
	CodeUnauthenticated        Code = "unauthenticated"
	CodeForbidden              Code = "forbidden"
	CodeNotFound               Code = "not_found"
	CodeAlreadyExists          Code = "already_exists"
	CodeConflict               Code = "conflict"
	CodeStateUnavailable       Code = "state_unavailable"
	CodeObjectStoreUnavailable Code = "object_store_unavailable"
	CodeFilesystemUnavailable  Code = "filesystem_unavailable"
	CodeProtocolMismatch       Code = "protocol_mismatch"
	CodeIntegrityFailure       Code = "integrity_failure"
	CodeInternal               Code = "internal"
)

var knownCodes = map[Code]struct{}{
	CodeInvalidArgument:        {},
	CodeUnauthenticated:        {},
	CodeForbidden:              {},
	CodeNotFound:               {},
	CodeAlreadyExists:          {},
	CodeConflict:               {},
	CodeStateUnavailable:       {},
	CodeObjectStoreUnavailable: {},
	CodeFilesystemUnavailable:  {},
	CodeProtocolMismatch:       {},
	CodeIntegrityFailure:       {},
	CodeInternal:               {},
}

func ParseCode(value string) (Code, error) {
	code := Code(value)
	if _, found := knownCodes[code]; !found {
		return "", fmt.Errorf("unknown domain error code %q", value)
	}
	return code, nil
}

type Failure struct {
	cause     error
	code      Code
	message   string
	retryable bool
}

func NewFailure(code Code, retryable bool, publicMessage string, cause error) (Failure, error) {
	if _, found := knownCodes[code]; !found {
		return Failure{}, fmt.Errorf("unknown domain error code %q", code)
	}
	if publicMessage == "" {
		return Failure{}, fmt.Errorf("domain failure requires a public message")
	}
	return Failure{
		code:      code,
		retryable: retryable,
		message:   publicMessage,
		cause:     cause,
	}, nil
}

func (failure Failure) Code() Code {
	return failure.code
}

func (failure Failure) Retryable() bool {
	return failure.retryable
}

func (failure Failure) Error() string {
	return failure.message
}

func (failure Failure) Unwrap() error {
	return failure.cause
}
