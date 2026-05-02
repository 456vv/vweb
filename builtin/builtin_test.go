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
