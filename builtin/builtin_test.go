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

func Test_Set_Map(t *testing.T) {
	t.Run("basic set and overwrite", func(t *testing.T) {
		m := map[string]int{"a": 1}
		if err := Set(m, "a", 10, "b", 20); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["a"] != 10 || m["b"] != 20 {
			t.Fatalf("got %v, want a=10 b=20", m)
		}
	})

	t.Run("nil value deletes key", func(t *testing.T) {
		m := map[string]int{"a": 1, "b": 2}
		if err := Set(m, "a", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := m["a"]; ok {
			t.Fatal("key a should be deleted")
		}
		if m["b"] != 2 {
			t.Fatal("key b should remain")
		}
	})

	t.Run("nil map returns ErrNilMap", func(t *testing.T) {
		var m map[string]int
		err := Set(m, "a", 1)
		if !errors.Is(err, ErrNilMap) {
			t.Fatalf("got %v, want ErrNilMap", err)
		}
	})

	t.Run("key type conversion string to int", func(t *testing.T) {
		m := map[int]string{}
		if err := Set(m, "42", "hello"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m[42] != "hello" {
			t.Fatalf("got %v", m)
		}
	})

	t.Run("key type conversion int to string", func(t *testing.T) {
		m := map[string]int{}
		if err := Set(m, 65, 200); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["A"] != 200 {
			t.Fatalf("got %v, expected m[\"65\"] to be 200", m)
		}
	})

	t.Run("invalid key type returns ErrMapKey", func(t *testing.T) {
		m := map[int]int{}
		err := Set(m, []byte("x"), 1)
		if !errors.Is(err, ErrMapKey) {
			t.Fatalf("got %v, want ErrMapKey", err)
		}
	})

	t.Run("value auto convert int to string", func(t *testing.T) {
		m := map[string]string{}
		if err := Set(m, "k", 65); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["k"] != "A" {
			t.Fatalf("got %q, expected \"65\"", m["k"])
		}
	})

	t.Run("pointer to map works", func(t *testing.T) {
		m := map[string]int{}
		if err := Set(&m, "x", 9); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["x"] != 9 {
			t.Fatalf("got %v", m)
		}
	})
}

func Test_Set_Slice(t *testing.T) {
	t.Run("basic index set", func(t *testing.T) {
		s := []int{1, 2, 3}
		if err := Set(s, 1, 99); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s[1] != 99 {
			t.Fatalf("got %v", s)
		}
	})

	t.Run("negative index", func(t *testing.T) {
		s := []int{10, 20, 30}
		if err := Set(s, -1, 99); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s[2] != 99 {
			t.Fatalf("got %v", s)
		}
	})

	t.Run("auto grow beyond current length", func(t *testing.T) {
		s := []int{1, 2}
		if err := Set(&s, 5, 100); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(s) != 6 || s[5] != 100 {
			t.Fatalf("got len=%d val=%v", len(s), s)
		}
		if s[2] != 0 || s[3] != 0 || s[4] != 0 {
			t.Fatalf("intermediate not zero: %v", s)
		}
	})

	t.Run("grow with negative index that is still out of range after normalize", func(t *testing.T) {
		s := []int{1}
		err := Set(&s, -10, 1)
		if !errors.Is(err, ErrIndexOutOfRange) {
			t.Fatalf("got %v, want ErrIndexOutOfRange", err)
		}
	})

	t.Run("non-integer key string format accepted", func(t *testing.T) {
		s := []int{1}
		err := Set(s, "0", 9)
		if err != nil {
			t.Fatalf("string index should be accepted, got %v", err)
		}
		if s[0] != 9 {
			t.Fatalf("got %v", s)
		}
	})

	t.Run("value conversion float to int", func(t *testing.T) {
		s := []int{0}
		if err := Set(s, 0, 3.14); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s[0] != 3 {
			t.Fatalf("got %d", s[0])
		}
	})

	t.Run("slice of structs", func(t *testing.T) {
		type P struct{ X int }
		s := []P{{1}, {2}}
		if err := Set(s, 0, P{99}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s[0].X != 99 {
			t.Fatalf("got %v", s)
		}
	})
}

func Test_Set_Array(t *testing.T) {
	t.Run("basic set", func(t *testing.T) {
		a := [3]int{1, 2, 3}
		if err := Set(&a, 1, 88); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a[1] != 88 {
			t.Fatalf("got %v", a)
		}
	})

	t.Run("cannot grow array", func(t *testing.T) {
		a := [2]int{1, 2}
		err := Set(&a, 5, 9)
		if !errors.Is(err, ErrIndexOutOfRange) {
			t.Fatalf("got %v, want ErrIndexOutOfRange", err)
		}
	})

	t.Run("negative index on array", func(t *testing.T) {
		a := [3]string{"a", "b", "c"}
		if err := Set(&a, -2, "X"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a[1] != "X" {
			t.Fatalf("got %v", a)
		}
	})

	t.Run("unaddressable array returns ErrUnaddressable", func(t *testing.T) {
		a := [2]int{1, 2}
		err := Set(a, 0, 9)
		if !errors.Is(err, ErrUnaddressable) {
			t.Fatalf("got %v, want ErrUnaddressable", err)
		}
	})
}

func Test_Set_Struct(t *testing.T) {
	type Person struct {
		Name string `json:"name" mapstructure:"fullname"`
		Age  int
		priv string // unexported
	}

	t.Run("set by exact field name", func(t *testing.T) {
		p := Person{Name: "old", Age: 1}
		if err := Set(&p, "Name", "new", "Age", 30); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name != "new" || p.Age != 30 {
			t.Fatalf("got %+v", p)
		}
	})

	t.Run("set by lowercase alias", func(t *testing.T) {
		p := Person{}
		if err := Set(&p, "name", "alice"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name != "alice" {
			t.Fatalf("got %q", p.Name)
		}
	})

	t.Run("set by json tag", func(t *testing.T) {
		p := Person{}
		if err := Set(&p, "name", "bob"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name != "bob" {
			t.Fatalf("got %q", p.Name)
		}
	})

	t.Run("set by mapstructure tag", func(t *testing.T) {
		p := Person{}
		if err := Set(&p, "fullname", "carol"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name != "carol" {
			t.Fatalf("got %q", p.Name)
		}
	})

	t.Run("set by integer index", func(t *testing.T) {
		p := Person{Name: "x", Age: 1}
		if err := Set(&p, 0, "y", 1, 99); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name != "y" || p.Age != 99 {
			t.Fatalf("got %+v", p)
		}
	})

	t.Run("negative field index", func(t *testing.T) {
		p := Person{Name: "a", Age: 10}
		if err := Set(&p, -2, 50); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Age != 50 {
			t.Fatalf("got %+v", p)
		}
	})

	t.Run("field not found", func(t *testing.T) {
		p := Person{}
		err := Set(&p, "Unknown", 1)
		if !errors.Is(err, ErrFieldNotFound) {
			t.Fatalf("got %v, want ErrFieldNotFound", err)
		}
	})

	t.Run("unexported field by index returns ErrUnaddressable", func(t *testing.T) {
		p := Person{}
		err := Set(&p, 2, "secret")
		if !errors.Is(err, ErrUnaddressable) {
			t.Fatalf("got %v, want ErrUnaddressable", err)
		}
	})

	t.Run("unaddressable struct (value not pointer)", func(t *testing.T) {
		p := Person{}
		err := Set(p, "Name", "x")
		if !errors.Is(err, ErrUnaddressable) {
			t.Fatalf("got %v, want ErrUnaddressable", err)
		}
	})

	t.Run("value conversion string number to int field", func(t *testing.T) {
		p := Person{}
		if err := Set(&p, "Age", "42"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Age != 42 {
			t.Fatalf("got %d", p.Age)
		}
	})
}

func Test_Set_PointerAndInterfaceUnwrap(t *testing.T) {
	t.Run("multi-level pointer to map", func(t *testing.T) {
		m := map[string]int{}
		pm := &m
		ppm := &pm
		if err := Set(ppm, "k", 7); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["k"] != 7 {
			t.Fatalf("got %v", m)
		}
	})

	t.Run("interface holding pointer to struct", func(t *testing.T) {
		type S struct{ V int }
		var i any = &S{}
		if err := Set(i, "V", 42); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if i.(*S).V != 42 {
			t.Fatalf("got %v", i)
		}
	})

	t.Run("reflect.Value input", func(t *testing.T) {
		m := map[string]int{}
		rv := reflect.ValueOf(m)
		if err := Set(rv, "x", 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["x"] != 1 {
			t.Fatalf("got %v", m)
		}
	})
}

func Test_Set_AutoConvertEdgeCases(t *testing.T) {
	t.Run("nil value to pointer field becomes nil pointer", func(t *testing.T) {
		type S struct{ N *int }
		s := S{}
		n := 5
		s.N = &n
		if err := Set(&s, "N", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.N != nil {
			t.Fatal("expected nil pointer after Set nil")
		}
	})

	t.Run("int to pointer auto takes address", func(t *testing.T) {
		type S struct{ N *int }
		s := S{}
		if err := Set(&s, "N", 99); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.N == nil || *s.N != 99 {
			t.Fatalf("got %v", s.N)
		}
	})

	t.Run("struct field copy when names and types match", func(t *testing.T) {
		type A struct {
			X int
			Y string
		}
		type B struct {
			X int
			Y string
		}
		dst := map[string]B{}
		src := A{X: 1, Y: "hi"}
		if err := Set(dst, "k", src); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dst["k"].X != 1 || dst["k"].Y != "hi" {
			t.Fatalf("got %+v", dst["k"])
		}
	})

	t.Run("bool to string", func(t *testing.T) {
		m := map[string]string{}
		if err := Set(m, "b", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["b"] != "true" {
			t.Fatalf("got %q", m["b"])
		}
	})
}

func Test_Set_UnexpectedAndBugs(t *testing.T) {
	t.Run("float key silently truncated to int", func(t *testing.T) {
		// 【BUG 演示】对于 Slice, 0.9 不是一个整数索引，但 toInt() 中的强转使其被转为 0 且静默修改成功。
		s := []int{10, 20}
		err := Set(s, 0.9, 99)
		if err != nil {
			t.Fatalf("unexpected error due to silent truncation, got: %v", err)
		}
		if s[0] != 99 {
			t.Fatalf("expected s[0] to be modified to 99, got %d", s[0])
		}
	})

	t.Run("struct conversion fails for differing field types despite comments", func(t *testing.T) {
		// 【BUG/LIMITATION 演示】尽管 X 在 A 和 B 中仅为 int 与 int64 的区别，
		// 但 autoConvert 中的 struct 拷贝过滤十分严格：sf.Type != tf.Type 直接退回 match = false。
		// 这直接破坏了注释宣称的“支持字段递归类型适配”功能。
		type A struct {
			X int
		}
		type B struct {
			X int64
		}
		a := A{X: 42}
		m := map[string]B{}
		err := Set(m, "key", a)
		if err == nil {
			t.Fatal("expected error due to strict type check in struct copy, but succeeded")
		}
	})

	t.Run("partial write on error", func(t *testing.T) {
		// 有支持事务或回滚，如果传入多个参数，前几对执行成功后，
		// 中途报错退出，数据会被污染（留在半更新状态）。
		s := []int{1, 2, 3}
		err := Set(s, 0, 99, "invalid_index", 88)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if s[0] != 1 {
			t.Fatalf("expected partial write s[0]=99, got %d", s[0])
		}
	})

	t.Run("nil value deletes key in map of interface", func(t *testing.T) {
		// 【意外的语义设计演示】对于 map[string]any，正常的 m["a"] = nil 意为将值设为 nil，
		// 而 Set 方法则会将 key 直接从 map 里 delete 掉。
		m := map[string]any{"a": 1}
		err := Set(m, "a", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, exists := m["a"]; exists {
			t.Fatal("expected key 'a' to be deleted")
		}
	})

	t.Run("nil value to non-pointer field becomes zero", func(t *testing.T) {
		// 【意外语义】将 Struct 非指针字段设置为 nil 不会报类型错误，而是强制清空为字段零值。
		type S struct {
			Age int
		}
		s := S{Age: 20}
		err := Set(&s, "Age", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.Age != 0 {
			t.Fatalf("expected Age to be reset to 0, got %d", s.Age)
		}
	})

	t.Run("capitalized lookup for lowercase tag fails", func(t *testing.T) {
		// 【限制演示】capitalize 是单向大小写补偿。
		// 结构体标签是驼峰偏小写的 "myField"，用首字母大写 "MyField" 去 Set 会报字段未找到。
		type S struct {
			F int `json:"myField"`
		}
		var s S
		err := Set(&s, "MyField", 100)
		if err == nil {
			t.Fatal("expected lookup of MyField to fail, but succeeded")
		}
	})
}

// Test_Get_FeatureAndBugs 针对 Get 功能特性及潜在 Bug 的测试集
func Test_Get_FeatureAndBugs(t *testing.T) {
	// 1. 测试空指针嵌套引发的 Panic Bug
	t.Run("1. Nil pointer in anonymous struct path should not panic", func(t *testing.T) {
		type Inner struct {
			Value int
		}
		type Outer struct {
			*Inner // 匿名嵌入指针
		}

		o := Outer{Inner: nil}

		// 拦截 Panic
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("【BUG 触发】当匿名嵌套结构体指针为 nil 时，Get 发生 panic: %v (期望返回错误或 nil)", r)
			}
		}()

		_, err := Get(o, "Value")
		if !errors.Is(err, ErrNilValue) {
			t.Logf("成功捕获预期错误")
		}
	})

	// 2. 测试嵌入字段遮蔽规则 Bug (Go 语言标准的遮蔽规则被破坏)
	t.Run("2. Outer field should shadow inner embedded field of the same name", func(t *testing.T) {
		type Embed struct {
			Target string
		}
		type Shadow struct {
			Embed
			Target string // 外层字段应该遮蔽 Embed.Target
		}

		s := Shadow{
			Embed:  Embed{Target: "inner"},
			Target: "outer",
		}

		val, err := Get(s, "Target")
		if err != nil {
			t.Fatalf("读取属性出错: %v", err)
		}

		// 当前实现：buildFieldIndex 深度优先遍历，先注册了 Embed.Target，导致外层的 Shadow.Target 无法注册
		if val != "outer" {
			t.Fatalf("【BUG 触发】期望获取外层值 'outer', 但获取到了内层值 '%v'，遮蔽机制失效", val)
		}
	})

	// 3. 测试大小写别名不对称行为
	t.Run("3. Case sensitivity asymmetry between Id and ID", func(t *testing.T) {
		type S1 struct{ Id int }
		type S2 struct{ ID int }

		s1 := S1{Id: 100}
		s2 := S2{ID: 200}

		val1, err1 := Get(s1, "Id")
		if err1 != nil {
			t.Errorf("无法获取 'Id' 字段: %v", err1)
		} else if val1 != 100 {
			t.Errorf("期望 100, 实际得到 %v", val1)
		}

		val2, err2 := Get(s2, "ID")
		if err2 != nil {
			t.Errorf("无法获取 'Id' 字段: %v", err2)
		} else if val2 != 200 {
			t.Errorf("期望 200, 实际得到 %v", val2)
		}
	})

	// 4. 测试 Map 键安全转换绕过（负数溢出为正数）
	t.Run("4. Negative key should not silently convert to huge uint", func(t *testing.T) {
		m := map[uint]string{
			1: "one",
		}

		// 传入 -1。期望得到转换错误（ErrMapKey），而不是溢出查找。
		// 原因: convertKey 中优先判断了 ConvertibleTo，-1 被转为了 18446744073709551615，绕过了后面对负数的安全拦截。
		val, err := Get(m, -1)
		if err == nil {
			t.Fatalf("【BUG 触发】负数 -1 被强转为 uint(%v) 进行了 Map 查询并返回 nil,nil，未触发类型检查错误", val)
		}
	})

	// 5. 测试数值转字符串键的不一致性
	t.Run("5. Inconsistent conversion of int vs float to string map key", func(t *testing.T) {
		m := map[string]int{
			"65": 1,
			"A":  2,
		}

		// 传递 int(65)，因为 int 可转为 string (Go 语言将 65 转为字符 'A')
		valInt, _ := Get(m, 65) // 实际匹配到键 "A" (值为 2)

		// 传递 float64(65.0)，因为 float64 无法直接转为 string，退化为 fmt.Sprint，得到 "65"
		valFloat, _ := Get(m, 65.0) // 实际匹配到键 "65" (值为 1)

		if valInt != valFloat {
			t.Fatalf("【意外行为】类型转换不一致: int(65) 匹配了 %v, float64(65.0) 匹配了 %v", valInt, valFloat)
		}
	})

	// 6. 测试不支持自定义 String 类型作为切片/数组键
	t.Run("6. Custom string type as slice index key", func(t *testing.T) {
		type MyString string
		slice := []int{10, 20, 30}

		// toInt 中有对自定义整数的 Kind 检查，但遗漏了对自定义字符串的 Kind 检查
		val, err := Get(slice, MyString("1"))
		if err != nil {
			t.Fatalf("【功能缺陷】自定义字符串类型作为索引时报错: %v", err)
		}
		if val != 20 {
			t.Fatalf("期望得到 20, 实际得到 %v", val)
		}
	})

	// 7. 测试空字符串越界与其他字符串越界不一致
	t.Run("7. Out of bounds index behavior on empty string vs non-empty string", func(t *testing.T) {
		// 非空字符串越界：返回 ErrIndexOutOfRange 错误
		_, errNormal := Get("abc", 5)
		if !errors.Is(errNormal, ErrIndexOutOfRange) {
			t.Errorf("非空字符串越界期望返回 ErrIndexOutOfRange, 实际为: %v", errNormal)
		}

		// 空字符串越界：返回值 0 且无错误
		// 原因: getString 中判断了 if n == 0 { return byte(0), nil }
		_, errEmpty := Get("", 0)
		if errEmpty != nil {
			t.Errorf("空字符串越界返回了错误: %v", errEmpty)
		} else {
			t.Fatalf("【设计缺陷】非空字符串越界会报错，而空字符串越界返回 byte(0) 且不报错，两处行为不一致")
		}
	})

	// 8. 测试未导出字段的返回值类型受地址状态影响
	t.Run("8. Unexported struct field type representation changes by addressability", func(t *testing.T) {
		type InnerStruct struct {
			Value int
		}
		type Parent struct {
			inner InnerStruct // 未导出字段
		}

		p := Parent{inner: InnerStruct{Value: 42}}

		// 1. 值传递 (不可寻址) -> 走 typeSelect 降级方案 -> 返回 map[string]any{"Value": 42}
		val1, _ := Get(p, "inner")

		// 2. 指针传递 (可寻址) -> 走 NewAt 强转方案 -> 返回原始结构体 InnerStruct{Value: 42}
		val2, _ := Get(&p, "inner")

		type1 := reflect.TypeOf(val1)
		type2 := reflect.TypeOf(val2)

		if type1 != type2 {
			t.Fatalf("【意外行为】未导出字段类型不一致: 值传递返回类型为 %v, 指针传递返回类型为 %v", type1, type2)
		}
	})
}
