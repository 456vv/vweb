package builtin

import (
	"fmt"
	"reflect"
	"strconv"
	"unsafe"
)

var zeroVal reflect.Value

// Value(v)
func Value(v any) reflect.Value {
	t := builtinType(v)
	return reflect.New(t)
}

// Type(v)
func Type(v any) reflect.Type {
	t := builtinType(v)
	return reflect.PointerTo(t)
}

// Panic(v)
func Panic(v any) {
	panic(v)
}

// Make([]T, length, cap)
// Make([T]T, length)
// Make(Chan, length)
// Make(func, func([]reflect.Value)[]reflect.Value)
func Make(typ any, args ...any) any {
	v := Value(typ)
	typeInit(v.Elem(), true, args...)
	return v.Elem().Interface()
}

// MapFrom(M, T1, V1, T2, V2, ...)
func MapFrom(m any, args ...any) any {
	n := len(args)
	if (n & 1) != 0 {
		panic("please use `MapFrom(T, key1, val1, key2, val2, ...)`")
	}
	if n == 0 {
		return make(map[string]any)
	}
	if m != nil {
		tt := reflect.TypeOf(m)
		if tt.Kind() == reflect.Map {
			return setMapMember(m, args...)
		}

		// 默认接口类型
		mkey := reflect.TypeOf((*any)(nil)).Elem()
		mval := reflect.TypeOf((*any)(nil)).Elem()

		mrkey := kind2Args(args, 0)
		if mrkey != reflect.Invalid {
			mtkey, ok := builtinTypes[mrkey.String()]
			if ok {
				// 是基本类型
				mkey = mtkey
			} else {
				// 不是基本类型
				mkey = reflect.TypeOf(args[0])
			}
		}

		mrval := kind2Args(args, 1)
		if mrval != reflect.Invalid {
			mrval, ok := builtinTypes[mrval.String()]
			if ok {
				// 是基本类型
				mval = mrval
			} else {
				// 不是基本类型
				mval = reflect.TypeOf(args[1])
			}
		}

		mt := reflect.MapOf(mkey, mval)
		mv := reflect.MakeMapWithSize(mt, n/2)
		return setMapMember(mv.Interface(), args...)
	}

	// 如果M是nil
	switch kind2Args(args, 0) {
	case reflect.String:
		switch kind2Args(args, 1) {
		case reflect.String:
			ret := make(map[string]string, n>>1)
			for i := 0; i < n; i += 2 {
				key, _ := args[i].(string)
				val, _ := args[i+1].(string)
				if key == "" {
					continue
				}
				ret[key] = val
			}
			return ret
		case reflect.Int:
			ret := make(map[string]int, n>>1)
			for i := 0; i < n; i += 2 {
				key, _ := args[i].(string)
				if key == "" {
					continue
				}
				ret[key] = asInt(args[i+1])
			}
			return ret
		case reflect.Float64:
			ret := make(map[string]float64, n>>1)
			for i := 0; i < n; i += 2 {
				key, _ := args[i].(string)
				if key == "" {
					continue
				}
				ret[key] = asFloat(args[i+1])
			}
			return ret
		default:
			ret := make(map[string]any, n>>1)
			for i := 0; i < n; i += 2 {
				key, _ := args[i].(string)
				if key == "" {
					continue
				}
				ret[key] = args[i+1]
			}
			return ret
		}
	case reflect.Int:
		switch kind2Args(args, 1) {
		case reflect.String:
			ret := make(map[int]string, n>>1)
			for i := 0; i < n; i += 2 {
				val, _ := args[i+1].(string)
				ret[asInt(args[i])] = val
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
			ret := make(map[int]any, n>>1)
			for i := 0; i < n; i += 2 {
				ret[asInt(args[i])] = args[i+1]
			}
			return ret
		}
	default:
		panic("MapFrom: key type only support `string`, `int` now")
	}
}

// SliceFrom(T, 值0, 值1,...)
func SliceFrom(t any, args ...any) any {
	n := len(args)
	if n == 0 {
		return []any(nil)
	}

	if t != nil {
		tt := reflect.TypeOf(t)
		if tt.Kind() == reflect.Array || tt.Kind() == reflect.Slice {
			return appendInterface(t, args...)
		}
		arr := reflect.MakeSlice(reflect.SliceOf(tt), 0, n)
		return appendInterface(arr.Interface(), args...)
	}

	// t == nil
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
		return append(make([]any, 0, n), args...)
	}
}

