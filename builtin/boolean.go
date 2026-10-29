package builtin

import (
	"cmp"
	"math"
	"reflect"
	"sync"
)

// &&
func And(arg0 any, args ...any) bool {
	ok := Bool(arg0)
	if !ok {
		return ok
	}
	for i := range args {
		ok = Bool(args[i])
		if !ok {
			break
		}
	}
	return ok
}

// ||
func Or(arg0 any, args ...any) bool {
	ok := Bool(arg0)
	if ok {
		return ok
	}
	for i := range args {
		ok = Bool(args[i])
		if ok {
			break
		}
	}
	return ok
}

// !1
func Not(a any) bool {
	return !Bool(a)
}

// a < b
func LT(a, b any) bool {
	c, ok, unordered := compareOrdered(a, b)
	if unordered {
		return false
	}
	if !ok {
		panicUnsupportedOp2("<", a, b)
	}
	return c < 0
}

// a > b
func GT(a, b any) bool {
	c, ok, unordered := compareOrdered(a, b)
	if unordered {
		return false
	}
	if !ok {
		panicUnsupportedOp2(">", a, b)
	}
	return c > 0
}

// a <= b
func LE(a, b any) bool {
	c, ok, unordered := compareOrdered(a, b)
	if unordered {
		return false
	}
	if !ok {
		panicUnsupportedOp2("<=", a, b)
	}
	return c <= 0
}

// a >= b
func GE(a, b any) bool {
	c, ok, unordered := compareOrdered(a, b)
	if unordered {
		return false
	}
	if !ok {
		panicUnsupportedOp2(">=", a, b)
	}
	return c >= 0
}

