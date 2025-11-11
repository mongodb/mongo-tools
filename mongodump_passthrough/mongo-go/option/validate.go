package option

import (
	"reflect"

	mapset "github.com/deckarep/golang-set/v2"
)

var nilable = mapset.NewThreadUnsafeSet(
	reflect.Chan,
	reflect.Func,
	reflect.Interface,
	reflect.Map,
	reflect.Pointer,
	reflect.Slice,
)

func isNil(val any) bool {
	if val == nil {
		return true
	}

	if nilable.Contains(reflect.TypeOf(val).Kind()) {
		return reflect.ValueOf(val).IsNil()
	}

	return false
}
