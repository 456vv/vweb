package builtin

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

var (
	ErrNilValue        = errors.New("value is nil")
	ErrUnsupported     = errors.New("unsupported type")
	ErrKeyType         = errors.New("invalid key type")
	ErrIndexOutOfRange = errors.New("index out of range")
	ErrFieldNotFound   = errors.New("field not found")
	ErrMapKey          = errors.New("map key type mismatch")

	ErrInvalidArgCount = errors.New("invalid argument count: Set(obj, key1, val1, key2, val2, ...)")
	ErrUnaddressable   = errors.New("value is unaddressable or field is unexported; pass a pointer to exported field")
	ErrNilMap          = errors.New("cannot set on nil map")
	ErrImmutable       = errors.New("immutable type (string/number) cannot be modified by index")
)

var zeroVal reflect.Value

// ---------------------------------------------------------------------------
// unsafe 导出支持（运行时探测，兼容 Wasm / AppEngine / TinyGo 等）
// ---------------------------------------------------------------------------
// flagRO = flagStickyRO | flagEmbedRO = 1<<5 | 1<<6，自 Go 1.5 起稳定。
const flagRO uintptr = 96

type valueHeader struct {
	typ  unsafe.Pointer
	ptr  unsafe.Pointer
	flag uintptr
}

var useUnsafeExport bool

func init() {
	defer func() {
		if recover() != nil {
			useUnsafeExport = false
		}
	}()

	type dummy struct{ x int }
	v := reflect.ValueOf(dummy{x: 42}).FieldByName("x")
	if v.IsValid() && !v.CanInterface() {
		ev := makeExported(v)
		if ev.CanInterface() && ev.Interface() == 42 {
			useUnsafeExport = true
		}
	}
}

// makeExported 清除 reflect.Value 的只读标志，使未导出字段可被 Interface()。
func makeExported(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	vh := (*valueHeader)(unsafe.Pointer(&v))
	vh.flag &^= flagRO
	return v
}

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

// Get 通用取值。
//
// 支持：
//   - map[K]V          → key 可赋值/可转换/常见数字↔字符串互转
//   - slice / array    → 整数索引（支持负索引）
//   - string           → 整数索引（按字节，支持负索引）
//   - struct / *struct → 字段名（精确 / 首字母大小写 / json·mapstructure 标签）或整数索引（支持负索引）
//   - 数值类型         → 转为十进制字符串后按索引取字符
//   - 指针 / interface → 自动多层解包
//   - reflect.Value    → 直接接受
//
// 行为约定：
//   - map 中不存在的 key          → (nil, nil)
//   - 结构体找不到字段            → (nil, ErrFieldNotFound)
//   - 索引越界                    → (nil, ErrIndexOutOfRange)
//   - 类型不支持 / key 类型错误   → 对应错误
func Get(m any, key any) (any, error) {
	if m == nil {
		return nil, ErrNilValue
	}

	var v reflect.Value
	if rv, ok := m.(reflect.Value); ok {
		v = rv
	} else {
		v = reflect.ValueOf(m)
	}

	// 多层解包指针与 interface
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, ErrNilValue
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return nil, ErrNilValue
	}

	switch v.Kind() {
	case reflect.Map:
		return getMap(v, key)
	case reflect.Slice, reflect.Array:
		return getIndexable(v, key)
	case reflect.String:
		return getString(v, key)
	case reflect.Struct:
		return getStruct(v, key)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128:
		return getNumber(v, key)
	default:
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, v.Kind())
	}
}

// ---------------------------------------------------------------------------
// map
// ---------------------------------------------------------------------------
func getMap(v reflect.Value, key any) (any, error) {
	kt := v.Type().Key()
	k, ok := convertKey(key, kt)
	if !ok {
		return nil, fmt.Errorf("%w: need %v, got %T", ErrMapKey, kt, key)
	}
	res := v.MapIndex(k)
	if !res.IsValid() {
		return nil, nil // 与 map 语义一致：key 不存在返回 (nil, nil)
	}
	return valueToInterface(res), nil
}

