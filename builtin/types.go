package builtin

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
)

// maxIndirect 限制指针/接口解引用层数，配合环检测避免自引用死循环。
const maxIndirect = 64

// maxConvertDepth 限制容器递归转换深度，避免恶意嵌套栈溢出。
const maxConvertDepth = 256

var builtinTypes = map[string]reflect.Type{
	"uintptr":     reflect.TypeFor[uintptr](),
	"int":         reflect.TypeFor[int](),
	"int8":        reflect.TypeFor[int8](),
	"int16":       reflect.TypeFor[int16](),
	"int32":       reflect.TypeFor[int32](),
	"int64":       reflect.TypeFor[int64](),
	"uint":        reflect.TypeFor[uint](),
	"uint8":       reflect.TypeFor[uint8](),
	"uint16":      reflect.TypeFor[uint16](),
	"uint32":      reflect.TypeFor[uint32](),
	"uint64":      reflect.TypeFor[uint64](),
	"bool":        reflect.TypeFor[bool](),
	"float32":     reflect.TypeFor[float32](),
	"float64":     reflect.TypeFor[float64](),
	"complex64":   reflect.TypeFor[complex64](),
	"complex128":  reflect.TypeFor[complex128](),
	"string":      reflect.TypeFor[string](),
	"byte":        reflect.TypeFor[byte](),
	"rune":        reflect.TypeFor[rune](),
	"any":         reflect.TypeFor[any](),
	"interface":   reflect.TypeFor[any](),
	"interface{}": reflect.TypeFor[any](),
	"error":       reflect.TypeFor[error](),
	"struct":      reflect.TypeFor[struct{}](),
	"struct{}":    reflect.TypeFor[struct{}](),
	"value":       reflect.TypeFor[reflect.Value](),
	"type":        reflect.TypeFor[reflect.Type](),
}

func inDirect(v reflect.Value) reflect.Value {
	// 快路径：0~2 层解引用不分配 map
	for i := 0; i < 2; i++ {
		if !v.IsValid() || (v.Kind() != reflect.Pointer && v.Kind() != reflect.Interface) || v.IsNil() {
			return v
		}
		v = v.Elem()
	}
	// 慢路径：>=3 层时启用环检测
	seen := make(map[uintptr]struct{}, 4)
	for {
		if !v.IsValid() || (v.Kind() != reflect.Pointer && v.Kind() != reflect.Interface) || v.IsNil() {
			return v
		}
		if v.Kind() == reflect.Pointer {
			p := v.Pointer()
			if _, ok := seen[p]; ok {
				return v
			}
			seen[p] = struct{}{}
		}
		v = v.Elem()
	}
}

// asValue 把 any / reflect.Value 统一成 reflect.Value，避免二次包装。
// Len/Cap/Bool/Compute/CopyStruct 等对已是 reflect.Value 的入参会 ValueOf 成 struct。
func asValue(v any) reflect.Value {
	if v == nil {
		return reflect.Value{}
	}
	if rv, ok := v.(reflect.Value); ok {
		return rv
	}
	return reflect.ValueOf(v)
}

func isSignedKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	}
	return false
}

func isUnsignedKind(k reflect.Kind) bool {
	switch k {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	}
	return false
}

func isFloatKind(k reflect.Kind) bool {
	return k == reflect.Float32 || k == reflect.Float64
}

func isComplexKind(k reflect.Kind) bool {
	return k == reflect.Complex64 || k == reflect.Complex128
}

func isNumericKind(k reflect.Kind) bool {
	return isSignedKind(k) || isUnsignedKind(k) || isFloatKind(k) || isComplexKind(k)
}

func isIntKind(k reflect.Kind) bool {
	return isSignedKind(k) || isUnsignedKind(k)
}

// safeInterface 在未导出类型上避免 Interface() panic。
func safeInterface(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	if v.CanInterface() {
		return v.Interface()
	}
	return valueToInterface(v)
}

// autoConvert 将任意值转换为目标类型。
func autoConvert(telem reflect.Type, v any) (reflect.Value, error) {
	if v == nil {
		switch telem.Kind() {
		case reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr:
			return reflect.Zero(telem), nil
		}
		return reflect.Value{}, errors.New("nil value cannot be converted")
	}

	return autoConvertReflect(telem, asValue(v))
}

func autoConvertReflect(telem reflect.Type, val reflect.Value) (rv reflect.Value, err error) {
	// recover 只放在顶层，避免嵌套容器每层 defer。
	defer func() {
		if r := recover(); r != nil {
			rv = reflect.Value{}
			err = fmt.Errorf("autoConvert: %v", r)
		}
	}()
	return autoConvertReflectN(telem, val, 0)
}

