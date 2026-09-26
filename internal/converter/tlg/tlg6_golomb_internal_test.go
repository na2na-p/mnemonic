package tlg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTLG6GolombBitLengthTable(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 圧縮表の各列を展開するとちょうど表の行数になる", func(t *testing.T) {
		t.Parallel()

		for n, counts := range tlg6GolombCompressed {
			total := 0
			for _, count := range counts {
				total += count
			}

			assert.Equal(t, tlg6GolombTableRows, total, "n=%d", n)
		}
	})

	// 期待値はkrkrz TVPTLG6InitGolombTableの展開規則（列nの先頭から
	// TVPTLG6GolombCompressed[n][i]行ずつ値iを並べる）から手で求めたもの。
	cases := map[string]struct {
		a, n     int
		expected uint8
	}{
		"正常系: 先頭行・列3はk=0":        {0, 3, 0},
		"正常系: 列3は3行目からk=1":       {2, 3, 1},
		"正常系: 列0は3行目までk=0":       {2, 0, 0},
		"正常系: 列0の4行目はk=1":        {3, 0, 1},
		"正常系: 最終行・列0はk=8":        {tlg6GolombTableRows - 1, 0, 8},
		"正常系: 列3のk=8は最後の511行":    {tlg6GolombTableRows - 511, 3, 8},
		"正常系: 列3のk=8直前の行はk=7":    {tlg6GolombTableRows - 512, 3, 7},
		"正常系: 列1の境界3+5+13行目でk=3": {3 + 5 + 13, 1, 3},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, tlg6GolombBitLengthTable[tc.a][tc.n])
		})
	}
}
