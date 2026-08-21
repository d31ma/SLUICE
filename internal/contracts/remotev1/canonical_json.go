package remotev1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const maxSafeJSONInteger int64 = 1<<53 - 1

var errInvalidCanonicalJSON = errors.New("invalid remote protocol JSON")

type jsonMember struct {
	value any
	name  string
}

type jsonObject []jsonMember

type jsonArray []any

type jsonInteger string

// CanonicalizeJSON parses a remote protocol document and emits the Sluice
// integer-only subset of RFC 8785. It rejects duplicate members, fractional or
// exponent-form numbers, integers outside the interoperable IEEE-754 range,
// invalid UTF-8, and trailing data.
func CanonicalizeJSON(document []byte) ([]byte, error) {
	if !utf8.Valid(document) {
		return nil, fmt.Errorf("%w: input is not UTF-8", errInvalidCanonicalJSON)
	}
	if err := validateUnicodeEscapes(document); err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	value, err := parseJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	if err := requireJSONEnd(decoder); err != nil {
		return nil, err
	}

	var canonical bytes.Buffer
	if err := writeCanonicalValue(&canonical, value); err != nil {
		return nil, err
	}
	return canonical.Bytes(), nil
}

func validateUnicodeEscapes(document []byte) error {
	inString := false
	for index := 0; index < len(document); index++ {
		if document[index] == '"' {
			inString = !inString
			continue
		}
		if document[index] != '\\' || !inString {
			continue
		}
		next, err := validateEscapedCodeUnit(document, index)
		if err != nil {
			return err
		}
		index = next
	}
	return nil
}

func validateEscapedCodeUnit(document []byte, escapeIndex int) (int, error) {
	if escapeIndex+1 >= len(document) || document[escapeIndex+1] != 'u' {
		return escapeIndex + 1, nil
	}
	code, ok := parseHexCodeUnit(document, escapeIndex+2)
	if !ok {
		return escapeIndex + 1, nil
	}
	end := escapeIndex + 5
	if code >= 0xdc00 && code <= 0xdfff {
		return 0, fmt.Errorf("%w: unpaired low surrogate", errInvalidCanonicalJSON)
	}
	if code < 0xd800 || code > 0xdbff {
		return end, nil
	}
	return validateLowSurrogate(document, end)
}

func validateLowSurrogate(document []byte, highEnd int) (int, error) {
	if highEnd+6 >= len(document) || document[highEnd+1] != '\\' || document[highEnd+2] != 'u' {
		return 0, fmt.Errorf("%w: unpaired high surrogate", errInvalidCanonicalJSON)
	}
	low, ok := parseHexCodeUnit(document, highEnd+3)
	if !ok || low < 0xdc00 || low > 0xdfff {
		return 0, fmt.Errorf("%w: unpaired high surrogate", errInvalidCanonicalJSON)
	}
	return highEnd + 6, nil
}

func parseHexCodeUnit(document []byte, start int) (uint16, bool) {
	if start+4 > len(document) {
		return 0, false
	}
	value, err := strconv.ParseUint(string(document[start:start+4]), 16, 16)
	return uint16(value), err == nil
}

func parseJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidCanonicalJSON, err)
	}

	switch value := token.(type) {
	case json.Delim:
		return parseDelimitedValue(decoder, value)
	case json.Number:
		return parseJSONInteger(value)
	case string, bool, nil:
		return value, nil
	default:
		return nil, fmt.Errorf("%w: unsupported token", errInvalidCanonicalJSON)
	}
}

func parseDelimitedValue(decoder *json.Decoder, delimiter json.Delim) (any, error) {
	switch delimiter {
	case '{':
		return parseJSONObject(decoder)
	case '[':
		return parseJSONArray(decoder)
	default:
		return nil, fmt.Errorf("%w: unexpected delimiter %q", errInvalidCanonicalJSON, delimiter)
	}
}

func parseJSONObject(decoder *json.Decoder) (jsonObject, error) {
	members := make(jsonObject, 0)
	seen := make(map[string]struct{})
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("%w: object member: %w", errInvalidCanonicalJSON, err)
		}
		name, ok := nameToken.(string)
		if !ok {
			return nil, fmt.Errorf("%w: object member name is not a string", errInvalidCanonicalJSON)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate object member %q", errInvalidCanonicalJSON, name)
		}
		seen[name] = struct{}{}

		value, err := parseJSONValue(decoder)
		if err != nil {
			return nil, err
		}
		members = append(members, jsonMember{name: name, value: value})
	}
	if err := consumeDelimiter(decoder, '}'); err != nil {
		return nil, err
	}
	return members, nil
}

