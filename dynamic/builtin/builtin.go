package builtin
	
import (
	"reflect"
	"strconv"
	"fmt"
	"unsafe"
)
var zeroVal reflect.Value
//Value(v)
func Value(v interface{}) reflect.Value {
	t := builtinType(v)
	return reflect.New(t)
}
//Type(v)
func Type(v interface{}) reflect.Type {
	
	return builtinType(v)
}
//Panic(v)
func Panic(v interface{}) {
	panic(v)
}
//Make([]T, length, cap)
//Make([T]T, length)
//Make(Chan, length)
func Make(typ interface{}, args ...int) interface{} {
	t := builtinType(typ)
	switch t.Kind() {
	case reflect.Slice:
		l, c := 0, 0
		if len(args) == 1 {
			l = args[0]
			c = l
		} else if len(args) > 1 {
			l, c = args[0], args[1]
		}
		return reflect.MakeSlice(t, l, c).Interface()
	case reflect.Map:
		if len(args) == 1 {
			return reflect.MakeMapWithSize(t, args[0]).Interface()
		}
		return reflect.MakeMap(t).Interface()
	//case reflect.Func:
	//	fn := func(args []Value) (results []Value)
	//
	//	reflect.FuncOf(in, out []Type, variadic bool) Type
	//	return reflect.MakeFunc(t, fn)
	case reflect.Chan:
		return MakeChan(t, args...)
	}
	panic(fmt.Sprintf("cannot make type `%v`", typ))
}
//MakeMap(T)
func MakeMap(typ interface{}, n ...int) interface{} {
	
	return reflect.MakeMap(builtinType(typ)).Interface()
}
//MapOf(T,T)
func MapOf(key, val interface{}) interface{} {
	return reflect.MapOf(builtinType(key), builtinType(val))
}
//MapFrom(T1,V1, T2,V2, ...)
func MapFrom(args ...interface{}) interface{} {
	n := len(args)
	if (n & 1) != 0 {
		panic("please use `MapFrom(key1, val1, key2, val2, ...)`")
	}
	if n == 0 {
		return make(map[string]interface{})
	}
	switch kind2Args(args, 0) {
	case reflect.String:
		switch kind2Args(args, 1) {
		case reflect.String:
			ret := make(map[string]string, n>>1)
			for i := 0; i < n; i += 2 {
				ret[args[i].(string)] = args[i+1].(string)
			}
			return ret
		case reflect.Int:
			ret := make(map[string]int, n>>1)
			for i := 0; i < n; i += 2 {
				ret[args[i].(string)] = asInt(args[i+1])
			}
			return ret
		case reflect.Float64:
			ret := make(map[string]float64, n>>1)
			for i := 0; i < n; i += 2 {
				ret[args[i].(string)] = asFloat(args[i+1])
			}
			return ret
		default:
			ret := make(map[string]interface{}, n>>1)
			for i := 0; i < n; i += 2 {
				ret[args[i].(string)] = args[i+1]
			}
			return ret
		}
	case reflect.Int:
		switch kind2Args(args, 1) {
		case reflect.String:
			ret := make(map[int]string, n>>1)
			for i := 0; i < n; i += 2 {
				ret[asInt(args[i])] = args[i+1].(string)
			}
			return ret
		case reflect.Int:
			ret := make(map[int]int, n>>1)
			for i := 0; i < n; i += 2 {
				ret[asInt(args[i])] = asInt(args[i+1])
			}
			return ret
		case reflect.Float64:
			ret := make(map[int]float64, n>>1)
			for i := 0; i < n; i += 2 {
				ret[asInt(args[i])] = asFloat(args[i+1])
			}
			return ret
		default:
			ret := make(map[int]interface{}, n>>1)
			for i := 0; i < n; i += 2 {
				ret[asInt(args[i])] = args[i+1];
			}
			return ret
		}
	default:
		panic("MapFrom: key type only support `string`, `int` now")
	}
}
//Delete(map[T]T, "key")
func Delete(m interface{}, key interface{}) {
	reflect.ValueOf(m).SetMapIndex(reflect.ValueOf(key), zeroVal)
}
//Set([]T, 位置0,值1, 位置1,值2, 位置2,值3)
//Set(map[T]T, 键名0,值1, 键名1,值2, 键名2,值3)
//Set(struct{}, 名称0,值1, 名称1,值2, 名称2,值3)
func Set(m interface{}, args ...interface{}) {
	n := len(args)
	if (n & 1) != 0 {
		panic("call with invalid argument count: please use `Set(obj, member1, val1, ...)")
	}
	o := reflect.ValueOf(m)
	switch o.Kind() {
	case reflect.Slice, reflect.Array:
		telem := reflect.TypeOf(m).Elem()
		for i := 0; i < n; i += 2 {
			val := autoConvert(telem, args[i+1])
			o.Index(args[i].(int)).Set(val)
		}
	case reflect.Map:
		setMapMember(o, args...)
	default:
		setMember(m, args...)
	}
}
//SetIndex(map[T]T, key, val)
//SetIndex([]T, index, val)
//SetIndex(struct{}, key, val)
func SetIndex(m, key, v interface{}) {
	o := reflect.ValueOf(m)
	switch o.Kind() {
	case reflect.Map:
		var val reflect.Value
		if v == nil {
			val = zeroVal
		} else {
			val = autoConvert(o.Type().Elem(), v)
		}
		o.SetMapIndex(reflect.ValueOf(key), val)
	case reflect.Slice, reflect.Array:
		if idx, ok := key.(int); ok {
			o.Index(idx).Set(reflect.ValueOf(v))
			return
		}
		panic("slice index isn't an int type")
	default:
		setMember(m, key, v)
	}
}
//Get(map[T]T, key)
//Get([]T, index)
//Get(struct{}, key)
//Get(string, index)
//Get(number, index)
func Get(m interface{}, key interface{}) interface{} {
	o := reflect.ValueOf(m)
	var s string
	switch o.Kind() {
	case reflect.Map:
		v := o.MapIndex(reflect.ValueOf(key))
		if v.IsValid() {
			return v.Interface()
		}
		return nil
	case reflect.Slice, reflect.String, reflect.Array:
		if idx, ok := key.(int); ok {
			if o.Len() > idx {
				return o.Index(idx).Interface()
			}
			panic(fmt.Errorf("index out of range [%d] with length %d", idx, o.Len()))
		}
		panic("slice key isn't an int type")
   	case reflect.Ptr, reflect.Interface, reflect.Struct:
		return getMember(m, key)
    case reflect.Complex64, reflect.Complex128:
    	return 0
    case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
    	s = strconv.Itoa(int(o.Int()))
    	fallthrough
    case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
    	s = strconv.Itoa(int(o.Uint()))
    	fallthrough
	default:
		if idx, ok := key.(int); ok {
			if len(s) > idx {
				return s[idx]
			}
			if len(s) != 0 {
				return 0
			}
		}
	}
	panic(fmt.Errorf("type %v does not support %v get", o.Kind(), key))
}
//Len([]T)
//Len(string)
//Len(map[T]T)
func Len(a interface{}) int {
	if a == nil {
		return 0
	}
	return reflect.ValueOf(a).Len()
}
//Cap([]T)
func Cap(a interface{}) int {
	if a == nil {
		return 0
	}
	return reflect.ValueOf(a).Cap()
}
//GetSlice([]T, 1, 5)
func GetSlice(a, i, j interface{}) interface{} {
	var va = reflect.ValueOf(a)
	var i1, j1 int
	if i != nil {
		i1 = asInt(i)
	}
	if j != nil {
		j1 = asInt(j)
	} else {
		j1 = va.Len()
	}
	return va.Slice(i1, j1).Interface()
}
//Copy([]T, []T)
func Copy(a, b interface{}) int {
	return reflect.Copy(reflect.ValueOf(a), reflect.ValueOf(b))
}
//Append([]T, value...)
func Append(a interface{}, vals ...interface{}) interface{} {
	switch arr := a.(type) {
	case []int:
		return appendInts(arr, vals...)
	case []interface{}:
		return append(arr, vals...)
	case []string:
		return appendStrings(arr, vals...)
	case []byte:
		return appendBytes(arr, vals...)
	case []rune:
		return appendRunes(arr, vals...)
	case []float64:
		return appendFloats(arr, vals...)
	}
	return appendInterface(a, vals ...)
}

