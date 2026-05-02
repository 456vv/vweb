package builtin

import (
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

func Test_typeInit(t *testing.T) {
	tests := []struct {
		name string
		f    func(*testing.T) bool
	}{
		{name: "1", f: func(t *testing.T) bool {
			var t1 *T1
			var t11 M1 = t1
			typeInit(reflect.ValueOf(&t11).Elem(), true)
			return t11 != nil && t11.(*T1) != nil
		}}, {name: "2", f: func(t *testing.T) bool {
			var t1 *T1
			var t11 any = &t1
			typeInit(reflect.ValueOf(&t11).Elem(), true)
			return t1 != nil
		}}, {name: "3", f: func(t *testing.T) bool {
			var t1 ***T1
			var t11 any = &t1
			typeInit(reflect.ValueOf(&t11).Elem(), true)
			return t1 != nil
		}}, {name: "4", f: func(t *testing.T) bool {
			var t1 *T1
			typeInit(reflect.ValueOf(&t1).Elem(), true)
			return t1 != nil
		}}, {name: "5", f: func(t *testing.T) bool {
			var t1 M2 = (*T1)(nil)
			typeInit(reflect.ValueOf(&t1).Elem(), false)
			t2, ok := t1.(*T1)
			if !ok {
				return false
			}
			return t2 != nil
		}}, {name: "6", f: func(t *testing.T) bool {
			var t1 func()
			typeInit(reflect.ValueOf(&t1).Elem(), true, func([]reflect.Value) []reflect.Value {
				return nil
			})
			return t1 != nil
		}}, {name: "7", f: func(t *testing.T) bool {
			var t1 map[string]string
			typeInit(reflect.ValueOf(&t1).Elem(), true, 10)
			return t1 != nil
		}}, {name: "8", f: func(t *testing.T) bool {
			var t1 chan bool
			typeInit(reflect.ValueOf(&t1).Elem(), true, 1)
			t1 <- true
			select {
			case <-t1:
				return true
			default:
				return false
			}
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

func Test_typeConvert(t *testing.T) {
	tests := []struct {
		name string
		f    func(*testing.T) bool
	}{
		{name: "1", f: func(t *testing.T) bool {
			var t1 M1 = (*T1)(nil)
			var t2 M2 = (*T2)(nil)
			av := reflect.ValueOf(&t1).Elem()
			bv := reflect.ValueOf(&t2).Elem()
			ok := typeConvert(av, bv)
			return ok && av.Elem().Type().String() == "*builtin.T2"
		}},
		{name: "2", f: func(t *testing.T) bool {
			var t1 M1 = new(T1)
			var t2 M2 = new(T2)
			av := reflect.ValueOf(&t1).Elem()
			bv := reflect.ValueOf(&t2).Elem()
			ok := typeConvert(av, bv)
			return ok && av.Elem().Type().String() == "*builtin.T2"
		}},
		{name: "3", f: func(t *testing.T) bool {
			var t1 M1 = new(T1)
			var t2 any = new(T2)
			av := reflect.ValueOf(&t1).Elem()
			bv := reflect.ValueOf(&t2).Elem()
			ok := typeConvert(av, bv)
			return ok && av.Elem().Type().String() == "*builtin.T2"
		}},
		{name: "4", f: func(t *testing.T) bool {
			var t1 M1 = (*T1)(nil)
			var t2 any = new(T2)
			av := reflect.ValueOf(&t1).Elem()
			bv := reflect.ValueOf(&t2).Elem()
			ok := typeConvert(av, bv)
			return ok && av.Elem().Type().String() == "*builtin.T2"
		}},
		{name: "5", f: func(t *testing.T) bool {
			var t1 M1 = &T1{I: 1}
			var t2 any = (*T2)(nil)
			av := reflect.ValueOf(&t1).Elem()
			bv := reflect.ValueOf(&t2).Elem()
			ok := typeConvert(av, bv)
			return ok && t1 == (*T2)(nil)
		}},
		{name: "6_panic_fix", f: func(t *testing.T) bool {
			var t1 any
			src := reflect.ValueOf(T1{I: 100})
			dst := reflect.ValueOf(&t1).Elem()
			ok := typeConvert(dst, src)
			return ok && t1.(T1).I == 100
		}},
		{name: "7_ptr_to_struct", f: func(t *testing.T) bool {
			var t1 T1
			src := reflect.ValueOf(&T1{I: 200})
			dst := reflect.ValueOf(&t1).Elem()
			ok := typeConvert(dst, src)
			return ok && t1.I == 200
		}},
		{name: "8_interface_struct_to_ptr", f: func(t *testing.T) bool {
			var t1 *T1
			var src any = T1{I: 300}
			sv := reflect.ValueOf(&src).Elem()
			dst := reflect.ValueOf(&t1).Elem()
			ok := typeConvert(dst, sv)
			return ok && t1 != nil && t1.I == 300
		}},
		{name: "9_invalid_nil_safe", f: func(t *testing.T) bool {
			var t1 *T1
			var src any = nil
			sv := reflect.ValueOf(&src).Elem()
			dst := reflect.ValueOf(&t1).Elem()
			ok := typeConvert(dst, sv)
			return !ok && t1 == nil
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
