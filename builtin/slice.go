package builtin

import (
	"cmp"
)

func Max[T cmp.Ordered](first T, rest ...T) T {
	max := first
	for _, v := range rest {
		if v > max {
			max = v
		}
	}
	return max
}

func Min[T cmp.Ordered](first T, rest ...T) T {
	min := first
	for _, v := range rest {
		if v < min {
			min = v
		}
	}
	return min
}
