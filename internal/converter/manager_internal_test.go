package converter

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCalculateWorkersFor はcalculateWorkersForをCPUコア数を固定して検証する
// white-boxテスト（cpuCount引数の直接指定でCPUコア数を固定する）。
func TestCalculateWorkersFor(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		availableMemoryMB *int
		cpuCount          int
		expected          int
	}{
		"正常系: メモリベースの計算(2000MBなら4ワーカー)": {new(2000), 8, 4},
		"正常系: CPUコア数による制限":              {new(10000), 2, 2},
		"正常系: 最小ワーカー数は1":                {new(100), 8, 1},
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

// TestRelativeMessage はrelativeMessageが、rootsのいずれかの配下を指す絶対パスだけを
// そのルートからの相対パスに置き換えることを検証する。
func TestRelativeMessage(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	src := filepath.Join(base, "src")
	dst := filepath.Join(base, "src", "out")
	under := func(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

	cases := map[string]struct {
		message string
		roots   []string
		want    string
	}{
		"正常系: 空白の後に続くルート配下の絶対パスを相対パスにする": {
			message: "同名の " + under(src, "video/op.mpg") + " を優先したため変換しません",
			roots:   []string{src},
			want:    "同名の " + filepath.FromSlash("video/op.mpg") + " を優先したため変換しません",
		},
		"正常系: 文頭の絶対パスも相対パスにする": {
			message: under(src, "bg.tlg") + ": Invalid data",
			roots:   []string{src},
			want:    "bg.tlg: Invalid data",
		},
		"正常系: 入れ子のルートは長い方のルートからの相対パスにする": {
			message: under(dst, "a.png") + " ← " + under(src, "a.tlg"),
			roots:   []string{src, dst},
			want:    "a.png ← a.tlg",
		},
		"正常系: 末尾に区切り文字を持つルートも同じルートとして扱う": {
			message: "open " + under(src, "a.ks"),
			roots:   []string{src + string(filepath.Separator)},
			want:    "open a.ks",
		},
		"正常系: 正規化されていないルートも正規化したルートとして扱う": {
			message: "open " + under(src, "a.ks"),
			roots:   []string{src + string(filepath.Separator) + "."},
			want:    "open a.ks",
		},
		"正常系: ルートが/なら絶対パスを/からの相対パスにする": {
			message: "open " + under(src, "a.ks"),
			roots:   []string{string(filepath.Separator)},
			want:    "open " + strings.TrimPrefix(under(src, "a.ks"), string(filepath.Separator)),
		},
		"正常系: 同じ名前で始まる別のディレクトリは置き換えない": {
			message: "open " + under(src+"2", "a.ks"),
			roots:   []string{src},
			want:    "open " + under(src+"2", "a.ks"),
		},
		"正常系: ルート自体を指すパスは置き換えない": {
			message: "走査に失敗しました: " + src,
			roots:   []string{src},
			want:    "走査に失敗しました: " + src,
		},
		"正常系: 別のパスの途中に現れるルートは置き換えない": {
			message: "open " + filepath.Join(base, "private") + under(src, "a.ks"),
			roots:   []string{src},
			want:    "open " + filepath.Join(base, "private") + under(src, "a.ks"),
		},
		"正常系: 相対パスのルートも配下のパスを相対パスにする": {
			message: "open " + filepath.FromSlash("src/a.ks"),
			roots:   []string{"src"},
			want:    "open a.ks",
		},
		"正常系: ルートが.でも親ディレクトリを指すパスは置き換えない": {
			message: "open " + filepath.FromSlash("../a.ks"),
			roots:   []string{"."},
			want:    "open " + filepath.FromSlash("../a.ks"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, relativeMessage(tc.message, tc.roots...))
		})
	}
}
