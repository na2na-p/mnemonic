package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEscapeLavfiPathForAmovie はescapeLavfiPathForAmovieのwhite-boxテスト。
// amovieフィルタへ渡すパス中のシングルクォートがエスケープされることを検証する。
func TestEscapeLavfiPathForAmovie(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		path     string
		expected string
	}{
		"正常系: 特殊文字を含まないパスはそのまま":      {"/tmp/1234567.wav", "/tmp/1234567.wav"},
		"異常系: シングルクォートを含むパスはエスケープする": {"/tmp/it's.wav", `/tmp/it'\''s.wav`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, escapeLavfiPathForAmovie(tc.path))
		})
	}
}
