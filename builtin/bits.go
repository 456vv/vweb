package builtin
	
// a << b
func BitLshr(a, b interface{}) interface{} {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 << uint(b1)
		}
	}
	return panicUnsupportedOp2("<<", a, b)
}

// a >> b
func BitRshr(a, b interface{}) interface{} {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 >> uint(b1)
		}
	}
	return panicUnsupportedOp2(">>", a, b)
}

// a ^ b
func BitXor(a, b interface{}) interface{} {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 ^ b1
		}
	}
	return panicUnsupportedOp2("^", a, b)
}
// a & b
func BitAnd(a, b interface{}) interface{} {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 & b1
		}
	}
	return panicUnsupportedOp2("&", a, b)
}
// a | b
func BitOr(a, b interface{}) interface{} {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 | b1
		}
	}
	return panicUnsupportedOp2("|", a, b)
}
// ^a
func BitNot(a interface{}) interface{} {
	switch a1 := a.(type) {
	case int:
		return ^a1
	}
	return panicUnsupportedOp1("^", a)
}
// a &^ b
func BitAndNot(a, b interface{}) interface{} {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 &^ b1
		}
	}
	return panicUnsupportedOp2("&^", a, b)
}
