package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// fnmatch.goはパッケージ非公開ヘルパー(matchGlob)のみを提供するため、
// ホワイトボックス（同一パッケージ）テストとして検証する。
func TestMatchGlob(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		target  string
		pattern string
		want    bool
	}{
		{"正常系: 単純な拡張子マッチ", "backup.bak", "*.bak", true},
		{"正常系: 拡張子不一致", "test.ks", "*.bak", false},
		{"正常系: *はパス区切りを越えてマッチする", "voice/sub/v001.ogg", "voice/*.ogg", true},
		{"正常系: ディレクトリ階層を含む完全一致パターン", "voice/v001.ogg", "voice/*.ogg", true},
		{"正常系: ?は1文字にマッチする", "a.ks", "?.ks", true},
		{"正常系: ?は2文字にはマッチしない", "ab.ks", "?.ks", false},
		{"正常系: 文字クラスにマッチする", "a.ks", "[ab].ks", true},
		{"正常系: 文字クラスに含まれない文字は不一致", "c.ks", "[ab].ks", false},
		{"正常系: 否定文字クラスにマッチする", "c.ks", "[!ab].ks", true},
		{"正常系: 否定文字クラスは除外文字に不一致", "a.ks", "[!ab].ks", false},
		{"正常系: パターン全体との完全一致が必要", "test.ks.bak", "*.bak", true},
		{"正常系: 部分一致は不可（fullmatch相当）", "test.ks", "test", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := matchGlob(tc.target, tc.pattern)

			assert.Equal(t, tc.want, got)
		})
	}
}
