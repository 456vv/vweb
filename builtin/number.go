package builtin

import (
	"fmt"
	"reflect"
)

// Compute 对两个值进行动态算术/位运算。
// 支持：string(+)、有符号/无符号整数(+ - * / % & | ^ &^ << >>)、
// float(+ - * /)、complex(+ - * /)。
// 两边必须具有相同的 reflect.Kind，结果类型与左操作数一致。
// 纯函数，并发安全。
func Compute(x any, symbol string, y any) (any, error) {
	if x == nil || y == nil {
		return nil, fmt.Errorf("invalid operation: nil values not supported")
	}

	xx := inDirect(reflect.ValueOf(x))
	yy := inDirect(reflect.ValueOf(y))

	if !xx.IsValid() || !yy.IsValid() {
		return nil, fmt.Errorf("invalid operation: untyped nil")
	}

	if xx.Kind() != yy.Kind() {
		return nil, fmt.Errorf("type mismatch: %v != %v", xx.Kind(), yy.Kind())
	}

	const unsupported = "operator %q not supported for type %s"

	switch xx.Kind() {
	case reflect.String:
		if symbol != "+" {
			return nil, fmt.Errorf(unsupported, symbol, "string")
		}
		return xx.String() + yy.String(), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return computeInt(xx, symbol, yy)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return computeUint(xx, symbol, yy)

	case reflect.Float32, reflect.Float64:
		return computeFloat(xx, symbol, yy)

	case reflect.Complex64, reflect.Complex128:
		return computeComplex(xx, symbol, yy)

	default:
		return nil, fmt.Errorf("unsupported type for computation: %v", xx.Kind())
	}
}

func computeInt(xx reflect.Value, symbol string, yy reflect.Value) (any, error) {
	xi := xx.Int()
	yi := yy.Int()

	if (symbol == "/" || symbol == "%") && yi == 0 {
		return nil, fmt.Errorf("integer divide by zero")
	}
	if (symbol == "<<" || symbol == ">>") && yi < 0 {
		return nil, fmt.Errorf("negative shift count: %d", yi)
	}

	var res int64
	switch symbol {
	case "+":
		res = xi + yi
	case "-":
		res = xi - yi
	case "*":
		res = xi * yi
	case "/":
		res = xi / yi
	case "%":
		res = xi % yi
	case "&":
		res = xi & yi
	case "|":
		res = xi | yi
	case "^":
		res = xi ^ yi
	case "&^":
		res = xi &^ yi
	case "<<":
		res = xi << uint64(yi)
	case ">>":
		res = xi >> uint64(yi)
	default:
		return nil, fmt.Errorf("operator %q not supported for integer type", symbol)
	}

	// 使用 Convert 保证与目标类型位宽一致的环绕语义（与 Go 原生行为一致）
	return reflect.ValueOf(res).Convert(xx.Type()).Interface(), nil
}

func computeUint(xx reflect.Value, symbol string, yy reflect.Value) (any, error) {
	xu := xx.Uint()
	yu := yy.Uint()

	if (symbol == "/" || symbol == "%") && yu == 0 {
		return nil, fmt.Errorf("integer divide by zero")
	}

	var res uint64
	switch symbol {
	case "+":
		res = xu + yu
	case "-":
		res = xu - yu
	case "*":
		res = xu * yu
	case "/":
		res = xu / yu
	case "%":
		res = xu % yu
	case "&":
		res = xu & yu
	case "|":
		res = xu | yu
	case "^":
		res = xu ^ yu
	case "&^":
		res = xu &^ yu
	case "<<":
		res = xu << yu
	case ">>":
		res = xu >> yu
	default:
		return nil, fmt.Errorf("operator %q not supported for unsigned integer type", symbol)
	}

	return reflect.ValueOf(res).Convert(xx.Type()).Interface(), nil
}

func computeFloat(xx reflect.Value, symbol string, yy reflect.Value) (any, error) {
	xf := xx.Float()
	yf := yy.Float()

	var res float64
	switch symbol {
	case "+":
		res = xf + yf
	case "-":
		res = xf - yf
	case "*":
		res = xf * yf
	case "/":
		// 浮点除零产生 ±Inf，属于 IEEE 754 安全行为，不报错
		res = xf / yf
	default:
		return nil, fmt.Errorf("operator %q not supported for float type", symbol)
	}

	return reflect.ValueOf(res).Convert(xx.Type()).Interface(), nil
}

func computeComplex(xx reflect.Value, symbol string, yy reflect.Value) (any, error) {
	xc := xx.Complex()
	yc := yy.Complex()

	var res complex128
	switch symbol {
	case "+":
		res = xc + yc
	case "-":
		res = xc - yc
	case "*":
		res = xc * yc
	case "/":
		res = xc / yc
	default:
		return nil, fmt.Errorf("operator %q not supported for complex type", symbol)
	}

	return reflect.ValueOf(res).Convert(xx.Type()).Interface(), nil
}

// Inc a+1
func Inc(a any) any {
	switch v := a.(type) {
	case int:
		return v + 1
	case uint:
		return v + 1
	case int64:
		return v + 1
	case uint64:
		return v + 1
	case int32:
		return v + 1
	case uint32:
		return v + 1
	case int16:
		return v + 1
	case uint16:
		return v + 1
	case int8:
		return v + 1
	case uint8:
		return v + 1
	case float32:
		return v + 1.0 // 修复盲区: 支持浮点数自增
	case float64:
		return v + 1.0
	}
	return panicUnsupportedOp1("++", a)
}

