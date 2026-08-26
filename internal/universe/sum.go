package universe

import (
	"errors"
	"fmt"
	"math"

	"go.starlark.net/starlark"
)

// sum returns the numeric total of one iterable of ints and floats.
//
// Empty input returns integer zero. The result stays an int until a float
// appears, then remaining ints are converted to float. Non-numeric values and
// extra arguments are errors.
func sum(
	_ *starlark.Thread,
	builtin *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var iterable starlark.Iterable
	if err := starlark.UnpackPositionalArgs(builtin.Name(), args, kwargs, 1, &iterable); err != nil {
		return nil, err
	}

	iterator := iterable.Iterate()
	defer iterator.Done()

	intTotal := starlark.MakeInt(0)
	var floatTotal starlark.Float
	useFloat := false
	var element starlark.Value
	for index := 0; iterator.Next(&element); index++ {
		switch value := element.(type) {
		case starlark.Int:
			if !useFloat {
				intTotal = intTotal.Add(value)
				continue
			}
			converted, err := asFiniteFloat(value)
			if err != nil {
				return nil, err
			}
			floatTotal += converted
		case starlark.Float:
			if !useFloat {
				converted, err := asFiniteFloat(intTotal)
				if err != nil {
					return nil, err
				}
				floatTotal = converted
				useFloat = true
			}
			floatTotal += value
		default:
			return nil, fmt.Errorf("sum: at index %d, got %s, want int or float", index, element.Type())
		}
	}
	if useFloat {
		return floatTotal, nil
	}
	return intTotal, nil
}

// asFiniteFloat converts an int to float or reports that the magnitude overflowed.
func asFiniteFloat(value starlark.Int) (starlark.Float, error) {
	converted := value.Float()
	if math.IsInf(float64(converted), 0) {
		return 0, errors.New("sum: int too large to convert to float")
	}
	return converted, nil
}