// convertKey 将 key 安全转换为 map 所需的目标类型。
// 支持：可赋值、可转换、字符串↔整数、常见数字类型互转。
func convertKey(key any, target reflect.Type) (reflect.Value, bool) {
	if key == nil {
		return reflect.Value{}, false
	}
	kv := reflect.ValueOf(key)
	if kv.Type().AssignableTo(target) {
		return kv, true
	}
	if kv.Type().ConvertibleTo(target) {
		return kv.Convert(target), true
	}

	switch target.Kind() {
	case reflect.String:
		return reflect.ValueOf(fmt.Sprint(key)), true

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var i int64
		switch k := key.(type) {
		case string:
			var err error
			i, err = strconv.ParseInt(k, 10, 64)
			if err != nil {
				return reflect.Value{}, false
			}
		default:
			switch kv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				i = kv.Int()
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				i = int64(kv.Uint())
			case reflect.Float32, reflect.Float64:
				i = int64(kv.Float())
			default:
				return reflect.Value{}, false
			}
		}
		tmp := reflect.ValueOf(i)
		if tmp.Type().ConvertibleTo(target) {
			return tmp.Convert(target), true
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		var u uint64
		switch k := key.(type) {
		case string:
			var err error
			u, err = strconv.ParseUint(k, 10, 64)
			if err != nil {
				return reflect.Value{}, false
			}
		default:
			switch kv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				if kv.Int() < 0 {
					return reflect.Value{}, false
				}
				u = uint64(kv.Int())
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				u = kv.Uint()
			case reflect.Float32, reflect.Float64:
				if kv.Float() < 0 {
					return reflect.Value{}, false
				}
				u = uint64(kv.Float())
			default:
				return reflect.Value{}, false
			}
		}
		tmp := reflect.ValueOf(u)
		if tmp.Type().ConvertibleTo(target) {
			return tmp.Convert(target), true
		}
	}
	return reflect.Value{}, false
}

// ---------------------------------------------------------------------------
// slice / array
// ---------------------------------------------------------------------------

func getIndexable(v reflect.Value, key any) (any, error) {
	idx, ok := toInt(key)
	if !ok {
		return nil, fmt.Errorf("%w: slice/array key must be integer, got %T", ErrKeyType, key)
	}
	n := v.Len()
	if idx < 0 {
		idx += n
	}
	if idx < 0 || idx >= n {
		return nil, fmt.Errorf("%w: [%d] with length %d", ErrIndexOutOfRange, idx, n)
	}
	return valueToInterface(v.Index(idx)), nil
}

// ---------------------------------------------------------------------------
// string（按字节索引）
// ---------------------------------------------------------------------------

func getString(v reflect.Value, key any) (any, error) {
	idx, ok := toInt(key)
	if !ok {
		return nil, fmt.Errorf("%w: string key must be integer, got %T", ErrKeyType, key)
	}
	s := v.String()
	n := len(s)
	if idx < 0 {
		idx += n
	}
	if idx < 0 || idx >= n {
		if n == 0 {
			return byte(0), nil
		}
		return nil, fmt.Errorf("%w: [%d] with length %d", ErrIndexOutOfRange, idx, n)
	}
	return s[idx], nil
}

// ---------------------------------------------------------------------------
// 数值类型 → 十进制字符串后按索引取字符
// ---------------------------------------------------------------------------

func getNumber(v reflect.Value, key any) (any, error) {
	var s string
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		s = strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		s = strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		s = strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case reflect.Complex64, reflect.Complex128:
		s = fmt.Sprintf("%g", v.Complex())
	}
	idx, ok := toInt(key)
	if !ok {
		return nil, fmt.Errorf("%w: number key must be integer, got %T", ErrKeyType, key)
	}
	n := len(s)
	if idx < 0 {
		idx += n
	}
	if idx < 0 || idx >= n {
		return byte(0), nil // 与历史行为保持一致：越界返回 0
	}
	return s[idx], nil
}

// ---------------------------------------------------------------------------
// struct
// ---------------------------------------------------------------------------

func getStruct(v reflect.Value, key any) (any, error) {
	switch k := key.(type) {
	case string:
		return getStructByName(v, k)
	default:
		if idx, ok := toInt(key); ok {
			return getStructByIndex(v, idx)
		}
		return nil, fmt.Errorf("%w: struct key must be string or int, got %T", ErrKeyType, key)
	}
}

type fieldIndex map[string]int

