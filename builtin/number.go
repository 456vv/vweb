package builtin

import (
	"fmt"
	"reflect"
)

// Compute 动态计算反射值
func Compute(x any, symbol string, y any) (i any, err error) {
	if x == nil || y == nil {
		return nil, fmt.Errorf("invalid operation: nil values not supported")
	}

	xx := inDirect(reflect.ValueOf(x))
	yy := inDirect(reflect.ValueOf(y))

	if !xx.IsValid() || !yy.IsValid() {
		return nil, fmt.Errorf("invalid operation: untyped nil")
	}

	es := "Algorithms not supported by this type(%s)?"
	if xx.Kind() != yy.Kind() {
		return 0, fmt.Errorf("two types are not equal? %v != %v", xx.Kind(), yy.Kind())
	}

	switch xx.Kind() {
	case reflect.String:
		if symbol == "+" {
			return xx.String() + yy.String(), nil
		}
		return nil, fmt.Errorf(es, symbol)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		XI := xx.Int()
		YI := yy.Int()
		var XYI int64

		// 检查除零
		if (symbol == "/" || symbol == "%") && YI == 0 {
			return nil, fmt.Errorf("integer divide by zero")
		}

		switch symbol {
		case "+":
			XYI = XI + YI
		case "-":
			XYI = XI - YI
		case "*":
			XYI = XI * YI
		case "/":
			XYI = XI / YI
		case "%":
			XYI = XI % YI
		case "&":
			XYI = XI & YI
		case "|":
			XYI = XI | YI
		case "^":
			XYI = XI ^ YI
		case "&^":
			XYI = XI &^ YI
		case "<<":
			XYI = XI << uint64(YI) // 修复: 支持有符号数的位移
		case ">>":
			XYI = XI >> uint64(YI)
		default:
			return nil, fmt.Errorf(es, symbol)
		}
		// 修复: 还原为原本的数据类型，而不是统一个返回 int64
		ret := reflect.New(xx.Type()).Elem()
		ret.SetInt(XYI)
		return ret.Interface(), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		XU := xx.Uint()
		YU := yy.Uint()
		var XYU uint64

		// 检查除零
		if (symbol == "/" || symbol == "%") && YU == 0 {
			return nil, fmt.Errorf("integer divide by zero")
		}

		switch symbol {
		case "+":
			XYU = XU + YU
		case "-":
			XYU = XU - YU
		case "*":
			XYU = XU * YU
		case "/":
			XYU = XU / YU
		case "%":
			XYU = XU % YU
		case "&":
			XYU = XU & YU
		case "|":
			XYU = XU | YU
		case "^":
			XYU = XU ^ YU
		case "&^":
			XYU = XU &^ YU
		case "<<":
			XYU = XU << YU
		case ">>":
			XYU = XU >> YU
		default:
			return nil, fmt.Errorf(es, symbol)
		}
		ret := reflect.New(xx.Type()).Elem()
		ret.SetUint(XYU)
		return ret.Interface(), nil

	case reflect.Float32, reflect.Float64:
		XF := xx.Float()
		YF := yy.Float()
		var XYF float64
		// 浮点除零会产生 +Inf, 不会 panic，属于安全行为
		switch symbol {
		case "+":
			XYF = XF + YF
		case "-":
			XYF = XF - YF
		case "*":
			XYF = XF * YF
		case "/":
			XYF = XF / YF
		default:
			return nil, fmt.Errorf(es, symbol)
		}
		ret := reflect.New(xx.Type()).Elem()
		ret.SetFloat(XYF)
		return ret.Interface(), nil

	default:
		return nil, fmt.Errorf("this is a type that does not match the calculation(%v)？", xx.Kind())
	}
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
