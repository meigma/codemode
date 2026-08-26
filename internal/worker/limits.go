package worker

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/meigma/codemode/internal/binding"
	"github.com/meigma/codemode/internal/universe"
)

const (
	minimumDottedSegments = 2
	jsonStringEscapeMax   = 6
)

// childLimits contains the child-visible positive execution budgets.
type childLimits struct {
	// MaxSourceBytes bounds submitted source length.
	MaxSourceBytes int `json:"max_source_bytes"`

	// MaxExecutionSteps bounds interpreter work.
	MaxExecutionSteps uint64 `json:"max_execution_steps"`

	// MaxNativeCalls bounds attempted native invocations.
	MaxNativeCalls uint64 `json:"max_native_calls"`

	// MaxValueDepth bounds recursive converted-value nesting.
	MaxValueDepth int `json:"max_value_depth"`

	// MaxValueBytes bounds type-preserving encoded value size.
	MaxValueBytes int `json:"max_value_bytes"`
}

// manifestEntry is one enabled capability advertised to the child.
type manifestEntry struct {
	// ID is the stable capability identity.
	ID string `json:"id"`

	// Name is the dotted Starlark path.
	// The first segment must not collide with a reserved Starlark universe root.
	Name string `json:"name"`

	// Input is the exact compiled input shape.
	Input []binding.FieldShape `json:"input"`
}

// childPayloadCap is the largest legal child-originated execution payload.
func childPayloadCap(maxValueBytes int, manifest []manifestEntry) (uint32, error) {
	valueBytes, err := configUint32(maxValueBytes)
	if err != nil {
		return 0, err
	}
	longestID, err := longestEncodedID(manifest)
	if err != nil {
		return 0, err
	}
	nativeCall, err := addConfigUint32(
		uint32(len(nativeCallPrefix)),
		longestID,
		uint32(len(nativeCallMid)),
		valueBytes,
		uint32(len(nativeCallSuffix)),
	)
	if err != nil {
		return 0, err
	}
	final, err := addConfigUint32(uint32(len(finalPrefix)), valueBytes, uint32(len(finalSuffix)))
	if err != nil {
		return 0, err
	}
	finalError, err := finalErrorPayloadCap()
	if err != nil {
		return 0, err
	}
	return maxUint32(nativeCall, final, finalError), nil
}

// finalErrorPayloadCap is the largest legal detailed final_error payload.
func finalErrorPayloadCap() (uint32, error) {
	escaped, err := mulConfigUint32(jsonStringEscapeMax, maxDiagnosticBytes)
	if err != nil {
		return 0, err
	}
	return addConfigUint32(
		uint32(len(finalErrorPrefix)),
		uint32(len(finalErrorDetail)),
		uint32(len(emptyJSONString)),
		escaped,
		uint32(len(finalErrorSuffix)),
	)
}

// parentPayloadCap is the largest legal parent-originated execution payload.
func parentPayloadCap(maxValueBytes int) (uint32, error) {
	valueBytes, err := configUint32(maxValueBytes)
	if err != nil {
		return 0, err
	}
	nativeResult, err := addConfigUint32(uint32(len(nativeResultPrefix)), valueBytes, uint32(len(nativeResultSuffix)))
	if err != nil {
		return 0, err
	}
	return maxUint32(nativeResult, uint32(len(nativeAbortPayload))), nil
}

// execPayloadCap is the largest legal initial exec payload.
func execPayloadCap(maxSourceBytes int, manifest []manifestEntry) (uint32, error) {
	if maxSourceBytes < 0 {
		return 0, errInvalidLimits
	}
	base, err := marshalFrame(execFrame{
		Type:     frameTypeExec,
		Version:  protocolVersion,
		Source:   "",
		Limits:   worstChildLimits(),
		Manifest: emptyManifest(manifest),
	})
	if err != nil {
		return 0, err
	}
	sourceBytes, err := configUint32(maxSourceBytes)
	if err != nil {
		return 0, err
	}
	expanded, err := mulConfigUint32(jsonStringEscapeMax, sourceBytes)
	if err != nil {
		return 0, err
	}
	baseLen, err := configUint32(len(base))
	if err != nil {
		return 0, err
	}
	return addConfigUint32(baseLen, expanded)
}

// validateChildLimits reports whether every child budget is positive.
func validateChildLimits(limits childLimits) error {
	if limits.MaxSourceBytes <= 0 ||
		limits.MaxExecutionSteps == 0 ||
		limits.MaxNativeCalls == 0 ||
		limits.MaxValueDepth <= 0 ||
		limits.MaxValueBytes <= 0 {
		return errInvalidLimits
	}
	return nil
}

