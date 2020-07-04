package builtin
import (
	"reflect"
)

//Max(a1, a2, ...)
func Max(args ...interface{}) (max interface{}) {
	if len(args) == 0 {
		return 0
	}
	switch kindArgs(args) {
	case reflect.Int:
		return maxInt(args)
	case reflect.Float64:
		return maxFloat(args)
	}
	return panicUnsupportedFn("max", args)
}

//Min(a1, a2, ...)
func Min(args ...interface{}) (min interface{}) {
	if len(args) == 0 {
		return 0
	}
	switch kindArgs(args) {
	case reflect.Int:
		return minInt(args)
	case reflect.Float64:
		return minFloat(args)
	}
	return panicUnsupportedFn("min", args)
}