func autoConvertReflectN(telem reflect.Type, val reflect.Value, depth int) (reflect.Value, error) {
	if depth > maxConvertDepth {
		return reflect.Value{}, fmt.Errorf("autoConvert: recursion limit exceeded")
	}
	if !val.IsValid() {
		return reflect.Zero(telem), nil
	}
	if isNumericKind(telem.Kind()) && isNumericKind(val.Kind()) {
		if cv, err := convertNumberChecked(telem, val); err == nil {
			return cv, nil
		} else {
			return reflect.Value{}, fmt.Errorf("autoConvert: %w", err)
		}
	}

	// 快速路径：类型完全一致或可直接赋值/转换
	srcType := val.Type()
	if srcType == telem || srcType.AssignableTo(telem) {
		return val, nil
	}

	// 切片→数组在长度不匹配时 val.CanConvert 会为 true，Convert 会 panic。
	if canConvertSafely(val, telem) {
		return val.Convert(telem), nil
	}

	// 接口 / 指针解包（支持多层）
	switch val.Kind() {
	case reflect.Interface, reflect.Pointer:
		if val.IsNil() {
			return reflect.Zero(telem), nil
		}
		return autoConvertReflectN(telem, val.Elem(), depth+1)
	}

	// 补齐容器元素级转换盲区（[]int→[]int64、map 键值互转、slice↔array、string→[N]byte）。
	if converted, herr, handled := convertContainer(telem, val, depth); handled {
		return converted, herr
	}

	// 目标是指针：先转换元素再取址（避免调用方手动取地址）
	if telem.Kind() == reflect.Pointer {
		elem, e2 := autoConvertReflectN(telem.Elem(), val, depth+1)
		if e2 != nil {
			return reflect.Value{}, e2
		}
		// 若元素已是指针且类型匹配，直接返回；否则新建
		if elem.Kind() == reflect.Pointer && elem.Type() == telem {
			return elem, nil
		}
		ptr := reflect.New(telem.Elem())
		if !elem.Type().AssignableTo(telem.Elem()) {
			if canConvertSafely(elem, telem.Elem()) {
				elem = elem.Convert(telem.Elem())
			} else {
				return reflect.Value{}, fmt.Errorf("autoConvert: cannot set pointer element from %s", elem.Type())
			}
		}
		ptr.Elem().Set(elem)
		return ptr, nil
	}

	// 结构体：字段名、嵌入标志一致则逐字段转换。
	// 先完整匹配再赋值，避免部分字段已写入后失败。
	if val.Kind() == reflect.Struct && telem.Kind() == reflect.Struct && val.NumField() == telem.NumField() {
		n := val.NumField()
		match := true
		for i := range n {
			sf := val.Type().Field(i)
			tf := telem.Field(i)
			if sf.Name != tf.Name || sf.Anonymous != tf.Anonymous {
				match = false
				break
			}
		}
		if match {
			dst := reflect.New(telem).Elem()
			for i := range n {
				df := dst.Field(i)
				if !df.CanSet() && df.CanAddr() {
					df = makeSettable(df, true)
				}
				if !df.CanSet() {
					if !isZeroValue(val.Field(i)) {
						return reflect.Value{}, fmt.Errorf("autoConvert: cannot set unexported field %s", telem.Field(i).Name)
					}
					continue
				}
				fv, e2 := autoConvertReflectN(telem.Field(i).Type, val.Field(i), depth+1)
				if e2 != nil {
					return reflect.Value{}, fmt.Errorf("autoConvert: struct field %s: %w", telem.Field(i).Name, e2)
				}
				if err := trySet(df, fv); err != nil {
					return reflect.Value{}, fmt.Errorf("autoConvert: struct field %s: %w", telem.Field(i).Name, err)
				}
			}
			return dst, nil
		}
	}

	// 数字 / bool / complex → 字符串（与 Get 的 getNumber 对称）
	if telem.Kind() == reflect.String {
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return reflect.ValueOf(strconv.FormatInt(val.Int(), 10)).Convert(telem), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return reflect.ValueOf(strconv.FormatUint(val.Uint(), 10)).Convert(telem), nil
		case reflect.Float32, reflect.Float64:
			return reflect.ValueOf(strconv.FormatFloat(val.Float(), 'f', -1, 64)).Convert(telem), nil
		case reflect.Bool:
			return reflect.ValueOf(strconv.FormatBool(val.Bool())).Convert(telem), nil
		case reflect.Complex64, reflect.Complex128:
			return reflect.ValueOf(fmt.Sprintf("%g", val.Complex())).Convert(telem), nil
		case reflect.Slice:
			// []rune / []byte 转 string 原先失败，与 string→[]byte 不对称。
			ek := val.Type().Elem().Kind()
			if ek == reflect.Uint8 {
				b := make([]byte, val.Len())
				reflect.Copy(reflect.ValueOf(b), val)
				return reflect.ValueOf(string(b)).Convert(telem), nil
			}
			if ek == reflect.Int32 {
				rs := make([]rune, val.Len())
				for i := range rs {
					rs[i] = rune(val.Index(i).Int())
				}
				return reflect.ValueOf(string(rs)).Convert(telem), nil
			}
		}
	}

	// 字符串 → 数字 / bool / complex
	// 使用带 bitSize 的 Parse*，保证 32/64 位平台与目标类型大小一致，并正确处理溢出
	if val.Kind() == reflect.String {
		s := val.String()
		switch telem.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			bitSize := int(telem.Bits())
			if bitSize == 0 { // 理论上不应发生，防御
				bitSize = strconv.IntSize
			}
			i, e2 := strconv.ParseInt(s, 10, bitSize)
			if e2 != nil {
				return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert string %q to %s: %w", s, telem, e2)
			}
			return reflect.ValueOf(i).Convert(telem), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			bitSize := int(telem.Bits())
			if bitSize == 0 {
				bitSize = strconv.IntSize
			}
			u, e2 := strconv.ParseUint(s, 10, bitSize)
			if e2 != nil {
				return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert string %q to %s: %w", s, telem, e2)
			}
			return reflect.ValueOf(u).Convert(telem), nil
		case reflect.Float32, reflect.Float64:
			bitSize := 64
			if telem.Kind() == reflect.Float32 {
				bitSize = 32
			}
			f, e2 := strconv.ParseFloat(s, bitSize)
			if e2 != nil {
				return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert string %q to %s: %w", s, telem, e2)
			}
			return reflect.ValueOf(f).Convert(telem), nil
		case reflect.Bool:
			b, e2 := strconv.ParseBool(s)
			if e2 != nil {
				return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert string %q to bool: %w", s, e2)
			}
			return reflect.ValueOf(b).Convert(telem), nil
		case reflect.Complex64, reflect.Complex128:
			bitSize := 128
			if telem.Kind() == reflect.Complex64 {
				bitSize = 64
			}
			c, e2 := strconv.ParseComplex(s, bitSize)
			if e2 != nil {
				return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert string %q to %s: %w", s, telem, e2)
			}
			return reflect.ValueOf(c).Convert(telem), nil
		}
	}

	// 额外兜底：同源数字种类之间的转换（CanConvert 已覆盖绝大多数，此处仅作防御）
	// 例如 int → uint 在值域内时由前面的 CanConvert 处理

	return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert %s to %s", srcType, telem)
}

