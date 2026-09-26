package builder

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSDL2CacheFailureDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "正常系: 原因のあるキャッシュエラーは原因の文言だけを返す",
			err:  &sdl2CacheError{op: "キャッシュ保存に失敗しました", cause: errors.New("mkdir /cache: not a directory")},
			want: "mkdir /cache: not a directory",
		},
		{
			name: "正常系: ラップされたキャッシュエラーも原因の文言だけを返す",
			err:  fmt.Errorf("外側: %w", &sdl2CacheError{op: "キャッシュ復元に失敗しました", cause: errors.New("open /cache/a: permission denied")}),
			want: "open /cache/a: permission denied",
		},
		{
			name: "正常系: 原因の無いキャッシュエラーは操作の文言を返す",
			err:  &sdl2CacheError{op: "有効なキャッシュがありません"},
			want: "有効なキャッシュがありません",
		},
		{
			name: "正常系: キャッシュエラー以外はそのままの文言を返す",
			err:  errors.New("その他のエラー"),
			want: "その他のエラー",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, sdl2CacheFailureDetail(tc.err))
		})
	}
}
