package vweb

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/issue9/assert/v2"
)

type TForMethod struct{}

func (tf *TForMethod) A1() {}
func (tf *TForMethod) A2() {}
func (tf *TForMethod) A3() {}
func (tf *TForMethod) a4() {}
func Test_ForMethod(t *testing.T) {
	tForMethod := &TForMethod{}
	t.Logf("\n%s", ForMethod(tForMethod))
}

type TForType struct {
	a int
	b string
	c float32
}

func Test_ForType(t *testing.T) {
	tForType := &TForType{}
	t.Logf("\n%s", ForType(tForType, false))
}

func Test_TypeSelect(t *testing.T) {
	var i int = 19
	t.Logf("%#v", typeSelect(reflect.ValueOf(i)))
}

func Test_InDirect(t *testing.T) {
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
	as := assert.New(t, true)
	a := A{}
	v, err := DepthField(a, "B", "C", "D")
	as.Error(err).Nil(v)

	v, err = DepthField(a, "B", "C")
	as.NotError(err).Nil(v)

	a.B.F = map[string]string{"1": "a"}
	v, err = DepthField(a, "B", "F", "1")
	as.NotError(err).Equal(v, "a")

	a.B.G = []string{"1"}
	v, err = DepthField(a, "B", "G", 0)
	as.NotError(err).Equal(v, "1")
}

func Test_CopyStruct(t *testing.T) {
	as := assert.New(t, true)
	a := A{
		B: B{
			F: map[string]string{"2": "2"},
		},
	}
	b := A{
		B: B{
			C: &C{D: 1},
			F: map[string]string{"1": "1"},
		},
	}
	err := CopyStruct(&a, &b, nil)
	as.NotError(err).Equal(&a, &b)

	as.NotEqual(a.B.F["2"], "2")

	delete(b.B.F, "1")
	as.Equal(&a, &b)

	a.D = 2
	as.Equal(&a, &b)
}

func Test_CopyStructDeep(t *testing.T) {
	as := assert.New(t, true)
	a := A{
		B: B{
			F: map[string]string{"2": "2"},
		},
	}
	b := A{
		B: B{
			C: &C{D: 1},
			F: map[string]string{"1": "1"},
		},
	}
	err := CopyStructDeep(&a, &b, nil)
	as.NotError(err).Length(a.B.F, 2)

	delete(a.B.F, "1")
	as.NotEqual(a, b)
	as.Equal(a.B.C, b.B.C).NotEqual(unsafe.Pointer(a.B.C), unsafe.Pointer(b.B.C))

	a.D = 2
	as.NotEqual(a.C, b.C)
}
