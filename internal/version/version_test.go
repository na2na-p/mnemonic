package version_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/na2na-p/mnemonic/internal/version"
)

func TestString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{
			name: "正常系: デフォルトのバージョン文字列を返す",
			want: "0.1.0-dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := version.String()

			assert.Equal(t, tt.want, got)
		})
	}
}