// convertNumberChecked 尝试把 src 转成目标数字类型，超出范围返回 error。
func convertNumberChecked(telem reflect.Type, val reflect.Value) (reflect.Value, error) {
	switch telem.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var i int64
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			i = val.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			u := val.Uint()
			if u > math.MaxInt64 {
				return reflect.Value{}, fmt.Errorf("overflow")
			}
			i = int64(u)
		case reflect.Float32, reflect.Float64:
			f := val.Float()
			// 检查是否能放入目标类型，避免 float64(int64(f) 对 Inf/≥2^63 是非可移植的，不能只靠 float64(int64(f))!=f
			if math.IsNaN(f) || f != math.Trunc(f) || f >= 9223372036854775808.0 || f < float64(math.MinInt64) {
				return reflect.Value{}, fmt.Errorf("not integer or overflow")
			}

			i = int64(f)
			if float64(i) != f {
				return reflect.Value{}, fmt.Errorf("not integer or overflow")
			}
		default:
			return reflect.Value{}, fmt.Errorf("unsupported")
		}
		// 检查是否能放入目标类型
		bitSize := int(telem.Bits())
		if bitSize == 0 {
			bitSize = strconv.IntSize
		}
		min := int64(-1) << (bitSize - 1)
		max := int64(1)<<(bitSize-1) - 1
		if i < min || i > max {
			return reflect.Value{}, fmt.Errorf("overflow")
		}
		return reflect.ValueOf(i).Convert(telem), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		var u uint64
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			i := val.Int()
			if i < 0 {
				return reflect.Value{}, fmt.Errorf("negative")
			}
			u = uint64(i)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			u = val.Uint()
		case reflect.Float32, reflect.Float64:
			f := val.Float()
			// math.MaxUint64 转 float64 已是 2^64，f>MaxUint64 挡不住 2^64；补 round-trip 防精度洞。
			if math.IsNaN(f) || f != math.Trunc(f) || f < 0 || f >= 18446744073709551616.0 {
				return reflect.Value{}, fmt.Errorf("not integer or overflow")
			}
			u = uint64(f)
			if float64(u) != f {
				return reflect.Value{}, fmt.Errorf("not integer or overflow")
			}
		default:
			return reflect.Value{}, fmt.Errorf("unsupported")
		}
		bitSize := int(telem.Bits())
		if bitSize == 0 {
			bitSize = strconv.IntSize
		}
		max := uint64(1)<<bitSize - 1
		if bitSize == 64 {
			max = math.MaxUint64
		}
		if u > max {
			return reflect.Value{}, fmt.Errorf("overflow")
		}
		return reflect.ValueOf(u).Convert(telem), nil
	case reflect.Float32, reflect.Float64:
		// float 之间通常直接 Convert 即可，精度损失一般可接受
		if canConvertSafely(val, telem) {
			return val.Convert(telem), nil
		}
	case reflect.Complex64, reflect.Complex128:
		var c complex128
		switch val.Kind() {
		case reflect.Complex64, reflect.Complex128:
			c = val.Complex()
		case reflect.Float32, reflect.Float64:
			c = complex(val.Float(), 0)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			c = complex(float64(val.Int()), 0)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			c = complex(float64(val.Uint()), 0)
		default:
			return reflect.Value{}, fmt.Errorf("unsupported")
		}
		if telem.Kind() == reflect.Complex64 {
			if err := checkComplex64Overflow(c); err != nil {
				return reflect.Value{}, err
			}
		}
		return reflect.ValueOf(c).Convert(telem), nil
	}
	return reflect.Value{}, fmt.Errorf("not number")
}

