package vweb

import (
	"testing"
	"reflect"

)

type TForMethod struct{}
func (tf *TForMethod) A1(){}
func (tf *TForMethod) A2(){}
func (tf *TForMethod) A3(){}
func Test_ForMethod(t *testing.T){
	var tForMethod = &TForMethod{}
	t.Logf("\n%s", ForMethod(tForMethod))
}

type TForType struct{
	a	int
	b	string
	c	float32
}
func Test_ForType(t *testing.T){
	var tForType = &TForType{}
	t.Logf("\n%s", ForType(tForType))
}


func Test_TypeSelect(t *testing.T){
	var i int = 19
	t.Logf("%#v", TypeSelect(reflect.ValueOf(i)))
}

func Test_InDirect(t *testing.T){
	var i int = 11
	j := &i
	b := &j
	t.Logf("%#v", inDirect(reflect.ValueOf(&b)))
}


type A struct {
	B
}
type B struct {
	*C
	F map[string]string
	G []string
}
type C struct {
	D int
}
func Test_DepthField(t *testing.T) {
    a := A{}
	v, err := DepthField(a, "B", "C", "D")
    if err == nil {
    	t.Fatalf("错误：由于 *C 默认是空，不可能正确读取到该值(%v)。", v )
    }

	v, err = DepthField(a, "B", "C")
    if err != nil {
    	t.Fatal(err)
    }
    
    a.B.F = map[string]string{"1":"a"}
	v, err = DepthField(a, "B", "F", "1")
    if err != nil {
    	t.Fatal(err)
    }
    
    a.B.G = []string{"1"}
	v, err = DepthField(a, "B", "G", 0)
    if err != nil {
    	t.Fatal(err)
    }
}
