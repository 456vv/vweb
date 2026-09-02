package builtin

import (
	"fmt"
	"reflect"
	"strconv"
)

// ---------------------------------------------------------------------------
// Internal type registry (read-mostly, concurrent-safe)
// ---------------------------------------------------------------------------

var builtinTypes = map[string]reflect.Type{
	"uintptr":    reflect.TypeOf(uintptr(0)),
	"int":        reflect.TypeOf(0),
	"int8":       reflect.TypeOf(int8(0)),
	"int16":      reflect.TypeOf(int16(0)),
	"int32":      reflect.TypeOf(int32(0)),
	"int64":      reflect.TypeOf(int64(0)),
	"uint":       reflect.TypeOf(uint(0)),
	"uint8":      reflect.TypeOf(uint8(0)),
	"uint16":     reflect.TypeOf(uint16(0)),
	"uint32":     reflect.TypeOf(uint32(0)),
	"uint64":     reflect.TypeOf(uint64(0)),
	"bool":       reflect.TypeOf(false),
	"float32":    reflect.TypeOf(float32(0)),
	"float64":    reflect.TypeOf(float64(0)),
	"complex64":  reflect.TypeOf(complex64(0)),
	"complex128": reflect.TypeOf(complex128(0)),
	"string":     reflect.TypeOf(""),
	"byte":       reflect.TypeOf(byte(0)),
	"rune":       reflect.TypeOf(rune(0)),
	"any":        reflect.TypeOf((*any)(nil)).Elem(),
	"interface":  reflect.TypeOf((*any)(nil)).Elem(), // alias
	"error":      reflect.TypeOf((*error)(nil)).Elem(),
	"struct":     reflect.TypeOf((*struct{})(nil)).Elem(),
	"value":      reflect.TypeOf((*reflect.Value)(nil)).Elem(),
	"type":       reflect.TypeOf((*reflect.Type)(nil)).Elem(),
}

func inDirect(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return v
		}
		v = v.Elem()
	}
	return v
}

// autoConvert 将任意值转换为目标类型。
func autoConvert(telem reflect.Type, v any) (reflect.Value, error) {
	if v == nil {
		return reflect.Zero(telem), nil
	}
	return autoConvertReflect(telem, reflect.ValueOf(v))
}