// Delete(map[T]T, "key")
func Delete(m any, key any) {
	reflect.ValueOf(m).SetMapIndex(reflect.ValueOf(key), zeroVal)
}

// Set([]T, 位置0,值1, 位置1,值2, 位置2,值3)
// Set(map[T]T, 键名0,值1, 键名1,值2, 键名2,值3)
// Set(struct{}, 名称0,值1, 名称1,值2, 名称2,值3)
func Set(m any, args ...any) {
	n := len(args)
	if (n & 1) != 0 {
		panic("call with invalid argument count: please use Set(obj, member1, val1, ...)")
	}
	o := reflect.Indirect(reflect.ValueOf(m))
	switch o.Kind() {
	case reflect.Slice, reflect.Array:
		telem := o.Type().Elem() // 修复盲区：原来为 reflect.TypeOf(m) 会导致传指针时获取 Elem 错误
		for i := 0; i < n; i += 2 {
			index, ok := args[i].(int)
			if !ok {
				panic("slice position is not a valid `int` type")
			}
			val := autoConvert(telem, args[i+1])

			idxRef := o.Index(index)
			if !idxRef.CanSet() { // 修复盲区：如果用户传入的是 Array 的值而非指针，拦截原生恐慌
				panic("array/slice element is unaddressable or unexported, please pass a pointer")
			}
			idxRef.Set(val)
		}
	case reflect.Map:
		setMapMember(m, args...)
	default:
		setMember(m, args...)
	}
}

// Get(map[T]T, key)
// Get([]T, index)
// Get(struct{}, key)
// Get(string, index)
// Get(number, index)
func Get(m any, key any) any {
	o := reflect.Indirect(reflect.ValueOf(m))
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
		panic("slice/array/string key isn't an int type")
	case reflect.Pointer, reflect.Interface, reflect.Struct:
		return getMember(m, key)
	case reflect.Complex64, reflect.Complex128:
		return 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		s = strconv.Itoa(int(o.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		s = strconv.Itoa(int(o.Uint()))
	}
	if idx, ok := key.(int); ok {
		if len(s) > idx {
			return s[idx]
		}
		if len(s) != 0 {
			return 0
		}
	}
	panic(fmt.Errorf("type %v does not support %v get", o.Kind(), key))
}

func Len(a any) int {
	if a == nil {
		return 0
	}
	v := inDirect(reflect.ValueOf(a))
	if !v.IsValid() {
		return 0
	}
	switch v.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return v.Len()
	}
	return 0
}

// Cap([]T)
func Cap(a any) int {
	if a == nil {
		return 0
	}
	v := inDirect(reflect.ValueOf(a))
	if !v.IsValid() {
		return 0
	}
	switch v.Kind() {
	case reflect.Array, reflect.Slice:
		return v.Cap()
	}
	return 0
}