// validateManifest reports whether capability identities and shapes are legal.
//
// The first dotted name segment must not collide with a reserved Starlark universe root.
func validateManifest(entries []manifestEntry) error {
	ids := make(map[string]struct{}, len(entries))
	names := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.ID == "" || entry.ID != strings.TrimSpace(entry.ID) {
			return errInvalidManifest
		}
		if _, duplicate := ids[entry.ID]; duplicate {
			return errInvalidManifest
		}
		if !isDottedName(entry.Name) {
			return errInvalidManifest
		}
		root, _, _ := strings.Cut(entry.Name, ".")
		if universe.IsReservedRoot(root) {
			return errInvalidManifest
		}
		if _, duplicate := names[entry.Name]; duplicate {
			return errInvalidManifest
		}
		if err := binding.ValidateInputShape(entry.Input); err != nil {
			return fmt.Errorf("%w: %w", errInvalidManifest, err)
		}
		ids[entry.ID] = struct{}{}
		names[entry.Name] = struct{}{}
	}
	return validateNamespaceCollisions(names)
}

// isDottedName reports whether name has at least two valid Starlark segments.
func isDottedName(name string) bool {
	segments := strings.Split(name, ".")
	if len(segments) < minimumDottedSegments {
		return false
	}
	for _, segment := range segments {
		if !binding.ValidIdentifier(segment) {
			return false
		}
	}
	return true
}

// validateNamespaceCollisions rejects a capability that is also a namespace.
func validateNamespaceCollisions(names map[string]struct{}) error {
	for name := range names {
		segments := strings.Split(name, ".")
		for end := 1; end < len(segments); end++ {
			prefix := strings.Join(segments[:end], ".")
			if _, collides := names[prefix]; collides {
				return errInvalidManifest
			}
		}
	}
	return nil
}

// longestEncodedID returns the longest JSON-encoded capability ID.
func longestEncodedID(manifest []manifestEntry) (uint32, error) {
	longest := uint32(len(emptyJSONString))
	for _, entry := range manifest {
		encoded, err := json.Marshal(entry.ID)
		if err != nil {
			return 0, errInvalidManifest
		}
		length, err := configUint32(len(encoded))
		if err != nil {
			return 0, err
		}
		if length > longest {
			longest = length
		}
	}
	return longest, nil
}

// emptyManifest returns a non-nil copy used for exec cap encoding.
func emptyManifest(manifest []manifestEntry) []manifestEntry {
	if manifest == nil {
		return []manifestEntry{}
	}
	return manifest
}

// worstChildLimits is the largest numeric spelling used for exec cap math.
func worstChildLimits() childLimits {
	return childLimits{
		MaxSourceBytes:    math.MaxInt,
		MaxExecutionSteps: math.MaxUint64,
		MaxNativeCalls:    math.MaxUint64,
		MaxValueDepth:     math.MaxInt,
		MaxValueBytes:     math.MaxInt,
	}
}

// uint32FromInt converts a non-negative int that fits a uint32 prefix.
func uint32FromInt(value int) (uint32, error) {
	if value < 0 || uint64(value) > math.MaxUint32 {
		return 0, errFrameTooLarge
	}
	return uint32(value), nil
}

// configUint32 converts a configured size that must fit the uint32 prefix.
func configUint32(value int) (uint32, error) {
	converted, err := uint32FromInt(value)
	if err != nil {
		return 0, errInvalidLimits
	}
	return converted, nil
}

// addUint32 adds values and rejects unsigned 32-bit overflow.
func addUint32(values ...uint32) (uint32, error) {
	var sum uint64
	for _, value := range values {
		sum += uint64(value)
		if sum > math.MaxUint32 {
			return 0, errFrameTooLarge
		}
	}
	return uint32(sum), nil
}

// mulUint32 multiplies two values and rejects unsigned 32-bit overflow.
func mulUint32(left uint32, right uint32) (uint32, error) {
	product := uint64(left) * uint64(right)
	if product > math.MaxUint32 {
		return 0, errFrameTooLarge
	}
	return uint32(product), nil
}

// addConfigUint32 adds configured sizes and rejects prefix overflow as invalid limits.
func addConfigUint32(values ...uint32) (uint32, error) {
	sum, err := addUint32(values...)
	if err != nil {
		return 0, errInvalidLimits
	}
	return sum, nil
}

// mulConfigUint32 multiplies configured sizes and rejects prefix overflow as invalid limits.
func mulConfigUint32(left uint32, right uint32) (uint32, error) {
	product, err := mulUint32(left, right)
	if err != nil {
		return 0, errInvalidLimits
	}
	return product, nil
}

// maxUint32 returns the largest supplied value.
func maxUint32(values ...uint32) uint32 {
	var largest uint32
	for _, value := range values {
		if value > largest {
			largest = value
		}
	}
	return largest
}
