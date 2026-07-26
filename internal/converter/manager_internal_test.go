package converter

import "testing"

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
			if got != tc.expected {
				t.Errorf("calculateWorkersFor(%v, %d) = %d, want %d", tc.availableMemoryMB, tc.cpuCount, got, tc.expected)
			}
		})
	}
}

func intPtr(v int) *int { return &v }
