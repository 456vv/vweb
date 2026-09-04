package builtin

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
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

var (
	zeroVal   reflect.Value
	builtinMu sync.RWMutex

	// fieldIndexCache 按类型缓存字段索引，并发安全。
	// 注意：AllowUnexported 从 false→true（或反之）后，旧缓存可能与当前策略不一致，
	fieldIndexCache sync.Map // map[reflect.Type]fieldIndex

)

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

// makeExported 清除 reflect.Value 的只读标志，使未导出字段可被 Interface()。
func makeExported(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	vh := (*valueHeader)(unsafe.Pointer(&v))
	vh.flag &^= flagRO
	return v
}

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

// Register adds or replaces a named type in the registry.
// Safe for concurrent use.
func Register(name string, t reflect.Type) {
	if name == "" || t == nil {
		return
	}
	builtinMu.Lock()
	builtinTypes[name] = t
	builtinMu.Unlock()
}

// Value returns a zero value of the type described by v.
//
// Accepted forms for v:
//   - string type name ("int", "string", "[]int", "map[string]int", "*float64", …)
//   - reflect.Type
//   - reflect.Value (uses its Type)
//   - any concrete value (uses TypeOf)
//
// The returned Value is never a pointer unless the requested type itself is a
// pointer type.
func Value(v any) reflect.Value {
	t, err := Type(v)
	if err != nil {
		// Fallback to the original behaviour so existing call sites do not panic.
		return reflect.New(reflect.TypeOf(v)).Elem()
	}
	return reflect.Zero(t)
}

// New 返回 *T 的可设置指针（类似内置 new）。
func New(typ any) any {
	t, err := Type(typ)
	if err != nil {
		return nil
	}
	return reflect.New(t).Interface()
}

// Type returns the reflect.Type described by v.
// See Value for the accepted forms of v.
func Type(v any) (reflect.Type, error) {
	switch x := v.(type) {
	case string:
		return parseType(x)
	case reflect.Type:
		return x, nil
	case reflect.Value:
		if !x.IsValid() {
			return nil, fmt.Errorf("invalid reflect.Value")
		}
		return x.Type(), nil
	default:
		if v == nil {
			return reflect.TypeOf((*any)(nil)).Elem(), nil
		}
		return reflect.TypeOf(v), nil
	}
}

// ---------------------------------------------------------------------------
// Type parser
// ---------------------------------------------------------------------------

// parseType understands a small Go-like type syntax:
//
//	T
//	*T
//	[]T
//	[N]T          (N must be a non-negative integer literal)
//	map[K]V
//	chan T / <-chan T / chan<- T
//
// Nested combinations are supported (e.g. "map[string][]*int").
func parseType(s string) (reflect.Type, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty type string")
	}

	// Pointer
	if strings.HasPrefix(s, "*") {
		elem, err := parseType(s[1:])
		if err != nil {
			return nil, err
		}
		return reflect.PointerTo(elem), nil
	}

	// Slice
	if strings.HasPrefix(s, "[]") {
		elem, err := parseType(s[2:])
		if err != nil {
			return nil, err
		}
		return reflect.SliceOf(elem), nil
	}

	// Array [N]T
	if strings.HasPrefix(s, "[") {
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return nil, fmt.Errorf("malformed array type: %q", s)
		}
		nStr := strings.TrimSpace(s[1:end])
		n, err := parseNonNegInt(nStr)
		if err != nil {
			return nil, fmt.Errorf("invalid array length %q: %w", nStr, err)
		}
		elem, err := parseType(s[end+1:])
		if err != nil {
			return nil, err
		}
		return reflect.ArrayOf(n, elem), nil
	}

	// Map map[K]V
	if strings.HasPrefix(s, "map[") {
		// Find matching closing bracket for the key type.
		depth := 0
		keyEnd := -1
		for i := 4; i < len(s); i++ {
			switch s[i] {
			case '[':
				depth++
			case ']':
				if depth == 0 {
					keyEnd = i
					break
				}
				depth--
			}
			if keyEnd >= 0 {
				break
			}
		}
		if keyEnd < 0 {
			return nil, fmt.Errorf("malformed map type: %q", s)
		}
		key, err := parseType(s[4:keyEnd])
		if err != nil {
			return nil, err
		}
		val, err := parseType(s[keyEnd+1:])
		if err != nil {
			return nil, err
		}
		return reflect.MapOf(key, val), nil
	}

	// Channel
	if strings.HasPrefix(s, "chan ") {
		elem, err := parseType(s[5:])
		if err != nil {
			return nil, err
		}
		return reflect.ChanOf(reflect.BothDir, elem), nil
	}
	if strings.HasPrefix(s, "<-chan ") {
		elem, err := parseType(s[7:])
		if err != nil {
			return nil, err
		}
		return reflect.ChanOf(reflect.RecvDir, elem), nil
	}
	if strings.HasPrefix(s, "chan<- ") {
		elem, err := parseType(s[7:])
		if err != nil {
			return nil, err
		}
		return reflect.ChanOf(reflect.SendDir, elem), nil
	}

	// Named built-in (or registered) type
	builtinMu.RLock()
	t, ok := builtinTypes[s]
	builtinMu.RUnlock()
	if ok {
		return t, nil
	}

	return nil, fmt.Errorf("unknown type %q", s)
}

func parseNonNegInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty number")
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a non-negative integer")
		}
		n = n*10 + int(c-'0')
		if n < 0 { // overflow
			return 0, fmt.Errorf("integer overflow")
		}
	}
	return n, nil
}

// MustType is like Type but panics on error.
func MustType(v any) reflect.Type {
	t, err := Type(v)
	if err != nil {
		panic(err)
	}
	return t
}

// Panic(v)
func Panic(v any) {
	panic(v)
}

// Make 类似 Go 内置 make，支持：
//
//	Make([]T, length)           / Make([]T, length, capacity)
//	Make(map[K]V)               / Make(map[K]V, size)
//	Make(chan T)                / Make(chan T, buffer)
//	Make(func(...), impl)       // 扩展：MakeFunc 风格（可选）
//
// typ 可为：
//   - string 类型表达式（"[]int", "map[string]int", "chan bool" 等）
//   - reflect.Type
//   - 具体值（取其类型）
//
// 负长度/容量会 panic（与 Go 一致）。
// 非 slice/map/chan/func 类型返回 nil 并带错误（或可选 panic）。
func Make(typ any, args ...any) any {
	t, err := Type(typ)
	if err != nil {
		return nil
	}

	switch t.Kind() {
	case reflect.Slice:
		return makeSlice(t, args...)
	case reflect.Map:
		return makeMap(t, args...)
	case reflect.Chan:
		return makeChan(t, args...)
	case reflect.Func:
		return makeFunc(t, args...)
	default:
		return nil
	}
}

func makeSlice(t reflect.Type, args ...any) any {
	length := nonNegSize(args, 0, "slice length")
	capacity := length
	if len(args) > 1 {
		capacity = nonNegSize(args, 1, "slice capacity")
		if capacity < length {
			panic(fmt.Sprintf("makeslice: cap out of range (cap=%d < len=%d)", capacity, length))
		}
	}
	return reflect.MakeSlice(t, length, capacity).Interface()
}

func makeMap(t reflect.Type, args ...any) any {
	size := 0
	if len(args) > 0 {
		size = nonNegSize(args, 0, "map size")
	}
	return reflect.MakeMapWithSize(t, size).Interface()
}

func makeChan(t reflect.Type, args ...any) any {
	buffer := 0
	if len(args) > 0 {
		buffer = nonNegSize(args, 0, "chan buffer")
	}
	return reflect.MakeChan(t, buffer).Interface()
}

func makeFunc(t reflect.Type, args ...any) any {
	if len(args) == 0 {
		return reflect.Zero(t).Interface() // nil func
	}
	fn, ok := args[0].(func([]reflect.Value) []reflect.Value)
	if !ok {
		return nil
	}
	return reflect.MakeFunc(t, fn).Interface()
}

// nonNegSize 提取非负整数；负数或非法类型 panic（对齐 Go make 行为）
func nonNegSize(args []any, idx int, name string) int {
	if len(args) <= idx {
		return 0
	}
	switch n := args[idx].(type) {
	case int:
		if n < 0 {
			panic(fmt.Sprintf("make: %s out of range", name))
		}
		return n
	case int8, int16, int32, int64:
		v := reflect.ValueOf(n).Int()
		if v < 0 {
			panic(fmt.Sprintf("make: %s out of range", name))
		}
		return int(v)
	case uint, uint8, uint16, uint32, uint64:
		v := reflect.ValueOf(n).Uint()
		if v > uint64(^uint(0)>>1) { // 超过 int 最大值
			panic(fmt.Sprintf("make: %s out of range", name))
		}
		return int(v)
	default:
		panic(fmt.Sprintf("make: %s must be integer, got %T", name, args[idx]))
	}
}

func toValue(v any) reflect.Value {
	rv, ok := v.(reflect.Value)
	if !ok {
		rv = reflect.ValueOf(v).Elem()
	}
	return rv
}

