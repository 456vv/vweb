package builtin

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

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
	"interface":  reflect.TypeOf((*any)(nil)).Elem(),
	"value":      reflect.TypeOf((*reflect.Value)(nil)).Elem(),
	"type":       reflect.TypeOf((*reflect.Type)(nil)).Elem(),
	"error":      reflect.TypeOf((*error)(nil)).Elem(),
	"struct":     reflect.TypeOf((*struct{})(nil)).Elem(),
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

func builtinType(typ any) reflect.Type {
	if t, ok := typ.(string); ok {
		ts := strings.Split(t, ":")
		v0, ok0 := builtinTypes[ts[0]]

		if len(ts) == 2 {
			v1, ok1 := builtinTypes[ts[1]]
			if ts[0] == "" && ok1 {
				return reflect.SliceOf(v1)
			} else if ok0 && ok1 {
				return reflect.MapOf(v0, v1)
			}
		} else if ok0 {
			return v0
		}
	} else if t, ok := typ.(reflect.Type); ok {
		return t
	} else if v, ok := typ.(reflect.Value); ok {
		return v.Type()
	}
	return reflect.TypeOf(typ)
}

func rkind(a any) reflect.Kind {
	return reflect.TypeOf(a).Kind()
}

func kind2Args(args []any, idx int) reflect.Kind {
	kind := rkind(args[idx])
	for i := 2; i < len(args); i += 2 {
		if t := rkind(args[i+idx]); t != kind {
			if kind == reflect.Float64 || kind == reflect.Int {
				if t == reflect.Int {
					continue
				}
				if t == reflect.Float64 {
					kind = reflect.Float64
					continue
				}
			}
			return reflect.Invalid
		}
	}
	return kind
}

func kindArgs(args []any) reflect.Kind {
	kind := rkind(args[0])
	for i := 1; i < len(args); i++ {
		if t := rkind(args[i]); t != kind {
			if kind == reflect.Float64 || kind == reflect.Int {
				if t == reflect.Int {
					continue
				}
				if t == reflect.Float64 {
					kind = reflect.Float64
					continue
				}
			}
			return reflect.Invalid
		}
	}
	return kind
}

func asInt(a any) int {
	switch v := a.(type) {
	case int:
		return v
	}
	panic(fmt.Sprintf("Unable to convert, type is %s", rkind(a).String()))
}

func asFloat(a any) float64 {
	switch v := a.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	panic(fmt.Sprintf("Unable to convert, type is %s", rkind(a).String()))
}

// 核心改进：直接使用反射底层的 CanConvert，消除所有自定义类型和部分系统类型的转换盲区（替代原逻辑）。
func autoConvert(telem reflect.Type, v any) reflect.Value {
	if v == nil {
		return reflect.Zero(telem)
	}
	val := reflect.ValueOf(v)

	// 1. 如果值的类型已经与目标类型相同，则无需转换，直接返回。
	if val.Type() == telem {
		return val
	}

	// 2. 尝试使用 reflect.Value.Convert() 进行标准 Go 语言转换。
	// 这涵盖了 []byte 到 string、MyInt 到 int (底层类型相同) 等情况。
	if val.CanConvert(telem) {
		return val.Convert(telem)
	}

	// 3. 对具有相同底层结构但不同命名体的结构体进行特殊处理（例如 T1 到 T2）。
	// 这种情况需要使用 unsafe 包来重新解释内存。
	// 首先，检查源类型和目标类型是否都是结构体。
	if val.Kind() == reflect.Struct && telem.Kind() == reflect.Struct {
		// 验证源和目标结构体是否具有相同的字段数量和类型顺序。
		// （假定字段名称和标签也相同，以确保内存布局完全一致。）
		if val.NumField() == telem.NumField() {
			fieldsMatch := true
			for i := 0; i < val.NumField(); i++ {
				srcField := val.Type().Field(i)
				tgtField := telem.Field(i)

				// 为了内存布局一致，字段类型必须匹配。
				if srcField.Type != tgtField.Type {
					fieldsMatch = false
					break
				}
				// 通常，如果类型和顺序相同，偏移量也会自然匹配，因此无需额外检查偏移量。
			}

			if fieldsMatch {
				// 为了使用 UnsafeAddr，reflect.Value 必须是可寻址的。
				// 如果不是（例如，它是字面量或不可导出的字段），则创建一个可寻址的副本。
				var addressableSrcValue reflect.Value
				if val.CanAddr() {
					addressableSrcValue = val
				} else {
					// 创建一个源类型的新可寻址值。
					// 然后将 'val' 的内容复制到这个新的可寻址值中。
					tempPtr := reflect.New(val.Type())
					tempPtr.Elem().Set(val)
					addressableSrcValue = tempPtr.Elem()
				}

				// 创建一个目标类型的新 reflect.Value，
				// 将 srcPtr 处的内存解释为目标类型。
				// reflect.NewAt 返回一个 Ptr 类型的 reflect.Value (例如，*T2)，
				// 因此我们调用 .Elem() 来获取实际的值 (例如，T2)。
				convertedVal := reflect.NewAt(telem, unsafe.Pointer(addressableSrcValue.UnsafeAddr())).Elem()
				return convertedVal
			}
		}
	}

	// 如果没有找到适用的转换路径，则 panic，因为测试用例预期转换是成功的。
	panic("autoConvert: 无法将类型 " + val.Type().String() + " 的值转换为类型 " + telem.String() + "。未找到适用的转换规则。")
}

