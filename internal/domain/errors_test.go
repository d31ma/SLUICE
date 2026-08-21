package domain_test

import (
	"errors"
	"testing"

	"github.com/d31ma/sluice/internal/domain"
)

func TestFailureExposesStableClassification(t *testing.T) {
	cause := errors.New("connection reset")
	failure, err := domain.NewFailure(domain.CodeObjectStoreUnavailable, true, "object store unavailable", cause)
	if err != nil {
		t.Fatalf("NewFailure() error = %v", err)
	}

	if failure.Code() != domain.CodeObjectStoreUnavailable {
		t.Fatalf("Code() = %q, want %q", failure.Code(), domain.CodeObjectStoreUnavailable)
	}
	if !failure.Retryable() {
		t.Fatal("Retryable() = false, want true")
	}
	if failure.Error() != "object store unavailable" {
		t.Fatalf("Error() = %q, want redacted public message", failure.Error())
	}
	if !errors.Is(failure, cause) {
		t.Fatal("errors.Is() did not preserve the private cause")
	}
}

func TestFailureRejectsUnknownCode(t *testing.T) {
	_, err := domain.ParseCode("not-a-real-code")
	if err == nil {
		t.Fatal("ParseCode() accepted an unknown code")
	}
	if _, err := domain.NewFailure(domain.Code("not-a-real-code"), false, "invalid", nil); err == nil {
		t.Fatal("NewFailure() accepted an unknown code")
	}
}
