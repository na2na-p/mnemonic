package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCalculateWorkersFor はcalculateWorkersForをCPUコア数を固定して検証する
// white-boxテスト。Python版test_manager.pyのTestCalculateWorkersに相当する
// （os.cpu_countのpatchに相当する処理をcpuCount引数の直接指定で代替する）。
func TestCalculateWorkersFor(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		availableMemoryMB *int
		cpuCount          int
		expected          int
	}{
		"正常系: メモリベースの計算(2000MBなら4ワーカー)": {intPtr(2000), 8, 4},
		"正常系: CPUコア数による制限":              {intPtr(10000), 2, 2},
		"正常系: 最小ワーカー数は1":                {intPtr(100), 8, 1},
		"正常系: メモリ指定なしはCPUコア数のみで決定":      {nil, 4, 4},
		"異常系: CPUコア数が0以下でも最小1":          {nil, 0, 1},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := calculateWorkersFor(tc.availableMemoryMB, tc.cpuCount)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func intPtr(v int) *int { return &v }

// TestIsASCII はisASCIIのwhite-boxテスト。ESC(0x1B)がASCII短絡から除外される
// ことをピン留めする（レビュー指摘: encoding.goのASCII短絡実装時の要件）。
func TestIsASCII(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		data     []byte
		expected bool
	}{
		"正常系: 純ASCII文字列":    {[]byte("key=value\n"), true},
		"正常系: 空バイト列":        {[]byte{}, true},
		"異常系: 0x80以上を含む":    {[]byte("caf\xe9"), false},
		"異常系: ESC(0x1B)を含む": {append([]byte{0x1b}, []byte("$B$3$s(B")...), false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, isASCII(tc.data))
		})
	}
}
