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
	//	return reflect.MakeFunc(t, fn)
	case reflect.Chan:
		return MakeChan(t, args...)
	}
	
	panic(fmt.Sprintf("cannot make type `%v`", typ))
}

//MapFrom(M, T1, V1, T2, V2, ...)
func MapFrom(m interface{}, args ...interface{}) interface{} {
	n := len(args)
	if (n & 1) != 0 {
		panic("please use `MapFrom(T, key1, val1, key2, val2, ...)`")
	}
	if n == 0 {
		return make(map[string]interface{})
	}
	if m != nil {
		tt := reflect.TypeOf(m)
		if tt.Kind() == reflect.Map {
			return setMapMember(m, args...)
		}
		
		//默认接口类型
		mkey := reflect.TypeOf((*interface{})(nil)).Elem()
		mval := reflect.TypeOf((*interface{})(nil)).Elem()
		
		mrkey := kind2Args(args, 0)
		if mrkey != reflect.Invalid {
			mtkey, ok := builtinTypes[mrkey.String()]
			if ok {
				//是基本类型
				mkey = mtkey
			}else{
				//不是基本类型
				mkey = reflect.TypeOf(args[0])
			}
		}
		
		mrval := kind2Args(args, 1)
		if mrval != reflect.Invalid {
			mrval, ok := builtinTypes[mrval.String()]
			if ok {
				//是基本类型
				mval = mrval
			}else{
				//不是基本类型
				mval = reflect.TypeOf(args[1])
			}
		}
		
		mt := reflect.MapOf( mkey, mval )
		mv := reflect.MakeMapWithSize(mt, n/2)
		return setMapMember(mv.Interface(), args...)
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

//SliceFrom(T, 值0, 值1,...)
func SliceFrom(t interface{}, args ...interface{}) interface{} {
	
	n := len(args)
	if n == 0 {
		return []interface{}(nil)
	}
	
	if t != nil {
		tt := reflect.TypeOf(t)
		if tt.Kind() == reflect.Array || tt.Kind() == reflect.Slice {
			return appendInterface(t, args...)
		}
		arr := reflect.MakeSlice(reflect.SliceOf(tt), 0, n)
		return appendInterface(arr.Interface(), args...)
	}
	
	//t == nil
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
		setMapMember(m, args...)
	default:
		setMember(m, args...)
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

//Cap([]T)
func Cap(a interface{}) int {
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

//GetSlice([]T, 1, 5)
func GetSlice(a, i, j interface{}) interface{} {
	var va = reflect.ValueOf(a)
	var i1, j1 int
	if i != nil {
		i1 = asInt(i)
	}
	if j != nil {
		j1 = asInt(j)
	}else{
		j1 = va.Len()
	}
	return va.Slice(i1, j1).Interface()
}

//GetSlice3([]T, 1, 5, 6)
func GetSlice3(a, i, j, c interface{}) interface{} {
	var va = reflect.ValueOf(a)
	var i1, j1, c1 int
	if i != nil {
		i1 = asInt(i)
	}
	if j != nil {
		j1 = asInt(j)
	}else{
		j1 = va.Len()
	}
	if c != nil {
		c1 = asInt(c)
	}else{
		c1 = va.Len()
	}
	return va.Slice3(i1, j1, c1).Interface()
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

//Float64 returns float64(a)
func Float64(a interface{}) float64 {
	switch a1 := a.(type) {
	case int:
		return float64(a1)
	case int64:
		return float64(a1)
	case float32:
		return float64(a1)
	case float64:
		return a1
	case unsafe.Pointer:
		return *(*float64)(unsafe.Pointer(a1))
	default:
		return autoConvert(reflect.TypeOf(float64(0)), a).Float()
	}
	//panicUnsupportedFn("float", a)
	//return 0
}

// Float32 returns float32(a)
func Float32(a interface{}) float32 {
	switch a1 := a.(type) {
	case int:
		return float32(a1)
	case int64:
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
	case int64:
		return int(a1)
	case uint:
		return int(a1)
	case uint64:
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
	case int64:
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
	case int64:
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
	case int64:
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
	case int64:
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
	case int64:
		return uint(a1)
	case uint64:
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
	case float64:
		return uint8(a1)
	case int:
		return uint8(a1)
	case int64:
		return uint8(a1)
	case uint64:
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
	case float64:
		return byte(a1)
	case int:
		return byte(a1)
	case int64:
		return byte(a1)
	case uint64:
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
	case int64:
		return uint16(a1)
	case uint:
		return uint16(a1)
	case uint64:
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
	case int64:
		return uint32(a1)
	case uint:
		return uint32(a1)
	case uint64:
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
	case int64:
		return uint64(a1)
	case uint:
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
}

// Uintptr returns uintptr(a)
func Pointer(a interface{}) unsafe.Pointer {
	switch a1 := a.(type) {
	case unsafe.Pointer:
		return a1
	case uintptr:
		return unsafe.Pointer(a1)
	default:
		return unsafe.Pointer(reflect.ValueOf(a).Pointer())
	}
}

// String returns string(a)
func String(a interface{}) string {
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
	panicUnsupportedFn("string", a)
	return ""
}

// Bool returns bool(a)
func Bool(a interface{}) bool {
	switch a1 := a.(type) {
	case bool:
		return a1
	default:
		return isTrue(inDirect(reflect.ValueOf(a1)))
	}
}


//该函数暂时测试，可能会改动。
//	v reflect.Value		一个还没初始化变量，可能是接口类型
//	typ ...interface{}	要把v初始化成 typ 类型，如果留空则初始化成nil
func GoTypeTo(v reflect.Value) func(typ ...interface{}) {
	vv := reflect.Indirect(v)
	return func (a ...interface{}){
		if len(a) >= 1 {
			if a[0] == nil {
				return
			}
			av := reflect.ValueOf(a[0])
			avt := av.Type()
			if avt.ConvertibleTo(vv.Type()) {
				//*{} to *{}
				av = av.Convert(vv.Type())
				vv.Set(av)
			}else if avt.ConvertibleTo(vv.Type().Elem()) {
				//{} to {}
				av = av.Convert(vv.Type().Elem())
				if vv.IsNil() {
					vv.Set( reflect.New(vv.Type().Elem()) )
					vv.Elem().Set(av)
				}
			}
			return
		}
		
		for ;vv.Kind() == reflect.Ptr ;{
			if vv.IsNil() {
				//Chan，Func，Interface，Map，Ptr，或Slice
				vv.Set( reflect.New(vv.Type().Elem()) )
			}
			vv = vv.Elem() 
		}
		if !vv.IsValid() && vv.CanSet() {
			vv.Set(reflect.Zero(vv.Type()))
		}
	}
}