// checkComplex64Overflow 检查 complex128 → complex64 时实部/虚部是否会因收窄溢出。
// NaN、±Inf 视为合法值放行；返回错误时指出具体是哪一部分溢出。
func checkComplex64Overflow(c complex128) error {
	r, i := real(c), imag(c)
	// NaN 比较恒为 false，自然放行；
	// ±Inf 本来就应该原样保留，不能被当作溢出。
	if !math.IsInf(r, 0) && !math.IsNaN(r) && math.Abs(r) > math.MaxFloat32 {
		return fmt.Errorf("complex overflow: real part %g exceeds float32 range", r)
	}
	if !math.IsInf(i, 0) && !math.IsNaN(i) && math.Abs(i) > math.MaxFloat32 {
		return fmt.Errorf("complex overflow: imag part %g exceeds float32 range", i)
	}
	return nil
}

func isZeroValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}

// convertContainer 尝试按元素/键值递归转换 slice、array、map。
// handled=false 表示不是容器对容器，交给后续逻辑。
func convertContainer(telem reflect.Type, val reflect.Value, depth int) (reflect.Value, error, bool) {
	switch telem.Kind() {
	case reflect.Slice:
		switch val.Kind() {
		case reflect.Slice:
			if val.IsNil() {
				return reflect.Zero(telem), nil, true
			}
			n := val.Len()
			out := reflect.MakeSlice(telem, n, max(val.Cap(), n))
			for i := range n {
				ev, err := autoConvertReflectN(telem.Elem(), val.Index(i), depth+1)
				if err != nil {
					return reflect.Value{}, err, true
				}
				if err := trySet(out.Index(i), ev); err != nil {
					return reflect.Value{}, err, true
				}
			}
			return out, nil, true
		case reflect.Array:
			n := val.Len()
			out := reflect.MakeSlice(telem, n, n)
			for i := range n {
				ev, err := autoConvertReflectN(telem.Elem(), val.Index(i), depth+1)
				if err != nil {
					return reflect.Value{}, err, true
				}
				if err := trySet(out.Index(i), ev); err != nil {
					return reflect.Value{}, err, true
				}
			}
			return out, nil, true
		case reflect.String:
			if telem.Elem().Kind() == reflect.Uint8 {
				b := []byte(val.String())
				out := reflect.MakeSlice(telem, len(b), len(b))
				reflect.Copy(out, reflect.ValueOf(b))
				return out, nil, true
			}
			if telem.Elem().Kind() == reflect.Int32 {
				rs := []rune(val.String())
				out := reflect.MakeSlice(telem, len(rs), len(rs))
				for i := range rs {
					out.Index(i).Set(reflect.ValueOf(rs[i]).Convert(telem.Elem()))
				}
				return out, nil, true
			}
		}
	case reflect.Array:
		n := telem.Len()
		switch val.Kind() {
		case reflect.Slice, reflect.Array:
			if val.Kind() == reflect.Slice && val.IsNil() {
				return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert nil slice to array %s", telem), true
			}
			if val.Len() != n {
				return reflect.Value{}, fmt.Errorf("autoConvert: length mismatch %d != %d", val.Len(), n), true
			}
			out := reflect.New(telem).Elem()
			for i := range n {
				ev, err := autoConvertReflectN(telem.Elem(), val.Index(i), depth+1)
				if err != nil {
					return reflect.Value{}, err, true
				}
				df := out.Index(i)
				if !df.CanSet() && df.CanAddr() {
					df = makeSettable(df, true)
				}
				if !df.CanSet() {
					return reflect.Value{}, fmt.Errorf("autoConvert: array element not settable"), true
				}
				if err := trySet(df, ev); err != nil {
					return reflect.Value{}, err, true
				}
			}
			return out, nil, true
		case reflect.String:
			if telem.Elem().Kind() == reflect.Uint8 && val.Len() == n {
				out := reflect.New(telem).Elem()
				reflect.Copy(out.Slice(0, n), val)
				return out, nil, true
			}
			if telem.Elem().Kind() == reflect.Int32 && len([]rune(val.String())) == n {
				rs := []rune(val.String())
				out := reflect.New(telem).Elem()
				for i := range rs {
					out.Index(i).Set(reflect.ValueOf(rs[i]).Convert(telem.Elem()))
				}
				return out, nil, true
			}
		}
	case reflect.Map:
		if val.Kind() != reflect.Map {
			return reflect.Value{}, nil, false
		}
		if val.IsNil() {
			return reflect.Zero(telem), nil, true
		}
		out := reflect.MakeMapWithSize(telem, val.Len())
		iter := val.MapRange()
		for iter.Next() {
			k, err := autoConvertReflectN(telem.Key(), iter.Key(), depth+1)
			if err != nil {
				return reflect.Value{}, err, true
			}
			v, err := autoConvertReflectN(telem.Elem(), iter.Value(), depth+1)
			if err != nil {
				return reflect.Value{}, err, true
			}
			// 修改原因：value 可能来自未导出字段，直接 SetMapIndex 可能 panic。
			if err := trySetMapIndex(out, k, v); err != nil {
				return reflect.Value{}, err, true
			}
		}
		return out, nil, true
	}
	return reflect.Value{}, nil, false
}

