// Package saturate はint64の飽和演算を提供する。
package saturate

import "math"

// Add は非負のa、bの和を返す。和がint64を超える場合はmath.MaxInt64を返す。
func Add(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}

	return a + b
}

// Mul は非負のaと正のbの積を返す。積がint64を超える場合はmath.MaxInt64を返す。
func Mul(a, b int64) int64 {
	if a > math.MaxInt64/b {
		return math.MaxInt64
	}

	return a * b
}
