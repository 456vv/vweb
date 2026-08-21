package builtin

import (
	"fmt"
	"reflect"
	"testing"
)

type M1 interface {
	M() int
}
type T1 struct {
	I int
}

func (T *T1) M() int { return T.I }

type M2 interface {
	M() int
}
type T2 struct {
	I int
}

func (T *T2) M() int { return T.I }

// 用于测试自动转化的自定义类型（验证 autoConvert 利用原生 API 解锁的强大新功能）
type (
	MyInt   int
	MyFloat float64
)

func Test_autoConvert(t *testing.T) {
	tests := []struct {
		name string
		f    func(*testing.T) bool
	}{
		{
			name: "测试原生 byte slice 到 string 强制转换 (盲区覆盖)",
			f: func(t *testing.T) bool {
				v := []byte("hello")
				ret := autoConvert(reflect.TypeOf(""), v)
				return ret.String() == "hello"
			},
		},
		{
			name: "测试自定义基本类型的隐式转换 (盲区覆盖)",
			f: func(t *testing.T) bool {
				v := MyInt(100)
				ret := autoConvert(reflect.TypeOf(0), v) // MyInt -> int
				return ret.Int() == 100
			},
		},
		{
			name: "测试相同底层结构但不同命名体强转",
			f: func(t *testing.T) bool {
				v := T1{I: 999}
				ret := autoConvert(reflect.TypeOf(T2{}), v) // T1 -> T2
				return ret.Interface().(T2).I == 999
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.f(t) {
				t.Fatalf("error in %s", tt.name)
			}
		})
	}
}

