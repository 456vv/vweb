package vweb

import (
	"testing"
)

func testExecFunc(a *testing.T, b ...interface{}) *testing.T{
	return a
}

func Test_ExecFunc1(t *testing.T){
	args := []interface{}{t, t, t}
	rets, err := ExecFunc(testExecFunc, t, args)
	if err != nil {
		t.Fatal(err)
	}
	tt, ok := rets[0].(*testing.T)
	if !ok {
		t.Fatal("error")
	}
	if tt != t {
		t.Fatal("error")
	}
}
func Test_ExecFunc2(t *testing.T){
	rets, err := ExecFunc(testExecFunc, t, t,t,t,t)
	if err != nil {
		t.Fatal(err)
	}
	tt, ok := rets[0].(*testing.T)
	if !ok {
		t.Fatal("error")
	}
	if tt != t {
		t.Fatal("error")
	}
}
func Test_ExecFunc3(t *testing.T){
	rets, err := ExecFunc(testExecFunc, t)
	if err != nil {
		t.Fatal(err)
	}
	tt, ok := rets[0].(*testing.T)
	if !ok {
		t.Fatal("error")
	}
	if tt != t {
		t.Fatal("error")
	}
}

func Test_GenerateRandomString(t *testing.T){
	code, err := GenerateRandomString(40)
	if err != nil {
		t.Fatal(err)
	}
	if l := len(code); l != 40 {
		t.Fatalf("生成长度错误，预定 40，结果 %d", l)
	}
}

func Benchmark_GenerateRandomString(t *testing.B){
	var length = 40
	for i:=0;i<t.N;i++ {
		code, err := GenerateRandomString(length)
		if err != nil {
			t.Fatal(err)
		}
		if l := len(code); l != length {
			t.Fatalf("生成长度错误，预定 %d，结果 %d", length, l)
		}
	//	t.Log(code)
	}
}

func Benchmark_AddSalt(t *testing.B){
	var length = 40
	for i:=0;i<t.N;i++ {
		p, err := GenerateRandom(length)
		if err != nil {
			t.Fatal(err)
		}
		code := AddSalt(p,"dkeinifjperiocjopirem")
		if l := len(code); l != length {
			t.Fatalf("生成长度错误，预定 %d，结果 %d", length, l)
		}
		//t.Log(code)
	}

}