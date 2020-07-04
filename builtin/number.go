package builtin

import (
	"reflect"
	"fmt"
)

//Compute(1, "+", 2)
func Compute(x interface{}, symbol string, y interface{}) (i interface{}, err error) {
    xx := reflect.ValueOf(x)
    yy := reflect.ValueOf(y)
    xx = inDirect(xx)
    yy = inDirect(yy)
    es := "Algorithms not supported by this type(%s)?"
    if xx.Kind() != yy.Kind() {
       	return 0, fmt.Errorf("Two types are not equal? %v != %v", xx.Kind(), yy.Kind())
    }
    switch xx.Kind() {
    case reflect.String:
    	XS := xx.String()
        YS := yy.String()
        var XYS string
        switch symbol {
            case "+":XYS = XS+YS
            default:
                err = fmt.Errorf(es, symbol)
        }
        return XYS, err
    case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
        XI := xx.Int()
        YI := yy.Int()
        var XYI int64
        switch symbol {
            case "+":XYI = XI+YI
            case "-":XYI = XI-YI
            case "*":XYI = XI*YI
            case "/":XYI = XI/YI
            case "%":XYI = XI%YI
            case "&":XYI = XI&YI
            case "|":XYI = XI|YI
            case "^":XYI = XI^YI
            case "&^":XYI = XI&^YI
            default:
                err = fmt.Errorf(es, symbol)
        }
        return XYI, err
    case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
        XU := xx.Uint()
        YU := yy.Uint()
        var XYU uint64
        switch symbol {
            case "+":XYU = XU+YU
            case "-":XYU = XU-YU
            case "*":XYU = XU*YU
            case "/":XYU = XU/YU
            case "%":XYU = XU%YU
            case "&":XYU = XU&YU
            case "|":XYU = XU|YU
            case "^":XYU = XU^YU
            case "&^":XYU = XU&^YU
            case "<<":XYU = XU<<YU
            case ">>":XYU = XU>>YU
            default:
                err = fmt.Errorf(es, symbol)
        }
        return XYU, err
    case reflect.Float32, reflect.Float64:
        XF := xx.Float()
        YF := yy.Float()
        var XYF float64
        switch symbol {
            case "+":XYF = XF+YF
            case "-":XYF = XF-YF
            case "*":XYF = XF*YF
            case "/":XYF = XF/YF
            default:
                err = fmt.Errorf(es, symbol)
        }
        return XYF, err
    case reflect.Uintptr:
        XP := xx.UnsafeAddr()
        YP := yy.UnsafeAddr()
        var XYP uintptr
        switch symbol {
            case "+":XYP = XP+YP
            case "-":XYP = XP-YP
            case "*":XYP = XP*YP
            case "/":XYP = XP/YP
            default:
                err = fmt.Errorf(es, symbol)
        }
        return XYP, err
   	default:
   		 return nil, fmt.Errorf("This is a type that does not match the calculation(%v)？", xx.Kind())
    }
}

// a+1
func Inc(a interface{}) interface{} {
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
	case uint8:
		return v + 1
	case int8:
		return v + 1
	case uint16:
		return v + 1
	case int16:
		return v + 1
	}
	return panicUnsupportedOp1("++", a)
}

// a-1
func Dec(a interface{}) interface{} {
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
	case uint8:
		return v - 1
	case int8:
		return v - 1
	case uint16:
		return v - 1
	case int16:
		return v - 1
	}
	return panicUnsupportedOp1("--", a)
}

// -a
func Neg(a interface{}) interface{} {

	switch a1 := a.(type) {
	case int:
		return -a1
	case float64:
		return -a1
	}
	return panicUnsupportedOp1("-", a)
}

// a*b
func Mul(a, b interface{}) interface{} {
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

// a/b
func Quo(a, b interface{}) interface{} {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
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

// a%b
func Mod(a, b interface{}) interface{} {
	if a1, ok := a.(int); ok {
		if b1, ok := b.(int); ok {
			return a1 % b1
		}
	}
	if result, err := Compute(a, "%", b); err == nil {
		return result
	}
	return panicUnsupportedOp2("%", a, b)
}

// a+b
func Add(a, b interface{}) interface{} {
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
		switch b1 := b.(type) {
		case int:
			return a1 + uint(b1)
		}
	case uint64:
		switch b1 := b.(type) {
		case int:
			return a1 + uint64(b1)
		}
	case int64:
		switch b1 := b.(type) {
		case int:
			return a1 + int64(b1)
		}
	case uint32:
		switch b1 := b.(type) {
		case int:
			return a1 + uint32(b1)
		}
	case int32:
		switch b1 := b.(type) {
		case int:
			return a1 + int32(b1)
		}
	case uint16:
		switch b1 := b.(type) {
		case int:
			return a1 + uint16(b1)
		}
	case int16:
		switch b1 := b.(type) {
		case int:
			return a1 + int16(b1)
		}
	case uint8:
		switch b1 := b.(type) {
		case int:
			return a1 + uint8(b1)
		}
	case int8:
		switch b1 := b.(type) {
		case int:
			return a1 + int8(b1)
		}
	}
	if result, err := Compute(a, "+", b); err == nil {
		return result
	}
	return panicUnsupportedOp2("+", a, b)
}

//a-b
func Sub(a, b interface{}) interface{} {
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
		switch b1 := b.(type) {
		case int:
			return a1 - uint(b1)
		}
	case uint64:
		switch b1 := b.(type) {
		case int:
			return a1 - uint64(b1)
		}
	case int64:
		switch b1 := b.(type) {
		case int:
			return a1 - int64(b1)
		}
	case uint32:
		switch b1 := b.(type) {
		case int:
			return a1 - uint32(b1)
		}
	case int32:
		switch b1 := b.(type) {
		case int:
			return a1 - int32(b1)
		}
	case uint16:
		switch b1 := b.(type) {
		case int:
			return a1 - uint16(b1)
		}
	case int16:
		switch b1 := b.(type) {
		case int:
			return a1 - int16(b1)
		}
	case uint8:
		switch b1 := b.(type) {
		case int:
			return a1 - uint8(b1)
		}
	case int8:
		switch b1 := b.(type) {
		case int:
			return a1 - int8(b1)
		}
	}
	if result, err := Compute(a, "-", b); err == nil {
		return result
	}
	return panicUnsupportedOp2("-", a, b)
}
