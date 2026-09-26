package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// isSupportedEncodingはパッケージ非公開ヘルパーであり、chardetの推定結果に依存せず
// 任意のエンコーディング名を与えて判定を検証するため、ホワイトボックステストとする。
func TestIsSupportedEncoding(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		enc  string
		want bool
	}{
		"正常系: SupportedEncodingsに無い名前は非対応と判定する": {"klingon", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isSupportedEncoding(tc.enc))
		})
	}
}