// GetSlice([]T, 1, 5)
func GetSlice(a, i, j any) any {
	va := reflect.ValueOf(a)
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

// GetSlice3([]T, 1, 5, 6)
func GetSlice3(a, i, j, c any) any {
	va := reflect.ValueOf(a)
	var i1, j1, c1 int
	if i != nil {
		i1 = asInt(i)
	}
	if j != nil {
		j1 = asInt(j)
	} else {
		j1 = va.Len()
	}
	if c != nil {
		c1 = asInt(c)
	} else {
		c1 = va.Len()
	}
	return va.Slice3(i1, j1, c1).Interface()
}

// Copy([]T, []T)
func Copy(a, b any) int {
	return reflect.Copy(reflect.ValueOf(a), reflect.ValueOf(b))
}

// Append([]T, value...)
func Append(a any, vals ...any) any {
	switch arr := a.(type) {
	case []int:
		return appendInts(arr, vals...)
	case []any:
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
	return appendInterface(a, vals...)
}

// Float64 returns float64(a)
func Float64(a any) float64 {
	switch a1 := a.(type) {
	case float32:
		return float64(a1)
	case float64:
		return float64(a1)
	case int:
		return float64(a1)
	case int8:
		return float64(a1)
	case int16:
		return float64(a1)
	case int32:
		return float64(a1)
	case int64:
		return float64(a1)
	case uint:
		return float64(a1)
	case uint8:
		return float64(a1)
	case uint16:
		return float64(a1)
	case uint32:
		return float64(a1)
	case uint64:
		return float64(a1)
	case unsafe.Pointer:
		return *(*float64)(a1)
	}
	return autoConvert(builtinType(float64(0)), a).Float()
}

// Float32 returns float32(a)
func Float32(a any) float32 {
	switch a1 := a.(type) {
	case float32:
		return float32(a1)
	case float64:
		return float32(a1)
	case int:
		return float32(a1)
	case int8:
		return float32(a1)
	case int16:
		return float32(a1)
	case int32:
		return float32(a1)
	case int64:
		return float32(a1)
	case uint:
		return float32(a1)
	case uint8:
		return float32(a1)
	case uint16:
		return float32(a1)
	case uint32:
		return float32(a1)
	case uint64:
		return float32(a1)
	case unsafe.Pointer:
		return *(*float32)(a1)
	}
	return float32(autoConvert(builtinType(float32(0)), a).Float())
}

// Int returns int(a)
func Int(a any) int {
	switch a1 := a.(type) {
	case float32:
		return int(a1)
	case float64:
		return int(a1)
	case int:
		return int(a1)
	case int8:
		return int(a1)
	case int16:
		return int(a1)
	case int32:
		return int(a1)
	case int64:
		return int(a1)
	case uint:
		return int(a1)
	case uint8:
		return int(a1)
	case uint16:
		return int(a1)
	case uint32:
		return int(a1)
	case uint64:
		return int(a1)
	case unsafe.Pointer:
		return *(*int)(a1)
	}
	return int(autoConvert(builtinType(int(0)), a).Int())
}

// Int8 returns int8(a)
func Int8(a any) int8 {
	switch a1 := a.(type) {
	case float32:
		return int8(a1)
	case float64:
		return int8(a1)
	case int:
		return int8(a1)
	case int8:
		return int8(a1)
	case int16:
		return int8(a1)
	case int32:
		return int8(a1)
	case int64:
		return int8(a1)
	case uint:
		return int8(a1)
	case uint8:
		return int8(a1)
	case uint16:
		return int8(a1)
	case uint32:
		return int8(a1)
	case uint64:
		return int8(a1)
	case unsafe.Pointer:
		return *(*int8)(a1)
	}
	return int8(autoConvert(builtinType(int8(0)), a).Int())
}

// Int16 returns int16(a)
func Int16(a any) int16 {
	switch a1 := a.(type) {
	case float32:
		return int16(a1)
	case float64:
		return int16(a1)
	case int:
		return int16(a1)
	case int8:
		return int16(a1)
	case int16:
		return int16(a1)
	case int32:
		return int16(a1)
	case int64:
		return int16(a1)
	case uint:
		return int16(a1)
	case uint8:
		return int16(a1)
	case uint16:
		return int16(a1)
	case uint32:
		return int16(a1)
	case uint64:
		return int16(a1)
	case unsafe.Pointer:
		return *(*int16)(a1)
	}
	return int16(autoConvert(builtinType(int16(0)), a).Int())
}

// Int32 returns int32(a)
func Int32(a any) int32 {
	switch a1 := a.(type) {
	case float32:
		return int32(a1)
	case float64:
		return int32(a1)
	case int:
		return int32(a1)
	case int8:
		return int32(a1)
	case int16:
		return int32(a1)
	case int32:
		return int32(a1)
	case int64:
		return int32(a1)
	case uint:
		return int32(a1)
	case uint8:
		return int32(a1)
	case uint16:
		return int32(a1)
	case uint32:
		return int32(a1)
	case uint64:
		return int32(a1)
	case unsafe.Pointer:
		return *(*int32)(a1)
	}
	return int32(autoConvert(builtinType(int32(0)), a).Int())
}

// rune returns rune(a)
func Rune(a any) rune {
	switch a1 := a.(type) {
	case float32:
		return rune(a1)
	case float64:
		return rune(a1)
	case int:
		return rune(a1)
	case int8:
		return rune(a1)
	case int16:
		return rune(a1)
	case int32:
		return rune(a1)
	case int64:
		return rune(a1)
	case uint:
		return rune(a1)
	case uint8:
		return rune(a1)
	case uint16:
		return rune(a1)
	case uint32:
		return rune(a1)
	case uint64:
		return rune(a1)
	case unsafe.Pointer:
		return *(*rune)(a1)
	}
	panicUnsupportedOp1("rune", a)
	return 0
}

// Int64 returns int64(a)
func Int64(a any) int64 {
	switch a1 := a.(type) {
	case float32:
		return int64(a1)
	case float64:
		return int64(a1)
	case int:
		return int64(a1)
	case int8:
		return int64(a1)
	case int16:
		return int64(a1)
	case int32:
		return int64(a1)
	case int64:
		return int64(a1)
	case uint:
		return int64(a1)
	case uint8:
		return int64(a1)
	case uint16:
		return int64(a1)
	case uint32:
		return int64(a1)
	case uint64:
		return int64(a1)
	case unsafe.Pointer:
		return *(*int64)(a1)
	}
	return autoConvert(builtinType(int64(0)), a).Int()
}

// Uint returns uint(a)
func Uint(a any) uint {
	switch a1 := a.(type) {
	case float32:
		return uint(a1)
	case float64:
		return uint(a1)
	case int:
		return uint(a1)
	case int8:
		return uint(a1)
	case int16:
		return uint(a1)
	case int32:
		return uint(a1)
	case int64:
		return uint(a1)
	case uint:
		return uint(a1)
	case uint8:
		return uint(a1)
	case uint16:
		return uint(a1)
	case uint32:
		return uint(a1)
	case uint64:
		return uint(a1)
	case unsafe.Pointer:
		return *(*uint)(a1)
	}
	return uint(autoConvert(builtinType(uint(0)), a).Uint())
}

// Uint8 returns uint8(a)
func Uint8(a any) uint8 {
	switch a1 := a.(type) {
	case float32:
		return uint8(a1)
	case float64:
		return uint8(a1)
	case int:
		return uint8(a1)
	case int8:
		return uint8(a1)
	case int16:
		return uint8(a1)
	case int32:
		return uint8(a1)
	case int64:
		return uint8(a1)
	case uint:
		return uint8(a1)
	case uint8:
		return uint8(a1)
	case uint16:
		return uint8(a1)
	case uint32:
		return uint8(a1)
	case uint64:
		return uint8(a1)
	case unsafe.Pointer:
		return *(*uint8)(a1)
	}
	return uint8(autoConvert(builtinType(uint8(0)), a).Uint())
}

// Byte returns byte(a)
func Byte(a any) byte {
	switch a1 := a.(type) {
	case float32:
		return byte(a1)
	case float64:
		return byte(a1)
	case int:
		return byte(a1)
	case int8:
		return byte(a1)
	case int16:
		return byte(a1)
	case int32:
		return byte(a1)
	case int64:
		return byte(a1)
	case uint:
		return byte(a1)
	case uint8:
		return byte(a1)
	case uint16:
		return byte(a1)
	case uint32:
		return byte(a1)
	case uint64:
		return byte(a1)
	case unsafe.Pointer:
		return *(*byte)(a1)
	}
	panicUnsupportedOp1("byte", a)
	return 0
}

// Uint16 returns uint16(a)
func Uint16(a any) uint16 {
	switch a1 := a.(type) {
	case float32:
		return uint16(a1)
	case float64:
		return uint16(a1)
	case int:
		return uint16(a1)
	case int8:
		return uint16(a1)
	case int16:
		return uint16(a1)
	case int32:
		return uint16(a1)
	case int64:
		return uint16(a1)
	case uint:
		return uint16(a1)
	case uint8:
		return uint16(a1)
	case uint16:
		return uint16(a1)
	case uint32:
		return uint16(a1)
	case uint64:
		return uint16(a1)
	case unsafe.Pointer:
		return *(*uint16)(a1)
	}
	return uint16(autoConvert(builtinType(uint16(0)), a).Uint())
}

// Uint32 returns uint32(a)
func Uint32(a any) uint32 {
	switch a1 := a.(type) {
	case float32:
		return uint32(a1)
	case float64:
		return uint32(a1)
	case int:
		return uint32(a1)
	case int8:
		return uint32(a1)
	case int16:
		return uint32(a1)
	case int32:
		return uint32(a1)
	case int64:
		return uint32(a1)
	case uint:
		return uint32(a1)
	case uint8:
		return uint32(a1)
	case uint16:
		return uint32(a1)
	case uint32:
		return uint32(a1)
	case uint64:
		return uint32(a1)
	case unsafe.Pointer:
		return *(*uint32)(a1)
	}
	return uint32(autoConvert(builtinType(uint32(0)), a).Uint())
}

// Uint64 returns uint64(a)
func Uint64(a any) uint64 {
	switch a1 := a.(type) {
	case float32:
		return uint64(a1)
	case float64:
		return uint64(a1)
	case int:
		return uint64(a1)
	case int8:
		return uint64(a1)
	case int16:
		return uint64(a1)
	case int32:
		return uint64(a1)
	case int64:
		return uint64(a1)
	case uint:
		return uint64(a1)
	case uint8:
		return uint64(a1)
	case uint16:
		return uint64(a1)
	case uint32:
		return uint64(a1)
	case uint64:
		return uint64(a1)
	case unsafe.Pointer:
		return *(*uint64)(a1)
	}
	return autoConvert(builtinType(uint64(0)), a).Uint()
}

// Complex64 returns complex64(a)
func Complex64(a any) complex64 {
	switch a1 := a.(type) {
	case complex64:
		return a1
	case complex128:
		return complex64(a1)
	case unsafe.Pointer:
		return *(*complex64)(a1)
	}
	return complex64(autoConvert(builtinType(complex64(0)), a).Complex())
}

// Complex128 returns complex128(a)
func Complex128(a any) complex128 {
	switch a1 := a.(type) {
	case complex64:
		return complex128(a1)
	case complex128:
		return a1
	case unsafe.Pointer:
		return *(*complex128)(a1)
	}
	return autoConvert(builtinType(complex128(0)), a).Complex()
}

// Uintptr returns uintptr(a)
func Uintptr(a any) uintptr {
	switch a1 := a.(type) {
	case uintptr:
		return a1
	case unsafe.Pointer:
		return uintptr(a1)
	}
	return reflect.ValueOf(a).Pointer()
}

// Uintptr returns uintptr(a)
func Pointer(a any) unsafe.Pointer {
	switch a1 := a.(type) {
	case unsafe.Pointer:
		return a1
	case uintptr:
		return unsafe.Pointer(a1)
	}
	return reflect.ValueOf(a).UnsafePointer()
}

// String returns string(a)
func String(a any) string {
	switch a1 := a.(type) {
	case []byte:
		return string(a1)
	case []rune:
		return string(a1)
	case int:
		return strconv.Itoa(a1)
	case string:
		return a1
	case unsafe.Pointer:
		return *(*string)(unsafe.Pointer(a1))
	}
	return autoConvert(builtinType("string"), a).String()
}

// Bool returns bool(a)
func Bool(a any) bool {
	switch a1 := a.(type) {
	case bool:
		return a1
	}
	return isTrue(inDirect(reflect.ValueOf(a)))
}

func Bytes(inf any) []byte {
	switch s := inf.(type) {
	case string:
		return []byte(s)
	case []byte:
		return s
	}
	return []byte(fmt.Sprintf("%s", inf))
}

func Runes(inf any) []rune {
	switch s := inf.(type) {
	case string:
		return []rune(s)
	case []rune:
		return s
	}
	return []rune(fmt.Sprintf("%s", inf))
}

func toValue(v any) reflect.Value {
	rv, ok := v.(reflect.Value)
	if !ok {
		rv = reflect.ValueOf(v).Elem()
	}
	return rv
}

func Convert(a, b any) bool {
	av := toValue(a)
	bv := toValue(b)
	return typeConvert(av, bv)
}

func Init(v any) {
	rv := toValue(v)
	typeInit(rv, false)
}

func CopyStruct(dsc, src any, exclude func(name string, dsc, src reflect.Value) bool) error {
	return copyStruct(reflect.ValueOf(dsc), reflect.ValueOf(src), "", exclude, false)
}

func CopyStructDeep(dsc, src any, exclude func(name string, dsc, src reflect.Value) bool) error {
	return copyStruct(reflect.ValueOf(dsc), reflect.ValueOf(src), "", exclude, true)
}

func copyStruct(dsc, src reflect.Value, name string, exclude func(name string, dsc reflect.Value, src reflect.Value) bool, deep bool) error {
	va := inDirect(dsc)
	vb := inDirect(src)
	if va.Kind() != vb.Kind() || va.Kind() != reflect.Struct {
		return fmt.Errorf("仅支持struct类型，dsc(%s)，src(%s)", va.Kind(), vb.Kind())
	}

	bt := vb.Type()
	for i := 0; i < bt.NumField(); i++ {

		bvField := vb.Field(i)
		if !bvField.IsValid() {
			continue
		}

		info := bt.Field(i)
		avField := va.FieldByName(info.Name)

		if exclude != nil && exclude(name+info.Name, avField, bvField) {
			continue
		}
		if !avField.IsValid() {
			continue
		}

		avfi := inDirect(avField)
		bvfi := inDirect(bvField)
		if !avfi.IsValid() && bvfi.IsValid() {
			typeInit(avField, false)
			avfi = inDirect(avField)
		}

		afKind := avfi.Kind()
		bfKind := bvfi.Kind()

		if deep && afKind == bfKind && afKind == reflect.Struct {
			copyStruct(avField, bvField, info.Name+".", exclude, deep)
			continue
		}

		if afKind == bfKind && afKind == reflect.Map {
			if bvfi.IsNil() {
				continue
			}
			bfType := bvfi.Type()
			afType := avfi.Type()

			if !bfType.Key().ConvertibleTo(afType.Key()) || !bfType.Elem().ConvertibleTo(afType.Elem()) {
				continue
			}
			if avfi.IsNil() {
				if !avfi.CanSet() { // 修复盲区：未公开字段不可复制 Map 分配
					continue
				}
				mt := reflect.MapOf(afType.Key(), afType.Elem())
				mv := reflect.MakeMapWithSize(mt, bvfi.Len())
				avfi.Set(mv)
			}
			bfmr := bvfi.MapRange()
			for bfmr.Next() {
				key := bfmr.Key().Convert(afType.Key())
				val := bfmr.Value().Convert(afType.Elem())
				avfi.SetMapIndex(key, val)
			}
			continue
		}

		if afKind == bfKind && afKind == reflect.Slice {
			if bvfi.IsNil() || avField.Type() != bvField.Type() {
				continue
			}
			nv := reflect.MakeSlice(bvField.Type(), 0, bvField.Cap())
			if bvField.Len() > 0 {
				nv = reflect.AppendSlice(nv, bvField)
			}
			if avField.CanSet() { // 修复盲区补充防崩
				avField.Set(nv)
			}
			continue
		}

		if avField.CanSet() {
			if bvField.Type().AssignableTo(avField.Type()) {
				avField.Set(bvField)
			} else if bvField.Type().ConvertibleTo(avField.Type()) {
				bvv := bvField.Convert(avField.Type())
				avField.Set(bvv)
			}
		}
	}
	return nil
}

func DepthField(s any, index ...any) (field any, err error) {
	field = s
	for _, i := range index {
		field, err = depthField(field, i)
		if err != nil {
			return nil, err
		}
	}
	return field, nil
}

func depthField(s any, index any) (any, error) {
	sv := inDirect(reflect.ValueOf(s))
	var v reflect.Value
	switch sv.Kind() {
	case reflect.Struct:
		switch idx := index.(type) {
		case string:
			v = sv.FieldByName(idx)
		case int:
			v = sv.Field(idx)
		}
	case reflect.Map:
		if sv.IsNil() {
			return nil, fmt.Errorf("该字段是 nil。错误的字段名为(%#v)", index)
		}
		v = sv.MapIndex(reflect.ValueOf(index))
	case reflect.Slice, reflect.Array:
		if i, ok := index.(int); ok && sv.Len() > i {
			v = sv.Index(i)
		}
	default:
		return nil, fmt.Errorf("非结构类型，无法正确读取。错误的类型为（%s）", sv.Kind())
	}

	if v.Kind() != reflect.Invalid {
		if !v.CanInterface() { // 修复盲区：不可导出的私有小写字段读取 panic 阻断
			return nil, fmt.Errorf("该字段未导出，不可读。字段名（%#v）", index)
		}
		return v.Interface(), nil
	}
	return nil, fmt.Errorf("该字段不是有效。错误的字段名为（%#v）", index)
}
