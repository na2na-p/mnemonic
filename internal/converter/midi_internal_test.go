package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEscapeLavfiPathForAmovie はescapeLavfiPathForAmovieのwhite-boxテスト。
// amovieフィルタへ渡すパス中のシングルクォート・コロン・バックスラッシュが
// エスケープされることを検証する。期待値は実ffprobe(TestMidiConverter_
// detectTrailingSilenceStart_SpecialCharacterPaths、midi_amovie_escape_
// internal_test.go)で実際に動作すると確認済みの変換結果をピン留めする
// （Windowsのリリースビルドでは一時ディレクトリが`C:\Users\...\Temp`のように
// コロン・バックスラッシュを含むため、この変換が正しく機能する必要がある）。
func TestEscapeLavfiPathForAmovie(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		path     string
		expected string
	}{
		"正常系: 特殊文字を含まないパスはそのまま":      {"/tmp/1234567.wav", "/tmp/1234567.wav"},
		"異常系: シングルクォートを含むパスはエスケープする": {"/tmp/it's.wav", `/tmp/it\\\'s.wav`},
		"異常系: コロンを含むパスはエスケープする":      {`C:\Users\test\out.wav`, `C\\:\\\\Users\\\\test\\\\out.wav`},
		"異常系: バックスラッシュを含むパスはエスケープする": {`\tmp\a.wav`, `\\\\tmp\\\\a.wav`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, escapeLavfiPathForAmovie(tc.path))
		})
	}
}
