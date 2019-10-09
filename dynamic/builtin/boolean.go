package builtin
	
func And(arg0 interface{}, args ...interface{}) interface{} {
	ok := Bool(arg0)
	if !ok {
		return ok
	}
	for i := range args {
		ok = Bool(args[i])
		if !ok {
			break
		}
	}
	return ok
}
func Or(arg0 interface{}, args ...interface{}) interface{} {
	ok := Bool(arg0)
	if ok {
		return ok
	}
	for i := range args {
		ok = Bool(args[i])
		if ok {
			break
		}
	}
	return ok
}
//Not returns !a
func Not(a interface{}) interface{} {
	switch  a1 := a.(type) {
	case bool:
		return !a1
	case int:
		return a1 == 0
	}
	return panicUnsupportedOp1("!", a)
}
//LT returns a < b
func LT(a, b interface{}) interface{} {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 < b1
		case float64:
			return float64(a1) < b1
		}
	case float64:
		switch b1 := b.(type) {
		case int:
			return a1 < float64(b1)
		case float64:
			return a1 < b1
		}
	case string:
		if b1, ok := b.(string); ok {
			return a1 < b1
		}
	}
	return panicUnsupportedOp2("<", a, b)
}
//GT returns a > b
func GT(a, b interface{}) interface{} {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 > b1
		case float64:
			return float64(a1) > b1
		}
	case float64:
		switch b1 := b.(type) {
		case int:
			return a1 > float64(b1)
		case float64:
			return a1 > b1
		}
	case string:
		if b1, ok := b.(string); ok {
			return a1 > b1
		}
	}
	return panicUnsupportedOp2(">", a, b)
}
//LE returns a <= b
func LE(a, b interface{}) interface{} {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 <= b1
		case float64:
			return float64(a1) <= b1
		}
	case float64:
		switch b1 := b.(type) {
		case int:
			return a1 <= float64(b1)
		case float64:
			return a1 <= b1
		}
	case string:
		if b1, ok := b.(string); ok {
			return a1 <= b1
		}
	}
	return panicUnsupportedOp2("<=", a, b)
}
//GE returns a >= b
func GE(a, b interface{}) interface{} {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 >= b1
		case float64:
			return float64(a1) >= b1
		}
	case float64:
		switch b1 := b.(type) {
		case int:
			return a1 >= float64(b1)
		case float64:
			return a1 >= b1
		}
	case string:
		if b1, ok := b.(string); ok {
			return a1 >= b1
		}
	}
	return panicUnsupportedOp2(">=", a, b)
}
//EQ returns a == b
func EQ(a, b interface{}) interface{} {
	return a == b
}
//NE returns a != b
func NE(a, b interface{}) interface{} {
	return a != b
}
