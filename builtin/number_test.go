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