//SliceOf(T)
func SliceOf(typ interface{}) interface{} {
	return reflect.SliceOf(builtinType(typ))
}
//MakeSlice(T, len, cap)
func MakeSlice(typ interface{}, args ...interface{}) interface{} {
	l, c := 0, 0
	if len(args) == 1 {
		if v, ok := args[0].(int); ok {
			l, c = v, v
		} else {
			panic("second param type of func `slice` must be `int`")
		}
	} else if len(args) > 1 {
		if v, ok := args[0].(int); ok {
			l = v
		} else {
			panic("2nd param type of func `slice` must be `int`")
		}
		if v, ok := args[1].(int); ok {
			c = v
		} else {
			panic("3rd param type of func `slice` must be `int`")
		}
	}
	typSlice := reflect.SliceOf(builtinType(typ))
	return reflect.MakeSlice(typSlice, l, c).Interface()
}
//SliceFrom(值0, 值1,...)
func SliceFrom(args ...interface{}) interface{} {
	n := len(args)
	if n == 0 {
		return []interface{}(nil)
	}
	
	switch kindArgs(args) {
	case reflect.Int:
		return appendInts(make([]int, 0, n), args...)
	case reflect.Float64:
		return appendFloats(make([]float64, 0, n), args...)
	case reflect.String:
		return appendStrings(make([]string, 0, n), args...)
	case reflect.Uint8:
		return appendBytes(make([]byte, 0, n), args...)
	default:
		return append(make([]interface{}, 0, n), args...)
	}
}