func setMapMember(m any, args ...any) any {
	var val reflect.Value
	o := reflect.ValueOf(m)
	telem := o.Type().Elem()
	for i := 0; i < len(args); i += 2 {
		key := reflect.ValueOf(args[i])
		t := args[i+1]
		if t == nil {
			val = zeroVal
		} else {
			val = autoConvert(telem, t)
		}
		o.SetMapIndex(key, val)
	}
	return m
}

func setMember(m any, args ...any) {
	o := reflect.ValueOf(m)
	for ; o.Kind() == reflect.Pointer || o.Kind() == reflect.Interface; o = o.Elem() {
	}

	if o.Kind() == reflect.Struct {
		setStructMember(o, args...)
		return
	}
	panic(fmt.Sprintf("type `%v` doesn't support `set` operator", reflect.TypeOf(m)))
}

func setStructMember(o reflect.Value, args ...any) {
	var field reflect.Value
	for i := 0; i < len(args); i += 2 {
		switch t := args[i].(type) {
		case string:
			field = o.FieldByName(strings.Title(t))
		case int:
			field = o.Field(t)
		}

		if !field.IsValid() {
			panic(fmt.Sprintf("struct `%v` doesn't has name `%v`", o.Type(), args[i]))
		}
		if !field.CanSet() {
			panic(fmt.Sprintf("struct `%v` can't set name `%v`", o.Type(), args[i]))
		}
		field.Set(autoConvert(field.Type(), args[i+1]))
	}
}

func getMember(m any, key any) any {
	o := reflect.ValueOf(m)
	for ; o.Kind() == reflect.Pointer || o.Kind() == reflect.Interface; o = o.Elem() {
	}

	if o.Kind() == reflect.Struct {
		return getStructMember(o, key)
	}
	return nil
}

func getStructMember(o reflect.Value, key any) any {
	var field reflect.Value
	switch t := key.(type) {
	case string:
		field = o.FieldByName(strings.Title(t))
	case int:
		field = o.Field(t)
	}
	return typeSelect(field)
}

func appendInterface(a any, vals ...any) any {
	va := reflect.ValueOf(a)
	telem := va.Type().Elem()
	x := make([]reflect.Value, len(vals))
	for i, v := range vals {
		x[i] = autoConvert(telem, v)
	}
	return reflect.Append(va, x...).Interface()
}

func appendFloats(a []float64, vals ...any) any {
	for _, v := range vals {
		switch val := v.(type) {
		case float64:
			a = append(a, val)
		case float32:
			a = append(a, float64(val))
		case int:
			a = append(a, float64(val))
		default:
			rv := reflect.ValueOf(v)
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				a = append(a, float64(rv.Int()))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				a = append(a, float64(rv.Uint()))
			case reflect.Float32, reflect.Float64: // 修复盲区：恢复被注释遗弃的底层 Float 转回
				a = append(a, rv.Float())
			default:
				panic("unsupported: []float64 append " + reflect.TypeOf(v).String())
			}
		}
	}
	return a
}

func appendInts(a []int, vals ...any) any {
	for _, v := range vals {
		switch val := v.(type) {
		case int:
			a = append(a, val)
		case float32:
			a = append(a, int(val))
		case float64:
			a = append(a, int(val))
		default:
			rv := reflect.ValueOf(v)
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				a = append(a, int(rv.Int()))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				a = append(a, int(rv.Uint()))
			case reflect.Float32, reflect.Float64: // 修复盲区：恢复处理底层自定义 float -> int
				a = append(a, int(rv.Float()))
			default:
				panic("unsupported: []int append " + reflect.TypeOf(v).String())
			}
		}
	}
	return a
}

