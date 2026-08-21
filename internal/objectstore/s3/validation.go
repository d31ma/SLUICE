package s3store

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	contractsv1 "github.com/d31ma/sluice/contracts/v1"
)

const (
	maximumObjectKeyBytes = 1024
	maximumCursorBytes    = 16 * 1024
	maximumCASBytes       = 4 * 1024 * 1024
)

func validateKey(key string) error {
	if key == "" || len(key) > maximumObjectKeyBytes || !utf8.ValidString(key) {
		return errors.New("object key must be valid UTF-8 between 1 and 1024 bytes")
	}
	if strings.HasPrefix(key, "/") || strings.ContainsRune(key, '\\') || containsControl(key) {
		return errors.New("object key is not canonical")
	}
	return nil
}

func validatePrefix(prefix string) error {
	if len(prefix) > maximumObjectKeyBytes || !utf8.ValidString(prefix) {
		return errors.New("prefix must be valid UTF-8 and no longer than 1024 bytes")
	}
	if strings.HasPrefix(prefix, "/") || strings.ContainsRune(prefix, '\\') || containsControl(prefix) {
		return errors.New("prefix is not canonical")
	}
	return nil
}

func containsControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func validateSHA256(value string) error {
	if len(value) != sha256HexLength {
		return errors.New("SHA-256 must contain 64 lowercase hexadecimal characters")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(decoded) != value {
		return errors.New("SHA-256 must contain 64 lowercase hexadecimal characters")
	}
	return nil
}

func validateIdempotencyKey(value string) error {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) || containsControl(value) {
		return errors.New("idempotency key must be valid UTF-8 between 1 and 256 bytes")
	}
	return nil
}

func validateUpload(request contractsv1.UploadRequest) error {
	if err := validateKey(request.Key); err != nil {
		return err
	}
	if request.Body == nil {
		return errors.New("upload body is required")
	}
	if request.Size < 0 {
		return errors.New("upload size cannot be negative")
	}
	if err := validateSHA256(request.SHA256); err != nil {
		return err
	}
	return validateIdempotencyKey(request.IdempotencyKey)
}

func validateCAS(request contractsv1.CASRequest) error {
	if err := validateKey(request.Key); err != nil {
		return err
	}
	if request.Body == nil {
		return errors.New("compare-and-swap body is required")
	}
	if request.Size < 0 || request.Size > maximumCASBytes {
		return fmt.Errorf("compare-and-swap size must be between 0 and %d bytes", maximumCASBytes)
	}
	if request.ExpectedAbsent == (request.ExpectedETag != "") {
		return errors.New("exactly one compare-and-swap precondition is required")
	}
	return validateIdempotencyKey(request.IdempotencyKey)
}

func validateDownload(request contractsv1.DownloadRequest) error {
	if err := validateKey(request.Key); err != nil {
		return err
	}
	if request.Writer == nil {
		return errors.New("download writer is required")
	}
	if request.Range == nil {
		return nil
	}
	if request.Range.Offset < 0 || request.Range.Length <= 0 {
		return errors.New("download range requires a non-negative offset and positive length")
	}
	if request.Range.Offset > math.MaxInt64-request.Range.Length {
		return errors.New("download range overflows")
	}
	return nil
}

func validateDelete(request contractsv1.DeleteRequest) error {
	if err := validateKey(request.Key); err != nil {
		return err
	}
	if err := validateIdempotencyKey(request.IdempotencyKey); err != nil {
		return err
	}
	if len(request.VersionID) > maximumCursorBytes || containsControl(request.VersionID) {
		return errors.New("version ID is invalid")
	}
	return nil
}
