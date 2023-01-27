package builtin

// &&
func And(arg0 any, args ...any) bool {
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
func Or(arg0 any, args ...any) bool {
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
func Not(a any) bool {
	switch  a1 := a.(type) {
	case bool:
		return !a1
	case int:
		return a1 == 0
	}
	panicUnsupportedOp1("!", a)
	return false
}

// a < b
func LT(a, b any) bool {
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
	panicUnsupportedOp2("<", a, b)
	return false
}

// a > b
func GT(a, b any) bool {
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
	panicUnsupportedOp2(">", a, b)
	return false
}

// a <= b
func LE(a, b any) bool {
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
	panicUnsupportedOp2("<=", a, b)
	return false
}

// a >= b
func GE(a, b any) bool {
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
	panicUnsupportedOp2(">=", a, b)
	return false
}

// a == b
func EQ(a, b any) bool {
	return a == b
}

// a != b
func NE(a, b any) bool {
	return a != b
}
