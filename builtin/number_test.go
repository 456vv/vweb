package builtin

import (
	"math"
	"reflect"
	"testing"
)

func TestCompute(t *testing.T) {
	tests := []struct {
		name    string
		x       any
		symbol  string
		y       any
		want    any
		wantErr bool
	}{
		{"int+int", int(10), "+", int(20), int(30), false},
		{"int8+int8 (type check)", int8(10), "+", int8(20), int8(30), false},
		{"float64+float64", float64(10.5), "+", float64(2.5), float64(13.0), false},
		{"string+string", "hello", "+", "world", "helloworld", false},
		{"string unsupported op", "hello", "-", "world", nil, true},
		{"int divide by zero", int(10), "/", int(0), nil, true},
		{"uint modulo by zero", uint(10), "%", uint(0), nil, true},
		{"int+float64 types", int(10), "+", float64(10), int(20), false},
		{"shift operation", int32(2), "<<", int32(3), int32(16), false},
		{"nil values", nil, "+", nil, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Compute(tt.x, tt.symbol, tt.y)
			if (err != nil) != tt.wantErr {
				t.Errorf("%s in Compute() error = %v, wantErr %v", tt.name, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Compute() got = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestCompute_Shift(t *testing.T) {
	t.Run("int32 << int32", func(t *testing.T) {
		v, err := Compute(int32(2), "<<", int32(3))
		if err != nil || v != int32(16) {
			t.Fatalf("got %v %v", v, err)
		}
	})
	t.Run("负移位 count", func(t *testing.T) {
		if _, err := Compute(1, "<<", -1); err == nil {
			t.Fatal("负移位应报错")
		}
	})
	t.Run("unsigned 移位 count", func(t *testing.T) {
		v, err := Compute(uint(1), "<<", uint(3))
		if err != nil || v != uint(8) {
			t.Fatalf("got %v %v", v, err)
		}
	})
	t.Run("移位超过位宽", func(t *testing.T) {
		v, err := Compute(int32(1), "<<", int32(40))
		if err != nil || v != int32(0) {
			t.Fatalf("应环绕为 0，got %v %v", v, err)
		}
	})
	t.Run("有符号右移负数", func(t *testing.T) {
		v, err := Compute(int32(-8), ">>", int32(2))
		if err != nil || v != int32(-2) {
			t.Fatalf("got %v %v", v, err)
		}
	})
	t.Run("有符号右移超过位宽", func(t *testing.T) {
		v, err := Compute(int32(-1), ">>", int32(40))
		if err != nil || v != int32(-1) {
			t.Fatalf("负数右移超位宽应保持 -1，got %v %v", v, err)
		}
	})
}

func TestCompute_DivideByZero(t *testing.T) {
	if _, err := Compute(1, "/", 0); err == nil {
		t.Fatal("int / 0 应报错")
	}
	if _, err := Compute(1, "%", 0); err == nil {
		t.Fatal("int %% 0 应报错")
	}
	if _, err := Compute(uint(1), "/", uint(0)); err == nil {
		t.Fatal("uint / 0 应报错")
	}
}

func TestCompute_IntMinDivNegOne(t *testing.T) {
	if _, err := Compute(math.MinInt64, "/", int64(-1)); err == nil {
		t.Fatal("min / -1 应报错")
	}
	if _, err := Compute(math.MinInt64, "%", int64(-1)); err == nil {
		t.Fatal("min %% -1 应报错")
	}
}

func TestCompute_IntToUintOverflow(t *testing.T) {
	v, err := Compute(int8(-1), "+", uint8(1))
	if err != nil || v != int8(0) {
		t.Fatalf("got %v %v", v, err)
	}
}

func TestCompute_UintWrap(t *testing.T) {
	v, err := Compute(uint8(0), "-", uint8(1))
	if err != nil || v != uint8(255) {
		t.Fatalf("got %v %v", v, err)
	}
}

func TestCompute_Float32(t *testing.T) {
	v, err := Compute(float32(0.1), "+", float32(0.2))
	if err != nil {
		t.Fatal(err)
	}
	if v.(float32) != float32(0.1)+float32(0.2) {
		t.Fatalf("float32 精度错误：%v", v)
	}
}

func TestCompute_Complex(t *testing.T) {
	v, err := Compute(complex64(1+2i), "+", complex64(3+4i))
	if err != nil || v != complex64(4+6i) {
		t.Fatalf("got %v %v", v, err)
	}
	if _, err := Compute(complex(1, 1), "<", complex(2, 2)); err == nil {
		t.Fatal("复数不支持比较，应报错")
	}
}

func TestCompute_NilValue(t *testing.T) {
	if _, err := Compute(nil, "+", 1); err == nil {
		t.Fatal("x 为 nil 应报错")
	}
	if _, err := Compute(1, "+", nil); err == nil {
		t.Fatal("y 为 nil 应报错")
	}
}

func TestCompute_StringPlusNonString(t *testing.T) {
	if _, err := Compute("a", "+", []int{1}); err == nil {
		t.Fatal("string + []int 应报错")
	}
}

// TestCompute_Boundary 表驱动覆盖溢出、NaN、类型混算等易错边界。
func TestCompute_Boundary(t *testing.T) {
	cases := []struct {
		name    string
		x       any
		op      string
		y       any
		want    any
		wantErr bool
	}{
		{"int64 MinInt/-1 溢出", math.MinInt64, "/", int64(-1), nil, true},
		{"int64 MinInt%-1 溢出", math.MinInt64, "%", int64(-1), nil, true},
		{"int+float 按左类型", int(10), "+", float64(10), int(20), false},
		{"int8(-1)+uint8(1)", int8(-1), "+", uint8(1), int8(0), false},
		{"uint8 环绕", uint8(0), "-", uint8(1), uint8(255), false},
		{"移位超位宽得 0", int32(1), "<<", int32(40), int32(0), false},
		{"负数右移超位宽保 -1", int32(-1), ">>", int32(40), int32(-1), false},
		{"负移位拒绝", 1, "<<", -1, nil, true},
		{"除 0", 10, "/", 0, nil, true},
		{"string+string", "a", "+", "b", "ab", false},
		{"string-拒绝", "a", "-", "b", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Compute(c.x, c.op, c.y)
			if c.wantErr {
				if err == nil {
					t.Fatalf("应报错，实得 %v", got)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got=%v err=%v want=%v", got, err, c.want)
			}
		})
	}
}