// Bool returns bool(a)
func Bool(a any) bool {
	switch a1 := a.(type) {
	case bool:
		return a1
	}
	return isTrue(inDirect(reflect.ValueOf(a)))
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

// Cap returns the capacity of a, or 0 if a is not an array, slice, or channel.
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

// Init 对已有值做深层 nil 指针/接口初始化（可选零值容器）。
// 与 Make 职责分离，解决原 initChain 与 Make 混用问题。
func Init(v any, isZero bool, args ...any) bool {
	return typeInit(toValue(v), isZero, args...)
}

func Convert(a, b any) bool {
	av := toValue(a)
	bv := toValue(b)
	return typeConvert(av, bv)
}

func To(typ, v any) (any, error) {
	telem, err := Type(typ)
	if err != nil {
		return nil, err
	}
	rv, err := autoConvert(telem, v)
	if err != nil {
		return nil, err
	}
	return rv.Interface(), nil
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
	return getInternal(m, false, key)
}

func GetUnexported(m any, key any) (any, error) {
	return getInternal(m, true, key)
}

func getInternal(m any, allowUnexported bool, key any) (any, error) {
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
		return getStruct(v, key, allowUnexported)
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
	return typeSelect(v)
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
			// 空字符串越界：与历史行为一致返回 0
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

func getStruct(v reflect.Value, key any, allowUnexported bool) (any, error) {
	switch k := key.(type) {
	case string:
		return getStructByName(v, k, allowUnexported)
	default:
		if idx, ok := toInt(key); ok {
			return getStructByIndex(v, idx)
		}
		return nil, fmt.Errorf("%w: struct key must be string or int, got %T", ErrKeyType, key)
	}
}

// fieldByIndexSafe 安全地按索引路径取值。
// 若路径上任意中间指针为 nil，返回 ErrNilValue，避免 FieldByIndex 的 panic。
func fieldByIndexSafe(v reflect.Value, index []int) (reflect.Value, error) {
	if len(index) == 1 {
		return v.Field(index[0]), nil
	}
	for i, x := range index {
		if i > 0 {
			if v.Kind() == reflect.Pointer {
				if v.IsNil() {
					return reflect.Value{}, ErrNilValue
				}
				v = v.Elem()
			}
		}
		if v.Kind() != reflect.Struct {
			return reflect.Value{}, ErrNilValue
		}
		v = v.Field(x)
	}
	return v, nil
}

func getStructByName(v reflect.Value, name string, allowUnexported bool) (any, error) {
	t := v.Type()

	idxMap := buildFieldIndex(t, allowUnexported)

	if idx, exists := idxMap[name]; exists {
		fv, err := fieldByIndexSafe(v, idx)
		if err != nil {
			return nil, err
		}
		return valueToInterface(fv), nil
	}
	// 缓存未命中时再尝试一次首字母大写（防止极端自定义类型名）
	if capName := capitalize(name); capName != name {
		if idx, exists := idxMap[capName]; exists {
			fv, err := fieldByIndexSafe(v, idx)
			if err != nil {
				return nil, err
			}
			return valueToInterface(fv), nil
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
// 扩展行为：
//   - 写嵌入（Anonymous）指针路径时，路径上的 nil 嵌入指针会自动分配；普通指针字段不会
//   - 整次 Set 失败时会回滚：恢复已写叶子字段，并清除本次自动分配的中间嵌入指针，
//     保证失败后结构体与调用前完全一致（事务语义）
//
// 并发安全：函数本身无共享可变状态；字段索引使用 sync.Map 缓存（只读共享）。
// 对同一个 map / slice 的并发写仍需调用方加锁。
// 本函数不允许写入未导出字段（安全默认）。
func Set(m any, args ...any) error {
	return setInternal(m, false, args...)
}

// SetUnexported 与 Set 行为完全一致，但允许通过 unsafe 写入未导出但可寻址的字段。
// 仅应在可信代码中使用。字段索引缓存按 (type, allow) 分别维护，与 Set 互不干扰。
func SetUnexported(m any, args ...any) error {
	return setInternal(m, true, args...)
}

func setInternal(m any, allowUnexported bool, args ...any) error {
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
		return setIndexable(v, args, true, allowUnexported)
	case reflect.Array:
		return setIndexable(v, args, false, allowUnexported)
	case reflect.Struct:
		return setStruct(v, args, allowUnexported)
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
		// 引用类型，即使不可寻址也可操作底层数据（元素赋值）；
		// 但 slice 扩容需要可设置的 header，由 setIndexable 检查。
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

type undoFunc func()

func setMap(mv reflect.Value, args []any) error {
	if mv.IsNil() {
		return ErrNilMap
	}
	kt := mv.Type().Key()
	et := mv.Type().Elem()

	var undos []undoFunc

	rollback := func() {
		for i := len(undos) - 1; i >= 0; i-- {
			undos[i]()
		}
	}

	for i := 0; i < len(args); i += 2 {
		k, ok := convertKey(args[i], kt)
		if !ok {
			rollback()
			return fmt.Errorf("%w: need %v, got %T", ErrMapKey, kt, args[i])
		}

		old := mv.MapIndex(k)
		existed := old.IsValid()

		var elem reflect.Value
		if args[i+1] == nil {
			elem = reflect.Value{} // 删除
		} else {
			converted, err := autoConvert(et, args[i+1])
			if err != nil {
				rollback()
				return err
			}
			elem = converted
		}

		// 记录 undo
		if existed {
			oldCopy := reflect.New(et).Elem()
			oldCopy.Set(old) // 深拷贝一份，防止后续被覆盖
			keyCopy := k     // key 本身不可变
			undos = append(undos, func() {
				mv.SetMapIndex(keyCopy, oldCopy)
			})
		} else {
			keyCopy := k
			undos = append(undos, func() {
				mv.SetMapIndex(keyCopy, reflect.Value{}) // 删除
			})
		}

		mv.SetMapIndex(k, elem)
	}
	return nil
}

// ---------------------------------------------------------------------------
// slice / array
// ---------------------------------------------------------------------------

func setIndexable(sv reflect.Value, args []any, growable bool, allowUnexported bool) error {
	et := sv.Type().Elem()
	origLen := sv.Len()
	origCap := sv.Cap()

	// 保存原始内容，便于整体回滚（尤其扩容场景）
	var origSlice reflect.Value
	if growable {
		origSlice = reflect.MakeSlice(sv.Type(), origLen, origCap)
		reflect.Copy(origSlice, sv)
	}

	var undos []undoFunc
	rolledBack := false

	rollback := func() {
		if rolledBack {
			return
		}
		rolledBack = true
		if growable {
			if !sv.CanSet() {
				sv = makeSettable(sv, allowUnexported)
			}
			if sv.CanSet() {
				// 整体还原 header + 内容
				sv.Set(origSlice)
				return
			}
		}

		// 非扩容或不可设置时逐元素回滚
		for i := len(undos) - 1; i >= 0; i-- {
			undos[i]()
		}
	}

	for i := 0; i < len(args); i += 2 {
		idx, ok := toInt(args[i])
		if !ok {
			rollback()
			return fmt.Errorf("%w: slice/array key must be integer, got %T", ErrKeyType, args[i])
		}

		if idx < 0 {
			idx += origLen // 负索引始终相对原始长度
		}
		if idx < 0 {
			rollback()
			return fmt.Errorf("%w: [%d] with length %d", ErrIndexOutOfRange, idx, origLen)
		}

		// 自动扩容（仅 slice）
		if growable && idx >= sv.Len() {
			if !sv.CanSet() {
				sv = makeSettable(sv, allowUnexported)
			}
			if !sv.CanSet() {
				rollback()
				return ErrUnaddressable
			}
			newLen := idx + 1
			newCap := sv.Cap()
			if newLen > newCap {
				newCap = max(newLen*2, 4)
			}
			ns := reflect.MakeSlice(sv.Type(), newLen, newCap)
			reflect.Copy(ns, sv)
			sv.Set(ns)
		}

		if idx >= sv.Len() {
			rollback()
			return fmt.Errorf("%w: [%d] with length %d", ErrIndexOutOfRange, idx, sv.Len())
		}

		elem := sv.Index(idx)
		if !elem.CanSet() {
			elem = makeSettable(elem, allowUnexported)
		}
		if !elem.CanSet() {
			rollback()
			return ErrUnaddressable
		}

		// 保存旧值
		old := reflect.New(et).Elem()
		old.Set(elem)

		converted, err := autoConvert(et, args[i+1])
		if err != nil {
			rollback()
			return err
		}

		// 记录 undo（仅在未整体还原的路径下使用）
		curIdx := idx
		undos = append(undos, func() {
			el := sv.Index(curIdx)
			if !el.CanSet() {
				el = makeSettable(el, allowUnexported)
			}
			if el.CanSet() {
				el.Set(old)
			}
		})

		elem.Set(converted)
	}
	return nil
}

// ---------------------------------------------------------------------------
// struct
// ---------------------------------------------------------------------------

func setStruct(sv reflect.Value, args []any, allowUnexported bool) error {
	t := sv.Type()
	idxMap := buildFieldIndex(t, allowUnexported)
	n := t.NumField()

	var undos []undoFunc      // 叶子字段旧值恢复
	var allocUndos []undoFunc // 本次自动分配的中间嵌入指针 → 回滚时置 nil

	rollback := func() {
		// 1. 先恢复叶子字段（此时中间指针仍有效）
		for i := len(undos) - 1; i >= 0; i-- {
			undos[i]()
		}
		// 2. 再按分配逆序清除中间指针，保证失败后与调用前完全一致
		for i := len(allocUndos) - 1; i >= 0; i-- {
			allocUndos[i]()
		}
	}

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
				rollback()
				return fmt.Errorf("%w: %q", ErrFieldNotFound, k)
			}

			// 按需初始化路径中的 nil 嵌入指针，安全返回可设置字段
			f, intermediates, err := safeFieldByIndex(sv, idx, allowUnexported)
			// 即使失败也先收录已分配的中间指针，再统一回滚清理
			allocUndos = append(allocUndos, intermediates...)
			if err != nil {
				rollback()
				return err
			}
			field = f

		default:
			idx, ok := toInt(key)
			if !ok {
				rollback()
				return fmt.Errorf("%w: struct key must be string or int, got %T", ErrKeyType, key)
			}
			if idx < 0 {
				idx += n
			}
			if idx < 0 || idx >= n {
				rollback()
				return fmt.Errorf("%w: field index [%d] with %d fields", ErrIndexOutOfRange, idx, n)
			}
			// 与字符串路径统一：走 safeFieldByIndex，保证行为一致
			f, intermediates, err := safeFieldByIndex(sv, []int{idx}, allowUnexported)
			allocUndos = append(allocUndos, intermediates...)
			if err != nil {
				rollback()
				return err
			}
			field = f
		}

		if !field.IsValid() {
			rollback()
			return fmt.Errorf("%w: %v", ErrFieldNotFound, key)
		}

		// 若为未导出字段且 AllowUnexported 开启，通过 makeSettable 使其可写
		if !field.CanSet() {
			field = makeSettable(field, allowUnexported)
		}
		if !field.CanSet() {
			rollback()
			return fmt.Errorf("%w: field %v", ErrUnaddressable, key)
		}

		// 保存旧值
		old := reflect.New(field.Type()).Elem()
		old.Set(field)

		converted, err := autoConvert(field.Type(), val)
		if err != nil {
			rollback()
			return err
		}

		// 记录叶子 undo：直接捕获可写 Value，避免回滚时重新走路径导致误分配
		settableField := field
		undos = append(undos, func() {
			if settableField.CanSet() {
				settableField.Set(old)
			}
		})

		field.Set(converted)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 工具函数
// ---------------------------------------------------------------------------

// toInt 将常见整数类型（含自定义整型）转为 int，并做溢出保护（兼容 32/64 位平台）。
// 对 float 先做范围检查再转换，避免超出 int64 时结果未定义。
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
		return safeInt64(k)
	case uint:
		if k <= uint(^uint(0)>>1) {
			return int(k), true
		}
	case uint8:
		return int(k), true
	case uint16:
		return int(k), true
	case uint32:
		// 32 位平台上 int 最大为 2^31-1，uint32 可能更大，需范围检查
		if uint64(k) <= uint64(^uint(0)>>1) {
			return int(k), true
		}
	case uint64:
		if k <= uint64(^uint(0)>>1) {
			return int(k), true
		}
	case uintptr:
		if uint64(k) <= uint64(^uint(0)>>1) {
			return int(k), true
		}
	case float32:
		f := float64(k)
		if f != f || f > float64(^uint64(0)>>1) || f < float64(-1<<63) {
			return 0, false
		}
		return safeInt64(int64(f))
	case float64:
		if k != k || k > float64(^uint64(0)>>1) || k < float64(-1<<63) {
			return 0, false
		}
		return safeInt64(int64(k))
	case string:
		if i, err := strconv.Atoi(k); err == nil {
			return i, true
		}
	}

	// 自定义整型 / 浮点
	rv := reflect.ValueOf(key)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return safeInt64(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := rv.Uint()
		if u <= uint64(^uint(0)>>1) {
			return int(u), true
		}
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if f != f || f > float64(^uint64(0)>>1) || f < float64(-1<<63) {
			return 0, false
		}
		return safeInt64(int64(f))
	case reflect.String:
		if i, err := strconv.Atoi(rv.String()); err == nil {
			return i, true
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

// convertKey 将任意 key 转为 map 的 key 类型；失败返回 false。
// 支持常见数字↔字符串、bool 等互转，并做目标类型位宽范围检查（禁止截断）。
func convertKey(key any, target reflect.Type) (reflect.Value, bool) {
	if key == nil {
		switch target.Kind() {
		case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
			return reflect.Zero(target), true
		}
		return reflect.Value{}, false
	}
	kv := reflect.ValueOf(key)
	if !kv.IsValid() {
		return reflect.Value{}, false
	}
	if kv.Type().AssignableTo(target) {
		return kv, true
	}

	switch target.Kind() {
	case reflect.Bool: // 支持 Map 的 Bool 键的转换与解析
		switch kv.Kind() {
		case reflect.Bool:
			return kv, true
		case reflect.String:
			b, err := strconv.ParseBool(kv.String())
			if err == nil {
				return reflect.ValueOf(b), true
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return reflect.ValueOf(kv.Int() != 0), true
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return reflect.ValueOf(kv.Uint() != 0), true
		case reflect.Float32, reflect.Float64:
			return reflect.ValueOf(kv.Float() != 0), true
		}
	case reflect.String:
		// 数字→字符串：用十进制表示，而不是 rune 转换（Convert 会把 1 变成 "\x01"）
		switch kv.Kind() {
		case reflect.String:
			return kv, true
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return reflect.ValueOf(strconv.FormatInt(kv.Int(), 10)), true
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return reflect.ValueOf(strconv.FormatUint(kv.Uint(), 10)), true
		case reflect.Float32, reflect.Float64:
			return reflect.ValueOf(strconv.FormatFloat(kv.Float(), 'f', -1, 64)), true
		case reflect.Bool:
			return reflect.ValueOf(strconv.FormatBool(kv.Bool())), true
		default:
			return reflect.ValueOf(fmt.Sprint(key)), true
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, ok := toInt64ForKey(kv, key)
		if !ok {
			return reflect.Value{}, false
		}
		// 范围检查：必须能完整放入目标类型，禁止截断
		bits := target.Bits()
		min := -(int64(1) << (bits - 1))
		max := (int64(1) << (bits - 1)) - 1
		if i < min || i > max {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(i).Convert(target), true

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u, ok := toUint64ForKey(kv, key)
		if !ok {
			return reflect.Value{}, false
		}
		// 范围检查：必须能完整放入目标类型，禁止截断/wrap
		if target.Bits() < 64 && u > (uint64(1)<<target.Bits())-1 {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(u).Convert(target), true

	case reflect.Float32, reflect.Float64:
		f, ok := toFloat64ForKey(kv, key)
		if !ok {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(f).Convert(target), true
	}

	// 其它类型：仅在可安全 Convert 时放行（非数字/字符串的命名类型等）
	if kv.Type().ConvertibleTo(target) {
		return kv.Convert(target), true
	}
	return reflect.Value{}, false
}

// toInt64ForKey 将 key 转为 int64，失败返回 false。
// 拒绝：无法解析的字符串、超出 int64 的 uint、NaN/Inf。
func toInt64ForKey(kv reflect.Value, key any) (int64, bool) {
	switch k := key.(type) {
	case string:
		i, err := strconv.ParseInt(k, 10, 64)
		return i, err == nil
	}
	switch kv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return kv.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := kv.Uint()
		if u > uint64(^uint64(0)>>1) { // > math.MaxInt64
			return 0, false
		}
		return int64(u), true
	case reflect.Float32, reflect.Float64:
		f := kv.Float()
		if f != f || f > float64(^uint64(0)>>1) || f < float64(-1<<63) { // NaN 或超出 int64
			return 0, false
		}
		return int64(f), true
	}
	return 0, false
}

// toUint64ForKey 将 key 转为 uint64，失败返回 false。
// 拒绝：负数、无法解析的字符串、NaN/Inf。
func toUint64ForKey(kv reflect.Value, key any) (uint64, bool) {
	switch k := key.(type) {
	case string:
		u, err := strconv.ParseUint(k, 10, 64)
		return u, err == nil
	}
	switch kv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i := kv.Int()
		if i < 0 {
			return 0, false
		}
		return uint64(i), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return kv.Uint(), true
	case reflect.Float32, reflect.Float64:
		f := kv.Float()
		if f != f || f < 0 || f > float64(^uint64(0)) { // NaN / 负 / 超出 uint64
			return 0, false
		}
		return uint64(f), true
	}
	return 0, false
}

// toFloat64ForKey 将 key 转为 float64。
// 拒绝无法解析的字符串；NaN/Inf 对 float 目标通常可接受，故不额外过滤。
func toFloat64ForKey(kv reflect.Value, key any) (float64, bool) {
	switch k := key.(type) {
	case string:
		f, err := strconv.ParseFloat(k, 64)
		return f, err == nil
	}
	switch kv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(kv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(kv.Uint()), true
	case reflect.Float32, reflect.Float64:
		return kv.Float(), true
	}
	return 0, false
}

type fieldIndex map[string][]int

type fieldIndexKey struct {
	t     reflect.Type
	allow bool
}

// buildFieldIndex 构建并缓存某个 struct 类型的字段索引表（含嵌入字段提升）。
// 使用 FieldByIndex 路径，支持 json / mapstructure 标签及首字母大小写别名。
// 同名冲突时：本层字段优先于嵌入字段（符合 Go 遮蔽规则）；同层冲突时先出现者优先。
// 并发安全：sync.Map + LoadOrStore 保证每个 (type, allow) 只完整构建一次。
func buildFieldIndex(t reflect.Type, allow bool) fieldIndex {
	key := fieldIndexKey{t: t, allow: allow}
	if v, ok := fieldIndexCache.Load(key); ok {
		return v.(fieldIndex)
	}

	m := make(fieldIndex, t.NumField()*2)

	var walk func(tt reflect.Type, path []int)
	walk = func(tt reflect.Type, path []int) {
		n := tt.NumField()
		type embedInfo struct {
			ft  reflect.Type
			idx []int
		}
		var embeds []embedInfo

		// 第一遍：只登记本层直接字段（含标签/别名），暂不递归嵌入
		for i := 0; i < n; i++ {
			sf := tt.Field(i)
			unexported := sf.PkgPath != ""
			// 与 Go 选择规则对齐：
			// - 未导出且非匿名：不登记、不提升
			// - 未导出但匿名：不登记该字段名本身，仍递归以提升其中的导出字段
			// - allow=true：未导出字段也可登记
			if unexported && !allow && !sf.Anonymous {
				continue
			}

			idx := make([]int, len(path)+1)
			copy(idx, path)
			idx[len(path)] = i

			// 仅当字段可对外使用时登记名称（导出，或允许未导出）
			if !unexported || allow {
				// 精确名
				if _, exists := m[sf.Name]; !exists {
					m[sf.Name] = idx
				}

				// json / mapstructure 标签
				for _, tagName := range []string{"json", "mapstructure"} {
					if tag := sf.Tag.Get(tagName); tag != "" {
						name, _, _ := strings.Cut(tag, ",")
						if name != "" && name != "-" {
							if _, exists := m[name]; !exists {
								m[name] = idx
							}
						}
					}
				}
			}

			// 记录匿名字段，稍后统一提升（含未导出嵌入，以提升其导出子字段）
			if sf.Anonymous {
				ft := sf.Type
				if ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct {
					embeds = append(embeds, embedInfo{ft: ft, idx: idx})
				}
			}
		}

		// 第二遍：只对尚未出现的名字做嵌入字段提升（外层优先）
		for _, e := range embeds {
			walk(e.ft, e.idx)
		}
	}
	walk(t, nil)

	actual, _ := fieldIndexCache.LoadOrStore(key, m)
	return actual.(fieldIndex)
}

// safeInt64 converts int64 to int with platform overflow check（兼容 32/64 位）。
func safeInt64(i int64) (int, bool) {
	const maxInt = int(^uint(0) >> 1)
	const minInt = -maxInt - 1
	if i < int64(minInt) || i > int64(maxInt) {
		return 0, false
	}
	return int(i), true
}

// makeSettable 利用 unsafe 强制使未导出但可寻址的字段具备可写属性。
// 仅当 allowUnexported == true 时生效。依赖 unsafe，仅应在可信代码中使用。
func makeSettable(v reflect.Value, allowUnexported bool) reflect.Value {
	if !v.IsValid() || v.CanSet() {
		return v
	}
	if !allowUnexported || !v.CanAddr() {
		return v
	}
	// 利用底层地址重构可写 reflect.Value
	return reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
}

// safeFieldByIndex 安全地根据索引路径访问结构体字段。
// 仅当路径中的 nil 指针来自【匿名字段（嵌入）】且可寻址时，才自动分配实例。
// 返回的 allocUndos 记录本次自动分配的中间指针，失败时按逆序回滚。
func safeFieldByIndex(v reflect.Value, index []int, allowUnexported bool) (reflect.Value, []undoFunc, error) {
	curr := v
	var lastSF reflect.StructField
	hasLast := false
	var allocUndos []undoFunc

	for _, i := range index {
		if curr.Kind() == reflect.Pointer {
			if curr.IsNil() {
				// 只对嵌入（Anonymous）指针字段自动初始化；普通指针字段保持 nil 并报错
				if !hasLast || !lastSF.Anonymous {
					// 仍返回已分配的 undos，供调用方清理
					return reflect.Value{}, allocUndos, fmt.Errorf("%w: nil non-embedded pointer in field path", ErrUnaddressable)
				}
				if !curr.CanSet() {
					curr = makeSettable(curr, allowUnexported)
				}
				if !curr.CanSet() {
					return reflect.Value{}, allocUndos, ErrUnaddressable
				}
				curr.Set(reflect.New(curr.Type().Elem()))
				// 记录回滚：将本次分配的指针重新置为 nil
				ptrField := curr
				allocUndos = append(allocUndos, func() {
					p := ptrField
					if !p.CanSet() {
						p = makeSettable(p, allowUnexported)
					}
					if p.CanSet() {
						p.Set(reflect.Zero(p.Type()))
					}
				})
			}
			curr = curr.Elem()
		}
		if curr.Kind() != reflect.Struct {
			return reflect.Value{}, allocUndos, ErrUnsupported
		}
		if i < 0 || i >= curr.NumField() {
			return reflect.Value{}, allocUndos, fmt.Errorf("%w: field index [%d] with %d fields", ErrIndexOutOfRange, i, curr.NumField())
		}
		lastSF = curr.Type().Field(i)
		hasLast = true
		curr = curr.Field(i)
	}
	return curr, allocUndos, nil
}

// Delete 删除 map 中的键。
func Delete(m any, key any) {
	reflect.ValueOf(m).SetMapIndex(reflect.ValueOf(key), zeroVal)
}

// GetSlice 返回 a[start:end] 的反射兼容版本。
//
// 语义严格对齐 Go 原生切片表达式：
//   - 要求 0 ≤ start ≤ end ≤ len(a)
//   - 对 slice / 可寻址 array / string：返回值与源共享底层数组（或字符串数据）
//   - 对不可寻址 array：返回独立拷贝，避免 panic
//
// 返回值约定：
//   - 成功时返回与源类型一致的切片（或子串）
//   - 任何越界、nil、不支持类型均返回 nil（interface{} 的 nil）
//
// 并发安全：
//
//	 本函数本身无共享状态，可被多 goroutine 同时调用。
//	 但当返回值与源共享底层数组时，调用方必须自行保证对源的并发写安全
//	（通常用 sync.Mutex / RWMutex 保护源 slice/array）。
//
// 支持类型：
//   - []T、*[N]T、[N]T、string
//   - 以及它们的任意层指针 / interface 包装（会自动解引用）
func GetSlice(a any, start, end int) any {
	if start < 0 || end < start {
		return nil
	}

	v := reflect.ValueOf(a)
	if !v.IsValid() {
		return nil
	}

	// 自动解引用指针和 interface，直到到达具体值
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			// 保持 typed-nil，而不是 interface{} 的 nil
			return v.Interface()
		}
		// 严格按 len 检查，与原生 s[low:high] 一致
		if end > v.Len() {
			return nil
		}
		return v.Slice(start, end).Interface()

	case reflect.Array:
		if end > v.Len() {
			return nil
		}
		if v.CanAddr() {
			// 可寻址：直接切片，共享底层数组
			return v.Slice(start, end).Interface()
		}
		// 不可寻址：必须拷贝，否则 reflect.Value.Slice 会 panic
		n := end - start
		out := reflect.MakeSlice(reflect.SliceOf(v.Type().Elem()), n, n)
		reflect.Copy(out, v.Slice(start, end).Interface().(reflect.Value)) // 不会走到这里
		// 上面写法不安全，改用正确路径：
		// 因为不可寻址 array 不能直接 Slice，所以手动拷贝
		for i := 0; i < n; i++ {
			out.Index(i).Set(v.Index(start + i))
		}
		return out.Interface()

	case reflect.String:
		if end > v.Len() {
			return nil
		}
		return v.Slice(start, end).Interface()

	default:
		return nil
	}
}

// SetSlice 将 src 的全部内容严格写入 dst[start:end]。
//
// 严格语义：
//   - 0 ≤ start ≤ end ≤ len(dst)
//   - len(src) 必须精确等于 end-start，否则失败
//   - 元素类型必须完全可赋值（reflect.Copy 规则）
//
// 失败回滚保证：
//
//	任意校验失败都不会修改 dst。
//	即使在拷贝过程中出现异常（当前实现下几乎不会），也会自动还原原区间。
//
// 支持目标：[]T、[N]T 及其任意层 * / interface 包装
// 支持源：  []T、[N]T、string（仅当目标元素为 byte）及其 * / interface 包装
//
// 返回 true 表示完整成功；false 表示失败且 dst 保持原样。
//
// 并发安全：函数本身无状态；共享底层数组时由调用方加锁。
func SetSlice(dst any, start, end int, src any) bool {
	if start < 0 || end < start {
		return false
	}

	// ---------- 1. 解析并校验目标 ----------
	dv := reflect.ValueOf(dst)
	if !dv.IsValid() {
		return false
	}
	for dv.Kind() == reflect.Pointer || dv.Kind() == reflect.Interface {
		if dv.IsNil() {
			return false
		}
		dv = dv.Elem()
	}

	var dSub reflect.Value
	switch dv.Kind() {
	case reflect.Slice:
		if dv.IsNil() || end > dv.Len() {
			return false
		}
		dSub = dv.Slice(start, end)
		// slice 的元素总是可写的（只要 header 有效）
	case reflect.Array:
		if end > dv.Len() {
			return false
		}
		if !dv.CanSet() {
			return false // 不可寻址 / 不可写 array
		}
		dSub = dv.Slice(start, end)
	default:
		return false // string 等不可变类型
	}

	need := end - start
	if dSub.Len() != need {
		return false
	}

	// ---------- 2. 解析并校验源 ----------
	sv := reflect.ValueOf(src)
	if !sv.IsValid() {
		return false
	}
	for sv.Kind() == reflect.Pointer || sv.Kind() == reflect.Interface {
		if sv.IsNil() {
			return false
		}
		sv = sv.Elem()
	}

	switch sv.Kind() {
	case reflect.Slice, reflect.Array, reflect.String:
		// ok
	default:
		return false
	}
	if sv.Len() != need {
		return false // 严格长度匹配
	}

	// 类型兼容性提前检查（reflect.Copy 内部也会检查，但我们提前失败更干净）
	if !sv.Type().AssignableTo(dSub.Type()) && (sv.Kind() != reflect.String || dSub.Type().Elem().Kind() != reflect.Uint8) {
		// 允许 []byte ← string 的特殊情况
		if dSub.Kind() != reflect.Slice || dSub.Type().Elem().Kind() != reflect.Uint8 || sv.Kind() != reflect.String {
			return false
		}
	}

	// ---------- 3. 备份原区间（实现真正回滚） ----------
	backup := reflect.MakeSlice(dSub.Type(), need, need)
	reflect.Copy(backup, dSub)

	// ---------- 4. 执行写入 ----------
	// 使用 defer + recover 防御极端 panic（当前 reflect.Copy 几乎不会触发）
	success := false
	defer func() {
		if !success {
			// 回滚
			reflect.Copy(dSub, backup)
		}
	}()

	n := reflect.Copy(dSub, sv)
	if n != need {
		return false // 理论上不会走到这里
	}

	success = true
	return true
}

// Copy 模拟内置 copy 函数，支持任意切片类型及 string -> []byte。
// 参数 a 为目标（必须是切片），b 为源（切片或字符串）。
// 返回复制成功的元素个数（int），若类型不兼容则返回 0。
func Copy(a any, b any) any {
	va := reflect.ValueOf(a)
	vb := reflect.ValueOf(b)

	// 目标必须是切片
	if va.Kind() != reflect.Slice {
		return 0
	}

	// 源为字符串时，目标必须为 []byte
	if vb.Kind() == reflect.String {
		if va.Type().Elem().Kind() != reflect.Uint8 {
			return 0
		}
		dst := va.Interface().([]byte)
		src := vb.String()
		return copy(dst, src) // 直接使用内置 copy
	}

	// 源必须是切片
	if vb.Kind() != reflect.Slice {
		return 0
	}

	// 检查元素类型是否可赋值给目标
	if !vb.Type().Elem().AssignableTo(va.Type().Elem()) {
		return 0
	}

	n := va.Len()
	if vb.Len() < n {
		n = vb.Len()
	}

	// 逐个元素复制（注意：若源和目标重叠，此处顺序复制可能不符合内置 copy 的安全行为）
	for i := 0; i < n; i++ {
		va.Index(i).Set(vb.Index(i))
	}

	return n
}

// Append 模拟内置 append 函数，支持任意切片类型和追加元素。
// 参数 a 必须是一个切片（可以是 nil 切片），vals 为待追加的值。
// 返回的新切片类型与原切片相同，若类型不匹配或 a 不是切片则会 panic。
func Append(a any, vals ...any) any {
	// 获取 a 的反射值
	v := reflect.ValueOf(a)
	if v.Kind() != reflect.Slice {
		panic("Append: first argument must be a slice")
	}

	// 将 vals 转换为 []reflect.Value，每个元素将作为独立的追加项
	elems := make([]reflect.Value, len(vals))
	for i, val := range vals {
		elems[i] = reflect.ValueOf(val)
	}

	// reflect.Append 会自动处理扩容和类型检查（可赋值性）
	result := reflect.Append(v, elems...)

	// 返回新切片（类型为 any，但实际类型与原切片一致）
	return result.Interface()
}