func typeString(a any) string {
	if a == nil {
		return "nil"
	}
	if rv, ok := a.(reflect.Value); ok {
		if !rv.IsValid() {
			return "invalid"
		}
		return rv.Type().String()
	}
	return reflect.TypeOf(a).String()
}

func panicUnsupportedOp2(op string, a, b any) any {
	panic("unsupported operator: " + typeString(a) + op + typeString(b))
}

func isTrue(val reflect.Value) bool {
	if !val.IsValid() {
		return false
	}
	switch val.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return val.Len() > 0
	case reflect.Bool:
		return val.Bool()
	case reflect.Complex64, reflect.Complex128:
		return val.Complex() != 0
	case reflect.Chan, reflect.Func, reflect.Pointer, reflect.Interface:
		return !val.IsNil()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return val.Int() != 0
	case reflect.Float32, reflect.Float64:
		return val.Float() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return val.Uint() != 0
	case reflect.Struct:
		return true
	}
	return false
}

// typeSelect 安全且高性能地将 reflect.Value 转换为 go 接口类型，支持各类未导出字段及复杂类型
func typeSelect(v reflect.Value) (r any) {
	// 未导出命名类型上 Interface() 会 panic，兜底路径必须永不崩溃。
	defer func() {
		if recover() != nil {
			r = nil
		}
	}()
	return typeSelect1(v, 0, nil)
}

