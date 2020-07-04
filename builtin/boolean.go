package builtin

// &&
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

// ||
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

// !1
func Not(a interface{}) interface{} {
	switch  a1 := a.(type) {
	case bool:
		return !a1
	case int:
		return a1 == 0
	}
	return panicUnsupportedOp1("!", a)
}

// a < b
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

// a > b
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

// a <= b
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

// a >= b
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

// a == b
func EQ(a, b interface{}) interface{} {
	return a == b
}

// a != b
func NE(a, b interface{}) interface{} {
	return a != b
}
