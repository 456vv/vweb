package vweb

import (
	"testing"
	"errors"
)

func testExecFunc(a *testing.T, b ...any) *testing.T{
	return a
}

func Test_ExecFunc1(t *testing.T){
	args := []any{t, t, t}
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

func testExecFuncError(v error) error {
	return v
}
func Test_ExecFunc4(t *testing.T){
	rets, err := ExecFunc(testExecFuncError, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rets[0] != nil {
		t.Fatal("error")
	}
}
func Test_ExecFunc5(t *testing.T){
	err := errors.New("error")
	rets, e := ExecFunc(testExecFuncError, err)
	if e != nil {
		t.Fatal(e)
	}
	rt, ok := rets[0].(error)
	if !ok {
		t.Fatal("error")
	}
	if rt != err {
		t.Fatal("error")
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
	}
}

func Benchmark_AddSalt_1(t *testing.B){
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
func Benchmark_AddSalt_2(t *testing.B){
	var length = 40
	for i:=0;i<t.N;i++ {
		p, err := GenerateRandom(length)
		if err != nil {
			t.Fatal(err)
		}
		code := AddSalt(p,"")
		if l := len(code); l != length {
			t.Fatalf("生成长度错误，预定 %d，结果 %d", length, l)
		}
		//t.Log(code)
	}

}