func typeSelect1(v reflect.Value, depth int, inProg map[uintptr]struct{}) any {
	if depth > maxConvertDepth || !v.IsValid() {
		return nil
	}

	// 1. 处理基础数值及字符类型（保留原逻辑的拓宽转换，如 int32 → int64）
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint()
	case reflect.Float32, reflect.Float64:
		return v.Float()
	case reflect.Bool:
		return v.Bool()
	case reflect.Complex64, reflect.Complex128:
		return v.Complex()
	case reflect.String:
		return v.String()
	case reflect.UnsafePointer:
		return v.Pointer()
	case reflect.Invalid:
		return nil
	}

	// 2. 如果是可直接导出的对象，直接返回 Interface() 性能最高
	if v.CanInterface() {
		return v.Interface()
	}

	// 3. 针对非导出字段，尝试通过清除只读标志直接返回（O(1) 无内存分配）
	if unsafeExportEnabled() {
		exportedV := makeExported(v)
		if exportedV.CanInterface() {
			return exportedV.Interface()
		}
	}

	// 4. 兜底回退：纯反射递归重建，兼容所有平台（含 Wasm / TinyGo 等）
	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			// nil slice 经 MakeSlice 会变成非 nil 空切片。
			if v.CanInterface() {
				return v.Interface()
			}
			return nil
		}
		l, c := v.Len(), v.Cap()
		elemType := v.Type().Elem()
		cv := reflect.MakeSlice(v.Type(), l, c)
		for i := range l {
			item := typeSelect1(v.Index(i), depth+1, inProg)
			if item == nil {
				continue
			}
			itemVal := asValue(item)
			if itemVal.IsValid() && itemVal.Type().AssignableTo(elemType) {
				cv.Index(i).Set(itemVal)
			} else if itemVal.IsValid() && itemVal.Type().ConvertibleTo(elemType) {
				cv.Index(i).Set(itemVal.Convert(elemType))
			}
		}
		if cv.CanInterface() {
			return cv.Interface()
		}
		return nil

	case reflect.Array:
		l := v.Len()
		elemType := v.Type().Elem()
		// 使用原数组类型，保留命名 array。
		cv := reflect.New(v.Type()).Elem()
		for i := range l {
			item := typeSelect1(v.Index(i), depth+1, inProg)
			if item == nil {
				continue
			}
			itemVal := asValue(item)
			if itemVal.IsValid() && itemVal.Type().AssignableTo(elemType) {
				cv.Index(i).Set(itemVal)
			} else if itemVal.IsValid() && itemVal.Type().ConvertibleTo(elemType) {
				cv.Index(i).Set(itemVal.Convert(elemType))
			}
		}
		if cv.CanInterface() {
			return cv.Interface()
		}
		return nil

	case reflect.Map:
		if v.IsNil() {
			if v.CanInterface() {
				return v.Interface()
			}
			return nil
		}
		mt := v.Type()
		resMap := reflect.MakeMapWithSize(mt, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			rawKey := typeSelect1(iter.Key(), depth+1, inProg)
			rawVal := typeSelect1(iter.Value(), depth+1, inProg)
			var finalKey, finalVal reflect.Value
			if rawKey != nil {
				rk := asValue(rawKey)
				if rk.IsValid() && rk.Type().AssignableTo(mt.Key()) {
					finalKey = rk
				} else if rk.IsValid() && rk.Type().ConvertibleTo(mt.Key()) {
					finalKey = rk.Convert(mt.Key())
				}
			}
			if rawVal != nil {
				rv := asValue(rawVal)
				if rv.IsValid() && rv.Type().AssignableTo(mt.Elem()) {
					finalVal = rv
				} else if rv.IsValid() && rv.Type().ConvertibleTo(mt.Elem()) {
					finalVal = rv.Convert(mt.Elem())
				}
			}
			if !finalKey.IsValid() {
				finalKey = reflect.Zero(mt.Key())
			}
			if !finalVal.IsValid() {
				finalVal = reflect.Zero(mt.Elem())
			}
			resMap.SetMapIndex(finalKey, finalVal)
		}
		if resMap.CanInterface() {
			return resMap.Interface()
		}
		return nil

	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		if v.Kind() == reflect.Pointer {
			p := v.Pointer()
			if inProg != nil {
				if _, ok := inProg[p]; ok {
					return nil
				}
			} else {
				inProg = make(map[uintptr]struct{}, 8)
			}
			inProg[p] = struct{}{}
			out := typeSelect1(v.Elem(), depth+1, inProg)
			delete(inProg, p)
			return out
		}
		return typeSelect1(v.Elem(), depth+1, inProg)

	case reflect.Struct:
		t := v.Type()
		res := make(map[string]any, t.NumField())
		for i := range v.NumField() {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			res[field.Name] = typeSelect1(v.Field(i), depth+1, inProg)
		}
		return res
	}
	return nil
}

