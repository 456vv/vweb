package builtin

import (
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
		{"int8+int8 (type check)", int8(10), "+", int8(20), int8(30), false}, // 检查是否保留了原始类型
		{"float64+float64", float64(10.5), "+", float64(2.5), float64(13.0), false},
		{"string+string", "hello", "+", "world", "helloworld", false},
		{"string unsupported op", "hello", "-", "world", nil, true},
		{"int divide by zero", int(10), "/", int(0), nil, true},
		{"uint modulo by zero", uint(10), "%", uint(0), nil, true},
		{"mismatched types", int(10), "+", float64(10), nil, true},
		{"shift operation", int32(2), "<<", int32(3), int32(16), false},
		{"nil values", nil, "+", nil, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Compute(tt.x, tt.symbol, tt.y)
			if (err != nil) != tt.wantErr {
				t.Errorf("Compute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Compute() got = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestIncDec(t *testing.T) {
	// 测试正常逻辑
	if got := Inc(int(10)); got != int(11) {
		t.Errorf("Inc() = %v, want %v", got, 11)
	}
	if got := Dec(float32(10.5)); got != float32(9.5) {
		t.Errorf("Dec() = %v, want %v", got, 9.5)
	}

	// 测试异常 Panic (使用闭包和 recover 捕获)
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic on unsupported type")
		}
	}()
	Inc("string-cannot-inc")
}

func TestNeg(t *testing.T) {
	if got := Neg(int(-10)); got != int(10) {
		t.Errorf("Neg() = %v, want %v", got, 10)
	}
	if got := Neg(float64(10.5)); got != float64(-10.5) {
		t.Errorf("Neg() = %v, want %v", got, -10.5)
	}
	if got := Neg(int8(5)); got != int8(-5) {
		t.Errorf("Neg() = %v, want %v", got, -5)
	}
}

func TestMathOperations(t *testing.T) {
	// 测试 Add
	if got := Add(int(10), int(20)); got != int(30) {
		t.Errorf("Add(10,20) = %v, want 30", got)
	}
	if got := Add("foo", "bar"); got != "foobar" {
		t.Errorf("Add('foo','bar') = %v, want 'foobar'", got)
	}
	// 测试跨类型 Add (命中 fast path)
	if got := Add(uint32(10), int(5)); got != uint32(15) {
		t.Errorf("Add(uint32(10), int(5)) = %v, want uint32(15)", got)
	}

	// 测试 Sub
	if got := Sub(float64(10.5), float64(2.5)); got != float64(8.0) {
		t.Errorf("Sub(10.5, 2.5) = %v, want 8.0", got)
	}

	// 测试 Mul
	if got := Mul(int(5), float64(2.5)); got != float64(12.5) {
		t.Errorf("Mul(5, 2.5) = %v, want 12.5", got)
	}

	// 测试 Quo
	if got := Quo(int(10), int(2)); got != int(5) {
		t.Errorf("Quo(10, 2) = %v, want 5", got)
	}

	// 测试 Mod 降级到 Compute
	if got := Mod(int8(10), int8(3)); got != int8(1) {
		t.Errorf("Mod(int8(10), int8(3)) = %v, want int8(1)", got)
	}
}

func TestDivideByZeroPanics(t *testing.T) {
	assertPanic := func(name string, f func()) {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Expected panic in %s, but got none", name)
				}
			}()
			f()
		})
	}

	assertPanic("Quo divide by zero", func() { Quo(int(10), int(0)) })
	assertPanic("Mod divide by zero", func() { Mod(int(10), int(0)) })
}