func autoConvertReflect(telem reflect.Type, val reflect.Value) (reflect.Value, error) {
	if !val.IsValid() {
		return reflect.Zero(telem), nil
	}

	// 快速路径：类型完全一致或可直接赋值/转换
	srcType := val.Type()
	if srcType == telem {
		return val, nil
	}
	if srcType.AssignableTo(telem) {
		return val, nil
	}
	if val.CanConvert(telem) {
		return val.Convert(telem), nil
	}

	// 接口 / 指针解包（支持多层）
	switch val.Kind() {
	case reflect.Interface, reflect.Pointer:
		if val.IsNil() {
			return reflect.Zero(telem), nil
		}
		return autoConvertReflect(telem, val.Elem())
	}

	// 目标是指针：先转换元素再取址（避免调用方手动取地址）
	if telem.Kind() == reflect.Pointer {
		elem, err := autoConvertReflect(telem.Elem(), val)
		if err != nil {
			return reflect.Value{}, err
		}
		// 若元素已是指针且类型匹配，直接返回；否则新建
		if elem.Kind() == reflect.Pointer && elem.Type() == telem {
			return elem, nil
		}
		ptr := reflect.New(telem.Elem())
		// 确保可设置（elem 可能来自不可寻址值）
		if elem.CanInterface() {
			ptr.Elem().Set(elem)
		} else {
			// 极少数不可接口情况回退 Zero + 错误
			return reflect.Value{}, fmt.Errorf("autoConvert: cannot set pointer element from %s", elem.Type())
		}
		return ptr, nil
	}

	// 结构体：字段名、类型、嵌入标志一致则逐字段拷贝（安全替代 unsafe 内存重解释）
	// 先完整匹配再赋值，避免部分字段已写入后失败
	if val.Kind() == reflect.Struct && telem.Kind() == reflect.Struct && val.NumField() == telem.NumField() {
		n := val.NumField()
		match := true
		for i := range n {
			sf := val.Type().Field(i)
			tf := telem.Field(i)
			if sf.Name != tf.Name || sf.Type != tf.Type || sf.Anonymous != tf.Anonymous {
				match = false
				break
			}
		}
		if match {
			dst := reflect.New(telem).Elem()
			for i := 0; i < n; i++ {
				if dst.Field(i).CanSet() {
					// 递归转换字段值，支持字段本身需要类型适配的情况
					fv, err := autoConvertReflect(telem.Field(i).Type, val.Field(i))
					if err != nil {
						return reflect.Value{}, fmt.Errorf("autoConvert: struct field %s: %w", telem.Field(i).Name, err)
					}
					dst.Field(i).Set(fv)
				}
			}
			return dst, nil
		}
	}

	// 数字 / bool / complex → 字符串（与 Get 的 getNumber 对称）
	if telem.Kind() == reflect.String {
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return reflect.ValueOf(strconv.FormatInt(val.Int(), 10)), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return reflect.ValueOf(strconv.FormatUint(val.Uint(), 10)), nil
		case reflect.Float32, reflect.Float64:
			return reflect.ValueOf(strconv.FormatFloat(val.Float(), 'f', -1, 64)), nil
		case reflect.Bool:
			return reflect.ValueOf(strconv.FormatBool(val.Bool())), nil
		case reflect.Complex64, reflect.Complex128:
			return reflect.ValueOf(fmt.Sprintf("%g", val.Complex())), nil
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
			i, err := strconv.ParseInt(s, 10, bitSize)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert string %q to %s: %w", s, telem, err)
			}
			return reflect.ValueOf(i).Convert(telem), nil

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			bitSize := int(telem.Bits())
			if bitSize == 0 {
				bitSize = strconv.IntSize
			}
			u, err := strconv.ParseUint(s, 10, bitSize)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert string %q to %s: %w", s, telem, err)
			}
			return reflect.ValueOf(u).Convert(telem), nil

		case reflect.Float32, reflect.Float64:
			bitSize := 64
			if telem.Kind() == reflect.Float32 {
				bitSize = 32
			}
			f, err := strconv.ParseFloat(s, bitSize)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert string %q to %s: %w", s, telem, err)
			}
			return reflect.ValueOf(f).Convert(telem), nil

		case reflect.Bool:
			b, err := strconv.ParseBool(s)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert string %q to bool: %w", s, err)
			}
			return reflect.ValueOf(b), nil

		case reflect.Complex64, reflect.Complex128:
			bitSize := 128
			if telem.Kind() == reflect.Complex64 {
				bitSize = 64
			}
			c, err := strconv.ParseComplex(s, bitSize)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert string %q to %s: %w", s, telem, err)
			}
			return reflect.ValueOf(c).Convert(telem), nil
		}
	}

	// 额外兜底：同源数字种类之间的转换（CanConvert 已覆盖绝大多数，此处仅作防御）
	// 例如 int → uint 在值域内时由前面的 CanConvert 处理

	return reflect.Value{}, fmt.Errorf("autoConvert: cannot convert %s to %s", srcType, telem)
}

func typeString(a any) string {
	if a == nil {
		return "nil"
	}
	return reflect.TypeOf(a).String()
}

func panicUnsupportedOp1(op string, a any) any {
	ta := typeString(a)
	panic("unsupported operator: " + op + ta)
}

