package builtin

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/issue9/assert/v4"
)

type A struct {
	B
}
type B struct {
	*C
	F map[string]string
	G []string
	H [5]string
	i int // 未导出字段
}
type B1 struct {
	*C
	F map[string]string
	G []int
	H [3]int
}
type C struct {
	D int
}

func Test_copyStruct(t *testing.T) {
	tests := []struct {
		name string
		f    func(t *testing.T) bool
	}{
		{
			name: "Map深度拷贝正常覆盖",
			f: func(t *testing.T) bool {
				a := B{G: []string{"1", "2", "3"}}
				b := B{G: []string{"4", "5", "6"}}
				copyStruct(reflect.ValueOf(&a), reflect.ValueOf(&b), "", nil, true)
				b.G[0] = "-"
				return fmt.Sprint(a.G) == "[4 5 6]"
			},
		}, {
			name: "不同类型自动跳过不崩溃",
			f: func(t *testing.T) bool {
				a := B{G: []string{"1", "2", "3"}}
				b := B1{G: []int{4, 5, 6}}
				copyStruct(reflect.ValueOf(&a), reflect.ValueOf(&b), "", nil, true)
				return fmt.Sprint(a.G) == "[1 2 3]"
			},
		}, {
			name: "复杂嵌套指针初始化和拷贝",
			f: func(t *testing.T) bool {
				var a A
				b := A{
					B: B{
						C: &C{D: 1},
					},
				}
				copyStruct(reflect.ValueOf(&a), reflect.ValueOf(&b), "", nil, true)
				return a.B.C.D == 1
			},
		}, {
			name: "空Map跳过拷贝",
			f: func(t *testing.T) bool {
				a := B{F: map[string]string{"a": "1"}}
				b := B{F: nil}
				copyStruct(reflect.ValueOf(&a), reflect.ValueOf(&b), "", nil, true)
				return len(a.F) == 1
			},
		}, {
			name: "排除指定字段",
			f: func(t *testing.T) bool {
				a := A{}
				b := A{
					B{
						C: &C{
							D: 1,
						},
						F: map[string]string{"a": "b"},
					},
				}
				copyStruct(reflect.ValueOf(&a), reflect.ValueOf(&b), "", func(name string, dsc reflect.Value, src reflect.Value) bool {
					return name == "B.C"
				}, true)
				return a.B.C == nil && len(a.B.F) == 1
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

func Test_Set(t *testing.T) {
	as := assert.New(t, true)

	// 1. Array 设置（使用指针修复的盲区测试）
	arr := [3]int{1, 2, 3}
	Set(&arr, 1, 999)
	as.Equal(arr[1], 999)

	// 2. 传值非法拦截验证 (不再抛出底层恐慌)
	as.PanicString(func() {
		Set(arr, 1, 999)
	}, "array/slice element is unaddressable or unexported, please pass a pointer")

	// 3. Slice 设置
	sl := []int{10, 20}
	Set(&sl, 0, 100)
	as.Equal(sl[0], 100)
}

func Test_DepthField(t *testing.T) {
	as := assert.New(t, true)
	a := A{}

	// 深度有效访问
	a.B.F = map[string]string{"1": "a"}
	v, err := DepthField(a, "B", "F", "1")
	as.NotError(err).Equal(v, "a")

	a.B.G = []string{"1"}
	v, err = DepthField(a, "B", "G", 0)
	as.NotError(err).Equal(v, "1")

	// 修复盲区：不可导出的私有小写字段读取阻断 panic
	a.B.i = 100
	v, err = DepthField(a, "B", "i")
	as.Error(err).Nil(v)
}

// Test struct for Init function
type MyStruct struct {
	Name    string
	Age     int
	Enabled bool
}

// Test struct with nested pointer
type NestedStruct struct {
	ID        int
	Data      *MyStruct
	Optional  *string
	IntPtr    *int
	DoublePtr **int // For testing double pointers
}

func Test_Init(t *testing.T) {
	t.Run("Initialize nil pointer to int", func(t *testing.T) {
		var i *int // i is nil
		Init(&i)   // Pass pointer to i
		if i == nil {
			t.Errorf("Expected i to be non-nil after Init, got nil")
		}
		if *i != 0 {
			t.Errorf("Expected *i to be 0, got %d", *i)
		}
	})

	t.Run("Initialize nil pointer to string", func(t *testing.T) {
		var s *string // s is nil
		Init(&s)
		if s == nil {
			t.Errorf("Expected s to be non-nil after Init, got nil")
		}
		if *s != "" {
			t.Errorf("Expected *s to be empty string, got %q", *s)
		}
	})

	t.Run("Initialize nil pointer to bool", func(t *testing.T) {
		var b *bool // b is nil
		Init(&b)
		if b == nil {
			t.Errorf("Expected b to be non-nil after Init, got nil")
		}
		if *b != false {
			t.Errorf("Expected *b to be false, got %t", *b)
		}
	})

	t.Run("Initialize nil pointer to float64", func(t *testing.T) {
		var f *float64 // f is nil
		Init(&f)
		if f == nil {
			t.Errorf("Expected f to be non-nil after Init, got nil")
		}
		if *f != 0.0 {
			t.Errorf("Expected *f to be 0.0, got %f", *f)
		}
	})

	t.Run("Initialize nil pointer to struct", func(t *testing.T) {
		var ms *MyStruct // ms is nil
		Init(&ms)
		if ms == nil {
			t.Errorf("Expected ms to be non-nil after Init, got nil")
		}
		expected := MyStruct{} // Zero-valued struct
		if !reflect.DeepEqual(*ms, expected) {
			t.Errorf("Expected *ms to be zero-valued struct, got %+v", *ms)
		}
	})

	t.Run("Initialize nil pointer to slice", func(t *testing.T) {
		var sl *[]int // sl is nil
		Init(&sl)
		if sl == nil {
			t.Errorf("Expected sl to be non-nil after Init, got nil")
		}
		if *sl != nil { // Init for slice only makes it point to a nil slice, not make([]T, 0)
			t.Errorf("Expected *sl to be a nil slice, got %+v", *sl)
		}
		// Test that it's actually a pointer to a slice
		if reflect.ValueOf(sl).Elem().Kind() != reflect.Slice {
			t.Errorf("Expected Init to create a pointer to a slice, got %v", reflect.ValueOf(sl).Elem().Kind())
		}
	})

	t.Run("Initialize nil pointer to map", func(t *testing.T) {
		var m *map[string]any // m is nil
		Init(&m)
		if m == nil {
			t.Errorf("Expected m to be non-nil after Init, got nil")
		}
		if *m != nil { // Init for map only makes it point to a nil map, not make(map[K, V])
			t.Errorf("Expected *m to be a nil map, got %+v", *m)
		}
		// Test that it's actually a pointer to a map
		if reflect.ValueOf(m).Elem().Kind() != reflect.Map {
			t.Errorf("Expected Init to create a pointer to a map, got %v", reflect.ValueOf(m).Elem().Kind())
		}
	})

	t.Run("Initialize nil pointer to channel", func(t *testing.T) {
		var c *chan int // c is nil
		Init(&c)
		if c == nil {
			t.Errorf("Expected c to be non-nil after Init, got nil")
		}
		if *c != nil { // Init for channel only makes it point to a nil channel
			t.Errorf("Expected *c to be a nil channel, got %+v", *c)
		}
		// Test that it's actually a pointer to a channel
		if reflect.ValueOf(c).Elem().Kind() != reflect.Chan {
			t.Errorf("Expected Init to create a pointer to a channel, got %v", reflect.ValueOf(c).Elem().Kind())
		}
	})

	t.Run("Initialize already initialized int pointer", func(t *testing.T) {
		initialVal := 42
		var i *int = &initialVal
		Init(&i) // Pass pointer to i
		if i == nil {
			t.Errorf("Expected i to be non-nil after Init, got nil")
		}
		if *i != initialVal {
			t.Errorf("Expected *i to remain %d, got %d", initialVal, *i)
		}
	})

	t.Run("Initialize already initialized struct pointer", func(t *testing.T) {
		initialStruct := MyStruct{Name: "Existing", Age: 99, Enabled: true}
		var ms *MyStruct = &initialStruct
		Init(&ms)
		if ms == nil {
			t.Errorf("Expected ms to be non-nil after Init, got nil")
		}
		if !reflect.DeepEqual(*ms, initialStruct) {
			t.Errorf("Expected *ms to remain %+v, got %+v", initialStruct, *ms)
		}
	})

	t.Run("Initialize nested struct with nil fields", func(t *testing.T) {
		var ns *NestedStruct // ns is nil
		Init(&ns)
		if ns == nil {
			t.Fatalf("Expected ns to be non-nil after Init, got nil")
		}
		Init(&ns.Data)
		if ns.Data == nil {
			t.Errorf("Expected ns.Data to be non-nil after Init, got nil")
		}
		Init(&ns.Optional)
		if ns.Optional == nil {
			t.Errorf("Expected ns.Optional to be non-nil after Init, got nil")
		}
		Init(&ns.IntPtr)
		if ns.IntPtr == nil {
			t.Errorf("Expected ns.IntPtr to be non-nil after Init, got nil")
		}
		if *ns.IntPtr != 0 {
			t.Errorf("Expected *ns.IntPtr to be 0, got %d", *ns.IntPtr)
		}
		Init(&ns.DoublePtr)
		if ns.DoublePtr == nil { // **int should be initialized
			t.Errorf("Expected ns.DoublePtr to be non-nil, got nil")
		}
		if *ns.DoublePtr == nil { // *int inside **int should be initialized
			t.Errorf("Expected *ns.DoublePtr to be non-nil, got nil")
		}
		if **ns.DoublePtr != 0 { // int inside *int inside **int should be 0
			t.Errorf("Expected **ns.DoublePtr to be 0, got %d", **ns.DoublePtr)
		}
	})

	t.Run("Initialize nested struct with partially nil fields", func(t *testing.T) {
		initialInt := 100
		nsVal := NestedStruct{
			ID:       1,
			Data:     nil,         // This should be initialized
			Optional: nil,         // This should be initialized
			IntPtr:   &initialInt, // This should remain
		}
		var ns *NestedStruct = &nsVal
		Init(&ns)

		if ns == nil {
			t.Fatalf("Expected ns to be non-nil after Init, got nil")
		}
		Init(&ns.Data)
		if ns.Data == nil {
			t.Errorf("Expected ns.Data to be non-nil after Init, got nil")
		}
		Init(&ns.Optional)
		if ns.Optional == nil {
			t.Errorf("Expected ns.Optional to be non-nil after Init, got nil")
		}
		if *ns.Optional != "" {
			t.Errorf("Expected *ns.Optional to be empty string, got %q", *ns.Optional)
		}
		Init(&ns.IntPtr)
		if ns.IntPtr == nil {
			t.Errorf("Expected ns.IntPtr to be non-nil, got nil")
		}
		if *ns.IntPtr != initialInt {
			t.Errorf("Expected *ns.IntPtr to remain %d, got %d", initialInt, *ns.IntPtr)
		}
	})

	t.Run("Call Init with non-pointer concrete type (expected no change)", func(t *testing.T) {
		var i int = 5 // Not a pointer
		valBefore := i
		Init(&i) // Passing &i means 'v' becomes reflect.ValueOf(i) (an int).
		if i != valBefore {
			t.Errorf("Expected i to remain %d, but changed to %d", valBefore, i)
		}
	})

	t.Run("Call Init with nil interface (expected no change due to `break` in typeInit)", func(t *testing.T) {
		var iface any = nil
		Init(&iface) // Pass pointer to iface
		if iface != nil {
			t.Errorf("Expected nil interface to remain nil, but got %+v", iface)
		}
	})

	t.Run("Call Init with interface holding nil pointer (expected no change due to `break` in typeInit)", func(t *testing.T) {
		var ptr *int // nil pointer
		var iface any = ptr
		Init(&iface) // Pass pointer to iface
		// The internal logic `if v.Kind() == reflect.Interface { break }` means it won't allocate.
		if iface == nil {
			t.Errorf("Expected interface holding nil pointer to still hold nil pointer, but became nil interface")
		}
		val, ok := iface.(*int)
		if !ok || val == nil {
			t.Errorf("Expected interface to still hold a nil *int, got type %T, value %v", iface, iface)
		}
	})

	t.Run("Call Init with interface holding non-nil pointer (expected no change)", func(t *testing.T) {
		val := 10
		ptr := &val
		var iface any = ptr
		Init(&iface)
		if val, ok := iface.(*int); !ok || *val != 10 {
			t.Errorf("Expected interface to still hold pointer to 10, got type %T, value %v", iface, iface)
		}
	})

	t.Run("Call Init with `reflect.Value` of a nil pointer", func(t *testing.T) {
		var i *int
		rv := reflect.ValueOf(&i) // rv is reflect.Value of *i
		Init(rv)                  // Pass reflect.Value itself
		if i == nil {
			t.Errorf("Expected i to be non-nil after Init, got nil")
		}
		if *i != 0 {
			t.Errorf("Expected *i to be 0, got %d", *i)
		}
	})

	t.Run("Call Init with `reflect.Value` of an already initialized pointer", func(t *testing.T) {
		val := 55
		var i *int = &val
		rv := reflect.ValueOf(&i)
		Init(rv)
		if i == nil {
			t.Errorf("Expected i to be non-nil after Init, got nil")
		}
		if *i != 55 {
			t.Errorf("Expected *i to remain 55, got %d", *i)
		}
	})

	t.Run("Call Init with literal nil (expected panic from reflect.ValueOf(nil).Elem())", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected Init(nil) to panic, but it did not")
			} else {
				if fmt.Sprintf("%s", r) != "reflect: call of reflect.Value.Elem on zero Value" {
					t.Errorf("Expected panic message 'reflect: call of reflect.Value.Elem on zero Value', got '%v'", r)
				}
			}
		}()
		Init(nil)
	})

	t.Run("Call Init with non-pointer type directly (expected panic from reflect.ValueOf(v).Elem())", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected Init(int) to panic, but it did not")
			} else {
				if fmt.Sprintf("%s", r) != "reflect: call of reflect.Value.Elem on int Value" {
					t.Errorf("Expected panic message 'reflect: call of reflect.Value.Elem on int Value', got '%v'", r)
				}
			}
		}()
		var i int = 10
		Init(i) // Directly pass a non-pointer int
	})
}
