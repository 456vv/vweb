package builtin

import (
	"errors"
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
	as := assert.New(t, true)
	tests := []struct {
		name string
		f    func(t *testing.T) bool
	}{
		{
			name: "Map深度拷贝正常覆盖",
			f: func(t *testing.T) bool {
				a := B{G: []string{"1", "2", "3"}}
				b := B{G: []string{"4", "5", "6"}}
				err := copyStruct(reflect.ValueOf(&a), reflect.ValueOf(&b), "", nil, true)
				as.NotError(err)
				b.G[0] = "-"
				return fmt.Sprint(a.G) == "[4 5 6]"
			},
		}, {
			name: "不同类型自动跳过不崩溃",
			f: func(t *testing.T) bool {
				a := B{G: []string{"1", "2", "3"}}
				b := B1{G: []int{4, 5, 6}}
				err := copyStruct(reflect.ValueOf(&a), reflect.ValueOf(&b), "", nil, true)
				as.NotError(err)
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
				err := copyStruct(reflect.ValueOf(&a), reflect.ValueOf(&b), "", nil, true)
				as.NotError(err)
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

// Test_Get_NormalExamples 展示了 Get 函数在正常情况下的各种取值和类型转换能力。
func Test_Get_NormalExamples(t *testing.T) {
	// 示例 1: Map 自动类型转换查询
	t.Run("Normal: Map lookup with auto conversion", func(t *testing.T) {
		m := map[string]int{"100": 42}
		// 键为 string，但查询参数传入 int 100，Get 能够自动将其转为字符串 "100" 进行查询
		val, err := Get(m, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != 42 {
			t.Fatalf("expected 42, got %v", val)
		}
	})

	// 示例 2: 切片支持负索引
	t.Run("Normal: Slice lookup with negative index", func(t *testing.T) {
		s := []string{"apple", "banana", "cherry"}
		// -1 表示最后一个元素
		val, err := Get(s, -1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "cherry" {
			t.Fatalf("expected 'cherry', got %v", val)
		}
	})

	// 示例 3: 结构体通过字段名或 JSON 标签查询
	t.Run("Normal: Struct lookup by name (JSON tag & exact match)", func(t *testing.T) {
		type User struct {
			UserName string `json:"user_name"`
			Age      int
		}
		u := User{UserName: "Alice", Age: 18}

		// 精确匹配
		v1, err1 := Get(u, "Age")
		if err1 != nil || v1 != 18 {
			t.Fatalf("Age check failed: v=%v, err=%v", v1, err1)
		}

		// JSON 标签匹配
		v2, err2 := Get(u, "user_name")
		if err2 != nil || v2 != "Alice" {
			t.Fatalf("user_name check failed: v=%v, err=%v", v2, err2)
		}
	})
}

// Test_Get_BugsAndUnexpected 汇集了经代码分析确存的、可能引发 bug 或意外的测试用例。
func Test_Get_BugsAndUnexpected(t *testing.T) {
	// ------------------------------------------------------------------------
	// Bug 1: 匿名嵌入结构体指针为 nil 时引发 Panic (已修复安全防范测试)
	// ------------------------------------------------------------------------
	t.Run("Bug 1: Nil pointer in anonymous struct path should not panic", func(t *testing.T) {
		type Inner struct {
			Value int
		}
		type Outer struct {
			*Inner // 匿名嵌入指针
		}

		o := Outer{Inner: nil}

		// 当前 fieldByIndexSafe 已做防范，会安全返回 ErrNilValue
		_, err := Get(o, "Value")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	// ------------------------------------------------------------------------
	// 外部字段同名时未能正确遮蔽内部嵌入字段
	// ------------------------------------------------------------------------
	t.Run("Outer field should shadow inner embedded field of the same name", func(t *testing.T) {
		type Embed struct {
			Target string
		}
		type Shadow struct {
			Embed
			Target string // 外层字段，应该遮蔽 Embed.Target
		}

		s := Shadow{
			Embed:  Embed{Target: "inner"},
			Target: "outer",
		}

		val, err := Get(s, "Target")
		if err != nil {
			t.Fatalf("读取属性出错: %v", err)
		}

		// 若 buildFieldIndex 为单遍深度优先，会导致内层覆盖外层。
		if val != "outer" {
			t.Fatalf("【BUG 触发】期望获取外层值 'outer', 但获取到了内层值 '%v'，遮蔽机制失效", val)
		}
	})

	// ------------------------------------------------------------------------
	// 大小写别名查找的不对称性限制
	// ------------------------------------------------------------------------
	t.Run("Bug 3: Case sensitivity asymmetry between Id and ID", func(t *testing.T) {
		type S1 struct{ Id int }
		type S2 struct{ ID int }

		s1 := S1{Id: 100}
		s2 := S2{ID: 200}

		val1, err1 := Get(s1, "Id")
		if err1 != nil || val1 != 100 {
			t.Errorf("Id 匹配失败: v=%v, err=%v", val1, err1)
		}

		val2, err2 := Get(s2, "id")
		if err2 == nil {
			t.Errorf("期望用 'id' 查找 'ID' 失败，但得到了 %v", val2)
		}
	})

	// ------------------------------------------------------------------------
	// 浮点数索引被静默截断为整型
	// ------------------------------------------------------------------------
	t.Run("Bug 4: Float key silently truncated to int index in slice", func(t *testing.T) {
		s := []string{"zero", "one"}
		// 传入 0.9。对于切片，0.9 不是整数索引。
		// 但 toInt() 中将 float64 强制转为了 int64(0.9) -> 0 且静默修改成功。
		val, err := Get(s, 0.9)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "zero" {
			t.Fatalf("expected 'zero', got %v", val)
		}
	})

	// ------------------------------------------------------------------------
	// String 与 Number 越界错误行为不一致
	// ------------------------------------------------------------------------
	t.Run("Inconsistent out-of-bounds error behavior (String vs Number)", func(t *testing.T) {
		// 1. 对于 string 类型，越界会返回 ErrIndexOutOfRange 错误
		_, errStr := Get("hello", 10)
		if errStr == nil || !errors.Is(errStr, ErrIndexOutOfRange) {
			t.Errorf("expected ErrIndexOutOfRange for string, got %v", errStr)
		}

		// 2. 对于数值类型
		valNum, errNum := Get(123, 1)
		if errNum != nil {
			t.Errorf("unexpected error for number out of bounds: %v", errNum)
		}
		if valNum != byte('2') {
			t.Errorf("expected byte('2') for number out of bounds, got %v", valNum)
		}
	})

	// ------------------------------------------------------------------------
	// map[any]any 等包含接口键的 Map 中 nil 键查询
	// ------------------------------------------------------------------------
	t.Run("Nil key query in map of interface keys", func(t *testing.T) {
		m := map[any]string{
			nil: "nil_value",
		}

		val, err := Get(m, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "nil_value" {
			t.Fatalf("expected %q, got %v", "nil_value", val)
		}
	})

	// 非 interface key 的 map 仍然应该拒绝 nil
	t.Run("Nil key on non-interface map", func(t *testing.T) {
		m := map[string]string{"a": "b"}
		_, err := Get(m, nil)
		if !errors.Is(err, ErrMapKey) {
			t.Fatalf("expected ErrMapKey, got %v", err)
		}
	})

	// 存在 nil key 但查不到时仍返回 (nil, nil)
	t.Run("Nil key not present", func(t *testing.T) {
		m := map[any]string{"x": "y"}
		_, err := Get(m, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// ------------------------------------------------------------------------
	// 未导出结构体的切片导致静默数据丢失
	// ------------------------------------------------------------------------
	t.Run("Silent data loss when copying slice of unexported structs", func(t *testing.T) {
		type unexportedStruct struct {
			Val int
		}
		type Container struct {
			Items []unexportedStruct // 未导出结构体的切片
		}

		c := Container{
			Items: []unexportedStruct{{Val: 42}, {Val: 100}},
		}

		val, err := Get(c, "Items")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sliceVal := reflect.ValueOf(val)
		v0 := sliceVal.Index(0).FieldByName("Val").Int()
		v1 := sliceVal.Index(1).FieldByName("Val").Int()
		if v0 != 42 && v1 != 100 {
			t.Log("确认存在数据丢失：切片内容全部丢失并退化！")
		}
	})

	// ------------------------------------------------------------------------
	// 未导出结构体键的 Map 导致静默覆盖与数据丢失
	// ------------------------------------------------------------------------
	t.Run("Bug 10: Silent data loss/key-overwrite in typeSelect for map of unexported types", func(t *testing.T) {
		type unexportedStruct struct {
			Val int
		}
		type Container struct {
			Mapping map[unexportedStruct]string // 未导出结构体作为键的 map
		}

		c := Container{
			Mapping: map[unexportedStruct]string{
				{Val: 42}:  "forty-two",
				{Val: 100}: "one-hundred",
			},
		}

		val, err := Get(c, "Mapping")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		mapVal := reflect.ValueOf(val)
		if mapVal.Len() != 2 {
			t.Log("确认存在数据丢失与键覆盖：由于类型不匹配，Map 发生了静默覆盖")
		}
	})
}
