package builtin

import (
	"bytes"
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
			return typeConvert(toValue(&dst), reflect.ValueOf(T1{I: 5})) && dst != nil && dst.I == 5
		}},
		{"ptr_to_concrete", func(t *testing.T) bool {
			var dst T1
			return typeConvert(toValue(&dst), reflect.ValueOf(&T1{I: 6})) && dst.I == 6
		}},
		{"ptr_T1_to_ptr_T2", func(t *testing.T) bool {
			var dst *T2
			return typeConvert(toValue(&dst), reflect.ValueOf(&T1{I: 7})) && dst != nil && dst.I == 7
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

func Test_typeInit(t *testing.T) {
	tests := []struct {
		name string
		f    func(*testing.T) bool
	}{
		// ========== 功能：分配 nil 指针链 ==========
		{"init_single_ptr", func(t *testing.T) bool {
			var p *int
			return typeInit(toValue(&p), false) && p != nil && *p == 0
		}},
		{"init_double_ptr", func(t *testing.T) bool {
			var pp **int

			return typeInit(toValue(&pp), false) && pp != nil && *pp != nil && **pp == 0
		}},
		{"init_ptr_to_struct", func(t *testing.T) bool {
			var p *T1
			return typeInit(toValue(&p), false) && p != nil && p.I == 0
		}},
		{"init_non_nil_ptr_unchanged", func(t *testing.T) bool {
			p := new(int)
			*p = 42
			return typeInit(toValue(&p), false) && p != nil && *p == 42
		}},
		{"init_ptr_to_interface", func(t *testing.T) bool {
			var p *any
			// 接口无法猜测具体类型，应保持 nil
			return !typeInit(toValue(&p), false) && p != nil && *p == nil
		}},
		{"init_ptr_to_ptr_to_interface", func(t *testing.T) bool {
			var p **any
			// 接口无法猜测具体类型，应保持 nil
			return !typeInit(toValue(&p), false) && p != nil && *p != nil && **p == nil
		}},
		{"init_zero_size_type", func(t *testing.T) bool {
			type Empty struct{}
			var p *Empty
			return typeInit(toValue(&p), false) && p != nil
		}},

		// ========== 功能：接口行为 ==========
		{"init_nil_interface_stays_nil", func(t *testing.T) bool {
			// nil 接口无法猜测具体类型，应保持 nil
			var i any
			return !typeInit(toValue(&i), false) && i == nil
		}},
		{"init_non_nil_interface_unchanged", func(t *testing.T) bool {
			var i any = 7
			return typeInit(toValue(&i), false) && i == 7
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
			if !typeInit(toValue(&i), false) {
				return false
			}
			p, ok := i.(**T1)
			if !ok || p == nil {
				return false
			}
			return *p != nil // 正确行为应为 true；当前为 nil → 暴露不一致
		}},

		// ========== 功能：isZero 初始化基础类型 ==========
		{"isZero_zeroes_int", func(t *testing.T) bool {
			x := 99
			return typeInit(toValue(&x), true) && x == 0
		}},
		{"isZero_zeroes_string", func(t *testing.T) bool {
			s := "abc"
			return typeInit(toValue(&s), true) && s == ""
		}},
		{"isZero_zeroes_struct", func(t *testing.T) bool {
			var t1 T1 = T1{I: 5}
			return typeInit(toValue(&t1), true) && t1.I == 0
		}},
		{"isZero_resets_pointee", func(t *testing.T) bool {
			p := new(int)
			*p = 5
			return typeInit(toValue(&p), true) && p != nil && *p == 0
		}},

		// ========== 功能：isZero 初始化容器 ==========
		{"isZero_make_map", func(t *testing.T) bool {
			var m map[string]int
			return typeInit(toValue(&m), true) && m != nil && len(m) == 0
		}},
		{"isZero_map_with_size_hint", func(t *testing.T) bool {
			var m map[int]int
			return typeInit(toValue(&m), true, 8) && m != nil && len(m) == 0
		}},
		{"isZero_resets_existing_map", func(t *testing.T) bool {
			m := map[string]int{"a": 1}
			return typeInit(toValue(&m), true) && m != nil && len(m) == 0
		}},
		{"isZero_make_slice", func(t *testing.T) bool {
			var s []int
			return typeInit(toValue(&s), true) && s != nil && len(s) == 0 && cap(s) == 0
		}},
		{"isZero_slice_len_cap", func(t *testing.T) bool {
			var s []int
			return typeInit(toValue(&s), true, 3, 5) && s != nil && len(s) == 3 && cap(s) == 5
		}},
		{"isZero_slice_cap_clamped", func(t *testing.T) bool {
			// len>cap 时 cap 被钳制到 >= len
			var s []int
			return typeInit(toValue(&s), true, 5, 3) && s != nil && len(s) == 5 && cap(s) >= 5
		}},
		{"isZero_make_chan", func(t *testing.T) bool {
			var ch chan int
			return typeInit(toValue(&ch), true) && ch != nil
		}},
		{"isZero_chan_buffered", func(t *testing.T) bool {
			var ch chan int
			typeInit(toValue(&ch), true, 3)
			if ch == nil {
				return false
			}
			for i := range 3 {
				ch <- i
			}
			return len(ch) == 3
		}},
		{"isZero_func_with_impl", func(t *testing.T) bool {
			var f func(int) int
			typeInit(toValue(&f), true, func(args []reflect.Value) []reflect.Value {
				return []reflect.Value{reflect.ValueOf(int(args[0].Int() * 2))}
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
			return typeInit(toValue(&pm), true) && pm != nil && *pm != nil
		}},
		{"isZero_ptr_to_slice", func(t *testing.T) bool {
			var ps *[]int
			return typeInit(toValue(&ps), true, 2, 4) && ps != nil && *ps != nil && len(*ps) == 2 && cap(*ps) == 4
		}},
		{"isZero_ptr_to_chan", func(t *testing.T) bool {
			var pc *chan int
			return typeInit(toValue(&pc), true, 2) && pc != nil && *pc != nil
		}},
		{"isZero_double_ptr_to_slice", func(t *testing.T) bool {
			var p **[]int
			return typeInit(toValue(&p), true, 3, 3) && p != nil && *p != nil && **p != nil && len(**p) == 3
		}},
		{"isZero_ptr_to_interface_nil", func(t *testing.T) bool {
			var p *any
			return !typeInit(toValue(&p), true) && p != nil && *p == nil
		}},

		// ========== 潜在缺陷探测 ==========
		{"BUG_non_settable_silent_noop", func(t *testing.T) bool {
			// 【Bug 3】传入不可设置 Value 时静默 no-op（不报错也不生效）。
			x := 5
			typeInit(reflect.ValueOf(&x), true) // 非 .Elem()，不可设置
			return x == 5                       // 未被修改，且无任何失败提示
		}},

		{"neg_slice_len_no_panic", func(t *testing.T) bool {
			// 修复后：负数长度钳制为 0，空切片，不 panic
			var s []int
			return typeInit(toValue(&s), true, -1) && s != nil && len(s) == 0 && cap(s) == 0
		}},
		{"neg_chan_buf_no_panic", func(t *testing.T) bool {
			// 修复后：负数缓冲钳制为 0，无缓冲 chan，不 panic
			var ch chan int
			return typeInit(toValue(&ch), true, -1) && ch != nil
		}},
		{"neg_map_hint_no_panic", func(t *testing.T) bool {
			var m map[string]int
			return typeInit(toValue(&m), true, -1) && m != nil && len(m) == 0
		}},
		{"non_settable_noop_no_panic", func(t *testing.T) bool {
			// 修复后：不可设置值直接返回，不 panic、不影响原值
			x := 5
			return !typeInit(reflect.ValueOf(&x), true) && x == 5
		}},
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

func Test_autoConvert(t *testing.T) {
	// ---------- 测试用结构体（包级别，供所有用例共享） ----------
	type srcStruct1 struct {
		A int
		B string
	}
	type dstStruct1 struct {
		A int
		B string
	}

	type srcStruct2 struct {
		X int
		Y string
	}
	type dstStruct2 struct {
		X int
		Y string
	}

	type srcStruct3 struct {
		A int
		B string
		C bool
	}
	type dstStruct3 struct {
		A int
		B string
		C bool
	}

	type srcEmbed struct {
		int
	}
	type dstEmbed struct {
		int
	}

	type srcEmbed2 struct {
		int
	}
	type dstEmbed2 struct {
		int
	}

	type srcDiffOffset struct {
		A int8
		B int64
	}
	type dstDiffOffset struct {
		B int64
		A int8
	}

	// ---------- 辅助函数 ----------
	intPtr := func(i int) *int {
		return &i
	}

	// =============================================
	// 1. 相同类型直接返回
	// =============================================
	t.Run("相同类型 int", func(t *testing.T) {
		v := 42
		ret, _ := autoConvert(reflect.TypeOf(0), v)
		if ret.Int() != 42 {
			t.Fatal("expected 42")
		}
	})

	t.Run("相同类型 string", func(t *testing.T) {
		v := "hello"
		ret, _ := autoConvert(reflect.TypeOf(""), v)
		if ret.String() != "hello" {
			t.Fatal("expected hello")
		}
	})

	t.Run("相同类型 结构体", func(t *testing.T) {
		v := srcStruct1{A: 1, B: "x"}
		ret, _ := autoConvert(reflect.TypeOf(srcStruct1{}), v)
		rv := ret.Interface().(srcStruct1)
		if rv.A != 1 || rv.B != "x" {
			t.Fatal("expected {1 x}")
		}
	})

	// =============================================
	// 2. 标准可转换（reflect.Convert）
	// =============================================
	t.Run("int 转 int64", func(t *testing.T) {
		v := 100
		ret, _ := autoConvert(reflect.TypeOf(int64(0)), v)
		if ret.Int() != 100 {
			t.Fatal("expected 100")
		}
	})

	t.Run("float64 转 int（截断）", func(t *testing.T) {
		v := 3.14
		ret, _ := autoConvert(reflect.TypeOf(0), v)
		if ret.Int() != 3 {
			t.Fatal("expected 3")
		}
	})

	t.Run("string 转 []byte", func(t *testing.T) {
		v := "abc"
		ret, _ := autoConvert(reflect.TypeOf([]byte{}), v)
		if !bytes.Equal(ret.Bytes(), []byte("abc")) {
			t.Fatal("expected abc")
		}
	})

	t.Run("[]byte 转 string", func(t *testing.T) {
		v := []byte("hello")
		ret, _ := autoConvert(reflect.TypeOf(""), v)
		if ret.String() != "hello" {
			t.Fatal("expected hello")
		}
	})

	t.Run("string 转 []rune", func(t *testing.T) {
		v := "hello"
		ret, _ := autoConvert(reflect.TypeOf([]rune{}), v)
		r := ret.Interface().([]rune)
		if len(r) != 5 || string(r) != "hello" {
			t.Fatal("expected hello")
		}
	})

	// =============================================
	// 3. nil / 零值处理
	// =============================================
	t.Run("nil 接口转 string 返回零值", func(t *testing.T) {
		var v any = nil
		ret, _ := autoConvert(reflect.TypeOf(""), v)
		if ret.String() != "" {
			t.Fatal("expected empty string")
		}
	})

	t.Run("nil 指针转 int 返回零值", func(t *testing.T) {
		var v *int = nil
		ret, _ := autoConvert(reflect.TypeOf(0), v)
		if ret.Int() != 0 {
			t.Fatal("expected 0")
		}
	})

	t.Run("nil any 转 interface{} 返回 nil", func(t *testing.T) {
		var v any
		ret, _ := autoConvert(reflect.TypeOf((*any)(nil)).Elem(), v)
		if !ret.IsNil() {
			t.Fatal("expected nil")
		}
	})

	// =============================================
	// 4. 指针解引用（源为指针）
	// =============================================
	t.Run("*int 转 int", func(t *testing.T) {
		x := 123
		v := &x
		ret, _ := autoConvert(reflect.TypeOf(0), v)
		if ret.Int() != 123 {
			t.Fatal("expected 123")
		}
	})

	t.Run("*string 转 string", func(t *testing.T) {
		s := "ptr"
		v := &s
		ret, _ := autoConvert(reflect.TypeOf(""), v)
		if ret.String() != "ptr" {
			t.Fatal("expected ptr")
		}
	})

	t.Run("***int 转 int（多层解引用）", func(t *testing.T) {
		x := 7
		p1 := &x
		p2 := &p1
		p3 := &p2
		ret, _ := autoConvert(reflect.TypeOf(0), p3)
		if ret.Int() != 7 {
			t.Fatal("expected 7")
		}
	})

	t.Run("nil 结构体指针转目标结构体零值", func(t *testing.T) {
		type A struct{ X int }
		type B struct{ X int }
		var v *A = nil
		ret, _ := autoConvert(reflect.TypeOf(B{}), v)
		rv := ret.Interface().(B)
		if rv != (B{}) {
			t.Fatal("expected zero B")
		}
	})

	// =============================================
	// 5. 目标为指针（自动取址包装）
	// =============================================
	t.Run("int 转 *int", func(t *testing.T) {
		v := 456
		ret, _ := autoConvert(reflect.TypeOf((*int)(nil)), v)
		if ret.Kind() != reflect.Pointer || ret.Elem().Int() != 456 {
			t.Fatal("expected pointer to 456")
		}
	})

	t.Run("string 转 *string", func(t *testing.T) {
		v := "str"
		ret, _ := autoConvert(reflect.TypeOf((*string)(nil)), v)
		if ret.Kind() != reflect.Pointer || ret.Elem().String() != "str" {
			t.Fatal("expected pointer to str")
		}
	})

	t.Run("结构体 转 *结构体", func(t *testing.T) {
		v := srcStruct1{A: 99, B: "ptr"}
		ret, _ := autoConvert(reflect.TypeOf((*srcStruct1)(nil)), v)
		rv := ret.Interface().(*srcStruct1)
		if rv.A != 99 || rv.B != "ptr" {
			t.Fatal("expected {99 ptr}")
		}
	})

	t.Run("*int 转 **int（嵌套指针包装）", func(t *testing.T) {
		v := intPtr(42)
		ret, _ := autoConvert(reflect.TypeOf((**int)(nil)), v)
		if ret.Kind() != reflect.Pointer ||
			ret.Elem().Kind() != reflect.Pointer ||
			ret.Elem().Elem().Int() != 42 {
			t.Fatal("expected **int to 42")
		}
	})

	// =============================================
	// 6. 接口解包
	// =============================================
	t.Run("接口内包含 int，目标 int", func(t *testing.T) {
		var v any = 99
		ret, _ := autoConvert(reflect.TypeOf(0), v)
		if ret.Int() != 99 {
			t.Fatal("expected 99")
		}
	})

	t.Run("接口内包含 string，目标 string", func(t *testing.T) {
		var v any = "world"
		ret, _ := autoConvert(reflect.TypeOf(""), v)
		if ret.String() != "world" {
			t.Fatal("expected world")
		}
	})

	t.Run("接口内包含结构体，目标相同结构体", func(t *testing.T) {
		var v any = srcStruct1{A: 10, B: "test"}
		ret, _ := autoConvert(reflect.TypeOf(srcStruct1{}), v)
		rv := ret.Interface().(srcStruct1)
		if rv.A != 10 || rv.B != "test" {
			t.Fatal("expected {10 test}")
		}
	})

	t.Run("嵌套接口 any->any->int", func(t *testing.T) {
		var inner any = 42
		var outer any = inner
		ret, _ := autoConvert(reflect.TypeOf(0), outer)
		if ret.Int() != 42 {
			t.Fatal("expected 42")
		}
	})

	// =============================================
	// 7. 结构体 unsafe 零拷贝（布局完全一致）
	// =============================================
	t.Run("srcStruct1 -> dstStruct1", func(t *testing.T) {
		v := srcStruct1{A: 55, B: "zero"}
		ret, _ := autoConvert(reflect.TypeOf(dstStruct1{}), v)
		rv := ret.Interface().(dstStruct1)
		if rv.A != 55 || rv.B != "zero" {
			t.Fatal("expected {55 zero}")
		}
	})

	t.Run("srcStruct2 -> dstStruct2", func(t *testing.T) {
		v := srcStruct2{X: 12, Y: "xy"}
		ret, _ := autoConvert(reflect.TypeOf(dstStruct2{}), v)
		rv := ret.Interface().(dstStruct2)
		if rv.X != 12 || rv.Y != "xy" {
			t.Fatal("expected {12 xy}")
		}
	})

	t.Run("匿名嵌入结构体 srcEmbed -> dstEmbed", func(t *testing.T) {
		v := srcEmbed{int: 88}
		ret, _ := autoConvert(reflect.TypeOf(dstEmbed{}), v)
		rv := ret.Interface().(dstEmbed)
		if rv.int != 88 {
			t.Fatal("expected 88")
		}
	})

	t.Run("不可寻址值的结构体转换（临时拷贝）", func(t *testing.T) {
		get := func() srcStruct1 { return srcStruct1{A: 77, B: "temp"} }
		v := get()
		ret, _ := autoConvert(reflect.TypeOf(dstStruct1{}), v)
		rv := ret.Interface().(dstStruct1)
		if rv.A != 77 || rv.B != "temp" {
			t.Fatal("expected {77 temp}")
		}
	})

	t.Run("结构体含指针字段", func(t *testing.T) {
		type A struct{ P *int }
		type B struct{ P *int }
		p := 42
		v := A{P: &p}
		ret, _ := autoConvert(reflect.TypeOf(B{}), v)
		b := ret.Interface().(B)
		if b.P == nil || *b.P != 42 {
			t.Fatal("expected pointer to 42")
		}
	})

	t.Run("结构体含 slice 字段", func(t *testing.T) {
		type A struct{ S []int }
		type B struct{ S []int }
		v := A{S: []int{1, 2, 3}}
		ret, _ := autoConvert(reflect.TypeOf(B{}), v)
		b := ret.Interface().(B)
		if len(b.S) != 3 || b.S[0] != 1 {
			t.Fatal("expected slice [1 2 3]")
		}
	})

	t.Run("嵌套嵌入结构体", func(t *testing.T) {
		type Base struct{ ID int }
		type Derived struct {
			Base
			Name string
		}
		type Derived2 struct {
			Base
			Name string
		}
		v := Derived{Base: Base{ID: 1}, Name: "test"}
		ret, _ := autoConvert(reflect.TypeOf(Derived2{}), v)
		d := ret.Interface().(Derived2)
		if d.ID != 1 || d.Name != "test" {
			t.Fatal("expected {1 test}")
		}
	})
}