// typeInit 初始化 nil 指针/接口，并可选择将最终值置为零值或指定初始容量。
// isZero 为 true 时对 Map/Slice/Chan/Func 等进行零值或带参数初始化。
func typeInit(v reflect.Value, isZero bool, args ...any) bool {
	if !v.IsValid() || !v.CanSet() {
		return false
	}
	return initChain(v, isZero, args...)
}

// initChain 沿指针/接口链向下分配所有 nil 指针（含接口内部），
// 若 isZero 则对最内层值做零值/容器初始化。
func initChain(v reflect.Value, isZero bool, args ...any) bool {
	for range maxIndirect {
		if !v.IsValid() || (v.Kind() != reflect.Pointer && v.Kind() != reflect.Interface) {
			break
		}
		switch v.Kind() {
		case reflect.Interface:
			if v.IsNil() {
				return false // nil 接口无法猜测具体类型，保持 nil
			}
			// 非 nil 接口取出具体值，放入可设置副本递归初始化
			// 内部指针链，再写回接口，从而支持“接口内嵌深层 nil 指针”完整分配。
			inner := v.Elem()
			p := reflect.New(inner.Type()).Elem()
			p.Set(inner)
			if !initChain(p, isZero, args...) {
				return false
			}
			v.Set(p)
			return true // 内层链已在递归中完整处理，无需再下钻
		case reflect.Pointer:
			if v.IsNil() {
				if !v.CanSet() {
					return false // 指针不可设置（入口已保证根可设置，此处仅作保守防护）
				}
				nv := reflect.New(v.Type().Elem())
				v.Set(nv)
				v = nv.Elem()
				continue
			}
			v = v.Elem()
		}
	}

	if !isZero {
		return true
	}

	if !v.IsValid() || !v.CanSet() {
		return false
	}
	switch v.Kind() {
	case reflect.Map:
		v.Set(reflect.MakeMapWithSize(v.Type(), sizeArg(args, 0)))
	case reflect.Slice:
		l := sizeArg(args, 0)
		c := l
		if len(args) > 1 {
			if n, ok := toInt(args[1]); ok {
				c = max(n, l)
			}
		}
		v.Set(reflect.MakeSlice(v.Type(), l, c))
	case reflect.Func:
		if len(args) > 0 {
			if f, ok := args[0].(func([]reflect.Value) []reflect.Value); ok {
				v.Set(reflect.MakeFunc(v.Type(), f))
				return true
			}
		}
		v.Set(reflect.Zero(v.Type()))
	case reflect.Chan:
		v.Set(reflect.MakeChan(v.Type(), sizeArg(args, 0)))
	default:
		v.Set(reflect.Zero(v.Type()))
	}
	return true
}

// initNestedNilPtrs 分配结构体里已有的 nil 指针字段。
// Init 注释承诺深层 nil 指针初始化，旧实现只走顶层指针链。
// 刚分配出来的值不再下钻，否则 type Node { Next *Node } 会无限分配。
func initNestedNilPtrs(v reflect.Value, depth int, seen map[copyVisit]struct{}) {
	if depth > maxIndirect || !v.IsValid() {
		return
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		p := v.Pointer()
		if p != 0 {
			key := copyVisit{addr: p, typ: v.Type()}
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
		}
		initNestedNilPtrs(v.Elem(), depth+1, seen)
		return
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() || !v.CanSet() {
			return
		}
		inner := v.Elem()
		cp := reflect.New(inner.Type()).Elem()
		cp.Set(inner)
		initNestedNilPtrs(cp, depth+1, seen)
		v.Set(cp)
		return
	}
	if v.Kind() == reflect.Array {
		for i := range v.Len() {
			initNestedNilPtrs(v.Index(i), depth+1, seen)
		}
		return
	}

	if v.Kind() != reflect.Struct {
		return
	}
	if v.CanAddr() {
		p := v.UnsafeAddr()
		key := copyVisit{addr: p, typ: v.Type()}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
	}
	for i := range v.NumField() {
		f := v.Field(i)

		if !f.IsValid() {
			continue
		}
		if f.Kind() == reflect.Pointer && f.IsNil() {
			if f.CanSet() {
				f.Set(reflect.New(f.Type().Elem()))
			}
			continue
		}
		if f.Kind() == reflect.Pointer || f.Kind() == reflect.Struct || f.Kind() == reflect.Array || f.Kind() == reflect.Interface {
			initNestedNilPtrs(f, depth+1, seen)
		}
	}
}

