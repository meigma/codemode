package binding

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

// tagOptions contains the supported JSON tag option state.
type tagOptions struct {
	// omitempty reports whether the field may be omitted when absent.
	omitempty bool
}

// compileFieldName validates field visibility, embedding, tags, and the name rule.
//
// keyword selects the Starlark identifier rule for keyword arguments; otherwise
// the name only needs to be a valid JSON field name.
func compileFieldName(field reflect.StructField, keyword bool) (string, tagOptions, error) {
	if field.Anonymous {
		return "", tagOptions{}, errors.New("embedded fields are not supported")
	}
	if field.PkgPath != "" {
		return "", tagOptions{}, errors.New("field must be exported")
	}

	name, options, err := parseJSONTag(field)
	if err != nil {
		return "", tagOptions{}, err
	}
	if keyword && !ValidIdentifier(name) {
		return "", tagOptions{}, fmt.Errorf("JSON name %q is not a Starlark identifier", name)
	}
	if !keyword && !validJSONName(name) {
		return "", tagOptions{}, fmt.Errorf("JSON name %q is not a valid JSON field name", name)
	}
	return name, options, nil
}

// parseJSONTag returns the field's JSON name and options from its only struct tag.
func parseJSONTag(field reflect.StructField) (string, tagOptions, error) {
	if field.Tag == "" {
		return field.Name, tagOptions{}, nil
	}
	jsonTag, ok := field.Tag.Lookup("json")
	if !ok || string(field.Tag) != fmt.Sprintf("json:%q", jsonTag) {
		return "", tagOptions{}, errors.New("only one json struct tag is supported")
	}
	parts := strings.Split(jsonTag, ",")
	if parts[0] == "-" {
		return "", tagOptions{}, errors.New("ignored JSON fields are not supported")
	}
	name := field.Name
	if parts[0] != "" {
		name = parts[0]
	}
	var options tagOptions
	for _, option := range parts[1:] {
		switch option {
		case "omitempty":
			if options.omitempty {
				return "", tagOptions{}, errors.New("duplicate omitempty option")
			}
			options.omitempty = true
		case "":
			return "", tagOptions{}, errors.New("empty JSON tag option")
		default:
			return "", tagOptions{}, fmt.Errorf("unsupported JSON tag option %q", option)
		}
	}
	return name, options, nil
}

// ValidIdentifier reports whether name is accepted as an identifier by the pinned Starlark scanner.
func ValidIdentifier(name string) bool {
	return !isKeyword(name) && isPlainName(name)
}

// isPlainName reports whether name has identifier syntax, ignoring reserved words.
func isPlainName(name string) bool {
	if name == "" {
		return false
	}
	for index, character := range name {
		if character == '_' || unicode.IsLetter(character) ||
			index > 0 && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return true
}

// validJSONName reports whether encoding/json accepts name as a struct tag name.
//
// This mirrors the unexported encoding/json isValidTag rule so every accepted
// name round-trips through standard JSON tooling unchanged.
func validJSONName(name string) bool {
	if name == "" {
		return false
	}
	for _, character := range name {
		switch {
		case strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", character):
		case !unicode.IsLetter(character) && !unicode.IsDigit(character):
			return false
		}
	}
	return true
}

// isKeyword reports whether name is reserved by Starlark syntax.
func isKeyword(name string) bool {
	switch name {
	case "False", "None", "True",
		"and", "as", "async", "await", "break", "class", "continue", "def", "del", "elif", "else",
		"except", "finally", "for", "from", "global", "if", "import", "in", "is", "lambda", "load", //nolint:goconst // Scanner keyword list.
		"nonlocal", "not", "or", "pass", "raise", "return", "try", "while", "with", "yield":
		return true
	default:
		return false
	}
}

// notationName renders a struct member name inside type notation.
//
// Plain names render bare, including Starlark keywords; any other name is
// double-quoted so separators inside the name cannot be misread.
func notationName(name string) string {
	if isPlainName(name) {
		return name
	}
	return strconv.Quote(name)
}

// segmentKind identifies one diagnostic path segment form.
type segmentKind uint8

const (
	// segmentMember is a struct member or root keyword name.
	segmentMember segmentKind = iota + 1

	// segmentIndex is a list position.
	segmentIndex

	// segmentKey is a dict key.
	segmentKey
)

// pathSegment is one step in a diagnostic argument path.
type pathSegment struct {
	// kind selects how the segment renders.
	kind segmentKind

	// name is the member name or dict key; it references caller-owned data.
	name string

	// index is the list position for segmentIndex.
	index int
}

// pathStackHint is the initial segment capacity for typical argument nesting.
const pathStackHint = 8

// argumentPath is a reusable stack of diagnostic path segments.
//
// Binding pushes and pops segments without building strings; String renders
// the path only when an error needs it, so success costs no copying in
// proportion to model-controlled key lengths.
type argumentPath struct {
	// segments are the active steps from the root keyword to the current value.
	segments []pathSegment
}

// newArgumentPath returns an empty path with preallocated segment capacity.
func newArgumentPath() argumentPath {
	return argumentPath{segments: make([]pathSegment, 0, pathStackHint)}
}

// push appends one segment.
func (path *argumentPath) push(segment pathSegment) {
	path.segments = append(path.segments, segment)
}

// pop removes the most recent segment.
func (path *argumentPath) pop() {
	path.segments = path.segments[:len(path.segments)-1]
}

// String renders the path as root.member[index]['key'].
//
// Plain member names use dots; other member names and every dict key use a
// single-quoted bracket so the segment stays readable after %q quoting.
func (path *argumentPath) String() string {
	var rendered strings.Builder
	for position, segment := range path.segments {
		switch segment.kind {
		case segmentMember:
			switch {
			case position == 0:
				rendered.WriteString(segment.name)
			case isPlainName(segment.name):
				rendered.WriteByte('.')
				rendered.WriteString(segment.name)
			default:
				writeKeySegment(&rendered, segment.name)
			}
		case segmentIndex:
			rendered.WriteByte('[')
			rendered.WriteString(strconv.Itoa(segment.index))
			rendered.WriteByte(']')
		case segmentKey:
			writeKeySegment(&rendered, segment.name)
		}
	}
	return rendered.String()
}

// writeKeySegment renders a dict key or non-plain member name as ['key'].
//
// Keys containing quotes, backslashes, or non-printable characters fall back
// to a Go-quoted ["key"] form.
func writeKeySegment(rendered *strings.Builder, key string) {
	if strings.ContainsAny(key, `'\`) || !strconv.CanBackquote(key) {
		rendered.WriteByte('[')
		rendered.WriteString(strconv.Quote(key))
		rendered.WriteByte(']')
		return
	}
	rendered.WriteString("['")
	rendered.WriteString(key)
	rendered.WriteString("']")
}