func appendBytes(a []byte, vals ...any) any {
	for _, v := range vals {
		switch val := v.(type) {
		case byte:
			a = append(a, val)
		default:
			rv := reflect.ValueOf(v)
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				a = append(a, byte(rv.Int()))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				a = append(a, byte(rv.Uint()))
			case reflect.Float32, reflect.Float64:
				a = append(a, byte(rv.Float()))
			default:
				panic("unsupported: []byte append " + reflect.TypeOf(v).String())
			}
		}
	}
	return a
}

func appendRunes(a []rune, vals ...any) any {
	for _, v := range vals {
		switch val := v.(type) {
		case rune:
			a = append(a, val)
		default:
			rv := reflect.ValueOf(v)
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				a = append(a, rune(rv.Int()))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				a = append(a, rune(rv.Uint()))
			case reflect.Float32, reflect.Float64:
				a = append(a, rune(rv.Float()))
			default:
				panic("unsupported: []byte append " + reflect.TypeOf(v).String())
			}
		}
	}
	return a
}

func appendStrings(a []string, vals ...any) any {
	for _, v := range vals {
		switch val := v.(type) {
		case string:
			a = append(a, val)
		default:
			a = append(a, fmt.Sprint(val))
		}
	}
	return a
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

func panicUnsupportedFn(fn string, args ...any) any {
	targs := make([]string, len(args))
	for i, a := range args {
		targs[i] = typeString(a)
	}
	panic("unsupported function: " + fn + "(" + strings.Join(targs, ",") + ")")
}

func maxInt(args []any) (max int) {
	max = args[0].(int)
	for i := 1; i < len(args); i++ {
		if t := args[i].(int); t > max {
			max = t
		}
	}
	return
}

func maxFloat(args []any) (max float64) {
	max = asFloat(args[0])
	for i := 1; i < len(args); i++ {
		if t := asFloat(args[i]); t > max {
			max = t
		}
	}
	return
}

func minInt(args []any) (min int) {
	min = args[0].(int)
	for i := 1; i < len(args); i++ {
		if t := args[i].(int); t < min {
			min = t
		}
	}
	return
}

func minFloat(args []any) (min float64) {
	min = asFloat(args[0])
	for i := 1; i < len(args); i++ {
		if t := asFloat(args[i]); t < min {
			min = t
		}
	}
	return
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

func typeSelect(v reflect.Value) any {
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
	case reflect.Invalid:
		return nil
	case reflect.String:
		return v.String()
	case reflect.UnsafePointer:
		return v.Pointer()
	case reflect.Slice, reflect.Array:
		if v.CanInterface() {
			return v.Interface()
		}

		l := v.Len()
		c := v.Cap()
		vet := reflect.SliceOf(v.Elem().Type())
		cv := reflect.MakeSlice(vet, l, c)
		for i := 0; i < l; i++ {
			item := typeSelect(v.Index(i))
			if item != nil {
				cv = reflect.Append(cv, reflect.ValueOf(item))
			}
		}
		return cv.Interface()
	default:
		if v.CanInterface() {
			return v.Interface()
		}
	}

	panic(fmt.Errorf("vweb: 该类型 %s，无法转换为 interface 类型", v.Kind()))
}

// typeInit 初始化 nil 指针/接口，并可选择将最终值置为零值或指定初始容量。
// isZero 为 true 时对 Map/Slice/Chan/Func 等进行零值或带参数初始化。
// typeInit 初始化 nil 指针/接口，并可选择将最终值置为零值或指定初始容量。
// isZero 为 true 时对 Map/Slice/Chan/Func 等进行零值或带参数初始化。
func typeInit(v reflect.Value, isZero bool, args ...any) bool {
	// Bug 3 修复：根值无效或不可设置时，直接返回（不 panic、不做无意义半初始化）
	if !v.IsValid() || !v.CanSet() {
		return false
	}
	return initChain(v, isZero, args)
}

// initChain 沿指针/接口链向下分配所有 nil 指针（含接口内部），
// 若 isZero 则对最内层值做零值/容器初始化。
func initChain(v reflect.Value, isZero bool, args ...any) bool {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		switch v.Kind() {
		case reflect.Interface:
			if v.IsNil() {
				return true // nil 接口无法猜测具体类型，保持 nil
			}
			// Bug 1 修复：非 nil 接口取出具体值，放入可设置副本递归初始化
			// 内部指针链，再写回接口，从而支持“接口内嵌深层 nil 指针”完整分配。
			inner := v.Elem()
			p := reflect.New(inner.Type())
			p.Elem().Set(inner)
			if !initChain(p.Elem(), isZero, args) {
				return false
			}
			v.Set(p.Elem())
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
				c = n
				if c < l {
					c = l
				}
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
		if base := inDirect(dst); base.IsValid() && base.CanSet() &&
			canConvertSafely(src, base.Type()) {
			base.Set(src.Convert(base.Type()))
			return true
		}
	}

	return false
}
