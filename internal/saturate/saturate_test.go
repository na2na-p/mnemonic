package saturate_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/na2na-p/mnemonic/internal/saturate"
)

func TestAdd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    int64
		b    int64
		want int64
	}{
		{name: "正常系: 0同士の和は0", a: 0, b: 0, want: 0},
		{name: "正常系: int64に収まる和はそのまま返す", a: 3, b: 4, want: 7},
		{name: "正常系: 和がちょうどmath.MaxInt64ならそのまま返す", a: math.MaxInt64 - 1, b: 1, want: math.MaxInt64},
		{name: "正常系: math.MaxInt64に0を足すとmath.MaxInt64", a: math.MaxInt64, b: 0, want: math.MaxInt64},
		{name: "境界値: 和がint64を1超えるとmath.MaxInt64に飽和する", a: math.MaxInt64, b: 1, want: math.MaxInt64},
		{name: "境界値: 両方がmath.MaxInt64でもmath.MaxInt64に飽和する", a: math.MaxInt64, b: math.MaxInt64, want: math.MaxInt64},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, saturate.Add(tc.a, tc.b))
		})
	}
}

func TestMul(t *testing.T) {
	t.Parallel()

	const factor int64 = 1032

	cases := []struct {
		name string
		a    int64
		b    int64
		want int64
	}{
		{"正常系: 0との積は0", 0, factor, 0},
		{"正常系: int64に収まる積はそのまま返す", 8, factor, 8 * 1032},
		{"正常系: 積がちょうどint64に収まる境界ではそのまま返す", math.MaxInt64 / factor, factor, math.MaxInt64 / factor * factor},
		{"正常系: 積がint64を超える場合は最大値で飽和する", math.MaxInt64/factor + 1, factor, math.MaxInt64},
		{"正常系: 最大値同士の積も最大値で飽和する", math.MaxInt64, math.MaxInt64, math.MaxInt64},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, saturate.Mul(tc.a, tc.b))
		})
	}
}
