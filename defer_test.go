package vweb

import (
	"reflect"
	"testing"
	"time"
)

func Test_exitCall_Defer(t *testing.T) {
	test1 := func(a, b, c string) bool { return a == "1" }
	test2 := func(a, b string, c ...string) bool { return a == "1" }
	test3 := func(a, b string, c []string) bool { return a == "1" }
	test4 := func() bool { return true }

	ns := ExitCall{}
	if err := ns.Defer(test1, "1", "2", "3"); err != nil {
		t.Fatal(err)
	}
	if err := ns.Defer(test1, "1", "2", "3", "4"); err == nil {
		t.Fatal("test1错误")
	}
	if err := ns.Defer(test2, "1", "2", "3", "4"); err != nil {
		t.Fatal(err)
	}
	if err := ns.Defer(test2, "1", "2", []string{"3", "4"}); err == nil {
		t.Fatal("test2错误")
	}
	if err := ns.Defer(test2, "1", "2", []int{2, 3}); err == nil {
		t.Fatal("test2错误")
	}
	if err := ns.Defer(test3, "1", "2", []string{"3", "4"}); err != nil {
		t.Fatal(err)
	}
	if err := ns.Defer(test3, "1", "2", []int{2, 3}); err == nil {
		t.Fatal("test3错误")
	}
	if err := ns.Defer(test4); err != nil {
		t.Fatal("test4错误")
	}
	if err := ns.Defer(test4, "1"); err == nil {
		t.Fatal("test4错误")
	}
}

func Test_exitCall_executeDefer(t *testing.T) {
	var ok bool
	test1 := func(a, b, c string) bool {
		ok = true
		return a == "1"
	}
	ns := ExitCall{}
	if err := ns.Defer(test1, "1", "2", "3"); err != nil {
		t.Fatal(err)
	}
	ns.Free()
	time.Sleep(time.Second)
	if ns.efs != nil || !ok {
		t.Fatalf("不能执行退出调用函数")
	}
}

func Test_ExecCall_Func_InvalidInputs(t *testing.T) {
	cases := []struct {
		name string
		call any
	}{
		{"nil", nil},
		{"not func", 42},
		{"nil ptr func", (*func())(nil)},
		{"nil func value", (func())(nil)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ec ExecCall
			if err := ec.Func(c.call); err == nil {
				t.Fatal("期望错误")
			}
		})
	}
}

func Test_ExecCall_Func_ArgCountMismatch(t *testing.T) {
	var ec ExecCall
	if err := ec.Func(func(a, b int) {}, 1); err == nil {
		t.Fatal("参数不足应报错")
	}
	ec = ExecCall{}
	if err := ec.Func(func(a int) {}, 1, 2); err == nil {
		t.Fatal("参数过多应报错")
	}
	ec = ExecCall{}
	if err := ec.Func(func(a int) {}, "x"); err == nil {
		t.Fatal("参数类型不兼容应报错")
	}
}

func Test_ExecCall_Func_NilArgUsesZero(t *testing.T) {
	var got any = "unset"
	var ec ExecCall
	if err := ec.Func(func(p *int) { got = p }, nil); err != nil {
		t.Fatal(err)
	}
	ec.Exec()
	if got != (*int)(nil) {
		t.Fatalf("nil 参数应转为零值指针，得到 %v", got)
	}
}

func Test_ExitCall_DeferAfterFree(t *testing.T) {
	var ec ExitCall
	ec.Free()
	if err := ec.Defer(func() {}); err == nil {
		t.Fatal("Free 后再 Defer 应报错")
	}
}

func Test_ExitCall_FreeIdempotent(t *testing.T) {
	var ec ExitCall
	var n int
	ec.Defer(func() { n++ })
	ec.Free()
	ec.Free()
	if n != 1 {
		t.Fatalf("Free 幂等，实际 %d 次", n)
	}
}

func Test_ExitCall_Free_PanicIsolation(t *testing.T) {
	var ec ExitCall
	called := 0
	ec.Defer(func() { called++ })
	ec.Defer(func() { panic("boom") })
	ec.Defer(func() { called++ })
	ec.Free()
	if called != 2 {
		t.Fatalf("panic 隔离失败，called=%d", called)
	}
}

func Test_ExitCall_Free_ReverseOrder(t *testing.T) {
	var ec ExitCall
	order := make([]int, 0, 3)
	for i := 1; i <= 3; i++ {
		i := i
		ec.Defer(func() { order = append(order, i) })
	}
	ec.Free()
	want := []int{3, 2, 1}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("顺序=%v，期望 %v", order, want)
		}
	}
}

func Test_ExecCall_Exec_ReturnsNilWhenNoOutput(t *testing.T) {
	var ec ExecCall
	ec.Func(func() {})
	if got := ec.Exec(); got != nil {
		t.Fatalf("无返回值应返回 nil，得到 %v", got)
	}
}

func Test_ExecCall_Exec_Uninitialized(t *testing.T) {
	var ec ExecCall
	if got := ec.Exec(); got != nil {
		t.Fatalf("未初始化应返回 nil，得到 %v", got)
	}
}

// TestExitCall_Boundary 幂等、逆序、panic 隔离、Free 后拒绝。
func TestExitCall_Boundary(t *testing.T) {
	t.Run("Free 幂等且逆序", func(t *testing.T) {
		var ec ExitCall
		var order []int
		for i := 1; i <= 3; i++ {
			i := i
			ec.Defer(func() { order = append(order, i) })
		}
		ec.Free()
		ec.Free()
		if !reflect.DeepEqual(order, []int{3, 2, 1}) {
			t.Fatalf("顺序=%v", order)
		}
	})

	t.Run("单 Defer panic 不影响后续", func(t *testing.T) {
		var ec ExitCall
		var n int
		ec.Defer(func() { n++ })
		ec.Defer(func() { panic("boom") })
		ec.Defer(func() { n++ })
		ec.Free()
		if n != 2 {
			t.Fatalf("panic 隔离失败 n=%d", n)
		}
	})

	t.Run("Free 后 Defer 报错", func(t *testing.T) {
		var ec ExitCall
		ec.Free()
		if err := ec.Defer(func() {}); err == nil {
			t.Fatal("Free 后 Defer 应报错")
		}
	})
}