// buildFieldIndex 构建并缓存某个 struct 类型的字段索引表。
func buildFieldIndex(t reflect.Type) fieldIndex {
	n := t.NumField()
	m := make(fieldIndex, n*2) // 预留别名空间
	for i := 0; i < n; i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" { // 跳过未导出
			continue
		}
		m[sf.Name] = i

		// 首字母小写别名（与 capitalize 对称）
		if r, size := utf8.DecodeRuneInString(sf.Name); unicode.IsUpper(r) {
			lower := string(unicode.ToLower(r)) + sf.Name[size:]
			if _, exists := m[lower]; !exists {
				m[lower] = i
			}
		}

		// json / mapstructure 标签
		for _, tagName := range []string{"json", "mapstructure"} {
			if tag := sf.Tag.Get(tagName); tag != "" {
				name := strings.Split(tag, ",")[0]
				if name != "" && name != "-" {
					if _, exists := m[name]; !exists {
						m[name] = i
					}
				}
			}
		}
	}
	return m
}

func getStructByName(v reflect.Value, name string) (any, error) {
	t := v.Type()

	idxMap := buildFieldIndex(t)

	if idx, exists := idxMap[name]; exists {
		return valueToInterface(v.Field(idx)), nil
	}
	// 缓存未命中时再尝试一次首字母大写（防止极端自定义类型名）
	if capName := capitalize(name); capName != name {
		if idx, exists := idxMap[capName]; exists {
			return valueToInterface(v.Field(idx)), nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrFieldNotFound, name)
}

func getStructByIndex(v reflect.Value, idx int) (any, error) {
	n := v.NumField()
	if idx < 0 {
		idx += n
	}
	if idx < 0 || idx >= n {
		return nil, fmt.Errorf("%w: field index [%d] with %d fields", ErrIndexOutOfRange, idx, n)
	}
	return valueToInterface(v.Field(idx)), nil
}

// ---------------------------------------------------------------------------
// 核心：reflect.Value → any
// 路径优先级：CanInterface → unsafe 清 flag → NewAt → 按 Kind 降级
// ---------------------------------------------------------------------------

func valueToInterface(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}

	// Fast path：导出字段
	if v.CanInterface() {
		return v.Interface()
	}

	// 未导出字段：优先使用已验证的 unsafe 方案
	if useUnsafeExport {
		ev := makeExported(v)
		if ev.CanInterface() {
			return ev.Interface()
		}
	}

	// 可寻址时用 NewAt（官方推荐安全写法）
	if v.CanAddr() {
		ptr := unsafe.Pointer(v.UnsafeAddr())
		exported := reflect.NewAt(v.Type(), ptr).Elem()
		if exported.CanInterface() {
			return exported.Interface()
		}
	}

	// 最终降级：按 Kind 手动提取
	return fallbackExport(v)
}

func fallbackExport(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint()
	case reflect.Float32, reflect.Float64:
		return v.Float()
	case reflect.Complex64, reflect.Complex128:
		return v.Complex()
	case reflect.String:
		return v.String()
	case reflect.UnsafePointer:
		return unsafe.Pointer(v.Pointer())

	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return valueToInterface(v.Elem())

	case reflect.Struct:
		t := v.Type()
		m := make(map[string]any, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			if sf.PkgPath != "" {
				continue
			}
			m[sf.Name] = valueToInterface(v.Field(i))
		}
		return m

	case reflect.Slice, reflect.Array:
		l := v.Len()
		out := make([]any, l)
		for i := 0; i < l; i++ {
			out[i] = valueToInterface(v.Index(i))
		}
		return out

	case reflect.Map:
		out := make(map[any]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out[valueToInterface(iter.Key())] = valueToInterface(iter.Value())
		}
		return out

	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// 工具函数
// ---------------------------------------------------------------------------

// toInt 将常见整数类型（含自定义整型）转为 int，并做溢出保护。
func toInt(key any) (int, bool) {
	if key == nil {
		return 0, false
	}
	switch k := key.(type) {
	case int:
		return k, true
	case int8:
		return int(k), true
	case int16:
		return int(k), true
	case int32:
		return int(k), true
	case int64:
		return int(k), true
	case uint:
		if k <= uint(^uint(0)>>1) {
			return int(k), true
		}
	case uint8:
		return int(k), true
	case uint16:
		return int(k), true
	case uint32:
		return int(k), true
	case uint64:
		if k <= uint64(^uint(0)>>1) {
			return int(k), true
		}
	case uintptr:
		if uint64(k) <= uint64(^uint(0)>>1) {
			return int(k), true
		}
	case string:
		if i, err := strconv.Atoi(k); err == nil {
			return i, true
		}
	}

	// 自定义整型
	rv := reflect.ValueOf(key)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := rv.Uint()
		if u <= uint64(^uint(0)>>1) {
			return int(u), true
		}
	}
	return 0, false
}