func parseJSONArray(decoder *json.Decoder) (jsonArray, error) {
	values := make(jsonArray, 0)
	for decoder.More() {
		value, err := parseJSONValue(decoder)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := consumeDelimiter(decoder, ']'); err != nil {
		return nil, err
	}
	return values, nil
}

func parseJSONInteger(number json.Number) (jsonInteger, error) {
	raw := number.String()
	if strings.ContainsAny(raw, ".eE") {
		return "", fmt.Errorf("%w: non-integer number %q", errInvalidCanonicalJSON, raw)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < -maxSafeJSONInteger || value > maxSafeJSONInteger {
		return "", fmt.Errorf("%w: unsafe integer %q", errInvalidCanonicalJSON, raw)
	}
	return jsonInteger(strconv.FormatInt(value, 10)), nil
}

func consumeDelimiter(decoder *json.Decoder, want json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%w: %w", errInvalidCanonicalJSON, err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != want {
		return fmt.Errorf("%w: missing %q", errInvalidCanonicalJSON, want)
	}
	return nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	if _, err := decoder.Token(); err == nil {
		return fmt.Errorf("%w: trailing data", errInvalidCanonicalJSON)
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: %w", errInvalidCanonicalJSON, err)
	}
	return nil
}

func writeCanonicalValue(destination *bytes.Buffer, value any) error {
	switch value := value.(type) {
	case jsonObject:
		return writeCanonicalObject(destination, value)
	case jsonArray:
		return writeCanonicalArray(destination, value)
	case jsonInteger:
		destination.WriteString(string(value))
	case string:
		writeJSONString(destination, value)
	case bool:
		destination.WriteString(strconv.FormatBool(value))
	case nil:
		destination.WriteString("null")
	default:
		return fmt.Errorf("%w: unsupported value", errInvalidCanonicalJSON)
	}
	return nil
}

func writeCanonicalObject(destination *bytes.Buffer, object jsonObject) error {
	sort.Slice(object, func(left, right int) bool {
		return utf16Less(object[left].name, object[right].name)
	})
	destination.WriteByte('{')
	for index, member := range object {
		if index > 0 {
			destination.WriteByte(',')
		}
		writeJSONString(destination, member.name)
		destination.WriteByte(':')
		if err := writeCanonicalValue(destination, member.value); err != nil {
			return err
		}
	}
	destination.WriteByte('}')
	return nil
}

func writeCanonicalArray(destination *bytes.Buffer, array jsonArray) error {
	destination.WriteByte('[')
	for index, value := range array {
		if index > 0 {
			destination.WriteByte(',')
		}
		if err := writeCanonicalValue(destination, value); err != nil {
			return err
		}
	}
	destination.WriteByte(']')
	return nil
}

func writeJSONString(destination *bytes.Buffer, value string) {
	destination.WriteByte('"')
	for _, character := range value {
		writeJSONStringRune(destination, character)
	}
	destination.WriteByte('"')
}

func writeJSONStringRune(destination *bytes.Buffer, character rune) {
	switch character {
	case '"', '\\':
		destination.WriteByte('\\')
		destination.WriteRune(character)
	case '\b':
		destination.WriteString(`\b`)
	case '\t':
		destination.WriteString(`\t`)
	case '\n':
		destination.WriteString(`\n`)
	case '\f':
		destination.WriteString(`\f`)
	case '\r':
		destination.WriteString(`\r`)
	default:
		writeUnescapedJSONRune(destination, character)
	}
}

func writeUnescapedJSONRune(destination *bytes.Buffer, character rune) {
	const hexDigits = "0123456789abcdef"

	if character >= ' ' {
		destination.WriteRune(character)
		return
	}
	destination.WriteString(`\u00`)
	destination.WriteByte(hexDigits[int(character)>>4])
	destination.WriteByte(hexDigits[int(character)&0x0f])
}

func utf16Less(left, right string) bool {
	leftUnits := utf16.Encode([]rune(left))
	rightUnits := utf16.Encode([]rune(right))
	limit := min(len(leftUnits), len(rightUnits))
	for index := range limit {
		if leftUnits[index] != rightUnits[index] {
			return leftUnits[index] < rightUnits[index]
		}
	}
	return len(leftUnits) < len(rightUnits)
}
