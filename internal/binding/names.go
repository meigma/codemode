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

// keySegment renders a dict key or non-plain member name as a diagnostic path segment.
//
// Single quotes keep the segment readable after the caller %q-quotes the path.
func keySegment(key string) string {
	if strings.ContainsAny(key, `'\`) || !strconv.CanBackquote(key) {
		return "[" + strconv.Quote(key) + "]"
	}
	return "['" + key + "']"
}

// memberPath appends a struct member to a diagnostic path.
func memberPath(parent string, name string) string {
	switch {
	case parent == "":
		return name
	case isPlainName(name):
		return parent + "." + name
	default:
		return parent + keySegment(name)
	}
}

// indexPath appends a list index to a diagnostic path.
func indexPath(parent string, index int) string {
	return parent + "[" + strconv.Itoa(index) + "]"
}