// capitalize 将首个字符转为大写（替代已废弃的 strings.Title）。
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(s)
	if unicode.IsUpper(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

// Set 通用赋值，与 Get 功能、错误、转换规则精确对称。
//
// 支持：
//   - map[K]V          → key 可赋值/可转换/常见数字↔字符串互转；值为 nil 时删除键
//   - slice / array    → 整数索引（支持负索引）；slice 可自动扩容
//   - struct / *struct → 字段名（精确 / 首字母大小写 / json·mapstructure 标签）或整数索引（支持负索引）
//   - 指针 / interface → 自动多层解包
//   - reflect.Value    → 直接接受（需最终可设置）
//
// 行为约定（与 Get 对应）：
//   - 结构体找不到字段            → ErrFieldNotFound
//   - 索引越界                    → ErrIndexOutOfRange
//   - 类型不支持 / key 类型错误   → ErrUnsupported / ErrKeyType / ErrMapKey
//   - 不可寻址或未导出字段        → ErrUnaddressable
//   - 字符串 / 数值按索引修改     → ErrImmutable
//   - map 为 nil                  → ErrNilMap
//
// 并发安全：函数本身无共享可变状态（字段缓存只读）。
// 对同一个 map 的并发写仍需调用方加锁。
func Set(m any, args ...any) error {
	n := len(args)
	if n == 0 || n&1 != 0 {
		return ErrInvalidArgCount
	}

	v, err := resolveSettable(m)
	if err != nil {
		return err
	}

	switch v.Kind() {
	case reflect.Map:
		return setMap(v, args)
	case reflect.Slice:
		return setIndexable(v, args, true)
	case reflect.Array:
		return setIndexable(v, args, false)
	case reflect.Struct:
		return setStruct(v, args)
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128:
		return ErrImmutable
	default:
		return fmt.Errorf("%w: %v", ErrUnsupported, v.Kind())
	}
}

// resolveSettable 多层解包指针/interface/reflect.Value，得到最终操作目标。
func resolveSettable(m any) (reflect.Value, error) {
	if m == nil {
		return reflect.Value{}, ErrNilValue
	}

	var v reflect.Value
	if rv, ok := m.(reflect.Value); ok {
		v = rv
	} else {
		v = reflect.ValueOf(m)
	}

	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}, ErrNilValue
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return reflect.Value{}, ErrNilValue
	}

	switch v.Kind() {
	case reflect.Map, reflect.Slice:
		// 引用类型，即使不可寻址也可操作底层数据
		return v, nil
	case reflect.Struct, reflect.Array:
		if !v.CanSet() && !v.CanAddr() {
			return reflect.Value{}, ErrUnaddressable
		}
		return v, nil
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128:
		return v, nil // 后续统一返回 ErrImmutable
	default:
		return reflect.Value{}, fmt.Errorf("%w: %v", ErrUnsupported, v.Kind())
	}
}

// ---------------------------------------------------------------------------
// map
// ---------------------------------------------------------------------------

func setMap(mv reflect.Value, args []any) error {
	if mv.IsNil() {
		return ErrNilMap
	}
	kt := mv.Type().Key()
	et := mv.Type().Elem()

	for i := 0; i < len(args); i += 2 {
		k, ok := convertKey(args[i], kt)
		if !ok {
			return fmt.Errorf("%w: need %v, got %T", ErrMapKey, kt, args[i])
		}

		var elem reflect.Value
		if args[i+1] == nil {
			// 与 reflect.SetMapIndex 一致：零 Value 删除键
			elem = reflect.Value{}
		} else {
			converted, err := autoConvert(et, args[i+1])
			if err != nil {
				return err
			}
			elem = converted
		}
		mv.SetMapIndex(k, elem)
	}
	return nil
}

// ---------------------------------------------------------------------------
// slice / array
// ---------------------------------------------------------------------------

