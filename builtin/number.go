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

// 如果要按位翻转，-1^1 == ^1
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
