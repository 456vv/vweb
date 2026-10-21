package builtin

import (
	"fmt"
	"reflect"
)

// Compute 对两个值进行动态算术/位运算。
// 支持：string(+)、有符号/无符号整数(+ - * / % & | ^ &^ << >>)、
// float(+ - * /)、complex(+ - * /)。
// Kind 不同但均为数值时，按左操作数类型转换。
// 纯函数，并发安全。
func Compute(x any, symbol string, y any) (any, error) {
	if x == nil || y == nil {
		return nil, fmt.Errorf("invalid operation: nil values not supported")
	}

	xx := inDirect(asValue(x))
	yy := inDirect(asValue(y))
	if !xx.IsValid() || !yy.IsValid() {
		return nil, fmt.Errorf("invalid operation: untyped nil")
	}

	// ：Go 允许任意整数类型作移位计数，且不得把 uint 计数先转成左操作数有符号类型。
	if symbol == "<<" || symbol == ">>" {
		return computeShift(xx, symbol, yy)
	}

	if xx.Kind() != yy.Kind() {
		if !isNumericKind(xx.Kind()) || !isNumericKind(yy.Kind()) {
			return nil, fmt.Errorf("type mismatch: %v != %v", xx.Kind(), yy.Kind())
		}
		cv, err := autoConvertReflect(xx.Type(), yy)
		if err != nil {
			return nil, fmt.Errorf("type mismatch: %v != %v", xx.Kind(), yy.Kind())
		}
		yy = cv
	}

	const unsupported = "operator %q not supported for type %s"

	switch xx.Kind() {
	case reflect.String:
		if symbol != "+" {
			return nil, fmt.Errorf(unsupported, symbol, "string")
		}
		sum := xx.String() + yy.String()
		return reflect.ValueOf(sum).Convert(xx.Type()).Interface(), nil
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

func computeShift(xx reflect.Value, symbol string, yy reflect.Value) (any, error) {
	if !isIntKind(xx.Kind()) || !isIntKind(yy.Kind()) {
		return nil, fmt.Errorf("operator %q not supported for type %s", symbol, xx.Kind())
	}

	var sh uint64
	if isSignedKind(yy.Kind()) {
		yi := yy.Int()
		if yi < 0 {
			return nil, fmt.Errorf("negative shift count: %d", yi)
		}
		sh = uint64(yi)
	} else {
		sh = yy.Uint()
	}

	bits := xx.Type().Bits()
	if bits <= 0 {
		bits = 64
	}

	if isSignedKind(xx.Kind()) {
		xi := xx.Int()
		var res int64
		if symbol == "<<" {
			if sh >= uint64(bits) {
				res = 0
			} else {
				res = xi << sh
			}
		} else if sh >= uint64(bits) {
			if xi < 0 {
				res = -1
			}
		} else {
			res = xi >> sh
		}
		return reflect.ValueOf(res).Convert(xx.Type()).Interface(), nil
	}

	xu := xx.Uint()
	var res uint64
	if symbol == "<<" {
		if sh < uint64(bits) {
			res = xu << sh
		}
	} else if sh < uint64(bits) {
		res = xu >> sh
	}
	return reflect.ValueOf(res).Convert(xx.Type()).Interface(), nil
}

// 如果要按位翻转，-1^1 == ^1
func computeInt(xx reflect.Value, symbol string, yy reflect.Value) (any, error) {
	xi := xx.Int()
	yi := yy.Int()

	if (symbol == "/" || symbol == "%") && yi == 0 {
		return nil, fmt.Errorf("integer divide by zero")
	}
	// 规范规定最负整数 / -1 与 % -1 都是溢出。
	// 旧代码只判断 int64，32 位平台的 int 以及其它位宽会 panic 或得到与运行时不一致的结果。
	if symbol == "/" || symbol == "%" {
		bits := xx.Type().Bits()
		if bits < 1 || bits > 64 {
			bits = 64
		}
		minVal := int64(-1) << (bits - 1)
		if yi == -1 && xi == minVal {
			return nil, fmt.Errorf("integer overflow")
		}
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
	default:
		return nil, fmt.Errorf("operator %q not supported for unsigned integer type", symbol)
	}
	return reflect.ValueOf(res).Convert(xx.Type()).Interface(), nil
}

func computeFloat(xx reflect.Value, symbol string, yy reflect.Value) (any, error) {
	// Go 对 float32 用 IEEE 单精度运算，不得先升 float64 再截回。
	if xx.Kind() == reflect.Float32 {
		xf := float32(xx.Float())
		yf := float32(yy.Float())
		var res float32
		switch symbol {
		case "+":
			res = xf + yf
		case "-":
			res = xf - yf
		case "*":
			res = xf * yf
		case "/":
			res = xf / yf
		default:
			return nil, fmt.Errorf("operator %q not supported for float type", symbol)
		}
		return reflect.ValueOf(res).Convert(xx.Type()).Interface(), nil
	}

	xf, yf := xx.Float(), yy.Float()
	var res float64
	switch symbol {
	case "+":
		res = xf + yf
	case "-":
		res = xf - yf
	case "*":
		res = xf * yf
	case "/":
		res = xf / yf
	default:
		return nil, fmt.Errorf("operator %q not supported for float type", symbol)
	}
	return reflect.ValueOf(res).Convert(xx.Type()).Interface(), nil
}

func computeComplex(xx reflect.Value, symbol string, yy reflect.Value) (any, error) {
	// complex64 必须按 complex64 运算，与 float32 路径对称。
	if xx.Kind() == reflect.Complex64 {
		xc := complex64(xx.Complex())
		yc := complex64(yy.Complex())
		var res complex64
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

	xc, yc := xx.Complex(), yy.Complex()
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