func setIndexable(sv reflect.Value, args []any, growable bool) error {
	et := sv.Type().Elem()
	length := sv.Len()

	for i := 0; i < len(args); i += 2 {
		idx, ok := toInt(args[i])
		if !ok {
			return fmt.Errorf("%w: slice/array key must be integer, got %T", ErrKeyType, args[i])
		}

		if idx < 0 {
			idx += length
		}
		if idx < 0 {
			return fmt.Errorf("%w: [%d] with length %d", ErrIndexOutOfRange, idx, length)
		}

		// 自动扩容（仅 slice）
		if growable && idx >= length {
			newLen := idx + 1
			newCap := sv.Cap()
			if newLen > newCap {
				newCap = newLen * 2
				if newCap < 4 {
					newCap = 4
				}
			}
			ns := reflect.MakeSlice(sv.Type(), newLen, newCap)
			reflect.Copy(ns, sv)
			sv.Set(ns)
			length = newLen
		}

		if idx >= sv.Len() {
			return fmt.Errorf("%w: [%d] with length %d", ErrIndexOutOfRange, idx, sv.Len())
		}

		elem := sv.Index(idx)
		if !elem.CanSet() {
			return ErrUnaddressable
		}

		converted, err := autoConvert(et, args[i+1])
		if err != nil {
			return err
		}
		elem.Set(converted)
	}
	return nil
}

// ---------------------------------------------------------------------------
// struct
// ---------------------------------------------------------------------------

