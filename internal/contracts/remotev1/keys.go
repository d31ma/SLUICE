// Package remotev1 defines the canonical paths and object keys used by the
// Sluice remote protocol version 1.
package remotev1

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	namespace       = ".sluice/v1"
	maxLogicalBytes = 4096
	maxSegmentBytes = 255
)

var errUnsafeComponent = errors.New("unsafe remote protocol component")

// CanonicalLogicalPath normalizes Unicode to NFC and rejects paths that cannot
// be installed safely on every supported desktop target. It never rewrites a
// separator because remote input must not be reinterpreted as another path.
func CanonicalLogicalPath(raw string) (string, error) {
	if raw == "" || !utf8.ValidString(raw) {
		return "", fmt.Errorf("%w: logical path is empty or invalid UTF-8", errUnsafeComponent)
	}
	if strings.HasPrefix(raw, "/") || hasWindowsDrivePrefix(raw) {
		return "", fmt.Errorf("%w: logical path must be root-relative", errUnsafeComponent)
	}
	if strings.ContainsRune(raw, '\\') {
		return "", fmt.Errorf("%w: backslash is not a logical-path separator", errUnsafeComponent)
	}

	path := norm.NFC.String(raw)
	if len(path) > maxLogicalBytes {
		return "", fmt.Errorf("%w: logical path exceeds %d bytes", errUnsafeComponent, maxLogicalBytes)
	}
	for _, segment := range strings.Split(path, "/") {
		if err := validatePathSegment(segment); err != nil {
			return "", err
		}
	}
	return path, nil
}

// LogicalPathHash returns the lowercase SHA-256 digest used to address a path
// manifest or tombstone.
func LogicalPathHash(logicalPath string) (string, error) {
	canonical, err := CanonicalLogicalPath(logicalPath)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:]), nil
}

// DescriptorKey returns the root descriptor object key.
func DescriptorKey(prefix, rootID string) (string, error) {
	base, err := protocolBase(prefix)
	if err != nil {
		return "", err
	}
	if validationErr := validateIdentifier(rootID); validationErr != nil {
		return "", fmt.Errorf("root ID: %w", validationErr)
	}
	return base + "/roots/" + rootID + "/descriptor.json", nil
}

// ManifestKey returns the path manifest key for a logical path.
func ManifestKey(prefix, rootID, logicalPath string) (string, error) {
	return pathRecordKey(prefix, "manifests", rootID, logicalPath)
}

// TombstoneKey returns the deletion record key for a logical path.
func TombstoneKey(prefix, rootID, logicalPath string) (string, error) {
	return pathRecordKey(prefix, "tombstones", rootID, logicalPath)
}

// ContentKey returns the immutable content-addressed object key.
func ContentKey(prefix, digest string) (string, error) {
	base, err := protocolBase(prefix)
	if err != nil {
		return "", err
	}
	if !isLowerHexDigest(digest) {
		return "", fmt.Errorf("content digest: %w", errUnsafeComponent)
	}
	return base + "/objects/sha256/" + digest[:2] + "/" + digest, nil
}

func pathRecordKey(prefix, collection, rootID, logicalPath string) (string, error) {
	base, err := protocolBase(prefix)
	if err != nil {
		return "", err
	}
	if validationErr := validateIdentifier(rootID); validationErr != nil {
		return "", fmt.Errorf("root ID: %w", validationErr)
	}
	digest, err := LogicalPathHash(logicalPath)
	if err != nil {
		return "", err
	}
	return base + "/" + collection + "/" + rootID + "/" + digest + ".json", nil
}

func protocolBase(prefix string) (string, error) {
	if prefix == "" {
		return namespace, nil
	}
	if !utf8.ValidString(prefix) || !norm.NFC.IsNormalString(prefix) || strings.HasPrefix(prefix, "/") || strings.HasSuffix(prefix, "/") || strings.Contains(prefix, "\\") {
		return "", fmt.Errorf("prefix: %w", errUnsafeComponent)
	}
	for _, segment := range strings.Split(prefix, "/") {
		if segment == "" || segment == "." || segment == ".." || containsControl(segment) {
			return "", fmt.Errorf("prefix: %w", errUnsafeComponent)
		}
	}
	return prefix + "/" + namespace, nil
}

func validatePathSegment(segment string) error {
	if segment == "" || segment == "." || segment == ".." {
		return fmt.Errorf("%w: empty or relative path segment", errUnsafeComponent)
	}
	if len(segment) > maxSegmentBytes || containsControl(segment) {
		return fmt.Errorf("%w: invalid path segment", errUnsafeComponent)
	}
	if strings.ContainsAny(segment, `:*?"<>|`) || strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
		return fmt.Errorf("%w: path segment is not portable", errUnsafeComponent)
	}
	name := strings.ToUpper(strings.TrimSuffix(segment, filepathExtension(segment)))
	if isWindowsDeviceName(name) {
		return fmt.Errorf("%w: reserved device name", errUnsafeComponent)
	}
	return nil
}

func validateIdentifier(value string) error {
	if value == "" || len(value) > 128 {
		return errUnsafeComponent
	}
	last := value[len(value)-1]
	if !isLowerLetterOrDigit(rune(last)) {
		return errUnsafeComponent
	}
	for index, char := range value {
		if isLowerLetterOrDigit(char) || index > 0 && (char == '-' || char == '_') {
			continue
		}
		return errUnsafeComponent
	}
	return nil
}

func isLowerLetterOrDigit(character rune) bool {
	return character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
}

func filepathExtension(segment string) string {
	index := strings.IndexByte(segment, '.')
	if index < 0 {
		return ""
	}
	return segment[index:]
}

func isWindowsDeviceName(name string) bool {
	switch name {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}

func isLowerHexDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func containsControl(value string) bool {
	for _, char := range value {
		if char < ' ' || char == '\u007f' {
			return true
		}
	}
	return false
}

func hasWindowsDrivePrefix(value string) bool {
	return len(value) >= 2 && value[1] == ':' && (value[0] >= 'A' && value[0] <= 'Z' || value[0] >= 'a' && value[0] <= 'z')
}