func Test_typeConvert(t *testing.T) {
	tests := []struct {
		name string
		f    func(*testing.T) bool
	}{
		// ---------- 原有用例（已修正接口实现，应全部通过） ----------
		{name: "1", f: func(t *testing.T) bool {
			var t1 M1 = (*T1)(nil)
			var t2 M2 = (*T2)(nil)
			av := toValue(&t1)
			bv := toValue(&t2)
			ok := typeConvert(av, bv)
			return ok && av.Elem().Type().String() == "*builtin.T2"
		}},
		{name: "2", f: func(t *testing.T) bool {
			var t1 M1 = new(T1)
			var t2 M2 = new(T2)
			av := toValue(&t1)
			bv := toValue(&t2)
			ok := typeConvert(av, bv)
			return ok && av.Elem().Type().String() == "*builtin.T2"
		}},
		{name: "3", f: func(t *testing.T) bool {
			var t1 M1 = new(T1)
			var t2 any = new(T2)
			av := toValue(&t1)
			bv := toValue(&t2)
			ok := typeConvert(av, bv)
			return ok && av.Elem().Type().String() == "*builtin.T2"
		}},
		{name: "4", f: func(t *testing.T) bool {
			var t1 M1 = (*T1)(nil)
			var t2 any = new(T2)
			av := toValue(&t1)
			bv := toValue(&t2)
			ok := typeConvert(av, bv)
			return ok && av.Elem().Type().String() == "*builtin.T2"
		}},
		{name: "5", f: func(t *testing.T) bool {
			var t1 M1 = &T1{I: 1}
			var t2 any = (*T2)(nil)
			ok := typeConvert(toValue(&t1), toValue(&t2))
			return ok && t1 == (*T2)(nil)
		}},
		{name: "6_panic_fix", f: func(t *testing.T) bool {
			var t1 any
			ok := typeConvert(toValue(&t1), reflect.ValueOf(T1{I: 100}))
			return ok && t1.(T1).I == 100
		}},
		{name: "7_ptr_to_struct", f: func(t *testing.T) bool {
			var t1 T1
			ok := typeConvert(toValue(&t1), reflect.ValueOf(&T1{I: 200}))
			return ok && t1.I == 200
		}},
		{name: "8_interface_struct_to_ptr", f: func(t *testing.T) bool {
			var t1 *T1
			var src any = T1{I: 300}
			ok := typeConvert(toValue(&t1), toValue(&src))
			return ok && t1 != nil && t1.I == 300
		}},
		{name: "9_invalid_nil_safe", f: func(t *testing.T) bool {
			var t1 *T1
			var src any = nil
			ok := typeConvert(toValue(&t1), toValue(&src))
			return !ok && t1 == nil
		}},

		// ---------- 基础：相同类型 ----------
		{"same_int", func(t *testing.T) bool {
			var dst int
			return typeConvert(toValue(&dst), reflect.ValueOf(42)) && dst == 42
		}},
		{"same_string", func(t *testing.T) bool {
			var dst string
			return typeConvert(toValue(&dst), reflect.ValueOf("hello")) && dst == "hello"
		}},
		{"same_bool", func(t *testing.T) bool {
			var dst bool
			return typeConvert(toValue(&dst), reflect.ValueOf(true)) && dst == true
		}},

		// ---------- 数值转换 ----------
		{"int_to_int64", func(t *testing.T) bool {
			var dst int64
			return typeConvert(toValue(&dst), reflect.ValueOf(100)) && dst == 100
		}},
		{"int64_to_int", func(t *testing.T) bool {
			var dst int
			return typeConvert(toValue(&dst), reflect.ValueOf(int64(1234567))) && dst == 1234567
		}},
		{"int_to_float", func(t *testing.T) bool {
			var dst float64
			return typeConvert(toValue(&dst), reflect.ValueOf(7)) && dst == 7.0
		}},
		{"float_to_int_trunc", func(t *testing.T) bool {
			// 3.9 -> 3：向零截断，属"意外结果"（静默丢精度）
			var dst int
			return typeConvert(toValue(&dst), reflect.ValueOf(3.9)) && dst == 3
		}},
		{"int_to_uint_neg", func(t *testing.T) bool {
			// -1 -> uint：按位回绕成最大值，属"意外结果"
			var dst uint
			ok := typeConvert(toValue(&dst), reflect.ValueOf(-1))
			return ok && dst == ^uint(0)
		}},
		{"narrow_overflow_wrap", func(t *testing.T) bool {
			// 128 -> int8：溢出回绕为 -128，属"意外结果"
			var dst int8
			return typeConvert(toValue(&dst), reflect.ValueOf(128)) && dst == -128
		}},
		{"complex_narrow", func(t *testing.T) bool {
			var dst complex64
			return typeConvert(toValue(&dst), reflect.ValueOf(complex128(1+2i))) && dst == complex64(1+2i)
		}},
		{"complex_to_float_fail", func(t *testing.T) bool {
			var dst float64
			return !typeConvert(toValue(&dst), reflect.ValueOf(complex(1, 2)))
		}},

		// ---------- 字符串 / 字节 / rune ----------
		{"string_to_bytes", func(t *testing.T) bool {
			var dst []byte
			return typeConvert(toValue(&dst), reflect.ValueOf("abc")) && len(dst) == 3 && string(dst) == "abc"
		}},
		{"bytes_to_string", func(t *testing.T) bool {
			var dst string
			return typeConvert(toValue(&dst), reflect.ValueOf([]byte{'x', 'y'})) && dst == "xy"
		}},
		{"string_to_runes", func(t *testing.T) bool {
			var dst []rune
			return typeConvert(toValue(&dst), reflect.ValueOf("ab")) && len(dst) == 2 && dst[0] == 'a'
		}},
		{"int_to_string_rune", func(t *testing.T) bool {
			// 65 -> "A"：int 转 string 产生的是单 rune，而不是 "65"，属"意外结果"
			var dst string
			return typeConvert(toValue(&dst), reflect.ValueOf(65)) && dst == "A"
		}},

		// ---------- 切片 / 数组 ----------
		{"slice_to_array", func(t *testing.T) bool {
			var dst [3]int
			return typeConvert(toValue(&dst), reflect.ValueOf([]int{4, 5, 6})) && dst == [3]int{4, 5, 6}
		}},
		{"slice_short_to_array_no_panic", func(t *testing.T) bool {
			// 切片长度 < 数组长度：值级上不可转换，应当安全返回 false。
			// 若 typeConvert 直接 panic（reflect.Convert 会 panic），此用例即失败并暴露该缺陷。
			var dst [5]int
			defer func() {
				if r := recover(); r != nil {
					t.Logf("BUG: typeConvert panicked on short slice->array: %v", r)
				}
			}()
			return !typeConvert(toValue(&dst), reflect.ValueOf([]int{1, 2}))
		}},
		{"slice_to_array_ptr", func(t *testing.T) bool {
			var dst *[2]int
			return typeConvert(toValue(&dst), reflect.ValueOf([]int{9, 8})) && dst != nil && *dst == [2]int{9, 8}
		}},

		// ---------- 命名类型 ↔ 底层类型 ----------
		{"named_to_underlying", func(t *testing.T) bool {
			type MyInt int
			var dst int
			return typeConvert(toValue(&dst), reflect.ValueOf(MyInt(9))) && dst == 9
		}},
		{"underlying_to_named", func(t *testing.T) bool {
			type MyStr string
			var dst MyStr
			return typeConvert(toValue(&dst), reflect.ValueOf("z")) && dst == "z"
		}},

		// ---------- 指针 ----------
		{"concrete_to_ptr", func(t *testing.T) bool {
			var dst *T1
			return typeConvert(toValue(&dst), reflect.ValueOf(T1{I: 5})) &&
				dst != nil && dst.I == 5
		}},
		{"ptr_to_concrete", func(t *testing.T) bool {
			var dst T1
			return typeConvert(toValue(&dst), reflect.ValueOf(&T1{I: 6})) && dst.I == 6
		}},
		{"ptr_T1_to_ptr_T2", func(t *testing.T) bool {
			var dst *T2
			return typeConvert(toValue(&dst), reflect.ValueOf(&T1{I: 7})) &&
				dst != nil && dst.I == 7
		}},
		{"nil_ptr_to_concrete", func(t *testing.T) bool {
			var dst T1
			return !typeConvert(toValue(&dst), reflect.ValueOf((*T1)(nil)))
		}},
		{"ptr_to_any", func(t *testing.T) bool {
			var p *any
			ok := typeConvert(toValue(&p), reflect.ValueOf(T1{I: 8}))
			return ok && p != nil && (*p).(T1).I == 8
		}},

		// ---------- 接口 ----------
		{"concrete_to_any", func(t *testing.T) bool {
			var dst any
			return typeConvert(toValue(&dst), reflect.ValueOf(T1{I: 10})) && dst.(T1).I == 10
		}},
		{"any_to_concrete", func(t *testing.T) bool {
			var dst T1
			var src any = T1{I: 11}
			return typeConvert(toValue(&dst), toValue(&src)) && dst.I == 11
		}},
		{"nil_interface_src", func(t *testing.T) bool {
			var dst any
			var src any = nil
			return !typeConvert(toValue(&dst), toValue(&src)) && dst == nil
		}},
		{"interface_to_interface", func(t *testing.T) bool {
			var dst M2
			var src any = &T1{I: 12}
			return typeConvert(toValue(&dst), toValue(&src)) && dst != nil && dst.M() == 12
		}},
		{"nil_typed_ptr_via_interface", func(t *testing.T) bool {
			var dst M2
			var src M1 = (*T1)(nil)
			ok := typeConvert(toValue(&dst), toValue(&src))
			return ok && dst != nil && reflect.TypeOf(dst) == reflect.TypeOf((*T1)(nil))
		}},
		{"int_to_unmatched_interface", func(t *testing.T) bool {
			var dst M1
			return !typeConvert(toValue(&dst), reflect.ValueOf(1))
		}},

		// ---------- 结构体 ----------
		{"struct_to_struct", func(t *testing.T) bool {
			var dst T2
			return typeConvert(toValue(&dst), reflect.ValueOf(T1{I: 20})) && dst.I == 20
		}},
		{"struct_ptr_to_other_ptr", func(t *testing.T) bool {
			var dst *T2
			return typeConvert(toValue(&dst), reflect.ValueOf(&T1{I: 21})) && dst != nil && dst.I == 21
		}},

		// ---------- map / chan / func ----------
		{"named_map_to_map", func(t *testing.T) bool {
			type MyMap map[string]int
			var dst map[string]int
			src := MyMap{"a": 1}
			return typeConvert(toValue(&dst), reflect.ValueOf(src)) && len(dst) == 1 && dst["a"] == 1
		}},
		{"chan_conversion", func(t *testing.T) bool {
			type MyChan chan int
			var dst MyChan
			src := make(chan int, 1)
			if !typeConvert(toValue(&dst), reflect.ValueOf(src)) || dst == nil {
				return false
			}
			select {
			case dst <- 3:
			default:
				return false
			}
			return <-dst == 3
		}},
		{"func_conversion", func(t *testing.T) bool {
			type MyFunc func(int) int
			var dst MyFunc
			src := func(x int) int { return x * 2 }
			return typeConvert(toValue(&dst), reflect.ValueOf(src)) && dst != nil && dst(5) == 10
		}},

		// ---------- 失败路径：不可转换应返回 false 且绝不 panic ----------
		{"incompatible_struct_to_int", func(t *testing.T) bool {
			var dst int
			return !typeConvert(toValue(&dst), reflect.ValueOf(T1{I: 1}))
		}},
		{"incompatible_int_to_slice", func(t *testing.T) bool {
			var dst []int
			return !typeConvert(toValue(&dst), reflect.ValueOf(1))
		}},
		{"incompatible_chan_to_int", func(t *testing.T) bool {
			var dst int
			return !typeConvert(toValue(&dst), reflect.ValueOf(make(chan int)))
		}},
		{"struct_to_chan", func(t *testing.T) bool {
			var dst chan int
			return !typeConvert(toValue(&dst), reflect.ValueOf(T1{I: 1}))
		}},
		{"dst_not_settable", func(t *testing.T) bool {
			return !typeConvert(reflect.ValueOf(0), reflect.ValueOf(1))
		}},
		{"dst_invalid", func(t *testing.T) bool {
			return !typeConvert(reflect.Value{}, reflect.ValueOf(1))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.f(t) {
				t.Fatalf("error in %s", tt.name)
			}
		})
	}
}