// a == b
func EQ(a, b any) (eq bool) {
	if a == nil || b == nil {
		return a == b
	}
	switch av := a.(type) {
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case int:
		bv, ok := b.(int)
		return ok && av == bv
	case int64:
		bv, ok := b.(int64)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta == nil || tb == nil {
		return a == b
	}

	// 含 interface 的 struct、array 在 Comparable()==true 时 a==b 仍可能 panic。
	// 仅对这些种类安装 recover，避免 int/string 热路径每次 defer。
	if typeMayPanicOnEqual(ta) || typeMayPanicOnEqual(tb) {
		defer func() {
			if recover() != nil {
				eq = reflect.DeepEqual(a, b)
			}
		}()
	}

	va, vb := asValue(a), asValue(b)
	if !va.IsValid() || !vb.IsValid() {
		return va.IsValid() == vb.IsValid()
	}
	if va.Kind() == reflect.Func || vb.Kind() == reflect.Func {
		if va.Kind() != reflect.Func || vb.Kind() != reflect.Func {
			return false
		}
		// 不同函数类型的 typed-nil 在 Go 中不能 ==。
		if va.Type() != vb.Type() {
			return false
		}
		if va.IsNil() || vb.IsNil() {
			return va.IsNil() == vb.IsNil()
		}
		// 非 nil 函数没有可靠身份。Pointer 只保证 nil 时为 0，闭包可能撞代码指针。
		return va.Pointer() == vb.Pointer()
	}
	if ta.Comparable() && tb.Comparable() {
		return a == b
	}
	return reflect.DeepEqual(a, b)
}

// a != b
func NE(a, b any) bool {
	return !EQ(a, b)
}

// panicEqualCache 缓存“== 是否可能 panic”。类型数量有限，且 reflect.Type 可比较。
var panicEqualCache sync.Map // map[reflect.Type]bool

// typeMayPanicOnEqual 报告在该动态类型上使用 == 是否可能 panic。
func typeMayPanicOnEqual(t reflect.Type) bool {
	if t == nil {
		return false
	}
	if v, ok := panicEqualCache.Load(t); ok {
		return v.(bool)
	}
	// 原先所有 struct/array 都安装 defer。
	// == 不会解引用指针，只有 interface（以及含 interface 的 struct/array）会在运行时 panic。
	res := typeMayPanicOnEqualRec(t, make(map[reflect.Type]struct{}))
	panicEqualCache.Store(t, res)
	return res
}

func typeMayPanicOnEqualRec(t reflect.Type, stack map[reflect.Type]struct{}) bool {
	if t == nil {
		return false
	}
	if _, ok := stack[t]; ok {
		return false
	}
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Array:
		return typeMayPanicOnEqualRec(t.Elem(), stack)
	case reflect.Struct:
		stack[t] = struct{}{}
		for i := range t.NumField() {
			ft := t.Field(i).Type
			// 指针比较不看元素；元素里的 interface 不会被这次 == 碰到。
			if ft.Kind() == reflect.Pointer {
				continue
			}
			if typeMayPanicOnEqualRec(ft, stack) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// compareOrdered 按自然序比较两个值。
// unordered=true 表示操作数类型合法但结果无序（例如 NaN）。
func compareOrdered(a, b any) (c int, ok bool, unordered bool) {
	av := inDirect(asValue(a))
	bv := inDirect(asValue(b))
	if !av.IsValid() || !bv.IsValid() {
		return 0, false, false
	}

	if av.Kind() == reflect.String && bv.Kind() == reflect.String {
		return cmp.Compare(av.String(), bv.String()), true, false
	}

	if isSignedKind(av.Kind()) && isSignedKind(bv.Kind()) {
		return cmp.Compare(av.Int(), bv.Int()), true, false
	}
	if isUnsignedKind(av.Kind()) && isUnsignedKind(bv.Kind()) {
		return cmp.Compare(av.Uint(), bv.Uint()), true, false
	}
	if isSignedKind(av.Kind()) && isUnsignedKind(bv.Kind()) {
		return cmpIntUint(av.Int(), bv.Uint()), true, false
	}
	if isUnsignedKind(av.Kind()) && isSignedKind(bv.Kind()) {
		return -cmpIntUint(bv.Int(), av.Uint()), true, false
	}
	if isFloatKind(av.Kind()) || isFloatKind(bv.Kind()) {
		af, aok := toCmpFloat(av)
		bf, bok := toCmpFloat(bv)
		if !aok || !bok {
			return 0, false, false
		}
		if af != af || bf != bf { // NaN
			return 0, true, true
		}
		if isIntKind(av.Kind()) && isFloatKind(bv.Kind()) {
			return cmpIntToFloat(av, bf), true, false
		}
		if isFloatKind(av.Kind()) && isIntKind(bv.Kind()) {
			return -cmpIntToFloat(bv, af), true, false
		}
		return cmp.Compare(af, bf), true, false
	}
	return 0, false, false
}

func cmpIntUint(i int64, u uint64) int {
	if i < 0 {
		return -1
	}
	return cmp.Compare(uint64(i), u)
}

func toCmpFloat(v reflect.Value) (float64, bool) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(v.Uint()), true
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	default:
		return 0, false
	}
}

// cmpIntToFloat 比较整数与浮点，返回负数表示整数更小。
// float64 在 2^53 以上不能表示所有整数，不能先把整数转成 float64。
func cmpIntToFloat(iv reflect.Value, f float64) int {
	if isUnsignedKind(iv.Kind()) {
		return cmpUintFloat(iv.Uint(), f)
	}
	return cmpSignedFloat(iv.Int(), f)
}

func cmpSignedFloat(i int64, f float64) int {
	// 2^63 能被 float64 精确表示，且大于一切 int64。
	if math.IsInf(f, 1) || f >= 9223372036854775808.0 {
		return -1
	}
	if math.IsInf(f, -1) || f < float64(math.MinInt64) {
		return 1
	}
	trunc := math.Trunc(f)
	ti := int64(trunc)
	if i < ti {
		return -1
	}
	if i > ti {
		return 1
	}
	if f > trunc {
		return -1
	}
	if f < trunc {
		return 1
	}
	return 0
}

func cmpUintFloat(u uint64, f float64) int {
	// 2^64 能被 float64 精确表示，且大于一切 uint64。
	if math.IsInf(f, 1) || f >= 18446744073709551616.0 {
		return -1
	}
	if math.IsInf(f, -1) || f < 0 {
		return 1
	}
	trunc := math.Trunc(f)
	tu := uint64(trunc)
	if u < tu {
		return -1
	}
	if u > tu {
		return 1
	}
	if f > trunc {
		return -1
	}
	if f < trunc {
		return 1
	}
	return 0
}
