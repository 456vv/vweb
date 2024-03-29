package builtin
	
// a << b
func BitLshr(a, b any) int {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 << uint(b1)
		}
	}
	panicUnsupportedOp2("<<", a, b)
	return 0
}

// a >> b
func BitRshr(a, b any) int {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 >> uint(b1)
		}
	}
	panicUnsupportedOp2(">>", a, b)
	return 0
}

// a ^ b
func BitXor(a, b any) int {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 ^ b1
		}
	}
	panicUnsupportedOp2("^", a, b)
	return 0
}
// a & b
func BitAnd(a, b any) int {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 & b1
		}
	}
	panicUnsupportedOp2("&", a, b)
	return 0
}
// a | b
func BitOr(a, b any) int {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 | b1
		}
	}
	panicUnsupportedOp2("|", a, b)
	return 0
}
// ^a
func BitNot(a any) int {
	switch a1 := a.(type) {
	case int:
		return ^a1
	}
	panicUnsupportedOp1("^", a)
	return 0
}
// a &^ b
func BitAndNot(a, b any) int {
	switch a1 := a.(type) {
	case int:
		switch b1 := b.(type) {
		case int:
			return a1 &^ b1
		}
	}
	panicUnsupportedOp2("&^", a, b)
	return 0
}
