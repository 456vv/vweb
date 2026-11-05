package builtin

import (
	"reflect"
	"testing"
)

func TestReviewEQNonNilFunctions(t *testing.T) {
	makeFunc := func(n int) func() int {
		return func() int {
			return n
		}
	}

	a := makeFunc(1)
	b := makeFunc(2)

	if EQ(a, b) {
		t.Fatal("different closures must not compare equal")
	}
	if EQ(a, a) {
		t.Fatal("non-nil functions have no supported equality identity")
	}

	var nilA func() int
	var nilB func() int
	if !EQ(nilA, nilB) {
		t.Fatal("same-typed nil functions must compare equal")
	}
}

func TestReviewComputeMinIntegerDivision(t *testing.T) {
	tests := []struct {
		name string
		x    any
		op   string
		y    any
		want any
	}{
		{
			name: "int8 division",
			x:    int8(-128),
			op:   "/",
			y:    int8(-1),
			want: int8(-128),
		},
		{
			name: "int8 remainder",
			x:    int8(-128),
			op:   "%",
			y:    int8(-1),
			want: int8(0),
		},
		{
			name: "int64 division",
			x:    int64(-1 << 63),
			op:   "/",
			y:    int64(-1),
			want: int64(-1 << 63),
		},
		{
			name: "int64 remainder",
			x:    int64(-1 << 63),
			op:   "%",
			y:    int64(-1),
			want: int64(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Compute(tt.x, tt.op, tt.y)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf(
					"got %T(%v), want %T(%v)",
					got,
					got,
					tt.want,
					tt.want,
				)
			}
		})
	}
}

func TestReviewInitSelfReferencingInterface(t *testing.T) {
	var x any
	x = &x

	if Init(&x, false) {
		t.Fatal("cyclic interface initialization must fail within depth limit")
	}
}
