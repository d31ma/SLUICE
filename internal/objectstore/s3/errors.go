package s3store

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/aws/smithy-go"
	"github.com/d31ma/sluice/internal/domain"
)

// ProviderError is the stable, redacted error returned by the S3 boundary.
// Ambiguous writes must be reconciled with Head before a caller retries them.
type ProviderError struct {
	cause     error
	code      domain.Code
	operation string
	retryable bool
	ambiguous bool
}

func (providerError *ProviderError) Error() string {
	return "S3 " + providerError.operation + " failed"
}

func (providerError *ProviderError) Unwrap() error {
	return providerError.cause
}

// Code returns a transport-independent domain error code.
func (providerError *ProviderError) Code() domain.Code {
	return providerError.code
}

// Retryable reports whether retry is appropriate after any required read-back.
func (providerError *ProviderError) Retryable() bool {
	return providerError.retryable
}

// Ambiguous reports whether a write may have reached the provider.
func (providerError *ProviderError) Ambiguous() bool {
	return providerError.ambiguous
}

// Operation returns the redacted operation name.
func (providerError *ProviderError) Operation() string {
	return providerError.operation
}

// AsProviderError extracts a provider classification without exposing its cause.
func AsProviderError(err error) (*ProviderError, bool) {
	var providerError *ProviderError
	found := errors.As(err, &providerError)
	return providerError, found
}

func invalid(operation string, cause error) error {
	return newProviderError(operation, domain.CodeInvalidArgument, false, false, cause)
}

func integrity(operation string, cause error) error {
	return newProviderError(operation, domain.CodeIntegrityFailure, false, false, cause)
}

func writerFailure(operation string, cause error) error {
	return newProviderError(operation, domain.CodeFilesystemUnavailable, false, false, cause)
}

func classify(operation string, cause error, write bool) error {
	status, apiCode := providerStatus(cause)
	code, retryable := domainClassification(status, apiCode, cause)
	ambiguous := write && writeOutcomeUnknown(status, cause)
	return newProviderError(operation, code, retryable, ambiguous, cause)
}

func newProviderError(operation string, code domain.Code, retryable, ambiguous bool, cause error) error {
	return &ProviderError{
		cause:     cause,
		code:      code,
		operation: operation,
		retryable: retryable,
		ambiguous: ambiguous,
	}
}

func providerStatus(err error) (int, string) {
	var statusError interface{ HTTPStatusCode() int }
	status := 0
	if errors.As(err, &statusError) {
		status = statusError.HTTPStatusCode()
	}

	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		return status, apiError.ErrorCode()
	}
	return status, ""
}

func domainClassification(status int, apiCode string, err error) (domain.Code, bool) {
	for _, classifier := range responseClassifiers {
		if classification, found := classifier(status, apiCode); found {
			return classification.code, classification.retryable
		}
	}
	return causeClassification(err)
}

type errorClassification struct {
	code      domain.Code
	retryable bool
}

type responseClassifier func(int, string) (errorClassification, bool)

var responseClassifiers = []responseClassifier{
	authenticationClassification,
	missingResourceClassification,
	statusClassification,
}

var authenticationStatuses = map[int]errorClassification{
	http.StatusUnauthorized: {code: domain.CodeUnauthenticated},
	http.StatusForbidden:    {code: domain.CodeForbidden},
}

var missingResourceAPICodes = map[string]struct{}{
	"NoSuchBucket": {},
	"NoSuchKey":    {},
}

var classifiedStatuses = map[int]errorClassification{
	http.StatusBadRequest:         {code: domain.CodeInvalidArgument},
	http.StatusConflict:           {code: domain.CodeConflict},
	http.StatusPreconditionFailed: {code: domain.CodeConflict},
	http.StatusRequestTimeout:     {code: domain.CodeObjectStoreUnavailable, retryable: true},
	http.StatusTooManyRequests:    {code: domain.CodeObjectStoreUnavailable, retryable: true},
}

func authenticationClassification(status int, _ string) (errorClassification, bool) {
	classification, found := authenticationStatuses[status]
	return classification, found
}

func missingResourceClassification(status int, apiCode string) (errorClassification, bool) {
	if status == http.StatusNotFound {
		return errorClassification{code: domain.CodeNotFound}, true
	}
	_, found := missingResourceAPICodes[apiCode]
	return errorClassification{code: domain.CodeNotFound}, found
}

func statusClassification(status int, _ string) (errorClassification, bool) {
	if status >= http.StatusInternalServerError {
		return errorClassification{code: domain.CodeObjectStoreUnavailable, retryable: true}, true
	}
	classification, found := classifiedStatuses[status]
	return classification, found
}

func causeClassification(err error) (domain.Code, bool) {
	if errors.Is(err, context.Canceled) {
		return domain.CodeObjectStoreUnavailable, false
	}
	if retryableCause(err) {
		return domain.CodeObjectStoreUnavailable, true
	}
	return domain.CodeInternal, false
}

func retryableCause(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || isNetworkError(err)
}

func isNetworkError(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError)
}

func writeOutcomeUnknown(status int, err error) bool {
	if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
		return false
	}
	return status >= http.StatusInternalServerError || status == 0 || errors.Is(err, context.DeadlineExceeded)
}