// catchPanic 运行 f，若 panic 返回 panic 描述，否则返回 ""。
func catchPanic(f func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprintf("%v", r)
		}
	}()
	f()
	return ""
}

func Test_typeInit(t *testing.T) {
	tests := []struct {
		name string
		f    func(*testing.T) bool
	}{
		// ========== 功能：分配 nil 指针链 ==========
		{"init_single_ptr", func(t *testing.T) bool {
			var p *int
			typeInit(toValue(&p), false)
			return p != nil && *p == 0
		}},
		{"init_double_ptr", func(t *testing.T) bool {
			var pp **int
			typeInit(toValue(&pp), false)
			return pp != nil && *pp != nil && **pp == 0
		}},
		{"init_ptr_to_struct", func(t *testing.T) bool {
			var p *T1
			typeInit(toValue(&p), false)
			return p != nil && p.I == 0
		}},
		{"init_non_nil_ptr_unchanged", func(t *testing.T) bool {
			p := new(int)
			*p = 42
			typeInit(toValue(&p), false)
			return p != nil && *p == 42
		}},
		{"init_ptr_to_interface", func(t *testing.T) bool {
			var p *any
			typeInit(toValue(&p), false)
			return p != nil && *p == nil
		}},
		{"init_ptr_to_ptr_to_interface", func(t *testing.T) bool {
			var p **any
			typeInit(toValue(&p), false)
			return p != nil && *p != nil && **p == nil
		}},
		{"init_zero_size_type", func(t *testing.T) bool {
			type Empty struct{}
			var p *Empty
			typeInit(toValue(&p), false)
			return p != nil
		}},

		// ========== 功能：接口行为 ==========
		{"init_nil_interface_stays_nil", func(t *testing.T) bool {
			// nil 接口无法猜测具体类型，应保持 nil
			var i any
			typeInit(toValue(&i), false)
			return i == nil
		}},
		{"init_non_nil_interface_unchanged", func(t *testing.T) bool {
			var i any = 7
			typeInit(toValue(&i), false)
			return i == 7
		}},
		{"init_interface_holding_nil_typed_ptr", func(t *testing.T) bool {
			// 接口内顶层 nil 指针应被分配为非 nil，且保持具体类型
			var i any = (*T1)(nil)
			typeInit(toValue(&i), false)
			p, ok := i.(*T1)
			return ok && p != nil
		}},
		{"BUG_interface_holding_double_nil_ptr_incomplete", func(t *testing.T) bool {
			// 【Bug 1】接口内双层 nil 指针：外层被分配，内层仍为 nil。
			// 期望完整初始化（*p != nil），当前实现不满足 → 本用例会失败以暴露缺陷。
			var i any = (**T1)(nil)
			typeInit(toValue(&i), false)
			p, ok := i.(**T1)
			if !ok || p == nil {
				return false
			}
			return *p != nil // 正确行为应为 true；当前为 nil → 暴露不一致
		}},

		// ========== 功能：isZero 初始化基础类型 ==========
		{"isZero_zeroes_int", func(t *testing.T) bool {
			x := 99
			typeInit(toValue(&x), true)
			return x == 0
		}},
		{"isZero_zeroes_string", func(t *testing.T) bool {
			s := "abc"
			typeInit(toValue(&s), true)
			return s == ""
		}},
		{"isZero_zeroes_struct", func(t *testing.T) bool {
			var t1 T1 = T1{I: 5}
			typeInit(toValue(&t1), true)
			return t1.I == 0
		}},
		{"isZero_resets_pointee", func(t *testing.T) bool {
			p := new(int)
			*p = 5
			typeInit(toValue(&p), true)
			return p != nil && *p == 0
		}},

		// ========== 功能：isZero 初始化容器 ==========
		{"isZero_make_map", func(t *testing.T) bool {
			var m map[string]int
			typeInit(toValue(&m), true)
			return m != nil && len(m) == 0
		}},
		{"isZero_map_with_size_hint", func(t *testing.T) bool {
			var m map[int]int
			typeInit(toValue(&m), true, 8)
			return m != nil && len(m) == 0
		}},
		{"isZero_resets_existing_map", func(t *testing.T) bool {
			m := map[string]int{"a": 1}
			typeInit(toValue(&m), true)
			return m != nil && len(m) == 0
		}},
		{"isZero_make_slice", func(t *testing.T) bool {
			var s []int
			typeInit(toValue(&s), true)
			return s != nil && len(s) == 0 && cap(s) == 0
		}},
		{"isZero_slice_len_cap", func(t *testing.T) bool {
			var s []int
			typeInit(toValue(&s), true, 3, 5)
			return s != nil && len(s) == 3 && cap(s) == 5
		}},
		{"isZero_slice_cap_clamped", func(t *testing.T) bool {
			// len>cap 时 cap 被钳制到 >= len
			var s []int
			typeInit(toValue(&s), true, 5, 3)
			return s != nil && len(s) == 5 && cap(s) >= 5
		}},
		{"isZero_make_chan", func(t *testing.T) bool {
			var ch chan int
			typeInit(toValue(&ch), true)
			return ch != nil
		}},
		{"isZero_chan_buffered", func(t *testing.T) bool {
			var ch chan int
			typeInit(toValue(&ch), true, 3)
			if ch == nil {
				return false
			}
			for i := 0; i < 3; i++ {
				ch <- i
			}
			return len(ch) == 3
		}},
		{"isZero_func_with_impl", func(t *testing.T) bool {
			var f func(int) int
			typeInit(toValue(&f), true, func(args []reflect.Value) []reflect.Value {
				return []reflect.Value{reflect.ValueOf(args[0].Int() * 2)}
			})
			return f != nil && f(4) == 8
		}},
		{"isZero_func_without_impl_nil", func(t *testing.T) bool {
			var f func(int) int
			typeInit(toValue(&f), true)
			return f == nil
		}},

		// ========== 功能：指针 + isZero 组合 ==========
		{"isZero_ptr_to_map", func(t *testing.T) bool {
			var pm *map[string]int
			typeInit(toValue(&pm), true)
			return pm != nil && *pm != nil
		}},
		{"isZero_ptr_to_slice", func(t *testing.T) bool {
			var ps *[]int
			typeInit(toValue(&ps), true, 2, 4)
			return ps != nil && *ps != nil && len(*ps) == 2 && cap(*ps) == 4
		}},
		{"isZero_ptr_to_chan", func(t *testing.T) bool {
			var pc *chan int
			typeInit(toValue(&pc), true, 2)
			return pc != nil && *pc != nil
		}},
		{"isZero_double_ptr_to_slice", func(t *testing.T) bool {
			var p **[]int
			typeInit(toValue(&p), true, 3, 3)
			return p != nil && *p != nil && **p != nil && len(**p) == 3
		}},
		{"isZero_ptr_to_interface_nil", func(t *testing.T) bool {
			var p *any
			typeInit(toValue(&p), true)
			return p != nil && *p == nil
		}},

		// ========== 潜在缺陷探测 ==========
		{"BUG_neg_slice_len_panics", func(t *testing.T) bool {
			// 【Bug 2】负数长度未防护 → reflect.MakeSlice 直接 panic。
			// 断言"确实 panic"，以说明 API 缺少负数防护这一意外行为。
			var s []int
			return catchPanic(func() {
				typeInit(toValue(&s), true, -1)
			}) != ""
		}},
		{"BUG_neg_chan_buf_panics", func(t *testing.T) bool {
			var ch chan int
			return catchPanic(func() {
				typeInit(toValue(&ch), true, -1)
			}) != ""
		}},
		{"BUG_non_settable_silent_noop", func(t *testing.T) bool {
			// 【Bug 3】传入不可设置 Value 时静默 no-op（不报错也不生效）。
			x := 5
			typeInit(reflect.ValueOf(&x), true) // 非 .Elem()，不可设置
			return x == 5                       // 未被修改，且无任何失败提示
		}},

		{"neg_slice_len_no_panic", func(t *testing.T) bool {
			// 修复后：负数长度钳制为 0，空切片，不 panic
			var s []int
			return catchPanic(func() { typeInit(toValue(&s), true, -1) }) == "" &&
				s != nil && len(s) == 0 && cap(s) == 0
		}},
		{"neg_chan_buf_no_panic", func(t *testing.T) bool {
			// 修复后：负数缓冲钳制为 0，无缓冲 chan，不 panic
			var ch chan int
			return catchPanic(func() { typeInit(toValue(&ch), true, -1) }) == "" && ch != nil
		}},
		{"neg_map_hint_no_panic", func(t *testing.T) bool {
			var m map[string]int
			return catchPanic(func() { typeInit(toValue(&m), true, -1) }) == "" &&
				m != nil && len(m) == 0
		}},
		{"non_settable_noop_no_panic", func(t *testing.T) bool {
			// 修复后：不可设置值直接返回，不 panic、不影响原值
			x := 5
			return catchPanic(func() { typeInit(reflect.ValueOf(&x), true) }) == "" && x == 5
		}},
		// Bug 1 用例改为断言“已完整初始化”（原来期望失败以暴露缺陷）
		{"interface_double_nil_ptr_fully_init", func(t *testing.T) bool {
			var i any = (**T1)(nil)
			typeInit(toValue(&i), false)
			p, ok := i.(**T1)
			return ok && p != nil && *p != nil
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.f(t) {
				t.Fatalf("error in %s", tt.name)
			}
		})
	}
}
