package vweb
import (
	"testing"
	"time"
)



func Test_exitCall_Defer(t *testing.T){

    var test1 = func(a, b, c string) bool {return a == "1"}
    var test2 = func(a, b string, c ... string) bool {return a == "1"}

    var test3 = func(a, b string, c []string) bool {return a == "1"}
    var test4 = func() bool {return true}

	ns := exitCall{}
	err := ns.Defer(test1, "1", "2", "3")
    if err != nil {t.Fatal(err) }

	err = ns.Defer(test1, "1", "2", "3", "4")
    if err == nil {t.Fatal("test1错误")}

	err = ns.Defer(test2, "1", "2", "3", "4")
    if err != nil {t.Fatal(err)}

	err = ns.Defer(test2, "1", "2", []string{"3","4"})
    if err != nil {t.Fatal(err)}

	err = ns.Defer(test2, "1", "2", []int{2, 3})
    if err == nil {t.Fatal("test2错误")}

	err = ns.Defer(test3, "1", "2", []string{"3","4"})
    if err != nil {t.Fatal(err)}

	err = ns.Defer(test3, "1", "2", []int{2, 3})
    if err == nil {t.Fatal("test3错误")}

	err = ns.Defer(test4)
    if err != nil {t.Fatal("test4错误")}

	err = ns.Defer(test4, "1")
    if err == nil {t.Fatal("test4错误")}

}

func Test_exitCall_executeDefer(t *testing.T){
	var ok bool
    var test1 = func(a, b, c string) bool {
    	ok =true
    	return a == "1"
    }
	ns := exitCall{}
	err := ns.Defer(test1, "1", "2", "3")
    if err != nil {t.Fatal(err) }
	ns.Free()
	time.Sleep(time.Second)
	if ns.expCall != nil || !ok {
		t.Fatalf("不能执行退出调用函数")
	}

}

