package universe

import (
	"slices"
	"sort"

	"go.starlark.net/lib/json"
	"go.starlark.net/lib/math"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

const (
	// jsonName is the reserved json module root.
	jsonName = "json"

	// mathName is the reserved math module root.
	mathName = "math"

	// setName is the dialect-gated universe builtin excluded from the documented surface.
	setName = "set"

	// sumName is the reserved numeric-only sum builtin.
	sumName = "sum"

	// jsonMemberCount is the selected json surface: decode, encode, and indent.
	jsonMemberCount = 3

	// fixedAdditionCount is the always-on extras: sum, json, and math.
	fixedAdditionCount = 3
)

//nolint:gochecknoglobals // Shared frozen language-surface values are process-wide and immutable.
var (
	// jsonModule is the filtered json module shared across Predeclared dictionaries.
	jsonModule = selectedJSONModule()

	// sumBuiltin is the numeric-only sum function shared across Predeclared dictionaries.
	sumBuiltin = starlark.NewBuiltin(sumName, sum)

	// reservedRoots contains every forbidden top-level capability namespace.
	reservedRoots = collectReservedRoots()

	// topLevelNames is the sorted documented available surface, excluding set.
	topLevelNames = collectTopLevelNames()

	// jsonMemberNames is the sorted selected json module surface.
	jsonMemberNames = slices.Clone(jsonModule.Members.Keys())

	// mathMemberNames is the sorted pinned math module surface.
	mathMemberNames = slices.Clone(math.Module.Members.Keys())
)

// Predeclared returns a fresh dictionary of the fixed language surface.
//
// The dictionary itself is unfrozen so callers can merge capability
// namespaces. Member values are shared across calls: sum is a shared builtin,
// json is a shared filtered module, and math is the pinned math.Module.
// Freezing the returned dictionary therefore freezes those shared values.
func Predeclared() starlark.StringDict {
	return starlark.StringDict{
		sumName:  sumBuiltin,
		jsonName: jsonModule,
		mathName: math.Module,
	}
}

// TopLevelNames returns a sorted copy of the documented available top-level names.
//
// The list includes standard Starlark universe names except set, which is
// reserved but unavailable under the execution dialect, plus sum, json, and math.
func TopLevelNames() []string {
	return slices.Clone(topLevelNames)
}

// JSONMemberNames returns a sorted copy of the selected json module members.
func JSONMemberNames() []string {
	return slices.Clone(jsonMemberNames)
}

// MathMemberNames returns a sorted copy of the pinned math module members.
func MathMemberNames() []string {
	return slices.Clone(mathMemberNames)
}

// IsReservedRoot reports whether name is a forbidden top-level capability namespace.
//
// Name is the first dotted segment only. Nested leaves such as stats.sum remain
// legal. Membership includes every standard Starlark universe name, including
// dialect-gated set, plus sum, json, and math.
func IsReservedRoot(name string) bool {
	_, ok := reservedRoots[name]
	return ok
}

// selectedJSONModule copies decode, encode, and indent from the pinned json module.
func selectedJSONModule() *starlarkstruct.Module {
	members := make(starlark.StringDict, jsonMemberCount)
	for _, name := range []string{"decode", "encode", "indent"} {
		value, ok := json.Module.Members[name]
		if !ok {
			panic("pinned json module missing " + name)
		}
		members[name] = value
	}
	module := &starlarkstruct.Module{Name: jsonName, Members: members}
	module.Freeze()
	return module
}

// collectReservedRoots records every universe name plus the fixed stdlib roots.
func collectReservedRoots() map[string]struct{} {
	roots := make(map[string]struct{}, len(starlark.Universe)+fixedAdditionCount)
	for name := range starlark.Universe {
		roots[name] = struct{}{}
	}
	roots[jsonName] = struct{}{}
	roots[mathName] = struct{}{}
	roots[sumName] = struct{}{}
	return roots
}

// collectTopLevelNames returns sorted documented names, excluding dialect-gated set.
func collectTopLevelNames() []string {
	names := make([]string, 0, len(starlark.Universe)+fixedAdditionCount)
	for name := range starlark.Universe {
		if name == setName {
			continue
		}
		names = append(names, name)
	}
	names = append(names, jsonName, mathName, sumName)
	sort.Strings(names)
	return names
}
