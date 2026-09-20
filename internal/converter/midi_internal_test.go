package converter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestEscapeLavfiPathForAmovie はescapeLavfiPathForAmovieのwhite-boxテスト。
// amovieフィルタへ渡すパス中のシングルクォート・コロン・バックスラッシュが
// エスケープされることを検証する。期待値は実ffprobe(TestTrailingSilenceDetector_
// silenceStart_SpecialCharacterPaths、midi_amovie_escape_internal_test.go)で
// 実際に動作すると確認済みの変換結果をピン留めする
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

// identifiableRunner は同一性を比較できるCommandRunner。
//
// why not: NewExecCommandRunnerは空の構造体値を返すため、値の比較では
// 「渡したrunnerがそのまま使われたか」と「既定のrunnerへ差し替わったか」を
// 区別できない。ポインタで同一性を比較する（ゼロサイズ型のポインタは別の値でも
// 同じアドレスになり得るため、フィールドを持たせている）。
type identifiableRunner struct{ _ int }

func (*identifiableRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func TestNewMidiConverter_SilenceDetectorDefaults(t *testing.T) {
	t.Parallel()

	customRunner := &identifiableRunner{}
	cases := map[string]struct {
		inputTimeout time.Duration
		runner       CommandRunner
		expected     time.Duration
	}{
		"明示したタイムアウトとnilランナー": {10 * time.Second, nil, 10 * time.Second},
		"未指定のタイムアウトとnilランナー": {0, nil, 300 * time.Second},
		"明示したタイムアウトとランナー":    {10 * time.Second, customRunner, 10 * time.Second},
		"未指定のタイムアウトとランナー":    {0, customRunner, 300 * time.Second},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := NewMidiConverter("soundfont.sf2", 0, "", 0, tc.inputTimeout, tc.runner)

			assert.Equal(t, tc.expected, c.Timeout())
			assert.Equal(t, c.Timeout(), c.silenceDetector.timeout)
			if tc.runner == nil {
				assert.Equal(t, NewExecCommandRunner(), c.silenceDetector.runner)
			} else {
				assert.Same(t, tc.runner, c.silenceDetector.runner)
			}
		})
	}
}
