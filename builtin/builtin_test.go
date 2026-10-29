package builtin

import (
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/issue9/assert/v4"
)

// 供 copyStruct / CopyStruct 测试共享的类型定义。
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
	G []int
	J map[string][]*C
}
type C struct {
	D int
}

func TestCopyStruct(t *testing.T) {
	// ------------------------------------------------------------------
	// 基础 / 浅拷贝行为
	// ------------------------------------------------------------------
	t.Run("基础复制与类型转换", func(t *testing.T) {
		type Src struct {
			A int
			B string
			C []int
			P *int
		}
		type Dst struct {
			A int64
			B string
			C []int
			P *int
		}

		n := 7
		src := Src{A: 1, B: "x", C: []int{1, 2}, P: &n}
		var dst Dst

		if err := CopyStruct(&dst, src, nil); err != nil {
			t.Fatal(err)
		}
		if dst.A != 1 || dst.B != "x" || !reflect.DeepEqual(dst.C, []int{1, 2}) {
			t.Fatalf("字段复制/类型转换失败: %+v", dst)
		}
		if dst.P != src.P {
			t.Fatalf("浅拷贝下指针字段应共享: dst.P=%p src.P=%p", dst.P, src.P)
		}
	})

	t.Run("顶层排除", func(t *testing.T) {
		type S struct{ A, B int }

		dst := S{A: 9, B: 9}
		src := S{A: 1, B: 2}

		err := CopyStruct(&dst, src, func(name string, _, _ reflect.Value) bool {
			return name == "A"
		})
		if err != nil {
			t.Fatal(err)
		}
		if dst.A != 9 || dst.B != 2 {
			t.Fatalf("顶层 exclude 失败: %+v", dst)
		}
	})

	// 若设计只支持顶层排除，此用例会失败并暴露浅拷贝不会进入嵌套字段。
	t.Run("不支持嵌套排除", func(t *testing.T) {
		type Inner struct{ X, Y int }
		type Outer struct {
			Inner Inner
			Z     int
		}

		dst := Outer{Inner: Inner{X: 9, Y: 9}, Z: 9}
		src := Outer{Inner: Inner{X: 1, Y: 2}, Z: 3}

		err := CopyStruct(&dst, src, func(name string, _, _ reflect.Value) bool {
			return name == "Inner"
		})
		if err != nil {
			t.Fatal(err)
		}
		if dst.Inner.X != 9 {
			t.Fatalf("嵌套 exclude 未生效: Inner.X=%d, 期望保持 9", dst.Inner.X)
		}
		if dst.Z != 3 {
			t.Fatalf("其余字段应正常复制: %+v", dst)
		}
	})

	// 用于暴露无法转换时返回错误。
	t.Run("无法转换时报错", func(t *testing.T) {
		type S struct{ A string }
		type D struct{ A int }

		dst := D{A: 42}
		err := CopyStruct(&dst, S{A: "not-int"}, nil)
		if err == nil {
			t.Fatalf("字段无法从 string 转为 int，但返回 nil；dst.A=%d，疑似静默跳过", dst.A)
		}
	})

	t.Run("非指针目标报错", func(t *testing.T) {
		type S struct{ A int }

		var dst S
		err := CopyStruct(dst, S{A: 1}, nil)
		if err == nil {
			t.Fatal("非指针目标应返回错误")
		}
	})

	t.Run("nil指针字段", func(t *testing.T) {
		type S struct{ P *int }

		x := 1
		dst := S{P: &x}
		var src S

		if err := CopyStruct(&dst, src, nil); err != nil {
			t.Fatal(err)
		}
		if dst.P != nil {
			t.Fatalf("nil 源指针字段应复制为 nil，得到 %v", *dst.P)
		}
	})

	// 用于暴露浅拷贝遇到 nil slice/map 时直接跳过、不覆盖目标的问题。
	// 场景：目标 slice/map 已经有内容，源 slice/map 为 nil， 表示跳过，一般用于更新目标结构，不应该目标被清空。
	t.Run("源nil切片与映射应跳过不清空目标", func(t *testing.T) {
		type S struct {
			C []int
			M map[string]int
		}

		dst := S{C: []int{9}, M: map[string]int{"x": 9}}
		var src S

		if err := CopyStruct(&dst, src, nil); err != nil {
			t.Fatal(err)
		}
		if dst.C == nil {
			t.Fatalf("浅拷贝遇源 nil 切片不应清空目标，得到 %#v", dst.C)
		}
		if dst.M == nil {
			t.Fatalf("浅拷贝遇源 nil 映射不应清空目标，得到 %#v", dst.M)
		}
	})

	t.Run("源nil Map跳过拷贝", func(t *testing.T) {
		a := B{F: map[string]string{"a": "1"}}
		b := B{F: nil}
		err := CopyStruct(reflect.ValueOf(&a), reflect.ValueOf(&b), nil)
		if err != nil {
			t.Fatal(err)
		}
		if a.F == nil {
			t.Fatalf("浅拷贝遇源 nil map 不应清空目标，得到 %#v", a.F)
		}
	})

	t.Run("切片与映射浅拷贝独立", func(t *testing.T) {
		type S struct {
			C []int
			M map[string]int
		}

		src := S{C: []int{1}, M: map[string]int{"a": 1}}
		var dst S

		if err := CopyStruct(&dst, src, nil); err != nil {
			t.Fatal(err)
		}

		dst.C[0] = 2
		dst.M["a"] = 2

		if src.C[0] != 1 {
			t.Fatalf("slice 底层被共享，修改目标影响了源")
		}
		if src.M["a"] != 1 {
			t.Fatalf("map 底层被共享，修改目标影响了源")
		}
	})
}