// Dec a-1
func Dec(a any) any {
	switch v := a.(type) {
	case int:
		return v - 1
	case uint:
		return v - 1
	case int64:
		return v - 1
	case uint64:
		return v - 1
	case int32:
		return v - 1
	case uint32:
		return v - 1
	case int16:
		return v - 1
	case uint16:
		return v - 1
	case int8:
		return v - 1
	case uint8:
		return v - 1
	case float32:
		return v - 1.0 // 修复盲区: 支持浮点数自减
	case float64:
		return v - 1.0
	}
	return panicUnsupportedOp1("--", a)
}

// Neg -a
func Neg(a any) any {
	switch a1 := a.(type) {
	case int:
		return -a1
	case int64:
		return -a1
	case int32:
		return -a1
	case int16:
		return -a1
	case int8:
		return -a1
	case float64:
		return -a1
	case float32:
		return -a1
	}
	// Note: go对于无符号数取负通常合法(回绕)，若有需求可额外添加 uint 系列
	return panicUnsupportedOp1("-", a)
}

// Mul a*b
func Mul(a, b any) any {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 * b1
		case float64:
			return float64(a1) * b1
		}
	case float64:
		switch b1 := b.(type) {
		case int:
			return a1 * float64(b1)
		case float64:
			return a1 * b1
		}
	}
	if result, err := Compute(a, "*", b); err == nil {
		return result
	}
	return panicUnsupportedOp2("*", a, b)
}

// Quo a/b
func Quo(a, b any) any {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			if b1 == 0 {
				panic("integer divide by zero")
			} // 修复除零Panic
			return a1 / b1
		case float64:
			return float64(a1) / b1
		}
	case float64:
		switch b1 := b.(type) {
		case int:
			return a1 / float64(b1)
		case float64:
			return a1 / b1
		}
	}
	if result, err := Compute(a, "/", b); err == nil {
		return result
	}
	return panicUnsupportedOp2("/", a, b)
}

// Mod a%b
func Mod(a, b any) any {
	if a1, ok := a.(int); ok {
		if b1, ok := b.(int); ok {
			if b1 == 0 {
				panic("integer divide by zero")
			} // 修复除零Panic
			return a1 % b1
		}
	}
	if result, err := Compute(a, "%", b); err == nil {
		return result
	}
	return panicUnsupportedOp2("%", a, b)
}

// Add a+b
func Add(a, b any) any {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 + b1
		case float64:
			return float64(a1) + b1
		}
	case float64:
		switch b1 := b.(type) {
		case int:
			return a1 + float64(b1)
		case float64:
			return a1 + b1
		}
	case string:
		if b1, ok := b.(string); ok {
			return a1 + b1
		}
	case uint:
		if b1, ok := b.(int); ok {
			return a1 + uint(b1)
		}
	case uint64:
		if b1, ok := b.(int); ok {
			return a1 + uint64(b1)
		}
	case int64:
		if b1, ok := b.(int); ok {
			return a1 + int64(b1)
		}
	case uint32:
		if b1, ok := b.(int); ok {
			return a1 + uint32(b1)
		}
	case int32:
		if b1, ok := b.(int); ok {
			return a1 + int32(b1)
		}
	case uint16:
		if b1, ok := b.(int); ok {
			return a1 + uint16(b1)
		}
	case int16:
		if b1, ok := b.(int); ok {
			return a1 + int16(b1)
		}
	case uint8:
		if b1, ok := b.(int); ok {
			return a1 + uint8(b1)
		}
	case int8:
		if b1, ok := b.(int); ok {
			return a1 + int8(b1)
		}
	}
	if result, err := Compute(a, "+", b); err == nil {
		return result
	}
	return panicUnsupportedOp2("+", a, b)
}

// Sub a-b
func Sub(a, b any) any {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 - b1
		case float64:
			return float64(a1) - b1
		}
	case float64:
		switch b1 := b.(type) {
		case int:
			return a1 - float64(b1)
		case float64:
			return a1 - b1
		}
	case uint:
		if b1, ok := b.(int); ok {
			return a1 - uint(b1)
		}
	case uint64:
		if b1, ok := b.(int); ok {
			return a1 - uint64(b1)
		}
	case int64:
		if b1, ok := b.(int); ok {
			return a1 - int64(b1)
		}
	case uint32:
		if b1, ok := b.(int); ok {
			return a1 - uint32(b1)
		}
	case int32:
		if b1, ok := b.(int); ok {
			return a1 - int32(b1)
		}
	case uint16:
		if b1, ok := b.(int); ok {
			return a1 - uint16(b1)
		}
	case int16:
		if b1, ok := b.(int); ok {
			return a1 - int16(b1)
		}
	case uint8:
		if b1, ok := b.(int); ok {
			return a1 - uint8(b1)
		}
	case int8:
		if b1, ok := b.(int); ok {
			return a1 - int8(b1)
		}
	}
	if result, err := Compute(a, "-", b); err == nil {
		return result
	}
	return panicUnsupportedOp2("-", a, b)
}