//StructInit(struct{}, key1, val1, ...)
func StructInit(args ...interface{}) interface{} {
	if (len(args) & 1) != 1 {
		panic("call with invalid argument count: please use `structInit(structType, member1, val1, ...)")
	}
	t := builtinType(args[0])
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("`%v` is not a struct type", args[0]))
	}
	
	ret := reflect.New(t)
	setStructMember(ret.Elem(), args[1:]...)
	return ret.Interface()
}
//MapInit(map[T]T, key1, val0, ...)
func MapInit(args ...interface{}) interface{} {
	if (len(args) & 1) != 1 {
		panic("call with invalid argument count: please use `mapInit(mapType, member1, val1, ...)")
	}
	
	t := builtinType(args[0])
	if t.Kind() != reflect.Map {
		panic(fmt.Sprintf("`%v` is not a map type", args[0]))
	}
	ret := reflect.MakeMap(t)
	setMapMember(ret, args[1:]...)
	return ret.Interface()
}
//Float64 returns float64(a)
func Float64(a interface{}) float64 {
	switch a1 := a.(type) {
	case int:
		return float64(a1)
	case float64:
		return a1
	case unsafe.Pointer:
		return *(*float64)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("float", a)
	return 0
}
// Float32 returns float32(a)
func Float32(a interface{}) float32 {
	switch a1 := a.(type) {
	case int:
		return float32(a1)
	case float64:
		return float32(a1)
	case float32:
		return a1
	case unsafe.Pointer:
		return *(*float32)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("float32", a)
	return 0
}
// Int returns int(a)
func Int(a interface{}) int {
	switch a1 := a.(type) {
	case float64:
		return int(a1)
	case int:
		return a1
	case unsafe.Pointer:
		return *(*int)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("int", a)
	return 0
}
// Int8 returns int8(a)
func Int8(a interface{}) int8 {
	switch a1 := a.(type) {
	case float64:
		return int8(a1)
	case int:
		return int8(a1)
	case int8:
		return a1
	case unsafe.Pointer:
		return *(*int8)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("int8", a)
	return 0
}
// Int16 returns int16(a)
func Int16(a interface{}) int16 {
	switch a1 := a.(type) {
	case float64:
		return int16(a1)
	case int:
		return int16(a1)
	case int16:
		return a1
	case unsafe.Pointer:
		return *(*int16)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("int16", a)
	return 0
}
// Int32 returns int32(a)
func Int32(a interface{}) int32 {
	switch a1 := a.(type) {
	case float64:
		return int32(a1)
	case int:
		return int32(a1)
	case int32:
		return a1
	case unsafe.Pointer:
		return *(*int32)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("int32", a)
	return 0
}
// rune returns rune(a)
func Rune(a interface{}) rune {
	switch a1 := a.(type) {
	case float64:
		return rune(a1)
	case int:
		return rune(a1)
	case rune:
		return a1
	case unsafe.Pointer:
		return *(*rune)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("rune", a)
	return 0
}
// Int64 returns int64(a)
func Int64(a interface{}) int64 {
	switch a1 := a.(type) {
	case float64:
		return int64(a1)
	case int:
		return int64(a1)
	case int64:
		return a1
	case unsafe.Pointer:
		return *(*int64)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("int64", a)
	return 0
}
// Uint returns uint(a)
func Uint(a interface{}) uint {
	switch a1 := a.(type) {
	case float64:
		return uint(a1)
	case int:
		return uint(a1)
	case uint:
		return a1
	case unsafe.Pointer:
		return *(*uint)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("uint", a)
	return 0
}
// Uint8 returns uint8(a)
func Uint8(a interface{}) uint8 {
	switch a1 := a.(type) {
	case int:
		return uint8(a1)
	case float64:
		return uint8(a1)
	case uint8:
		return a1
	case unsafe.Pointer:
		return *(*uint8)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("uint8", a)
	return 0
}
// Byte returns byte(a)
func Byte(a interface{}) byte {
	switch a1 := a.(type) {
	case int:
		return byte(a1)
	case float64:
		return byte(a1)
	case byte:
		return a1
	case unsafe.Pointer:
		return *(*byte)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("byte", a)
	return 0
}
// Uint16 returns uint16(a)
func Uint16(a interface{}) uint16 {
	switch a1 := a.(type) {
	case float64:
		return uint16(a1)
	case int:
		return uint16(a1)
	case uint16:
		return a1
	case unsafe.Pointer:
		return *(*uint16)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("uint16", a)
	return 0
}
// Uint32 returns uint32(a)
func Uint32(a interface{}) uint32 {
	switch a1 := a.(type) {
	case float64:
		return uint32(a1)
	case int:
		return uint32(a1)
	case uint32:
		return a1
	case unsafe.Pointer:
		return *(*uint32)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("uint32", a)
	return 0
}
// Uint64 returns uint64(a)
func Uint64(a interface{}) uint64 {
	switch a1 := a.(type) {
	case float64:
		return uint64(a1)
	case int:
		return uint64(a1)
	case uint64:
		return a1
	case unsafe.Pointer:
		return *(*uint64)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("uint64", a)
	return 0
}
// Complex64 returns complex64(a)
func Complex64(a interface{}) complex64 {
	switch a1 := a.(type) {
	case complex64:
		return a1
	case unsafe.Pointer:
		return *(*complex64)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("complex64", a)
	return 0
}
// Complex128 returns complex128(a)
func Complex128(a interface{}) complex128 {
	switch a1 := a.(type) {
	case complex128:
		return a1
	case unsafe.Pointer:
		return *(*complex128)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("complex128", a)
	return 0
}
// Uintptr returns uintptr(a)
func Uintptr(a interface{}) uintptr {
	switch a1 := a.(type) {
	case uintptr:
		return a1
	default:
		return reflect.ValueOf(a).Pointer()
	}
	panicUnsupportedFn("uintptr", a)
	return 0
}
// Uintptr returns uintptr(a)
func Pointer(a interface{}) unsafe.Pointer {
	switch a1 := a.(type) {
	case unsafe.Pointer:
		return a1
	default:
		return unsafe.Pointer(reflect.ValueOf(a).Pointer())
	}
	panicUnsupportedFn("uintptr", a)
	return unsafe.Pointer(uintptr(0))
}
// String returns string(a)
func String(a interface{}) string {
	switch a1 := a.(type) {
	case []byte:
		return string(a1)
	case int:
		return string(a1)
	case string:
		return a1
	case unsafe.Pointer:
		return *(*string)(unsafe.Pointer(a1))
	}
	panicUnsupportedFn("string", a)
	return ""
}
// Bool returns bool(a)
func Bool(a interface{}) bool {
	switch a1 := a.(type) {
	case bool:
		return a1
	case int:
		return a1 == 1
	}
	panicUnsupportedFn("bool", a)
	return false
}