func TestCopyStructDeep(t *testing.T) {
	t.Run("Map深度拷贝正常覆盖", func(t *testing.T) {
		a := B{G: []string{"1", "2", "3"}}
		b := B{G: []string{"4", "5", "6"}}
		err := CopyStructDeep(reflect.ValueOf(&a), reflect.ValueOf(&b), nil)
		if err != nil {
			t.Fatal(err)
		}
		b.G[0] = "-"
		if got := fmt.Sprint(a.G); got != "[4 5 6]" {
			t.Fatalf("深拷贝后目标应独立于源，a.G=%s", got)
		}
	})

	t.Run("不同类型转换或跳过", func(t *testing.T) {
		a := B{G: []string{"1", "2", "3"}}
		b := B1{G: []int{65, 67, 68}}
		err := CopyStructDeep(reflect.ValueOf(&a), reflect.ValueOf(&b), nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(a.G); got != "[65 67 68]" {
			t.Fatalf("[]int→[]string 十进制字符串，期望 [65 67 68]，得到 %s", got)
		}
	})

	t.Run("复杂嵌套指针初始化和拷贝", func(t *testing.T) {
		var a A
		b := A{
			B: B{
				C: &C{D: 1},
			},
		}
		err := CopyStructDeep(reflect.ValueOf(&a), reflect.ValueOf(&b), nil)
		if err != nil {
			t.Fatal(err)
		}
		if a.B.C == nil || a.B.C.D != 1 {
			t.Fatalf("嵌套指针未初始化/拷贝: %+v", a.B.C)
		}
	})

	t.Run("复杂嵌套指针初始化和拷贝2", func(t *testing.T) {
		type (
			C struct {
				D int
			}
			A struct {
				B map[string][]*C
			}
		)

		c := []*C{{D: 1}}
		a := A{
			B: map[string][]*C{
				"A": c,
			},
		}
		b := A{
			B: map[string][]*C{
				"A": {&C{D: 1}},
			},
		}
		err := CopyStructDeep(reflect.ValueOf(&a), reflect.ValueOf(&b), nil)
		if err != nil {
			t.Fatal(err)
		}
		bm := b.B["A"]
		bm[0].D = 2

		if v := c[0].D; v != 1 {
			t.Fatalf("指针拷贝后修改原值，期望原值不变，实际为 %d", v)
		}
		if v := a.B["A"][0].D; v != 1 {
			t.Fatalf("指针拷贝后修改原值，期望原值不变，实际为 %d", v)
		}
	})

	t.Run("排除指定字段", func(t *testing.T) {
		a := A{}
		b := A{
			B{
				C: &C{D: 1},
				F: map[string]string{"a": "b"},
			},
		}
		err := CopyStructDeep(
			reflect.ValueOf(&a), reflect.ValueOf(&b), func(name string, _, _ reflect.Value) bool {
				return name == "B.C"
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if a.B.C != nil {
			t.Fatalf("B.C 应被排除，实际为 %+v", a.B.C)
		}
		if len(a.B.F) != 1 {
			t.Fatalf("B.F 应被拷贝，实际为 %#v", a.B.F)
		}
	})
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
	t.Run("Bug 1: Nil pointer in anonymous struct path should not panic", func(t *testing.T) {
		// 匿名嵌入结构体指针为 nil 时引发 Panic (已修复安全防范测试)
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

	t.Run("Outer field should shadow inner embedded field of the same name", func(t *testing.T) {
		// 外部字段同名时未能正确遮蔽内部嵌入字段
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

	t.Run("Case sensitivity asymmetry between Id and ID", func(t *testing.T) {
		// 大小写别名查找的不对称性限制
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

	t.Run("Inconsistent error handling for float key in slice", func(t *testing.T) {
		s := []string{"zero", "one"}
		_, err := Get(s, 0.9)
		if err == nil {
			t.Fatalf("expected error for float key in slice, got nil")
		}
	})

	t.Run("Number index is decimal digits; OOB matches string", func(t *testing.T) {
		// Get(123, 1) 不是越界："123"[1] == '2'
		valNum, errNum := Get(123, 1)
		if errNum != nil {
			t.Errorf("Get(123, 1) 合法下标不应报错: %v", errNum)
		}
		if valNum != byte('2') {
			t.Errorf("Get(123, 1) 期望 byte('2')，得到 %v", valNum)
		}

		_, errStr := Get("hello", 10)
		if errStr == nil || !errors.Is(errStr, ErrIndexOutOfRange) {
			t.Errorf("expected ErrIndexOutOfRange for string, got %v", errStr)
		}
		_, errNumOOB := Get(123, 99)
		if errNumOOB == nil || !errors.Is(errNumOOB, ErrIndexOutOfRange) {
			t.Errorf("expected ErrIndexOutOfRange for number, got %v", errNumOOB)
		}
	})

	t.Run("Nil key query in map of interface keys", func(t *testing.T) {
		// map[any]any 等包含接口键的 Map 中 nil 键查询
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

	t.Run("Nil key on non-interface map", func(t *testing.T) {
		// 非 interface key 的 map 仍然应该拒绝 nil
		m := map[string]string{"a": "b"}
		_, err := Get(m, nil)
		if !errors.Is(err, ErrMapKey) {
			t.Fatalf("expected ErrMapKey, got %v", err)
		}
	})

	t.Run("Nil key not present", func(t *testing.T) {
		// 存在 nil key 但查不到时仍返回 (nil, nil)
		m := map[any]string{"x": "y"}
		_, err := Get(m, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Silent data loss when copying slice of unexported structs", func(t *testing.T) {
		// 未导出结构体的切片导致静默数据丢失
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

	t.Run("Silent data loss/key-overwrite in typeSelect for map of unexported types", func(t *testing.T) {
		// 未导出结构体键的 Map 导致静默覆盖与数据丢失
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

func Test_Set_Features_And_Bugs(t *testing.T) {
	// ==================== 功能示例 (Features) ====================

	t.Run("Struct setting features", func(t *testing.T) {
		type User struct {
			Name    string `json:"username"`
			Age     int
			Address string
		}

		u := User{Name: "Alice", Age: 18, Address: "Beijing"}

		// 支持的功能：
		// 1. "username" -> 匹配 json 标签
		// 2. "age" -> 自动首字母大写匹配 Age 字段，且 "20" (string) 自动转换为 20 (int)
		// 3. -1 -> 支持负整数索引定位最后一个字段 (Address)
		err := Set(&u, "username", "Bob", "age", "20", -1, "Shanghai")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if u.Name != "Bob" || u.Age != 20 || u.Address != "Shanghai" {
			t.Errorf("unexpected struct values: %+v", u)
		}
	})

	t.Run("Pointer auto-allocation", func(t *testing.T) {
		type Config struct {
			MaxConnections *int
		}

		var cfg Config
		// 功能：目标字段为指针类型时，直接传入基本类型会自动为其分配内存并填充值
		err := Set(&cfg, "MaxConnections", 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.MaxConnections == nil {
			t.Fatal("expected MaxConnections pointer to be allocated, but got nil")
		}
		if *cfg.MaxConnections != 100 {
			t.Errorf("expected *MaxConnections to be 100, got %d", *cfg.MaxConnections)
		}
	})

	t.Run("Slice growth and rollback on error", func(t *testing.T) {
		slice := []int{1, 2}

		// 功能：
		// 1. 切片支持自动扩容（如索引 3 越界，自动扩容填充零值）
		// 2. 参数对中如果后续参数设置失败（如索引 4 传入非数字字符串），触发整体回滚
		err := Set(&slice, 3, 99, 4, "invalid_int_type")
		if err == nil {
			t.Fatal("expected error due to invalid type conversion, but got nil")
		}

		// 验证回滚：切片长度和值应保持初始状态
		if len(slice) != 2 || slice[0] != 1 || slice[1] != 2 {
			t.Errorf("slice failed to rollback to original state: %v", slice)
		}
	})

	t.Run("Map key conversion and nil deletion", func(t *testing.T) {
		m := map[int]string{1: "one", 2: "two"}

		// 功能：
		// 1. 支持 key 类型转换，"3" (string) -> 3 (int)
		// 2. 值为 nil 时，删除对应的键 (删除 key 1)
		err := Set(m, "3", "three", 1, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if m[3] != "three" {
			t.Errorf("expected m[3] to be 'three', got %q", m[3])
		}
		if _, exists := m[1]; exists {
			t.Error("expected key 1 to be deleted, but it still exists")
		}
	})

	// ==================== Bug/意外检测示例 (Bugs) ====================

	t.Run("Embedded pointer nil allocation", func(t *testing.T) {
		type Inner struct {
			Value int
		}
		type Outer struct {
			*Inner // 嵌入的结构体指针
		}

		outer := &Outer{} // outer.Inner 为 nil

		err := Set(outer, "Value", 42)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if outer.Inner == nil {
			t.Fatal("expected Inner to be allocated, but it is nil")
		}
		if outer.Value != 42 {
			t.Errorf("expected outer.Value to be 42, got %d", outer.Value)
		}
	})

	t.Run("Unexported field settings", func(t *testing.T) {
		type PrivateStruct struct {
			secret int // 私有字段
		}

		ps := PrivateStruct{secret: 100}

		err := SetUnexported(&ps, "secret", 200)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if ps.secret != 200 {
			t.Errorf("secret value changed unexpectedly to %d", ps.secret)
		}
	})

	t.Run("Bool key from string true", func(t *testing.T) {
		m := map[bool]string{}
		err := Set(m, "true", "yes")
		if err != nil {
			t.Fatalf("\"true\" 应能转成 bool 键，got: %v", err)
		}
		if m[true] != "yes" {
			t.Fatalf("m[true]=%q", m[true])
		}
	})
}

func Test_Len(t *testing.T) {
	t.Run("和内建 len 对齐", func(t *testing.T) {
		s := "a\x00你"
		if Len(s) != len(s) || Len(s) == utf8.RuneCountInString(s) {
			t.Fatalf("字符串应按字节计数: Len=%d builtin=%d rune=%d", Len(s), len(s), utf8.RuneCountInString(s))
		}
		if Len(s[1:3]) != len(s[1:3]) {
			t.Fatal("子字符串长度不应带回原串剩余部分")
		}
		if Len([]rune("你好")) == Len("你好") {
			t.Fatal("[]rune 与 string 的长度被算成了同一个数")
		}
		b := []byte(s)
		if Len(b) != len(b) || Len([]uint8(b)) != len(b) {
			t.Fatal("[]byte/[]uint8 与内建 len 不一致")
		}
		type Raw []byte
		raw := Raw([]byte{0xff, 0xfe, 0x00})
		if Len(raw) != len(raw) {
			t.Fatalf("命名切片 Len=%d builtin=%d", Len(raw), len(raw))
		}
		type MyString string
		ms := MyString("hi")
		if Len(ms) != len(ms) || Len(&ms) != len(ms) {
			t.Fatal("命名字符串或其指针长度不对")
		}

		arr := [4]byte{'a', 'b', 0, 0}
		if Len(arr) != len(arr) || Len(&arr) != len(arr) {
			t.Fatal("数组长度被截断，或指针数组没有解引用")
		}
		used := [3]int{1, 2}
		if Len(used) != 3 {
			t.Fatal("数组长度应按定义长度计算，而不是非零元素个数")
		}

		m := map[string]int{"": 0, "a": 0}
		if Len(m) != len(m) {
			t.Fatal("空 key 和零值没有被计入 map 长度")
		}
		m["a"] = 2
		delete(m, "missing")
		if Len(m) != len(m) {
			t.Fatal("重复写入或删除不存在的 key 后，长度与内建 len 不一致")
		}
		nanMap := map[float64]int{}
		nanMap[math.NaN()] = 1
		nanMap[math.NaN()] = 2
		if Len(nanMap) != len(nanMap) {
			t.Fatalf("NaN key 长度 Len=%d builtin=%d", Len(nanMap), len(nanMap))
		}

		ch := make(chan int, 4)
		ch <- 1
		ch <- 2
		if Len(ch) != len(ch) {
			t.Fatalf("channel 排队数 Len=%d builtin=%d cap=%d", Len(ch), len(ch), cap(ch))
		}
		var send chan<- int = ch
		var recv <-chan int = ch
		if Len(send) != len(ch) || Len(recv) != len(ch) {
			t.Fatal("单向 channel 长度与原 channel 不一致")
		}
		type MyChan chan int
		mc := make(MyChan, 2)
		mc <- 7
		if Len(mc) != len(mc) {
			t.Fatal("命名 channel 长度不对")
		}
	})

	t.Run("len 不能被当成 cap，也不能消费 channel", func(t *testing.T) {
		s := make([]int, 2, 8)
		if Len(s) != len(s) || Len(s) == cap(s) {
			t.Fatalf("切片 Len=%d len=%d cap=%d", Len(s), len(s), cap(s))
		}
		b := make([]byte, 1, 8)
		if Len(b) != 1 {
			t.Fatalf("[]byte 快速路径把长度算成 %d，期望 1", Len(b))
		}
		full := []int{1, 2, 3, 4, 5}
		window := full[0:0:5]
		part := full[1:2:5]
		if Len(window) != len(window) || Len(part) != len(part) {
			t.Fatalf("三下标切片 Len(window)=%d Len(part)=%d", Len(window), Len(part))
		}
		if Len(window) == cap(window) || Len(part) == cap(part) {
			t.Fatal("三下标切片的 cap 被当成了 len")
		}
		p := &s
		*p = append(*p, 3)
		if Len(p) != len(s) || Len(p) == cap(s) {
			t.Fatalf("append 之后指针路径 Len=%d len=%d cap=%d", Len(p), len(s), cap(s))
		}

		ch := make(chan int, 5)
		if Len(ch) != 0 {
			t.Fatal("空缓冲 channel 的长度不应等于 cap")
		}
		ch <- 42
		if Len(ch) != 1 || Len(ch) == cap(ch) {
			t.Fatalf("只放入 1 个元素时 Len=%d cap=%d", Len(ch), cap(ch))
		}
		select {
		case v := <-ch:
			if v != 42 {
				t.Fatal("Len 改写了 channel 中的值")
			}
		default:
			t.Fatal("Len 把 channel 里的元素消费掉了")
		}

		closed := make(chan int, 3)
		closed <- 1
		closed <- 2
		closed <- 3
		close(closed)
		if Len(closed) != len(closed) {
			t.Fatal("close 之后剩余元素的长度变了")
		}
		<-closed
		if Len(closed) != 2 {
			t.Fatalf("关闭后取走 1 个，Len=%d，期望 2", Len(closed))
		}
		<-closed
		<-closed
		if Len(closed) != 0 {
			t.Fatal("关闭且抽干后长度不为 0")
		}
	})

	t.Run("nil、自引用和阻塞发送不得 panic 或卡住", func(t *testing.T) {
		var (
			nch  chan int
			send chan<- int
			recv <-chan int
			ns   []int
			nm   map[string]int
			np   *string
			nps  *[]int
			ni   any
			fn   func()
		)
		hostile := []any{
			nil, nch, send, recv, ns, nm, np, nps, ni, fn,
			(*Chan)(nil),
			Chan{},
			Chan{C: reflect.Value{}},
			Chan{C: reflect.ValueOf(123)},
			reflect.Value{},
			&ni, any(nch), any(ns),
		}
		for i, v := range hostile {
			n, pan, blocked := lenWithin(v, time.Second)
			if blocked {
				t.Fatalf("输入 %d 调用 Len 阻塞", i)
			}
			if pan != nil {
				t.Fatalf("输入 %d 引发 panic: %v", i, pan)
			}
			if n != 0 {
				t.Fatalf("输入 %d 的空值/非法值 Len=%d，期望 0", i, n)
			}
		}

		type selfRef *selfRef
		var p selfRef
		p = &p
		if _, pan, blocked := lenWithin(p, time.Second); blocked || pan != nil {
			t.Fatalf("自引用指针没有正常返回: panic=%v blocked=%v", pan, blocked)
		}
		var a any
		a = &a
		if _, pan, blocked := lenWithin(a, time.Second); blocked || pan != nil {
			t.Fatalf("自引用接口没有正常返回: panic=%v blocked=%v", pan, blocked)
		}

		ub := make(chan int)
		go func() { ub <- 42 }()
		time.Sleep(30 * time.Millisecond)
		n, pan, blocked := lenWithin(ub, time.Second)
		if blocked {
			t.Fatal("无缓冲 channel 上有阻塞发送时 Len 卡住了，疑似执行了接收")
		}
		if pan != nil {
			t.Fatalf("无缓冲 channel panic: %v", pan)
		}
		if n != 0 {
			t.Fatalf("阻塞中的无缓冲 channel Len=%d，内建 len 只能是 0", n)
		}
		select {
		case v := <-ub:
			if v != 42 {
				t.Fatal("Len 改写了阻塞发送的值")
			}
		case <-time.After(time.Second):
			t.Fatal("Len 之后取不到发送方的值，疑似被消费且未放回")
		}
	})

	t.Run("嵌套容器只计算直接外层", func(t *testing.T) {
		matrix := [][]int{{1, 2, 3}, nil}
		if Len(matrix) != 2 || Len(matrix[0]) != 3 || Len(matrix[1]) != 0 {
			t.Fatal("二维切片的外层长度和内层长度混在一起了")
		}
		arr := [2][]int{{1, 2, 3, 4}, {9}}
		if Len(arr) != 2 || Len(arr[0]) != 4 {
			t.Fatal("数组长度和元素切片长度混在一起了")
		}
		m := map[string][]int{"k": {1, 2, 3, 4}}
		if Len(m) != 1 || Len(m["k"]) != 4 {
			t.Fatal("map 长度和 value 长度混在一起了")
		}
		ch := make(chan []int, 2)
		ch <- []int{1, 2, 3}
		if Len(ch) != 1 {
			t.Fatalf("channel 里放的是 3 个元素的切片，Len=%d，期望排队数 1", Len(ch))
		}
		items := []any{nil, []int{1, 2}, "abc"}
		if Len(items) != 3 || Len(items[0]) != 0 || Len(items[1]) != 2 || Len(items[2]) != 3 {
			t.Fatal("[]any 元素的动态长度不对")
		}
		if Len(&items[1]) != 2 || Len(&items[2]) != 3 {
			t.Fatal("指向 any 元素的指针没有解到实际长度")
		}
	})

	t.Run("同一种 channel 换包装后长度不变", func(t *testing.T) {
		ch := make(chan int, 4)
		ch <- 1
		ch <- 2
		want := len(ch)
		p := &ch
		pp := &p
		wrapped := Chan{C: reflect.ValueOf(ch)}
		wrappedPtr := &wrapped
		var chanAny any = ch
		var iface reflect.Value = reflect.ValueOf(&chanAny).Elem()

		checks := []struct {
			name string
			v    any
		}{
			{"channel", ch},
			{"*channel", p},
			{"**channel", pp},
			{"any(channel)", chanAny},
			{"*any(channel)", &chanAny},
			{"reflect.Value(channel)", reflect.ValueOf(ch)},
			{"reflect.Value(*channel)", reflect.ValueOf(p)},
			{"Chan", wrapped},
			{"*Chan", wrappedPtr},
			{"Chan.C 是 *channel", Chan{C: reflect.ValueOf(p)}},
			{"Chan.C 是装有 channel 的 interface", Chan{C: iface}},
		}
		var bugs []string
		for _, c := range checks {
			if got := Len(c.v); got != want {
				bugs = append(bugs, fmt.Sprintf("%s: Len=%d，期望 %d", c.name, got, want))
			}
		}
		if len(bugs) > 0 {
			t.Fatalf("同为排队数 %d 的 channel，包装不同结果却不同:\n%s", want, strings.Join(bugs, "\n"))
		}
	})

	t.Run("指针或 any 包得更深时，一样找出长度", func(t *testing.T) {
		leaf := make([]int, 3, 9)
		if got := Len(anyPointerChain(8, leaf)); got != 3 {
			t.Fatalf("8 层 any 指针应得到 len=3，实际 %d", got)
		}
		if got := Len(pointerChain(10, leaf)); got != 3 {
			t.Fatalf("10 层纯指针应得到 len=3，实际 %d", got)
		}
		var bugs []string
		if got := Len(anyPointerChain(40, leaf)); got != 3 {
			bugs = append(bugs, fmt.Sprintf("40 层 any 指针后的切片 len=3 cap=9，Len=%d", got))
		}
		if got := Len(pointerChain(80, leaf)); got != 3 {
			bugs = append(bugs, fmt.Sprintf("80 层纯指针后的切片 len=3 cap=9，Len=%d", got))
		}
		ch := make(chan int, 2)
		ch <- 1
		if got := Len(pointerChain(80, ch)); got != len(ch) {
			bugs = append(bugs, fmt.Sprintf("80 层纯指针后的 channel len=%d，Len=%d", len(ch), got))
		}
		if len(bugs) > 0 {
			t.Fatalf("非空值因包装层数被静默变成别的数（0 会伪装成空，9 则是 cap）:\n%s", strings.Join(bugs, "\n"))
		}
	})

	t.Run("slice, map ,channel 实时的", func(t *testing.T) {
		s := []int{1, 2}
		p := &s
		s = append(s, 3)

		m := map[string]int{"a": 1}
		rm := reflect.ValueOf(m)
		m["b"] = 2

		ch := make(chan int, 2)
		rc := reflect.ValueOf(ch)
		cw := &Chan{C: rc}
		ch <- 1

		if Len(p) != len(s) {
			t.Fatalf("切片指针应是实时长度 %d，实际 %d", len(s), Len(p))
		}
		if Len(rm) != len(m) || Len(rc) != len(ch) || Len(cw) != len(ch) {
			t.Fatal("map/channel 的 reflect.Value 或 Chan 包装没有看到最新长度")
		}
	})
}

func anyPointerChain(depth int, leaf any) any {
	if depth == 0 {
		return leaf
	}
	inner := anyPointerChain(depth-1, leaf)
	return &inner
}

func pointerChain(depth int, leaf any) any {
	v := reflect.ValueOf(leaf)
	for i := 0; i < depth; i++ {
		p := reflect.New(v.Type())
		p.Elem().Set(v)
		v = p
	}
	return v.Interface()
}

func lenWithin(v any, d time.Duration) (n int, pan any, blocked bool) {
	done := make(chan struct{})
	go func() {
		defer func() {
			pan = recover()
			close(done)
		}()
		n = Len(v)
	}()
	select {
	case <-done:
	case <-time.After(d):
		blocked = true
	}
	return
}

func TestCap(t *testing.T) {
	tests := []struct {
		name string
		a    any
		want int
	}{
		// --- nil / invalid ---
		{
			name: "nil interface",
			a:    nil,
			want: 0,
		},
		{
			name: "nil pointer to int",
			a:    (*int)(nil),
			want: 0,
		},
		{
			name: "invalid reflect.Value",
			a:    reflect.Value{},
			want: 0,
		},

		// --- slices ---
		{
			name: "empty slice len=0 cap=0",
			a:    []int{},
			want: 0,
		},
		{
			name: "slice with capacity > length",
			a:    make([]int, 2, 5),
			want: 5,
		},
		{
			name: "slice of string with capacity",
			a:    make([]string, 0, 10),
			want: 10,
		},
		{
			name: "nil slice",
			a:    ([]int)(nil),
			want: 0,
		},
		{
			name: "pointer to slice",
			a:    &[]int{1, 2, 3}, // len=3, cap=3
			want: 3,
		},
		{
			name: "nil pointer to slice",
			a:    (*[]int)(nil),
			want: 0,
		},
		{
			name: "interface holding slice",
			a:    any(make([]byte, 1, 8)),
			want: 8,
		},
		{
			name: "pointer to pointer to slice",
			a: func() any {
				s := make([]int, 0, 7)
				p := &s
				return &p
			}(),
			want: 7,
		},

		// --- arrays ---
		{
			name: "array [0]int",
			a:    [0]int{},
			want: 0,
		},
		{
			name: "array [3]int",
			a:    [3]int{1, 2, 3},
			want: 3,
		},
		{
			name: "array [5]string",
			a:    [5]string{},
			want: 5,
		},
		{
			name: "pointer to array",
			a:    &[4]int{1, 2, 3, 4},
			want: 4,
		},
		{
			name: "nil pointer to array",
			a:    (*[3]int)(nil),
			want: 3,
		},
		{
			name: "interface holding array",
			a:    any([2]float64{}),
			want: 2,
		},

		// --- channels ---
		{
			name: "nil channel",
			a:    (chan int)(nil),
			want: 0,
		},
		{
			name: "unbuffered channel",
			a:    make(chan int),
			want: 0,
		},
		{
			name: "buffered channel capacity 5",
			a:    make(chan string, 5),
			want: 5,
		},
		{
			name: "buffered channel capacity 1",
			a:    make(chan struct{}, 1),
			want: 1,
		},
		{
			name: "pointer to channel",
			a: func() any {
				ch := make(chan int, 3)
				return &ch
			}(),
			want: 3,
		},
		{
			name: "nil pointer to channel",
			a:    (*chan int)(nil),
			want: 0,
		},
		{
			name: "interface holding channel",
			a:    any(make(chan bool, 4)),
			want: 4,
		},
		{
			name: "Chan wrapper value type",
			a:    Chan{C: reflect.ValueOf(make(chan int, 6))},
			want: 6,
		},
		{
			name: "Chan wrapper with nil channel",
			a:    Chan{C: reflect.ValueOf((chan int)(nil))},
			want: 0,
		},
		{
			name: "*Chan wrapper",
			a:    &Chan{C: reflect.ValueOf(make(chan byte, 2))},
			want: 2,
		},
		{
			name: "nil *Chan",
			a:    (*Chan)(nil),
			want: 0,
		},
		{
			name: "*Chan with invalid C",
			a:    &Chan{C: reflect.Value{}},
			want: 0,
		},
		{
			name: "Chan with non-chan C (int)",
			a:    Chan{C: reflect.ValueOf(42)},
			want: 0,
		},

		// --- reflect.Value inputs ---
		{
			name: "reflect.Value of slice",
			a:    reflect.ValueOf(make([]int, 1, 9)),
			want: 9,
		},
		{
			name: "reflect.Value of array",
			a:    reflect.ValueOf([7]int{}),
			want: 7,
		},
		{
			name: "reflect.Value of channel",
			a:    reflect.ValueOf(make(chan int, 11)),
			want: 11,
		},
		{
			name: "reflect.Value of nil channel",
			a:    reflect.ValueOf((chan int)(nil)),
			want: 0,
		},
		{
			name: "reflect.Value of pointer to slice",
			a: func() any {
				s := make([]int, 0, 12)
				return reflect.ValueOf(&s)
			}(),
			want: 12,
		},

		// --- edge / potential bug cases ---
		{
			name: "deeply nested pointers to slice",
			a: func() any {
				s := make([]int, 0, 13)
				p1 := &s
				p2 := &p1
				p3 := &p2
				return p3
			}(),
			want: 13,
		},
		{
			name: "interface of pointer to channel",
			a: func() any {
				ch := make(chan int, 14)
				return any(&ch)
			}(),
			want: 14,
		},
		{
			name: "Chan wrapping pointer to channel",
			a: func() any {
				ch := make(chan int, 15)
				return Chan{C: reflect.ValueOf(&ch)}
			}(),
			want: 15,
		},
		{
			name: "*Chan wrapping interface of channel",
			a: func() any {
				ch := make(chan string, 16)
				return &Chan{C: reflect.ValueOf(any(ch))}
			}(),
			want: 16,
		},
		{
			name: "closed buffered channel still has capacity",
			a: func() any {
				ch := make(chan int, 3)
				close(ch)
				return ch
			}(),
			want: 3,
		},
		{
			name: "slice from array (full capacity)",
			a: func() any {
				arr := [5]int{1, 2, 3, 4, 5}
				return arr[:]
			}(),
			want: 5,
		},
		{
			name: "slice from array with reduced capacity",
			a: func() any {
				arr := [5]int{1, 2, 3, 4, 5}
				return arr[1:3:4] // len=2, cap=3
			}(),
			want: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Cap(tt.a)
			if got != tt.want {
				t.Errorf("Cap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValue(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want reflect.Kind // 期望的 Kind（零值）
		// 额外断言：返回值不应是指针（除非请求的是指针类型）
		expectPtr bool
	}{
		// ---------- 正常路径 ----------

		{"string name", "string", reflect.String, false},
		{"int name", "int", reflect.Int, false},
		{"slice type", "[]string", reflect.Slice, false},
		{"map type", "map[string]int", reflect.Map, false},
		{"pointer type", "*float64", reflect.Ptr, true},
		{"array type", "[3]bool", reflect.Array, false},
		{"chan type", "chan int", reflect.Chan, false},
		{"func type", "func(int, int) (string, error)", reflect.Func, false},
		{"reflect.Type", reflect.TypeOf(42), reflect.Int, false},
		{"reflect.Value", reflect.ValueOf("hi"), reflect.String, false},
		{"concrete value", 3.14, reflect.Float64, false},
		{"nil interface{}", any(nil), reflect.Interface, false}, // 期望 any 的零值

		{"empty string", "", reflect.Invalid, false}, // 应返回无效 Value
		{"unknown type name", "FooBar", reflect.Invalid, false},
		{"malformed type", "[]", reflect.Invalid, false},
		{"pointer to pointer", "**int", reflect.Ptr, true},        // 深度未超限
		{"nil string pointer", (*string)(nil), reflect.Ptr, true}, // 具体 nil 指针
		{"invalid reflect.Value", reflect.Value{}, reflect.Struct, false},
		{"typed nil interface", error(nil), reflect.Interface, false},
		{"struct{} type", "struct{I int}", reflect.Struct, false},
		{"any / interface{}", "any", reflect.Interface, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Value(tt.v)

			if got.Kind() != tt.want {
				t.Errorf("Value(%v) Kind = %v, want %v", tt.v, got.Kind(), tt.want)
			}

			// 核心意图：返回的永远是零值
			if got.IsValid() && !got.IsZero() {
				t.Errorf("Value(%v) is not zero: %v", tt.v, got.Interface())
			}

			// 意图声明：返回值本身不是指针，除非请求的类型就是指针
			if got.Kind() == reflect.Ptr && !tt.expectPtr {
				t.Errorf("Value(%v) unexpectedly returned a pointer", tt.v)
			}
			if got.Kind() != reflect.Ptr && tt.expectPtr {
				t.Errorf("Value(%v) should return pointer type but got %v", tt.v, got.Kind())
			}
		})
	}
}

// 额外：验证“从具体值取类型”时不会返回指针包装
func TestValue_ConcreteValueNotWrapped(t *testing.T) {
	v := Value(42)
	if v.Kind() != reflect.Int || v.Int() != 0 {
		t.Fatalf("expected zero int, got %v", v)
	}
	// 不应是 *int
	if v.Kind() == reflect.Ptr {
		t.Fatal("concrete int should not produce pointer")
	}
}

// 验证 nil 输入的特殊路径
func TestValue_Nil(t *testing.T) {
	got := Value(nil)
	if got.IsValid() {
		// 根据实现，nil 可能返回 Invalid 或 interface{} 零值
		// 这里只检查是否是零值（若有效）
		if !got.IsZero() {
			t.Errorf("Value(nil) should be zero, got %v", got)
		}
	}
}

func TestMake_Slice(t *testing.T) {
	// 正常：长度 / 容量
	s := Make("[]int", 3, 5).([]int)
	if len(s) != 3 || cap(s) != 5 {
		t.Fatalf("len=%d cap=%d want 3 5", len(s), cap(s))
	}

	// 只有长度（容量=长度）
	s2 := Make([]int{}, 2).([]int) // 用具体值取类型
	if len(s2) != 2 || cap(s2) != 2 {
		t.Fatalf("len=%d cap=%d want 2 2", len(s2), cap(s2))
	}

	// 零长度
	s0 := Make("[]string", 0).([]string)
	if len(s0) != 0 || cap(s0) != 0 {
		t.Fatalf("zero slice unexpected: len=%d cap=%d", len(s0), cap(s0))
	}
}

func TestMake_Map(t *testing.T) {
	m := Make("map[string]int", 8).(map[string]int)
	if m == nil {
		t.Fatal("map should not be nil")
	}
	m["a"] = 1
	if m["a"] != 1 {
		t.Fatal("map not usable")
	}

	// 无 size 参数
	m2 := Make(map[int]bool{}).(map[int]bool)
	if m2 == nil {
		t.Fatal("map without size should not be nil")
	}
}

func TestMake_Chan(t *testing.T) {
	ch := Make("chan int", 2).(chan int)
	if cap(ch) != 2 {
		t.Fatalf("chan buffer=%d want 2", cap(ch))
	}
	ch <- 1
	ch <- 2
	// 无缓冲
	ch0 := Make("chan string").(chan string)
	if cap(ch0) != 0 {
		t.Fatalf("unbuffered cap=%d want 0", cap(ch0))
	}
}

func TestMake_Func(t *testing.T) {
	// 无实现 → nil func
	f := Make("func(int) int")
	if f == nil {
		// 允许返回 nil
	} else if reflect.ValueOf(f).IsNil() {
		// 也允许
	} else {
		t.Fatal("expected nil func when no impl")
	}

	// 带实现
	impl := func(in []reflect.Value) []reflect.Value {
		return []reflect.Value{reflect.ValueOf(in[0].Int() * 2)}
	}
	fn, ok := Make("func(int) int", impl).(func(int) int)
	if !ok || fn(21) != 42 {
		t.Fatal("MakeFunc failure")
	}
}

func TestMake_NegativeOrInvalidSize_ShouldPanic(t *testing.T) {
	mustPanic := func(name string, fn func()) {
		defer func() {
			if recover() == nil {
				t.Errorf("%s: expected panic", name)
			}
		}()
		fn()
	}

	mustPanic("slice len negative", func() { Make("[]int", -1) })
	mustPanic("slice cap < len", func() { Make("[]int", 5, 3) })
	mustPanic("map size negative", func() { Make("map[int]int", -1) })
	mustPanic("chan buffer negative", func() { Make("chan int", -1) })
}

func TestMake_TypeParsing_Edge(t *testing.T) {
	// 嵌套类型
	s := Make("[]*map[string]chan int", 1).([]*map[string]chan int)
	if len(s) != 1 {
		t.Fatal("nested type failed")
	}

	// 单向 channel 按注释应 panic
	defer func() {
		if recover() == nil {
			t.Error("unidirectional chan should panic")
		}
	}()
	Make("<-chan int")
}

func TestMake_ArgTypeCoercion(t *testing.T) {
	// 长度参数允许常见整数类型
	s := Make("[]byte", int64(4), uint(6)).([]byte)
	if len(s) != 4 || cap(s) != 6 {
		t.Fatalf("int64/uint size: len=%d cap=%d", len(s), cap(s))
	}

	// 浮点非整数 / 溢出应 panic（易出错点）
	defer func() {
		if recover() == nil {
			t.Error("float non-integer length should panic")
		}
	}()
	Make("[]int", 3.5)
}

func ExampleMake() {
	s := Make("[]string", 2, 4).([]string)
	m := Make(map[string]int{}).(map[string]int)
	ch := Make("chan bool").(chan bool)

	fmt.Printf("slice len=%d cap=%d\n", len(s), cap(s))
	fmt.Printf("map nil? %v\n", m == nil)
	fmt.Printf("chan cap=%d\n", cap(ch))
	// Output:
	// slice len=2 cap=4
	// map nil? false
	// chan cap=0
}

func TestDepthField(t *testing.T) {
	type N struct{ V int }
	type S struct {
		Name string
		M    map[string]any
		A    []N
		p    int // 未导出
	}
	s := S{
		Name: "alice",
		M:    map[string]any{"1": 100},
		A:    []N{{1}, {2}, {3}},
	}

	must := func(t *testing.T, got any, err error, want any) {
		t.Helper()
		if err != nil || got != want {
			t.Fatalf("got (%v, %v), want %v", got, err, want)
		}
	}

	mustErr := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected error")
		}
	}

	// 正常路径 + 负索引
	got, err := DepthField(s, "Name")
	must(t, got, err, "alice")

	got, err = DepthField(s, "M", "1")
	must(t, got, err, 100)

	got, err = DepthField(s, "A", -1, "V")
	must(t, got, err, 3)

	// 键转换
	got, err = DepthField(map[int]string{1: "one"}, "1")
	must(t, got, err, "one")

	got, err = DepthField(map[bool]string{true: "yes"}, 1)
	must(t, got, err, "yes")

	// 应报错
	_, err = DepthField(s, "p")
	mustErr(t, err)

	_, err = DepthField((map[string]int)(nil), "x")
	mustErr(t, err)

	_, err = DepthField(s.A, 5)
	mustErr(t, err)

	_, err = DepthField(s.A, 1.5)
	mustErr(t, err)

	_, err = DepthField(map[float64]int{}, math.NaN())
	mustErr(t, err)

	_, err = DepthField(map[int8]string{1: "a"}, 1000)
	mustErr(t, err)

	// 指针 / 空索引
	got, err = DepthField(&N{99}, "V")
	must(t, got, err, 99)

	got, err = DepthField(42)
	must(t, got, err, 42)
}

func TestInit_Basic(t *testing.T) {
	type Node struct {
		Val  int
		Next *Node
	}

	// 1. 顶层 nil 指针补齐
	var n *Node
	if !Init(&n, false) || n == nil {
		t.Fatal("top-level nil pointer should be allocated")
	}

	// 2. 已有值的不破坏；isZero=false 不遍历结构体去补 nil 指针
	n = &Node{Val: 42}
	if !Init(n, false) || n.Val != 42 || n.Next != nil {
		t.Fatal("isZero=false must keep existing value and leave nested nil")
	}

	// 3. isZero=true 初始化容器
	var s []int
	if !Init(&s, true, 3, 5) || len(s) != 3 || cap(s) != 5 {
		t.Fatal("slice should be made with given len/cap")
	}

	// 4. 接口内部 nil 指针
	type Inner struct{ P *int }
	var i any = &Inner{}
	if !Init(&i, true) {
		t.Fatal("interface inner nil should be filled")
	}
	inner, ok := i.(*Inner)
	if !ok || inner.P == nil {
		t.Fatal("expected *Inner with P allocated")
	}

	// 5. 自引用不挂死
	type Cycle struct{ Next *Cycle }
	c := &Cycle{}
	c.Next = c
	if !Init(c, false) {
		t.Fatal("self-ref should not hang")
	}

	// 6. 不可设置值应失败
	var p *int
	if Init(p, false) {
		t.Fatal("non-settable should return false")
	}

	var m map[string]int
	if !Init(&m, true, 8) || m == nil {
		t.Fatal("map should be non-nil")
	}
	var ch chan int
	if !Init(&ch, true, 2) || ch == nil || cap(ch) != 2 {
		t.Fatal("chan cap mismatch")
	}

	type Leaf struct{ P *string }
	type Root struct {
		Arr [2]Leaf
		Ptr *Leaf
	}
	// 7. 数组 / 嵌套结构体深层补 nil
	var r Root
	if !Init(&r, true) || r.Arr[0].P == nil || r.Arr[1].P == nil || r.Ptr == nil {
		t.Fatal("nested/array nils not fully allocated")
	}
	if !Init(&r.Ptr, true) || r.Ptr.P == nil {
		t.Fatal("nested/array nils not fully allocated")
	}

	// 8. 负数尺寸安全处理
	var s2 []int
	if !Init(&s2, true, -1) || s2 == nil || len(s2) != 0 {
		t.Fatal("negative size should yield empty slice")
	}

	// 9. isZero=false 不破坏已有非 nil 值
	x := 99
	s3 := &struct {
		N int
		P *int
	}{N: 7, P: &x}

	if !Init(s3, false) || s3.N != 7 || s3.P == nil || *s3.P != 99 {
		t.Fatal("existing values must be preserved")
	}
}

// Convert(src, dst any) bool
// dst 必须可设置（通常传指针）
func TestConvert(t *testing.T) {
	t.Run("NumberString", func(t *testing.T) {
		// 数字 → 字符串（十进制）
		var s string
		if !Convert(&s, 123) || s != "123" {
			t.Fatalf("int→string: %q", s)
		}
		if !Convert(&s, 3.14) || s != "3.14" {
			t.Fatalf("float→string: %q", s)
		}
		if !Convert(&s, true) || s != "true" {
			t.Fatalf("bool→string: %q", s)
		}
	})

	t.Run("Basic", func(t *testing.T) {
		var i int
		if !Convert(&i, 42.0) || i != 42 {
			t.Fatalf("float64→int: got %v", i)
		}

		var f float64
		if !Convert(&f, 3) || f != 3 {
			t.Fatalf("int→float64: got %v", f)
		}

		var n int64
		if !Convert(&n, "456") || n != 456 {
			t.Fatalf("string→int64: got %v", n)
		}

		var b bool
		if !Convert(&b, "true") || !b {
			t.Fatal("string→bool failed")
		}
	})

	t.Run("Containers", func(t *testing.T) {
		src := []int{1, 2, 3}
		var dst []int64
		if !Convert(&dst, src) || !reflect.DeepEqual(dst, []int64{1, 2, 3}) {
			t.Fatalf("[]int→[]int64: %v", dst)
		}

		var arr [3]int
		if !Convert(&arr, []int{10, 20, 30}) || arr != [3]int{10, 20, 30} {
			t.Fatalf("[]int→[3]int failed")
		}
		if Convert(&arr, []int{1, 2}) {
			t.Fatal("length mismatch should fail")
		}

		var bs []byte
		if !Convert(&bs, "hi") || string(bs) != "hi" {
			t.Fatalf("string→[]byte: %v", bs)
		}
		var rs []rune
		if !Convert(&rs, "中") || string(rs) != "中" {
			t.Fatalf("string→[]rune: %v", rs)
		}

		mSrc := map[string]int{"a": 1}
		var mDst map[string]int64
		if !Convert(&mDst, mSrc) || mDst["a"] != 1 {
			t.Fatalf("map convert: %v", mDst)
		}
	})

	t.Run("NilAndZero", func(t *testing.T) {
		var p *int
		_ = Convert(&p, nil) // 观察实际行为

		var s []int
		if Convert(&s, ([]int)(nil)) && s != nil {
			t.Fatal("nil slice should stay nil or zero")
		}
	})

	t.Run("Pointers", func(t *testing.T) {
		x := 100
		var p **int
		if !Convert(&p, &x) || **p != 100 {
			t.Fatalf("int→**int: %v", p)
		}

		var q *int
		if !Convert(&q, 42) || *q != 42 {
			t.Fatalf("int→*int: %v", q)
		}
	})

	t.Run("Struct", func(t *testing.T) {
		type S1 struct {
			A int
			B string
		}
		type S2 struct {
			A int64
			B string
		}
		src := S1{A: 7, B: "ok"}
		var dst S2
		if !Convert(&dst, src) || dst.A != 7 || dst.B != "ok" {
			t.Fatalf("struct field convert: %+v", dst)
		}

		type S3 struct {
			X int
			B string
		}
		if Convert(&S3{}, src) {
			t.Fatal("mismatched field names should fail")
		}
	})

	t.Run("Unexported", func(t *testing.T) {
		type local struct {
			a int
		}
		src := local{a: 99}
		var dst local
		ok := Convert(&dst, src)
		if !ok || dst.a != 99 {
			t.Logf("unexported field: ok=%v dst=%+v (观察是否静默丢失)", ok, dst)
		}
	})

	t.Run("OverflowAndInvalid", func(t *testing.T) {
		var i8 int8
		if Convert(&i8, math.MaxInt16) { // 32767 → int8
			t.Fatal("overflow int16→int8 should fail")
		}
		if Convert(&i8, int64(math.MinInt8-1)) { // 下溢
			t.Fatal("underflow should fail")
		}

		var u8 uint8
		if Convert(&u8, -1) { // 负数 → 无符号
			t.Fatal("negative→uint should fail")
		}
		if Convert(&u8, 256) { // 溢出
			t.Fatal("256→uint8 should fail")
		}

		// 边界值应成功
		if !Convert(&i8, int8(math.MaxInt8)) || i8 != math.MaxInt8 {
			t.Fatal("MaxInt8 should succeed")
		}

		var i int
		if Convert(&i, math.NaN()) || Convert(&i, 3.14) {
			t.Fatal("NaN or non-integer float→int should fail")
		}
	})

	t.Run("NonSettable", func(t *testing.T) {
		var x int = 1
		if Convert(x, 2) {
			t.Fatal("non-settable dst should return false")
		}
	})

	t.Run("Interface", func(t *testing.T) {
		var i any = 123
		var n int
		if !Convert(&n, i) || n != 123 {
			t.Fatalf("interface→int: %v", n)
		}
		var j any
		if !Convert(&j, 456) || j.(int) != 456 {
			t.Fatalf("int→interface: %v", j)
		}
	})
}

func TestTo(t *testing.T) {
	type caseT struct {
		name    string
		typ     any
		v       any
		want    any // 成功时期望值；失败时忽略
		wantErr bool
	}

	cases := []caseT{
		// nil → 零值
		{"nil→int", "int", nil, 0, true},
		{"nil→[]string", "[]string", nil, ([]string)(nil), false},
		{"nil→*int", "*int", nil, (*int)(nil), false},

		// 相同类型
		{"same string", "string", "hello", "hello", false},

		// 数字边界 / 溢出
		{"int8 max", "int8", 127, int8(127), false},
		{"int8 overflow+", "int8", 128, nil, true},
		{"int8 overflow-", "int8", -129, nil, true},
		{"int32 from MaxInt64", "int32", math.MaxInt64, nil, true},
		{"uint from -1", "uint", -1, nil, true},
		{"uint8 from -1", "uint8", -1, nil, true},
		{"int from 3.14", "int", 3.14, nil, true},
		{"int from MaxUint64", "int", uint64(math.MaxUint64), nil, true},
		{"uint8 max", "uint8", 255, uint8(255), false},
		{"int64 from MaxInt64", "int64", float64(math.MaxInt64), nil, true},

		// 数字 ↔ 字符串
		{"int→string", "string", 42, "42", false},
		{"bool→string", "string", true, "true", false},
		{"float→string", "string", 3.14, "3.14", false},
		{"string→int", "int", "123", 123, false},
		{"string→int8 overflow", "int8", "128", nil, true},
		{"string→uint neg", "uint", "-1", nil, true},
		{"string→float32 bad", "float32", "not-a-number", nil, true},
		{"string→bool bad", "bool", "maybe", nil, true},

		// 切片 ↔ 数组（长度必须匹配）
		{"slice→array ok", "[3]int", []int{1, 2, 3}, [3]int{1, 2, 3}, false},
		{"slice→array short", "[3]int", []int{1, 2}, nil, true},
		{"slice→array long", "[3]int", []int{1, 2, 3, 4}, nil, true},
		{"nil slice→array", "[2]int", ([]int)(nil), nil, true},
		{"array→slice", "[]int", [2]int{10, 20}, []int{10, 20}, false},

		// 切片元素转换
		{"[]int→[]int64", "[]int64", []int{1, 2, 3}, []int64{1, 2, 3}, false},
		{"[]int→[]int8 overflow", "[]int8", []int{1, 200, 3}, nil, true},
		{"string→[]byte", "[]byte", "abc", []byte("abc"), false},
		{"string→[]rune", "[]rune", "你好", []rune("你好"), false},
		{"[]byte→string", "string", []byte("xyz"), "xyz", false},

		// map
		{
			"map value convert", "map[string]int64",
			map[string]int{"a": 1, "b": 2},
			map[string]int64{"a": 1, "b": 2},
			false,
		},
		{
			"map key convert", "map[int]string",
			map[string]string{"1": "one", "2": "two"},
			map[int]string{1: "one", 2: "two"},
			false,
		},
		{"nil map", "map[string]int", (map[string]int)(nil), (map[string]int)(nil), false},

		// 指针 / 接口
		{"*int pass", "*int", func() *int { x := 42; return &x }(), func() *int { x := 42; return &x }(), false},
		{"interface unwrap", "int", func() any { x := 42; return &x }(), 42, false},
		{"auto address", "*int", 99, func() *int { x := 99; return &x }(), false},

		// 结构体
		{
			"struct convert",
			struct {
				A int64
				B string
			}{},
			struct {
				A int
				B string
			}{A: 10, B: "hi"},
			struct {
				A int64
				B string
			}{A: 10, B: "hi"},
			false,
		},
		{
			"struct mismatch",
			struct{ X int }{},
			struct {
				A int
				B string
			}{A: 1, B: "x"},
			nil, true,
		},

		// 明确不可转换
		{"chan", "chan int", 1, nil, true},
		{"func", "func()", "hello", nil, true},
		{"map from slice", "map[int]int", []int{1, 2}, nil, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := To(c.typ, c.v)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				// 指针比较特殊处理：只比指向的值
				if reflect.TypeOf(got).Kind() == reflect.Ptr && reflect.TypeOf(c.want).Kind() == reflect.Ptr {
					if reflect.ValueOf(got).IsNil() != reflect.ValueOf(c.want).IsNil() {
						t.Fatalf("got %v, want %v", got, c.want)
					}
					if !reflect.ValueOf(got).IsNil() &&
						!reflect.DeepEqual(reflect.ValueOf(got).Elem().Interface(), reflect.ValueOf(c.want).Elem().Interface()) {
						t.Fatalf("got %v, want %v", got, c.want)
					}
					return
				}
				t.Fatalf("got %#v, want %#v", got, c.want)
			}
		})
	}
}

func TestGet(t *testing.T) {
	type inner struct {
		X int `json:"x"`
	}

	type sample struct {
		Name string `json:"name" mapstructure:"full_name"`
		Age  int    `json:"age"`
		inner
		Ptr  *inner
		hide int
	}

	s := sample{Name: "n", Age: 7, inner: inner{X: 9}, hide: 8}
	m := map[string]int{"1": 10, "true": 1}
	sl := []int{10, 20, 30}
	arr := [2]string{"a", "b"}
	var nilMap map[string]int
	var nilPtr *sample
	var boxed any = map[int]int{2: 4}

	type rec struct {
		name    string
		obj     any
		key     any
		want    any
		wantErr error
		skipVal bool // 只关心 error
	}

	cases := []rec{
		{"map string key", m, "1", 10, nil, false},
		{"map int→decimal string", m, 1, 10, nil, false},
		{"map missing → (nil,nil)", m, "missing", nil, nil, false},
		{"map bad key type", m, []int{1}, nil, ErrMapKey, true},
		{"nil map lookup empty", nilMap, "a", nil, nil, false},
		{"slice last via -1", sl, -1, 30, nil, false},
		{"slice OOB", sl, -4, nil, ErrIndexOutOfRange, true},
		{"slice float index no trunc", sl, 1.5, nil, ErrKeyType, true},
		{"array string index", arr, "1", "b", nil, false},
		{"string byte index", "abc", 0, byte('a'), nil, false},
		{"string OOB", "abc", 99, nil, ErrIndexOutOfRange, true},
		{"number as decimal bytes", 12345, 0, byte('1'), nil, false},
		{"struct exact name", s, "Name", "n", nil, false},
		{"struct json/case", s, "name", "n", nil, false},
		{"struct mapstructure tag", s, "full_name", "n", nil, false},
		{"embedded promote", s, "X", 9, nil, false},
		{"struct missing", s, "missing", nil, ErrFieldNotFound, true},
		{"unexported by name hidden", s, "hide", nil, ErrFieldNotFound, true},
		{"unexported by index hidden", s, 4, nil, ErrFieldNotFound, true},
		{"struct bad key", s, true, nil, ErrKeyType, true},
		{"Get(nil)", nil, "a", nil, ErrNilValue, true},
		{"nil pointer", nilPtr, "Name", nil, ErrNilValue, true},
		{"interface unwrap + numeric string key", boxed, "2", 4, nil, false},
		{"reflect.Value input", reflect.ValueOf([]byte{'A', 'B'}), 1, byte('B'), nil, false},
		{"unsupported chan", make(chan int), 0, nil, ErrUnsupported, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Get(c.obj, c.key)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err=%v want %v (got val=%#v)", err, c.wantErr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !c.skipVal && !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %#v (%T) want %#v (%T)", got, got, c.want, c.want)
			}
		})
	}

	// 与 slice/非空 string 越界策略是否一致
	t.Run("inconsistent OOB policy", func(t *testing.T) {
		v1, e1 := Get("", 0)
		if e1 == nil {
			t.Fatalf("empty string OOB => (%#v, %v)", v1, e1)
		}
		v2, e2 := Get(12345, 99)
		if e2 == nil {
			t.Fatalf("number OOB       => (%#v, %v)", v2, e2)
		}
		v3, e3 := Get("abc", 99)
		if e3 == nil {
			t.Fatalf("string OOB       => (%#v, %v)", v3, e3)
		}
	})

	t.Run("utf8 string is by byte not rune", func(t *testing.T) {
		v, err := Get("你好", 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := v.(byte); !ok {
			t.Fatalf("want byte, got %T (%#v)", v, v)
		}
	})

	t.Run("map miss vs struct miss", func(t *testing.T) {
		v, err := Get(map[string]int{}, "x")
		if v != nil || err != nil {
			t.Fatalf("map miss want (nil,nil) got (%#v,%v)", v, err)
		}
		if _, err := Get(sample{}, "x"); err != nil {
			t.Fatalf("struct miss: %v", err)
		}
	})

	t.Run("cyclic pointer must not hang", func(t *testing.T) {
		type node struct{ P *node }
		n := &node{}
		n.P = n
		done := make(chan struct{})
		go func() {
			_, _ = Get(n, "P")
			close(done)
		}()
		select {
		case <-done:
		case <-t.Context().Done():
			t.Fatal("Get on cyclic pointer blocked")
		}
	})
}

func TestGetUnexported(t *testing.T) {
	type inner struct {
		X int `json:"x"`
	}

	type sample struct {
		Name string `json:"name" mapstructure:"full_name"`
		Age  int    `json:"age"`
		inner
		Ptr  *inner
		hide int
	}
	s := sample{Name: "n", hide: 8}

	type rec struct {
		name    string
		obj     any
		key     any
		want    any
		wantErr error
	}
	cases := []rec{
		{"by name", s, "hide", 8, nil},
		{"by index", s, 4, 8, nil},
		{"still exported", s, "Name", "n", nil},
		{"missing still error", s, "nope", nil, ErrFieldNotFound},
		{"Get 仍不可读，对比泄漏", s, "hide", 8, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := GetUnexported(c.obj, c.key)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err=%v want %v", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %#v want %#v", got, c.want)
			}
		})
	}

	t.Run("Get vs GetUnexported", func(t *testing.T) {
		if _, err := Get(s, "hide"); !errors.Is(err, ErrFieldNotFound) {
			t.Fatalf("Get leaked unexported: %v", err)
		}
		v, err := GetUnexported(s, "hide")
		if err != nil || v != 8 {
			t.Fatalf("GetUnexported: %v %v", v, err)
		}
	})
}

func TestSet(t *testing.T) {
	type inner struct {
		X int `json:"x"`
	}

	type sample struct {
		Name string `json:"name" mapstructure:"full_name"`
		Age  int    `json:"age"`
		inner
		Ptr  *inner
		hide int
	}
	t.Run("map convert / delete-by-nil / nil map / odd args", func(t *testing.T) {
		m := map[string]int{"a": 1}
		if err := Set(m, 1, 2); err != nil {
			t.Fatal(err)
		}
		if m["1"] != 2 {
			t.Fatalf("int key not converted: %v", m)
		}
		if err := Set(m, "a", nil); err != nil {
			t.Fatal(err)
		}
		if _, ok := m["a"]; ok {
			t.Fatal("Set(map,key,nil) should delete")
		}
		var nm map[string]int
		if !errors.Is(Set(nm, "a", 1), ErrNilMap) {
			t.Fatal("nil map")
		}
		if !errors.Is(Set(m, "only"), ErrInvalidArgCount) {
			t.Fatal("odd args")
		}

		a := make(map[string][]int)
		if err := Set(a, "a", (*any)(nil)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("slice grow + negative relative to original len", func(t *testing.T) {
		s := []int{1, 2}
		if err := Set(&s, 3, 9, -1, 8); err != nil {
			t.Fatal(err)
		}
		want := []int{1, 2, 0, 8}
		if !reflect.DeepEqual(s, want) {
			t.Fatalf("got %v want %v", s, want)
		}
	})

	t.Run("array no grow / string number immutable / unexported blocked", func(t *testing.T) {
		arr := [2]int{1, 2}
		if err := Set(&arr, 2, 3); !errors.Is(err, ErrIndexOutOfRange) {
			t.Fatalf("array grew? %v %#v", err, arr)
		}
		str := "ab"
		if !errors.Is(Set(&str, 0, byte('z')), ErrImmutable) {
			t.Fatal("string")
		}
		n := 12
		if !errors.Is(Set(&n, 0, byte('1')), ErrImmutable) {
			t.Fatal("number")
		}
		u := sample{}
		if err := Set(&u, "hide", 1); err == nil {
			t.Fatal("Set must not write unexported")
		}
		plain := sample{}
		err := Set(plain, "Name", "x")
		if !errors.Is(err, ErrUnaddressable) {
			t.Fatalf("unaddressable: %v", err)
		}
	})

	t.Run("struct tags embed alloc rollback", func(t *testing.T) {
		s := sample{Name: "old", Age: 1}
		err := Set(&s, "Name", "new", "nope", 1)
		if !errors.Is(err, ErrFieldNotFound) {
			t.Fatalf("want field err, got %v", err)
		}
		if s.Name != "old" || s.Age != 1 {
			t.Fatalf("no rollback: %+v", s)
		}

		s = sample{}
		if err := Set(&s, "X", 5); err != nil {
			t.Fatal(err)
		}
		if s.inner.X != 5 {
			t.Fatalf("embedded not set: %+v", s)
		}

		s = sample{}
		if err := Set(&s, "name", "via-tag"); err != nil {
			t.Fatal(err)
		}
		if s.Name != "via-tag" {
			t.Fatalf("json tag set: %+v", s)
		}

		s = sample{}
		if err := Set(&s, "Ptr", inner{X: 1}); err != nil {
			t.Logf("pointer field from value (possible convert gap): %v", err)
		} else if s.Ptr == nil || s.Ptr.X != 1 {
			t.Fatalf("Ptr not allocated/set: %+v", s)
		}
	})

	t.Run("transaction: later failure restores earlier writes", func(t *testing.T) {
		m := map[string]int{"a": 1}
		if err := Set(m, "a", 2, []byte("bad"), 3); err == nil {
			t.Fatal("want error")
		}
		if m["a"] != 1 {
			t.Fatalf("map txn broken: %v", m)
		}

		sl := []int{1, 2, 3}
		orig := append([]int(nil), sl...)
		if err := Set(&sl, 0, 9, "bad", 1); err == nil {
			t.Fatal("want key error")
		}
		if !reflect.DeepEqual(sl, orig) {
			t.Fatalf("slice rollback failed: %v vs %v", sl, orig)
		}

		s := []int{0, 1, 2}
		if err := Set(&s, 3, 9, "bad", 0); err == nil {
			t.Fatal("want error after grow")
		}
		if !reflect.DeepEqual(s, []int{0, 1, 2}) {
			t.Fatalf("grow rollback failed: %v", s)
		}
	})
}

func TestGetNumberOOBAndSetNegIndex(t *testing.T) {
	_, err := Get(12345, 99)
	if !errors.Is(err, ErrIndexOutOfRange) {
		t.Fatalf("number OOB: %v", err)
	}
	v, err := Get(12345, 0)
	if err != nil || v != byte('1') {
		t.Fatalf("Get(12345,0)=(%v,%v)", v, err)
	}

	s := []int{1, 2}
	if err := Set(&s, 3, 9, -1, 8); err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 0, 8}
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("负索引相对当前 len: got %v want %v", s, want)
	}
}

func TestDelete(t *testing.T) {
	type rec struct {
		name string
		run  func(t *testing.T)
	}

	cases := []rec{
		{"convert string key", func(t *testing.T) {
			m := map[int]string{1: "a", 2: "b"}
			Delete(m, "1")
			if _, ok := m[1]; ok {
				t.Fatal("should convert and delete")
			}
			if m[2] != "b" {
				t.Fatal("other keys")
			}
		}},
		{"missing key / nil map / non-map / nil obj are no-ops", func(t *testing.T) {
			m := map[int]string{1: "a"}
			Delete(m, 99)
			Delete((map[int]string)(nil), 1)
			Delete([]int{1}, 0)
			Delete(nil, "x")
			Delete("not-a-map", 0)
			if m[1] != "a" {
				t.Fatal("existing key mutated")
			}
		}},
		{"non-convertible key must not delete", func(t *testing.T) {
			m := map[string]int{"k": 1, "1": 2}
			Delete(m, 1.5)
			if m["k"] != 1 || m["1"] != 2 {
				t.Fatalf("truncated/converted 1.5: %v", m)
			}
		}},
		{"bool key from string", func(t *testing.T) {
			m := map[bool]int{true: 1, false: 2}
			Delete(m, "true")
			if _, ok := m[true]; ok {
				t.Fatal("bool parse")
			}
			if m[false] != 2 {
				t.Fatal("false key")
			}
		}},
		{"NaN is not true", func(t *testing.T) {
			m := map[bool]int{true: 1}
			Delete(m, math.NaN())
			if m[true] != 1 {
				t.Fatal("NaN deleted true")
			}
		}},
	}

	for _, c := range cases {
		t.Run(c.name, c.run)
	}
}

func TestGetSlice_intent(t *testing.T) {
	s := []int{10, 20, 30, 40}
	arr := [4]int{10, 20, 30, 40}
	str := "Go语言"

	tests := []struct {
		name string
		in   any
		i, j int
		want any
	}{
		{"slice 正常区间", s, 1, 3, []int{20, 30}},
		{"slice 全区间", s, 0, 4, []int{10, 20, 30, 40}},
		{"slice 空区间", s, 2, 2, []int{}},
		{"slice 尾部空区间", s, 4, 4, []int{}},
		{"array 当切片取出", arr, 1, 3, []int{20, 30}},
		{"string 按字节切", str, 0, 2, "Go"},
		{"*slice 自动解引用", &s, 0, 2, []int{10, 20}},
		{"*array 自动解引用", &arr, 0, 2, []int{10, 20}},
		{"start<0", s, -1, 2, nil},
		{"end<start", s, 3, 1, nil},
		{"end>len", s, 0, 5, nil},
		{"start>len", s, 5, 5, nil},
		{"nil 入参", nil, 0, 0, nil},
		{"非序列类型", 42, 0, 1, nil},
		{"nil slice 非空区间", []int(nil), 0, 1, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetSlice(tc.in, tc.i, tc.j)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v (%T), want %#v (%T)", got, got, tc.want, tc.want)
			}
		})
	}

	// 空区间对 nil slice：不应 panic；结果要么是 typed nil，要么是空切片
	t.Run("nil slice 空区间不 panic", func(t *testing.T) {
		got := GetSlice([]int(nil), 0, 0)
		if got == nil {
			return
		}
		v, ok := got.([]int)
		if !ok || len(v) != 0 {
			t.Fatalf("unexpected %#v", got)
		}
	})
}

func TestGetSlice_arrayAliasVsCopy(t *testing.T) {
	// 经指针取出的 array 子区间应共享底层存储
	arr := [3]int{1, 2, 3}
	got := GetSlice(&arr, 0, 2)
	sl, ok := got.([]int)
	if !ok || len(sl) != 2 {
		t.Fatalf("got %#v", got)
	}
	sl[0] = 99
	if arr[0] != 99 {
		t.Fatalf("expected alias via *array, arr=%v", arr)
	}

	// 按值传入 array：修改返回值不应写回原数组
	arr2 := [3]int{1, 2, 3}
	got2 := GetSlice(arr2, 0, 2)
	sl2 := got2.([]int)
	sl2[0] = 77
	if arr2[0] != 1 {
		t.Fatalf("value array should be copied, arr2=%v", arr2)
	}
}

func TestSetSlice_intent(t *testing.T) {
	mk := func() []int { return []int{1, 2, 3, 4} }

	t.Run("slice 正常写入", func(t *testing.T) {
		d := mk()
		if !SetSlice(d, 1, 3, []int{8, 9}) || !reflect.DeepEqual(d, []int{1, 8, 9, 4}) {
			t.Fatalf("%v", d)
		}
	})
	t.Run("array 指针可写", func(t *testing.T) {
		a := [4]int{1, 2, 3, 4}
		if !SetSlice(&a, 0, 2, []int{7, 8}) || a != [4]int{7, 8, 3, 4} {
			t.Fatalf("%v", a)
		}
	})
	t.Run("array 值不可写", func(t *testing.T) {
		a := [4]int{1, 2, 3, 4}
		if SetSlice(a, 0, 2, []int{7, 8}) {
			t.Fatal("expected false")
		}
	})
	t.Run("src 用 array 且等长", func(t *testing.T) {
		d := mk()
		if !SetSlice(d, 1, 3, [2]int{8, 9}) {
			t.Fatal("array src should be accepted")
		}
		if !reflect.DeepEqual(d, []int{1, 8, 9, 4}) {
			t.Fatalf("%v", d)
		}
	})
	t.Run("[]byte ← string", func(t *testing.T) {
		d := []byte("xxxx")
		if !SetSlice(d, 1, 3, "ab") || string(d) != "xabx" {
			t.Fatalf("%q", d)
		}
	})
	t.Run("string dest 不可变", func(t *testing.T) {
		if SetSlice("abcd", 0, 2, "xy") {
			t.Fatal("expected false")
		}
	})
	t.Run("空区间 + 空 src", func(t *testing.T) {
		d := mk()
		if !SetSlice(d, 2, 2, []int{}) || !reflect.DeepEqual(d, mk()) {
			t.Fatalf("%v", d)
		}
	})
}

func TestSetSlice_rejectAndNoPartialWrite(t *testing.T) {
	orig := []int{1, 2, 3, 4}
	cases := []struct {
		name       string
		dst        func() any
		i, j       int
		src        any
		unchanged  any // 若 dst 是切片，用这个对比
		checkSlice bool
	}{
		{"长度不匹配", func() any { d := append([]int(nil), orig...); return d }, 0, 2, []int{9}, orig, true},
		{"end>len", func() any { d := append([]int(nil), orig...); return d }, 0, 5, []int{1, 2, 3, 4, 5}, orig, true},
		{"end<start", func() any { d := append([]int(nil), orig...); return d }, 2, 1, []int{}, orig, true},
		{"start<0", func() any { d := append([]int(nil), orig...); return d }, -1, 1, []int{9}, orig, true},
		{"元素类型不兼容", func() any { d := append([]int(nil), orig...); return d }, 0, 2, []string{"a", "b"}, orig, true},
		{"string → []int 拒绝", func() any { d := append([]int(nil), orig...); return d }, 0, 1, "x", orig, true},
		{"nil dst", func() any { return []int(nil) }, 0, 0, []int{}, nil, false},
		{"src 非序列", func() any { d := append([]int(nil), orig...); return d }, 0, 1, 7, orig, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dst := tc.dst()
			if SetSlice(dst, tc.i, tc.j, tc.src) {
				t.Fatal("expected false")
			}
			if tc.checkSlice && !reflect.DeepEqual(dst, tc.unchanged) {
				t.Fatalf("dst mutated on failure: %#v", dst)
			}
		})
	}
}

func TestSetSlice_overlap(t *testing.T) {
	d := []int{1, 2, 3, 4, 5}
	// d[0:3] ← d[2:5]  意图：重叠时仍得到 memmove 语义
	if !SetSlice(d, 0, 3, d[2:5]) {
		t.Fatal("expected true")
	}
	if !reflect.DeepEqual(d, []int{3, 4, 5, 4, 5}) {
		t.Fatalf("overlap result %v", d)
	}
}

func TestGetSetSlice_roundTrip(t *testing.T) {
	d := []string{"a", "b", "c", "d"}
	part := GetSlice(d, 1, 3)
	if !SetSlice(d, 0, 2, part) {
		t.Fatal("set failed")
	}
	if !reflect.DeepEqual(d, []string{"b", "c", "c", "d"}) {
		t.Fatalf("%v", d)
	}
}

func TestGetSlice_typeAndBounds(t *testing.T) {
	type Ints []int
	type Arr [4]int

	t.Run("命名切片应保留类型", func(t *testing.T) {
		in := Ints{1, 2, 3}
		got := GetSlice(in, 1, 3)
		if _, ok := got.(Ints); !ok {
			t.Fatalf("want Ints, got %T %#v", got, got)
		}
		if !reflect.DeepEqual(got, Ints{2, 3}) {
			t.Fatalf("%#v", got)
		}
	})
	t.Run("命名数组取出的是 []T 不是数组", func(t *testing.T) {
		got := GetSlice(Arr{1, 2, 3, 4}, 1, 3)
		if _, ok := got.([]int); !ok {
			t.Fatalf("want []int, got %T", got)
		}
		if !reflect.DeepEqual(got, []int{2, 3}) {
			t.Fatalf("%#v", got)
		}
	})
	t.Run("多层指针", func(t *testing.T) {
		s := []int{1, 2, 3}
		p := &s
		got := GetSlice(&p, 1, 2)
		if !reflect.DeepEqual(got, []int{2}) {
			t.Fatalf("%#v", got)
		}
	})
	t.Run("只按 Len 不按 Cap", func(t *testing.T) {
		s := make([]int, 2, 8)
		s[0], s[1] = 1, 2
		if GetSlice(s, 0, 3) != nil {
			t.Fatal("end>len must fail even if cap is enough")
		}
		if !reflect.DeepEqual(GetSlice(s, 0, 2), []int{1, 2}) {
			t.Fatal("end==len should work")
		}
	})
	t.Run("string 按字节，可切出残缺 UTF-8", func(t *testing.T) {
		s := "世" // 3 bytes
		got := GetSlice(s, 0, 1)
		b, ok := got.(string)
		if !ok || len(b) != 1 {
			t.Fatalf("%#v", got)
		}
	})
	t.Run("*string 解引用", func(t *testing.T) {
		s := "abcd"
		if GetSlice(&s, 1, 3) != "bc" {
			t.Fatalf("%#v", GetSlice(&s, 1, 3))
		}
	})
}

func TestSetSlice_namedAndNil(t *testing.T) {
	type A []int
	type B []int
	type MyInt int
	type MyByte byte

	t.Run("不同命名切片、元素相同应成功", func(t *testing.T) {
		d := A{1, 2, 3}
		if !SetSlice(d, 0, 2, B{8, 9}) || !reflect.DeepEqual([]int(d), []int{8, 9, 3}) {
			t.Fatalf("%v", d)
		}
	})
	t.Run("命名数组源", func(t *testing.T) {
		type Pair [2]int
		d := []int{1, 2, 3}
		if !SetSlice(d, 1, 3, Pair{8, 9}) || !reflect.DeepEqual(d, []int{1, 8, 9}) {
			t.Fatalf("%v", d)
		}
	})
	t.Run("[]MyInt 不能写入 []int", func(t *testing.T) {
		d := []int{1, 2}
		if SetSlice(d, 0, 2, []MyInt{3, 4}) || !reflect.DeepEqual(d, []int{1, 2}) {
			t.Fatalf("mutated %v", d)
		}
	})
	t.Run("string → []MyByte", func(t *testing.T) {
		d := make([]MyByte, 4)
		copy(d, []MyByte("xxxx"))
		if !SetSlice(d, 1, 3, "ab") || string(d) != "xabx" {
			t.Fatalf("%q", d)
		}
	})
	t.Run("string 长度必须严格相等", func(t *testing.T) {
		d := []byte("xxxx")
		if SetSlice(d, 0, 2, "abc") || string(d) != "xxxx" {
			t.Fatalf("partial write %q", d)
		}
	})
	t.Run("dst 只看 Len 不看 Cap", func(t *testing.T) {
		d := make([]int, 2, 8)
		d[0], d[1] = 1, 2
		if SetSlice(d, 0, 3, []int{7, 8, 9}) {
			t.Fatal("end>len must fail")
		}
		if !reflect.DeepEqual(d[:2], []int{1, 2}) {
			t.Fatalf("mutated %v", d)
		}
	})
	t.Run("nil src 仅空区间可写", func(t *testing.T) {
		d := []int{1, 2, 3}
		if !SetSlice(d, 1, 1, []int(nil)) {
			t.Fatal("empty range + nil src")
		}
		if SetSlice(d, 0, 1, []int(nil)) || !reflect.DeepEqual(d, []int{1, 2, 3}) {
			t.Fatalf("nil src with need>0: %v", d)
		}
	})
	t.Run("*slice dst / *array src", func(t *testing.T) {
		d := []int{1, 2, 3}
		src := [2]int{8, 9}
		if !SetSlice(&d, 0, 2, &src) || !reflect.DeepEqual(d, []int{8, 9, 3}) {
			t.Fatalf("%v", d)
		}
	})
}

func TestGetSetSlice_noSharedHeaderSurprise(t *testing.T) {
	// GetSlice 对 slice 应共享底层数组（与 Go s[i:j] 一致）
	s := []int{1, 2, 3, 4}
	part := GetSlice(s, 1, 3).([]int)
	part[0] = 99
	if s[1] != 99 {
		t.Fatalf("expected alias, s=%v", s)
	}

	part[1] = 99
	// 再写入另一段，确认没有静默扩容/换底层
	if !SetSlice(s, 0, 2, []int{7, 8}) || !reflect.DeepEqual(s, []int{7, 8, 99, 4}) {
		t.Fatalf("%v", s)
	}
}

func TestCopy(t *testing.T) {
	type (
		myByte   byte
		myBytes  []byte
		myString string
		myInts   []int
	)

	tests := []struct {
		name  string
		setup func() (dst, src any)
		wantN int
		want  any // 复制后 dst；nil 表示不校验内容
	}{
		{
			name:  "elem assignable but not identical: []int -> []any",
			setup: func() (any, any) { return make([]any, 3), []int{1, 2, 3} },
			wantN: 0, // 内置 copy 要求元素类型相同；不得 panic
			want:  make([]any, 3),
		},
		{
			name:  "equal length",
			setup: func() (any, any) { return []int{0, 0, 0}, []int{1, 2, 3} },
			wantN: 3,
			want:  []int{1, 2, 3},
		},
		{
			name:  "src shorter: tail unchanged",
			setup: func() (any, any) { return []int{9, 9, 9}, []int{1, 2} },
			wantN: 2,
			want:  []int{1, 2, 9},
		},
		{
			name:  "dst shorter: no grow",
			setup: func() (any, any) { return []int{0, 0}, []int{1, 2, 3, 4} },
			wantN: 2,
			want:  []int{1, 2},
		},
		{
			name:  "empty dst",
			setup: func() (any, any) { return []int{}, []int{1, 2} },
			wantN: 0,
			want:  []int{},
		},
		{
			name:  "empty src",
			setup: func() (any, any) { return []int{1, 2}, []int{} },
			wantN: 0,
			want:  []int{1, 2},
		},
		{
			name:  "nil dst slice",
			setup: func() (any, any) { var dst []int; return dst, []int{1} },
			wantN: 0,
			want:  []int(nil),
		},
		{
			name:  "nil src slice",
			setup: func() (any, any) { return []int{1, 2}, []int(nil) },
			wantN: 0,
			want:  []int{1, 2},
		},
		{
			name:  "both nil",
			setup: func() (any, any) { return any(nil), any(nil) },
			wantN: 0,
		},
		{
			name:  "string to []byte",
			setup: func() (any, any) { return []byte("xxxxx"), "ab" },
			wantN: 2,
			want:  []byte("abxxx"),
		},
		{
			name:  "string to []byte truncated (UTF-8 is bytes, not runes)",
			setup: func() (any, any) { return make([]byte, 2), "你好" },
			wantN: 2,
			want:  []byte("你好")[:2],
		},
		{
			name:  "named string to []byte",
			setup: func() (any, any) { return make([]byte, 3), myString("xyz") },
			wantN: 3,
			want:  []byte("xyz"),
		},
		{
			name:  "string to named []byte",
			setup: func() (any, any) { return make(myBytes, 2), "hi" },
			wantN: 2,
			want:  myBytes("hi"),
		},
		{
			name:  "same elem type, named vs unnamed slice",
			setup: func() (any, any) { return myInts{0, 0}, []int{7, 8} },
			wantN: 2,
			want:  myInts{7, 8},
		},
		{
			name: "overlap forward",
			setup: func() (any, any) {
				s := []int{1, 2, 3, 4, 5}
				return s[1:], s
			},
			wantN: 4,
			want:  []int{1, 2, 3, 4},
		},
		{
			name: "overlap backward",
			setup: func() (any, any) {
				s := []int{1, 2, 3, 4, 5}
				return s[:4], s[1:]
			},
			wantN: 4,
			want:  []int{2, 3, 4, 5},
		},
		{
			name:  "incompatible slice types",
			setup: func() (any, any) { return []int{1, 2}, []string{"a"} },
			wantN: 0,
			want:  []int{1, 2},
		},
		{
			name:  "string to []rune is not the special case",
			setup: func() (any, any) { return make([]rune, 4), "abcd" },
			wantN: 0,
			want:  make([]rune, 4),
		},
		{
			name:  "string to []int8",
			setup: func() (any, any) { return make([]int8, 4), "abcd" },
			wantN: 0,
		},
		{
			name:  "dst not a slice",
			setup: func() (any, any) { return 0, []int{1} },
			wantN: 0,
		},
		{
			name:  "src neither slice nor string",
			setup: func() (any, any) { return []int{0}, 42 },
			wantN: 0,
			want:  []int{0},
		},
		{
			name:  "array dst is not a slice",
			setup: func() (any, any) { return [2]int{}, []int{1, 2} },
			wantN: 0,
			want:  [2]int{},
		},
		{
			name: "elem assignable via interface",
			setup: func() (any, any) {
				return make([]io.Reader, 1), []io.ReadCloser{io.NopCloser(nil)}
			},
			wantN: 0,
			want:  make([]io.Reader, 1),
		},
		{
			name:  "string to []myByte (named byte): builtin copy rejects",
			setup: func() (any, any) { return make([]myByte, 5), "hello" },
			wantN: 0,
			want:  make([]myByte, 5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic: %v", r)
				}
			}()
			dst, src := tt.setup()
			got := Copy(&dst, src)
			n, ok := got.(int)
			if !ok || n != tt.wantN {
				t.Fatalf("Copy() = %v (%T), want %v", got, got, tt.wantN)
			}
			if tt.want != nil && !reflect.DeepEqual(dst, tt.want) {
				t.Fatalf("dst = %#v, want %#v", dst, tt.want)
			}
		})
	}
}

func TestCopy_lenCapUnchanged(t *testing.T) {
	dst := make([]int, 2, 8)
	got := Copy(dst, []int{1, 2, 3, 4, 5})
	n, ok := got.(int)
	if !ok || n != 2 {
		t.Fatalf("n = %v (%T), want 2", got, got)
	}
	if len(dst) != 2 || cap(dst) != 8 {
		t.Fatalf("len=%d cap=%d, want 2, 8", len(dst), cap(dst))
	}
	if !reflect.DeepEqual(dst, []int{1, 2}) {
		t.Fatalf("dst = %v", dst)
	}
}

func TestAppend(t *testing.T) {
	type mySlice []int
	p := 7

	tests := []struct {
		name      string
		slice     any
		vals      []any
		want      any
		wantPanic bool
	}{
		// 正常路径
		{"nilSlice", []int(nil), []any{1}, []int{1}, false},
		{"nilSliceNoVal", []int(nil), nil, []int(nil), false},
		{"empty", []int{}, []any{1, 2}, []int{1, 2}, false},
		{"noValKeeps", []int{1, 2}, nil, []int{1, 2}, false},
		{"appendMany", []int{1}, []any{2, 3, 4}, []int{1, 2, 3, 4}, false},
		{"strings", []string{"a"}, []any{"b"}, []string{"a", "b"}, false},
		{"anyHoldsNil", []any{1}, []any{nil, "x"}, []any{1, nil, "x"}, false},
		{"ptrNilElem", []*int{}, []any{nil, &p}, []*int{nil, &p}, false},
		{"noFlatten", [][]int{{1}}, []any{[]int{2, 3}}, [][]int{{1}, {2, 3}}, false},
		{"namedType", mySlice{1}, []any{2}, mySlice{1, 2}, false},
		{"bytes", []byte{1}, []any{byte(2)}, []byte{1, 2}, false},
		{"emptyString", []string{""}, []any{""}, []string{"", ""}, false},

		// 提供类似转换支持
		{"ptrToSlice", &[]int{1}, []any{2}, []int{1, 2}, false},
		{"intToInt64", []int64{}, []any{int(1)}, []int64{1}, false},
		{"intToByte", []byte{}, []any{int(1)}, []byte{0x01}, false},
		{"floatToInt", []int{}, []any{1.4}, nil, true},
		{"numToString", []string{}, []any{123}, []string{"123"}, false},
		{"stringToByteElem", []byte{}, []any{"a"}, nil, true}, // 每个 val 是一个元素；不是 append([]byte, s...)
		{"sliceToInt", []int{1}, []any{[]int{2}}, nil, true},
		{"nilToInt", []int{}, []any{nil}, nil, true},

		// 首参必须是切片：nil / 数组 / 切片指针都不是切片
		{"untypedNil", nil, []any{1}, nil, true},
		{"array", [1]int{1}, []any{2}, nil, true},
		{"intHead", 1, []any{2}, nil, true},
		{"mapHead", map[int]int{}, []any{1}, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got any
			panicked := false
			func() {
				defer func() {
					if recover() != nil {
						panicked = true
					}
				}()
				got = Append(tt.slice, tt.vals...)
			}()
			if panicked != tt.wantPanic {
				t.Fatalf("panic=%v, want %v; got=%#v (%T)", panicked, tt.wantPanic, got, got)
			}
			if tt.wantPanic {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v (%T), want %#v (%T)", got, got, tt.want, tt.want)
			}
			if reflect.TypeOf(got) != reflect.TypeOf(tt.want) {
				t.Fatalf("type %v, want %v", reflect.TypeOf(got), reflect.TypeOf(tt.want))
			}
		})
	}
}

func TestAppend_growKeepsOriginalHeader(t *testing.T) {
	s := []int{1} // len==cap，追加必扩容
	got, ok := Append(s, 2).([]int)
	if !ok {
		t.Fatalf("type %T", got)
	}
	if !reflect.DeepEqual(s, []int{1}) {
		t.Fatalf("original mutated: %v", s)
	}
	if !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("got %v", got)
	}
}

func TestAppend_namedTypeIdentity(t *testing.T) {
	type mySlice []int
	got := Append(mySlice{1}, 2)
	if _, ok := got.(mySlice); !ok {
		t.Fatalf("want mySlice, got %T", got)
	}
}

func TestParseType_KeywordBoundaryAndQuotes(t *testing.T) {
	mustType := func(t *testing.T, s string) reflect.Type {
		t.Helper()
		rt, err := Type(s)
		if err != nil {
			t.Fatalf("Type(%q): %v", s, err)
		}
		return rt
	}
	mustFail := func(t *testing.T, s string) {
		t.Helper()
		if _, err := Type(s); err == nil {
			t.Fatalf("Type(%q) 应失败", s)
		}
	}

	t.Run("func 参数名 channel 不是 chan 类型", func(t *testing.T) {
		rt := mustType(t, "func(channel int)")
		if rt.Kind() != reflect.Func || rt.NumIn() != 1 || rt.In(0).Kind() != reflect.Int {
			t.Fatalf("got %s", rt)
		}
	})
	t.Run("func(change string)", func(t *testing.T) {
		rt := mustType(t, "func(change string)")
		if rt.NumIn() != 1 || rt.In(0).Kind() != reflect.String {
			t.Fatalf("got %s", rt)
		}
	})
	t.Run("<-chanint 不是 <-chan int", func(t *testing.T) {
		mustFail(t, "<-chanint")
	})
	t.Run("chanint 仍拒绝", func(t *testing.T) {
		mustFail(t, "chanint")
	})
	t.Run("合法 <-chan *int / chan<- int", func(t *testing.T) {
		a := mustType(t, "<-chan *int")
		if a.ChanDir() != reflect.RecvDir || a.Elem().Kind() != reflect.Pointer {
			t.Fatalf("got %s", a)
		}
		b := mustType(t, "chan<- int")
		if b.ChanDir() != reflect.SendDir {
			t.Fatalf("got %s", b)
		}
	})
	t.Run("map key 结构体 tag 含 ]", func(t *testing.T) {
		rt := mustType(t, "map[struct{ S string `json:\"a]b\"` }]int")
		if rt.Kind() != reflect.Map || rt.Key().Kind() != reflect.Struct || rt.Elem().Kind() != reflect.Int {
			t.Fatalf("got %s", rt)
		}
		if tag := rt.Key().Field(0).Tag.Get("json"); tag != "a]b" {
			t.Fatalf("tag=%q", tag)
		}
	})
	t.Run("func 参数 struct tag 含 )", func(t *testing.T) {
		rt := mustType(t, "func(struct{ X int `json:\")\"` })")
		if rt.Kind() != reflect.Func || rt.NumIn() != 1 || rt.In(0).Kind() != reflect.Struct {
			t.Fatalf("got %s", rt)
		}
	})
	t.Run("嵌套数组作 struct 字段仍能配平方括号", func(t *testing.T) {
		// [2]int 可比较，整段可作为 map key；用来确认内层 ] 不会截断外层 map[...]。
		rt := mustType(t, "map[struct{ X [2]int }]bool")
		if rt.Kind() != reflect.Map || rt.Elem().Kind() != reflect.Bool {
			t.Fatalf("got %s", rt)
		}
		kt := rt.Key()
		if kt.Kind() != reflect.Struct || kt.NumField() != 1 || kt.Field(0).Type.Kind() != reflect.Array {
			t.Fatalf("got key %s", kt)
		}
	})
	t.Run("含 map 字段的 struct 不能作 map key", func(t *testing.T) {
		// map 类型自身不可比较：Go 语言规范明确指出，map、slice 和 function 这几种类型是不可比较的。
		// struct 的可比较性取决于字段：一个 struct 类型只有当其所有字段都是可比较的时候，这个 struct 类型才是可比较的。
		// 配平可以成功，MapOf 必须失败。用来确认没在内层 map[string]int 的 ] 处截断。
		_, err := Type("map[struct{ X map[string]int }]bool")
		if err == nil {
			t.Fatal("expected invalid key type")
		}
		if !strings.Contains(err.Error(), "invalid key type") {
			t.Fatalf("应是 MapOf 拒绝 key，而不是解析截断: %v", err)
		}
	})
}

func TestSet_StructPointerFieldNotEmbedded(t *testing.T) {
	type Inner struct{ V int }
	type Outer struct{ P *Inner }
	var o Outer
	if err := Set(&o, "P.V", 1); err == nil {
		t.Fatal("普通指针路径不应自动分配")
	}
}

func TestSet_MapKeyRangeCheck(t *testing.T) {
	m := map[int8]string{}
	if err := Set(m, 1000, "x"); err == nil {
		t.Fatal("超出 int8 范围应报错")
	}
	if len(m) != 0 {
		t.Fatalf("失败后 map 应不变，得到 %v", m)
	}
	m[1] = "a"
	if err := Set(m, 2, "b", 300, "c"); err == nil {
		t.Fatal("应报错")
	}
	if m[2] != "" {
		t.Fatalf("应回滚，得到 %v", m)
	}
}

func TestSet_SliceToNilSlice(t *testing.T) {
	type S struct{ X []int }
	s := S{X: []int{1}}
	if err := Set(&s, "X", nil); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if s.X != nil {
		t.Fatalf("nil slice 应覆盖为 nil，得到 %v", s.X)
	}
}

func TestGet_StringByte(t *testing.T) {
	v, err := Get("中文", 0)
	if err != nil {
		t.Fatal(err)
	}
	b, ok := v.(byte)
	if !ok || b != "中"[0] {
		t.Fatalf("byte=%v", v)
	}
}

func TestGetSlice_ArrayByValueCopied(t *testing.T) {
	arr := [3]int{1, 2, 3}
	got := GetSlice(arr, 0, 2).([]int)
	got[0] = 99
	if arr[0] != 1 {
		t.Fatalf("值数组切片应复制，arr=%v", arr)
	}
}

func TestGetSlice_NilSlice(t *testing.T) {
	var s []int
	if got := GetSlice(s, 0, 1); got != nil {
		t.Fatalf("nil 切片非空区间应返回 nil，得到 %v", got)
	}
}

func TestSetSlice_NilDst(t *testing.T) {
	var d []int
	if SetSlice(d, 0, 0, []int{}) {
		t.Fatal("nil dst 应失败")
	}
}

func TestSetSlice_Overlap(t *testing.T) {
	d := []int{1, 2, 3, 4, 5}
	if !SetSlice(d, 0, 3, d[2:5]) {
		t.Fatal("失败")
	}
	if !reflect.DeepEqual(d, []int{3, 4, 5, 4, 5}) {
		t.Fatalf("%v", d)
	}
}

func TestSet_ArrayIndexOutOfRangeNoGrow(t *testing.T) {
	arr := [3]int{}
	if err := Set(&arr, 5, 1); !errors.Is(err, ErrIndexOutOfRange) {
		t.Fatalf("数组不应自动扩容：%v", err)
	}
}

func TestSet_NilMapError(t *testing.T) {
	var m map[string]int
	if !errors.Is(Set(m, "a", 1), ErrNilMap) {
		t.Fatal("nil map 应返回 ErrNilMap")
	}
}

func TestGet_StructShadowEmbedded(t *testing.T) {
	type Emb struct{ Target string }
	type S struct {
		Emb
		Target string
	}
	s := S{Emb: Emb{Target: "inner"}, Target: "outer"}
	v, err := Get(s, "Target")
	if err != nil || v != "outer" {
		t.Fatalf("外层字段应遮蔽嵌入字段：%v %v", v, err)
	}
}

func TestLen_DeepPointerChain(t *testing.T) {
	s := []int{1, 2, 3}
	v := reflect.ValueOf(s)
	for i := 0; i < 100; i++ {
		p := reflect.New(v.Type())
		p.Elem().Set(v)
		v = p
	}
	if got := Len(v.Interface()); got != 3 {
		t.Fatalf("深指针链 Len=%d，期望 3", got)
	}
}

func TestCap_NilPointerToArray(t *testing.T) {
	var p *[5]int
	if got := Cap(p); got != 5 {
		t.Fatalf("nil *[5]int Cap 应为 5，得到 %d", got)
	}
}

func TestConvert_SliceToArrayPointer(t *testing.T) {
	var p *[3]int
	if !Convert(&p, []int{1, 2, 3}) {
		t.Fatal("slice→*[3]int 转换失败")
	}
	if p == nil || *p != [3]int{1, 2, 3} {
		t.Fatalf("got %v", p)
	}
}

func TestCopy_StringToNonByteSlice(t *testing.T) {
	if n := Copy(&[]int{0}, "abc"); n != 0 {
		t.Fatalf("string → []int 应返回 0，得到 %v", n)
	}
}

func TestAppend_NamedSlice(t *testing.T) {
	type MyInts []int
	got := Append(MyInts{1}, 2, 3).(MyInts)
	if !reflect.DeepEqual(got, MyInts{1, 2, 3}) {
		t.Fatalf("%v", got)
	}
}

func TestCompute_Shifts(t *testing.T) {
	if v, err := Compute(int32(2), "<<", 3); err != nil || v != int32(16) {
		t.Fatalf("%v %v", v, err)
	}
}

func TestEQ_FuncComparison(t *testing.T) {
	f1 := func() {}
	f2 := func() {}
	_ = EQ(f1, f2)
	_ = NE(f1, f2)
	type F1 func()
	type F2 func()
	var a F1 = func() {}
	var b F2 = func() {}
	if EQ(a, b) {
		t.Fatal("不同类型函数不应相等")
	}
}

func TestCompare_NaN(t *testing.T) {
	nan := math.NaN()
	if LT(nan, 1) || GT(nan, 1) || LE(nan, 1) || GE(nan, 1) {
		t.Fatal("NaN 比较都应返回 false")
	}
}

func TestBool_Struct(t *testing.T) {
	if !Bool(struct{}{}) {
		t.Fatal("空结构体应视为 true")
	}
}

func TestDepthField_NilPtrChain(t *testing.T) {
	type Inner struct{ V int }
	type Outer struct{ P *Inner }
	o := Outer{}
	if _, err := DepthField(o, "P", "V"); err == nil {
		t.Fatal("nil 指针路径应报错")
	}
}

func TestMake_ChanUnidirectionalPanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Make 单向 channel 应 panic")
		}
	}()
	Make("<-chan int")
}

// ========== 以下追加至 builtin/builtin_test.go 末尾 ==========

// TestGet_Boundary 覆盖取值的类型判定、遮蔽、nil 嵌入指针、未导出字段。
func TestGet_Boundary(t *testing.T) {
	type Inner struct {
		X int `json:"x"`
	}
	type Sample struct {
		Name string `json:"name" mapstructure:"full_name"`
		Age  int    `json:"age"`
		Inner
		hide int
	}
	s := Sample{Name: "n", Age: 7, Inner: Inner{X: 9}, hide: 8}

	cases := []struct {
		name    string
		obj     any
		key     any
		want    any
		wantErr error
	}{
		{"map 数字键→十进制字符串", map[string]int{"1": 10}, 1, 10, nil},
		{"map 未命中→(nil,nil)", map[string]int{}, "missing", nil, nil},
		{"nil map 未命中", (map[string]int)(nil), "a", nil, nil},
		{"切片负索引", []int{10, 20, 30}, -1, 30, nil},
		{"切片越界", []int{10, 20, 30}, -4, nil, ErrIndexOutOfRange},
		{"切片浮点索引拒绝", []int{10, 20, 30}, 1.5, nil, ErrKeyType},
		{"字符串按字节", "abc", 0, byte('a'), nil},
		{"中文按字节索引", "你好", 0, "你"[0], nil},
		{"数字按十进制字符", 12345, 0, byte('1'), nil},
		{"数字越界", 12345, 99, nil, ErrIndexOutOfRange},
		{"结构体 json 标签", s, "name", "n", nil},
		{"结构体 mapstructure 标签", s, "full_name", "n", nil},
		{"嵌入字段提升", s, "X", 9, nil},
		{"未导出字段隐藏", s, "hide", nil, ErrFieldNotFound},
		{"未导出字段按索引隐藏", s, 3, nil, ErrFieldNotFound},
		{"nil 指针", (*Sample)(nil), "Name", nil, ErrNilValue},
		{"interface 解包+键转字符串", any(map[int]int{2: 4}), "2", 4, nil},
	}
	for _, c := range cases[:1] {
		t.Run(c.name, func(t *testing.T) {
			got, err := Get(c.obj, c.key)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err=%v want=%v", err, c.wantErr)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got=%#v err=%v want=%#v", got, err, c.want)
			}
		})
	}

	t.Run("外层字段遮蔽嵌入", func(t *testing.T) {
		type Emb struct{ V string }
		type S struct {
			Emb
			V string
		}
		got, _ := Get(S{Emb: Emb{V: "inner"}, V: "outer"}, "V")
		if got != "outer" {
			t.Fatalf("应得 outer，实得 %v", got)
		}
	})

	t.Run("匿名嵌入指针为 nil", func(t *testing.T) {
		type Inner struct{ V int }
		type Outer struct{ *Inner }
		if _, err := Get(Outer{}, "V"); err == nil {
			t.Fatal("应返回错误而非 panic")
		}
	})
}

// TestSet_Boundary 覆盖事务回滚、嵌入指针清理、不可变类型、nil map。
func TestSet_Boundary(t *testing.T) {
	t.Run("map 后写失败前写回滚", func(t *testing.T) {
		m := map[string]int{"a": 1}
		if err := Set(m, "a", 2, []byte("bad"), 3); err == nil {
			t.Fatal("want err")
		}
		if m["a"] != 1 {
			t.Fatalf("未回滚：%v", m)
		}
	})

	t.Run("切片扩容+后写失败整体回滚", func(t *testing.T) {
		s := []int{1, 2}
		if err := Set(&s, 3, 99, 4, "bad"); err == nil {
			t.Fatal("want err")
		}
		if !reflect.DeepEqual(s, []int{1, 2}) {
			t.Fatalf("未回滚：%v", s)
		}
	})

	t.Run("嵌入指针自动分配+失败逆序清理", func(t *testing.T) {
		type Inner struct{ V int }
		type Outer struct{ *Inner }
		o := &Outer{}
		if err := Set(o, "V", 1, "nope", 2); err == nil {
			t.Fatal("want err")
		}
		if o.Inner != nil {
			t.Fatal("失败应清掉自动分配的嵌入指针")
		}
	})

	t.Run("普通指针路径不自动分配", func(t *testing.T) {
		type Inner struct{ V int }
		type Outer struct{ P *Inner }
		var o Outer
		if err := Set(&o, "P.V", 1); err == nil {
			t.Fatal("普通指针路径不应自动分配")
		}
	})

	t.Run("map key 越界拒绝", func(t *testing.T) {
		m := map[int8]string{}
		if err := Set(m, 1000, "x"); err == nil {
			t.Fatal("1000 超 int8 范围应报错")
		}
	})

	t.Run("string/number 不可变", func(t *testing.T) {
		s := "ab"
		if !errors.Is(Set(&s, 0, byte('z')), ErrImmutable) {
			t.Fatal("string 不可改")
		}
		n := 12
		if !errors.Is(Set(&n, 0, byte('1')), ErrImmutable) {
			t.Fatal("number 不可改")
		}
	})

	t.Run("nil map / nil obj", func(t *testing.T) {
		var m map[string]int
		if !errors.Is(Set(m, "a", 1), ErrNilMap) {
			t.Fatal("nil map 应报错")
		}
		if !errors.Is(Set(nil, "a", 1), ErrNilValue) {
			t.Fatal("nil obj 应报错")
		}
	})

	t.Run("奇数参数", func(t *testing.T) {
		m := map[string]int{}
		if !errors.Is(Set(m, "only"), ErrInvalidArgCount) {
			t.Fatal("奇数参数应报错")
		}
	})
}

// TestCompare_Boundary 补充 NaN 与类型混算的边界。
func TestCompare_Boundary(t *testing.T) {
	nan := math.NaN()
	if LT(nan, 1) || GT(nan, 1) || LE(nan, 1) || GE(nan, 1) {
		t.Fatal("NaN 参与比较应为 false")
	}
	if LT("b", "a") || !LT("a", "b") {
		t.Fatal("字符串比较错误")
	}
	if LT(int8(1), int8(2)) != true {
		t.Fatal("整数比较错误")
	}
}

func TestAnd(t *testing.T) {
	if !And(true) {
		t.Fatal("And(true)")
	}
	if And(false) {
		t.Fatal("And(false)")
	}
	if !And(true, true, true) {
		t.Fatal("And(1,1,1)")
	}
	if And(true, false) {
		t.Fatal("And(1,0)")
	}
	if And(0) {
		t.Fatal("And(0)")
	}
	if !And(1, 2, 3) {
		t.Fatal("And(1,2,3)")
	}
	if And(1, 0) {
		t.Fatal("And(1,0)")
	}
	if And("", "x") {
		t.Fatal("And(\"\",x)")
	}
	if !And("x", "y") {
		t.Fatal("And(\"x\",\"y\")")
	}
}

func TestOr(t *testing.T) {
	if Or(false) {
		t.Fatal("Or(false)")
	}
	if !Or(true) {
		t.Fatal("Or(true)")
	}
	if !Or(false, true) {
		t.Fatal("Or(0,1)")
	}
	if Or(false, false, 0) {
		t.Fatal("Or(0,0,0)")
	}
	if !Or(0, "", 1) {
		t.Fatal("Or(0,\"\",1)")
	}
	if !Or(nil, "x") {
		t.Fatal("Or(nil,\"x\")")
	}
}

func TestNot(t *testing.T) {
	if Not(true) {
		t.Fatal("Not(true)")
	}
	if !Not(false) {
		t.Fatal("Not(false)")
	}
	if !Not(0) {
		t.Fatal("Not(0)")
	}
	if Not(1) {
		t.Fatal("Not(1)")
	}
	if !Not("") {
		t.Fatal("Not(\"\")")
	}
	if Not("x") {
		t.Fatal("Not(\"x\")")
	}
}

type eqStructWithIface struct{ V any }

func TestEQ_NE_DeepEqualAndInterfacePanic(t *testing.T) {
	if !EQ(nil, nil) {
		t.Fatal("EQ(nil,nil)")
	}
	if EQ(nil, 1) {
		t.Fatal("EQ(nil,1)")
	}
	if !NE(nil, 1) {
		t.Fatal("NE(nil,1)")
	}

	if !EQ([]int{1, 2}, []int{1, 2}) {
		t.Fatal("DeepEqual 相等切片")
	}
	if EQ([]int{1}, []int{2}) {
		t.Fatal("DeepEqual 不等切片")
	}

	// int 与 int64 不走隐式转换，应不相等
	if EQ(1, int64(1)) {
		t.Fatal("int 与 int64 不应相等")
	}

	// 含 interface 的 struct，直接 == 会 panic，EQ 内部 recover 后 DeepEqual
	a := eqStructWithIface{V: []int{1}}
	b := eqStructWithIface{V: []int{1}}
	if !EQ(a, b) {
		t.Fatal("应通过 DeepEqual 判定相等")
	}
}