func panicUnsupportedOp2(op string, a, b any) any {
	ta := typeString(a)
	tb := typeString(b)
	panic("unsupported operator: " + ta + op + tb)
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
func typeSelect(v reflect.Value) any {
	if !v.IsValid() {
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
	if useUnsafeExport {
		exportedV := makeExported(v)
		if exportedV.CanInterface() {
			return exportedV.Interface()
		}
	}

	// 4. 兜底回退：纯反射递归重建，兼容所有平台（含 Wasm / TinyGo 等）
	switch v.Kind() {
	case reflect.Slice:
		l := v.Len()
		c := v.Cap()
		elemType := v.Type().Elem()
		// 创建长度为 0、容量为 c 的切片，避免后续 Append 产生不必要的重新分配
		cv := reflect.MakeSlice(reflect.SliceOf(elemType), 0, c)
		for i := 0; i < l; i++ {
			item := typeSelect(v.Index(i))
			if item != nil {
				itemVal := reflect.ValueOf(item)
				if itemVal.Type().AssignableTo(elemType) {
					cv = reflect.Append(cv, itemVal)
				} else if itemVal.Type().ConvertibleTo(elemType) {
					cv = reflect.Append(cv, itemVal.Convert(elemType))
				} else {
					cv = reflect.Append(cv, reflect.Zero(elemType))
				}
			} else {
				cv = reflect.Append(cv, reflect.Zero(elemType))
			}
		}
		return cv.Interface()

	case reflect.Array:
		l := v.Len()
		elemType := v.Type().Elem()
		cv := reflect.New(reflect.ArrayOf(l, elemType)).Elem()
		for i := 0; i < l; i++ {
			item := typeSelect(v.Index(i))
			if item != nil {
				itemVal := reflect.ValueOf(item)
				if itemVal.Type().AssignableTo(elemType) {
					cv.Index(i).Set(itemVal)
				} else if itemVal.Type().ConvertibleTo(elemType) {
					cv.Index(i).Set(itemVal.Convert(elemType))
				}
				// 否则保持零值
			}
		}
		return cv.Interface()

	case reflect.Map:
		mt := v.Type()
		resMap := reflect.MakeMapWithSize(mt, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			val := iter.Value()
			rawKey := typeSelect(key)
			rawVal := typeSelect(val)

			var finalKey, finalVal reflect.Value
			if rawKey != nil {
				rk := reflect.ValueOf(rawKey)
				if rk.Type().AssignableTo(mt.Key()) {
					finalKey = rk
				} else if rk.Type().ConvertibleTo(mt.Key()) {
					finalKey = rk.Convert(mt.Key())
				}
			}
			if rawVal != nil {
				rv := reflect.ValueOf(rawVal)
				if rv.Type().AssignableTo(mt.Elem()) {
					finalVal = rv
				} else if rv.Type().ConvertibleTo(mt.Elem()) {
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
		return resMap.Interface()

	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return typeSelect(v.Elem())

	case reflect.Struct:
		// 纯反射回退：只导出已导出字段为 map[string]any（与原行为一致）
		t := v.Type()
		res := make(map[string]any, t.NumField())
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			res[field.Name] = typeSelect(v.Field(i))
		}
		return res
	}

	return nil
}

// typeInit 初始化 nil 指针/接口，并可选择将最终值置为零值或指定初始容量。
// isZero 为 true 时对 Map/Slice/Chan/Func 等进行零值或带参数初始化。
func typeInit(v reflect.Value, isZero bool, args ...any) bool {
	// Bug 3 修复：根值无效或不可设置时，直接返回（不 panic、不做无意义半初始化）
	if !v.IsValid() || !v.CanSet() {
		return false
	}
	return initChain(v, isZero, args...)
}

// initChain 沿指针/接口链向下分配所有 nil 指针（含接口内部），
// 若 isZero 则对最内层值做零值/容器初始化。
func initChain(v reflect.Value, isZero bool, args ...any) bool {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
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
		l := sizeArg(args, 0)
		v.Set(reflect.MakeMapWithSize(v.Type(), l))
	case reflect.Slice:
		l := sizeArg(args, 0)
		c := l
		if len(args) > 1 {
			if n, ok := args[1].(int); ok {
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
		l := sizeArg(args, 0)
		v.Set(reflect.MakeChan(v.Type(), l))
	default:
		v.Set(reflect.Zero(v.Type()))
	}
	return true
}

// sizeArg 读取 int 类型的尺寸参数；负数钳制为 0，
// 避免 MakeSlice/MakeChan/MakeMapWithSize 收到负数 panic（Bug 2 修复）。
func sizeArg(args []any, idx int) int {
	if len(args) > idx {
		if n, ok := args[idx].(int); ok && n > 0 {
			return n
		}
	}
	return 0
}

// canConvertSafely 判断 src 是否能安全转换到 typ。
// 除类型层面（CanConvert）外，还处理运行期越界情形：
// 切片→数组 / 切片→数组指针 要求长度匹配，否则 reflect.Convert 会 panic。
func canConvertSafely(src reflect.Value, typ reflect.Type) bool {
	if !src.CanConvert(typ) {
		return false
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

	// 剥离源接口层，直到得到具体类型
	for src.Kind() == reflect.Interface {
		if src.IsNil() {
			return false
		}
		src = src.Elem()
	}
	if !src.IsValid() || src.Kind() == reflect.Invalid {
		return false
	}

	// 1. 直接可转换（数值、字符串、切片/数组、命名↔底层类型等）
	if canConvertSafely(src, dst.Type()) {
		dst.Set(src.Convert(dst.Type()))
		return true
	}

	// 2. 源是非 nil 指针，解引用后递归转换
	if src.Kind() == reflect.Pointer && !src.IsNil() {
		if typeConvert(dst, src.Elem()) {
			return true
		}
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

	return false
}
