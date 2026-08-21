package filesystem

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const windowsForbiddenCharacters = `<>:"|?*`

var windowsReservedNames = map[string]struct{}{
	"AUX": {}, "CON": {}, "CONIN$": {}, "CONOUT$": {}, "NUL": {}, "PRN": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {},
	"COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"COM\u00b9": {}, "COM\u00b2": {}, "COM\u00b3": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {},
	"LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
	"LPT\u00b9": {}, "LPT\u00b2": {}, "LPT\u00b3": {},
}

// LogicalPath is a validated, root-relative path in canonical wire form.
// Forward slash is the only separator, regardless of the host operating system.
type LogicalPath struct {
	value string
}

// ParseLogicalPath validates a portable Sluice logical path. It deliberately
// rejects ambiguous forms instead of rewriting them.
func ParseLogicalPath(input string) (LogicalPath, error) {
	if input == "" {
		return LogicalPath{}, failure(CodeEmptyPath, nil)
	}
	if !utf8.ValidString(input) {
		return LogicalPath{}, failure(CodeInvalidUTF8, nil)
	}
	if isAbsoluteLogicalPath(input) {
		return LogicalPath{}, failure(CodeAbsolutePath, nil)
	}
	if strings.ContainsRune(input, '\\') {
		return LogicalPath{}, failure(CodeInvalidSeparator, nil)
	}

	for _, segment := range strings.Split(input, "/") {
		if err := validateSegment(segment); err != nil {
			return LogicalPath{}, err
		}
	}
	return LogicalPath{value: input}, nil
}

// String returns the canonical slash-separated wire representation.
func (path LogicalPath) String() string {
	return path.value
}

func isAbsoluteLogicalPath(input string) bool {
	if strings.HasPrefix(input, "/") || strings.HasPrefix(input, `\`) {
		return true
	}
	return len(input) >= 2 && isASCIILetter(input[0]) && input[1] == ':'
}

func isASCIILetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func validateSegment(segment string) error {
	if segment == "" {
		return failure(CodeEmptySegment, nil)
	}
	if segment == "." || segment == ".." {
		return failure(CodeDotSegment, nil)
	}
	if strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
		return failure(CodeTrailingDotOrSpace, nil)
	}
	if hasControlCharacter(segment) {
		return failure(CodeControlCharacter, nil)
	}
	if strings.ContainsAny(segment, windowsForbiddenCharacters) {
		return failure(CodeForbiddenCharacter, nil)
	}
	if isWindowsReservedName(segment) {
		return failure(CodeReservedName, nil)
	}
	return nil
}

func hasControlCharacter(segment string) bool {
	for _, character := range segment {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func isWindowsReservedName(segment string) bool {
	base, _, _ := strings.Cut(segment, ".")
	_, reserved := windowsReservedNames[strings.ToUpper(base)]
	return reserved
}
