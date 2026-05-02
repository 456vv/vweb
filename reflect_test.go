package vweb

import (
	"testing"
)

type TForMethod struct {
	a int
	A int
	b string
	B string
	c float32
	C float32
	d []byte
	D []byte
	e map[string]string
	E map[string]string
}

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
	A int
	b string
	B string
	c float32
	C float32
	d []byte
	D []byte
	e map[string]any
	E map[string]any
	f []*TForMethod
	F []*TForMethod
}

func Test_ForType(t *testing.T) {
	tForType := &TForType{
		a: 100,
		A: 100,
		b: "100",
		B: "100",
		c: 100.00,
		C: 100.00,
		d: []byte{1, 0, 0, 0},
		D: []byte{1, 0, 0, 0},
		e: map[string]any{"a": 1, "b": 2, "c": 3},
		E: map[string]any{"a": 1, "b": 2, "c": 3},
		f: []*TForMethod{{
			a: 100,
			A: 100,
			b: "100",
			B: "100",
			c: 100.00,
			C: 100.00,
			d: []byte{1, 0, 0, 0},
			D: []byte{1, 0, 0, 0},
			e: map[string]string{"a": "1", "b": "2", "c": "3"},
			E: map[string]string{"a": "1", "b": "2", "c": "3"},
		}, {}, {}},
		F: []*TForMethod{{}, {}, {}},
	}
	t.Logf("\n%s", ForType(tForType, true, 3))
}
