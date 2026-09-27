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