// sizeArg 读取 int 类型的尺寸参数；负数钳制为 0，
// 避免 MakeSlice/MakeChan/MakeMapWithSize 收到负数 panic（Bug 2 修复）。
func sizeArg(args []any, idx int) int {
	if len(args) > idx {
		if n, ok := toInt(args[idx]); ok && n > 0 {
			return n
		}
	}
	return 0
}

// canConvertSafely 判断 src 是否能安全转换到 typ。
// 除类型层面（CanConvert）外，还处理运行期越界情形：
// 切片→数组 / 切片→数组指针 要求长度匹配，否则 reflect.Convert 会 panic。
func canConvertSafely(src reflect.Value, typ reflect.Type) bool {
	if !src.IsValid() || typ == nil || !src.CanConvert(typ) {
		return false
	}

	// 数字/布尔/复数 → string 不走原生 Convert（会变成 rune）
	if typ.Kind() == reflect.String {
		switch src.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
			reflect.Float32, reflect.Float64,
			reflect.Bool,
			reflect.Complex64, reflect.Complex128:
			return false
		}
	}

	// 切片→数组 / 数组指针：长度必须匹配
	if src.Kind() == reflect.Slice && isArrayOrArrayPtr(typ) {
		want := typ
		if typ.Kind() == reflect.Pointer {
			want = typ.Elem()
		}
		if src.Len() != want.Len() {
			return false
		}
	}
	// 不可寻址数组转切片时 CanConvert 仍可能为 true，Convert 会 panic。
	if src.Kind() == reflect.Array && typ.Kind() == reflect.Slice && !src.CanAddr() {
		return false
	}
	return true
}

func isArrayOrArrayPtr(t reflect.Type) bool {
	return t.Kind() == reflect.Array || (t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Array)
}

// typeConvert 将 src 转换为 dst 的变量类型并写入，返回是否成功。
// 约定：任何情况下都不应 panic；转换失败一律返回 false。
func typeConvert(dst, src reflect.Value) (ok bool) {
	// 安全网：任何未预期的 reflect 越界（如转换过程 panic）都当作失败，
	// 确保函数对外始终不 panic，配合上面的长度守卫双保险。
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	if !dst.IsValid() || !dst.CanSet() {
		return false
	}

	for range maxIndirect {
		if src.Kind() != reflect.Interface {
			break
		}
		if src.IsNil() {
			return false
		}
		src = src.Elem()
	}

	if !src.IsValid() || src.Kind() == reflect.Invalid {
		return false
	}

	// 每一层都先尝试直接转换，保留 *T → *T 不必解引用的语义。
	cur := src
	for i := 0; i < maxIndirect; i++ {
		// 数字类型优先走溢出检查，避免静默截断
		if isNumericKind(dst.Type().Kind()) && isNumericKind(cur.Kind()) {
			if cv, err := convertNumberChecked(dst.Type(), cur); err == nil {
				dst.Set(cv)
				return true
			}
			// 溢出或无法转换，继续尝试解引用或后续逻辑
		} else if canConvertSafely(cur, dst.Type()) {
			dst.Set(cur.Convert(dst.Type()))
			return true
		}
		if cur.Kind() == reflect.Pointer && !cur.IsNil() {
			cur = cur.Elem()
			continue
		}
		break
	}

	// 3. 目标是指针/接口时，先初始化再尝试转换到底层
	if dst.Kind() == reflect.Pointer || dst.Kind() == reflect.Interface {
		typeInit(dst, false)
		if base := inDirect(dst); base.IsValid() && base.CanSet() {
			if canConvertSafely(src, base.Type()) {
				base.Set(src.Convert(base.Type()))
				return true
			}
			// 源与目标均为结构体时，继续递归按底层转换
			if src.Kind() == reflect.Struct && base.Kind() == reflect.Struct {
				return typeConvert(base, src)
			}
		}
	}

	// 4. 源是结构体，初始化目标底层后转换
	if src.Kind() == reflect.Struct {
		typeInit(dst, false)
		if base := inDirect(dst); base.IsValid() && base.CanSet() && canConvertSafely(src, base.Type()) {
			base.Set(src.Convert(base.Type()))
			return true
		}
	}

	if cv, err := autoConvertReflect(dst.Type(), src); err == nil && cv.IsValid() && cv.Type().AssignableTo(dst.Type()) {
		dst.Set(cv)
		return true
	}
	return false
}