func setStruct(sv reflect.Value, args []any) error {
	t := sv.Type()
	idxMap := buildFieldIndex(t)
	n := t.NumField()

	for i := 0; i < len(args); i += 2 {
		key := args[i]
		val := args[i+1]

		var field reflect.Value

		switch k := key.(type) {
		case string:
			idx, exists := idxMap[k]
			if !exists {
				if capName := capitalize(k); capName != k {
					idx, exists = idxMap[capName]
				}
			}
			if !exists {
				return fmt.Errorf("%w: %q", ErrFieldNotFound, k)
			}
			field = sv.Field(idx)

		default:
			idx, ok := toInt(key)
			if !ok {
				return fmt.Errorf("%w: struct key must be string or int, got %T", ErrKeyType, key)
			}
			if idx < 0 {
				idx += n
			}
			if idx < 0 || idx >= n {
				return fmt.Errorf("%w: field index [%d] with %d fields", ErrIndexOutOfRange, idx, n)
			}
			// 与 Get 一致：按声明顺序取字段（未导出字段 Get 能读，Set 因 CanSet=false 会失败）
			field = sv.Field(idx)
		}

		if !field.IsValid() {
			return fmt.Errorf("%w: %v", ErrFieldNotFound, key)
		}
		if !field.CanSet() {
			return fmt.Errorf("%w: field %v", ErrUnaddressable, key)
		}

		converted, err := autoConvert(field.Type(), val)
		if err != nil {
			return err
		}
		field.Set(converted)
	}
	return nil
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

func Init(v any, isZero bool, args ...any) bool {
	return typeInit(toValue(v), isZero, args...)
}

func CopyStruct(dsc, src any, exclude func(name string, dsc, src reflect.Value) bool) error {
	return copyStruct(reflect.ValueOf(dsc), reflect.ValueOf(src), "", exclude, false)
}

func CopyStructDeep(dsc, src any, exclude func(name string, dsc, src reflect.Value) bool) error {
	return copyStruct(reflect.ValueOf(dsc), reflect.ValueOf(src), "", exclude, true)
}

func copyStruct(dsc, src reflect.Value, name string, exclude func(name string, dsc, src reflect.Value) bool, deep bool) error {
	va := inDirect(dsc)
	vb := inDirect(src)

	if !va.IsValid() || !vb.IsValid() {
		return fmt.Errorf("无效的值，dsc(%v)，src(%v)", va.Kind(), vb.Kind())
	}
	if va.Kind() != vb.Kind() || va.Kind() != reflect.Struct {
		return fmt.Errorf("仅支持struct类型，dsc(%s)，src(%s)", va.Kind(), vb.Kind())
	}

	bt := vb.Type()
	numField := bt.NumField()
	for i := range numField {
		bvField := vb.Field(i)
		if !bvField.IsValid() {
			continue
		}

		info := bt.Field(i)
		fieldName := name + info.Name
		avField := va.FieldByName(info.Name)

		if exclude != nil && exclude(fieldName, avField, bvField) {
			continue
		}
		if !avField.IsValid() {
			continue
		}

		// 解引用后的实际值
		avfi := inDirect(avField)
		bvfi := inDirect(bvField)

		// 目标为 nil 指针/接口且源有效时，尝试初始化
		if !avfi.IsValid() && bvfi.IsValid() {
			typeInit(avField, false)
			avfi = inDirect(avField)
		}

		afKind := avfi.Kind()
		bfKind := bvfi.Kind()

		// 深度复制嵌套结构体（值类型）
		if deep && afKind == reflect.Struct && bfKind == reflect.Struct {
			if err := copyStruct(avField, bvField, info.Name+".", exclude, true); err != nil {
				return err
			}
			continue
		}

		// Map 复制（支持键值类型转换，深度时对结构体值递归）
		if afKind == reflect.Map && bfKind == reflect.Map {
			if bvfi.IsNil() {
				continue
			}
			bfType := bvfi.Type()
			afType := avfi.Type()

			if !bfType.Key().ConvertibleTo(afType.Key()) || !bfType.Elem().ConvertibleTo(afType.Elem()) {
				continue
			}

			// 目标为 nil 时分配新 Map（仅可设置时）
			if avfi.IsNil() {
				if !avfi.CanSet() {
					continue
				}
				mt := reflect.MapOf(afType.Key(), afType.Elem())
				mv := reflect.MakeMapWithSize(mt, bvfi.Len())
				avfi.Set(mv)
			}

			bfmr := bvfi.MapRange()
			for bfmr.Next() {
				key := bfmr.Key().Convert(afType.Key())
				val := bfmr.Value()

				if deep && val.Kind() == reflect.Struct {
					newVal := reflect.New(afType.Elem()).Elem()
					if err := copyStruct(newVal, val, fieldName+".", exclude, true); err != nil {
						return err
					}
					avfi.SetMapIndex(key, newVal.Convert(afType.Elem()))
				} else {
					avfi.SetMapIndex(key, val.Convert(afType.Elem()))
				}
			}
			continue
		}

		// Slice 复制
		if afKind == reflect.Slice && bfKind == reflect.Slice {
			if bvfi.IsNil() {
				continue
			}
			// 类型必须完全一致（保持原逻辑）
			if avField.Type() != bvField.Type() && avfi.Type() != bvfi.Type() {
				continue
			}

			srcSlice := bvfi
			if !srcSlice.IsValid() {
				srcSlice = bvField
			}

			nv := reflect.MakeSlice(srcSlice.Type(), 0, srcSlice.Cap())
			if srcSlice.Len() > 0 {
				nv = reflect.AppendSlice(nv, srcSlice)
			}

			// 深度时对元素递归（仅元素为 Struct 时）
			if deep && srcSlice.Len() > 0 {
				elemKind := srcSlice.Type().Elem().Kind()
				if elemKind == reflect.Struct {
					for j := 0; j < nv.Len(); j++ {
						elem := nv.Index(j)
						if err := copyStruct(elem, srcSlice.Index(j), fieldName+".", exclude, true); err != nil {
							return err
						}
					}
				}
			}

			if avField.CanSet() {
				avField.Set(nv)
			} else if avfi.CanSet() {
				avfi.Set(nv)
			}
			continue
		}

		// Array（定长数组）复制
		if afKind == reflect.Array && bfKind == reflect.Array {
			if avfi.Type() != bvfi.Type() {
				continue
			}
			if !avfi.CanSet() {
				continue
			}
			reflect.Copy(avfi, bvfi)

			// 深度时对元素递归
			if deep {
				elemKind := avfi.Type().Elem().Kind()
				if elemKind == reflect.Struct {
					for j := 0; j < avfi.Len(); j++ {
						if err := copyStruct(avfi.Index(j), bvfi.Index(j), fieldName+".", exclude, true); err != nil {
							return err
						}
					}
				}
			}
			continue
		}

		// 普通字段赋值 / 类型转换
		if avField.CanSet() {
			if bvField.Type().AssignableTo(avField.Type()) {
				avField.Set(bvField)
			} else if bvField.Type().ConvertibleTo(avField.Type()) {
				avField.Set(bvField.Convert(avField.Type()))
			}
		} else if avfi.CanSet() {
			// 解引用后的值可设置时（例如指针目标）
			if bvfi.Type().AssignableTo(avfi.Type()) {
				avfi.Set(bvfi)
			} else if bvfi.Type().ConvertibleTo(avfi.Type()) {
				avfi.Set(bvfi.Convert(avfi.Type()))
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
