package fsutil_test

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/fsutil"
)

func TestFreeSpace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    func(t *testing.T) string
		wantErr error
	}{
		{
			name:    "一時ディレクトリの空き容量は正の値で返る",
			path:    func(t *testing.T) string { t.Helper(); return t.TempDir() },
			wantErr: nil,
		},
		{
			name:    "存在しないパスはfs.ErrNotExistに該当するエラーを返す",
			path:    func(t *testing.T) string { t.Helper(); return filepath.Join(t.TempDir(), "missing") },
			wantErr: fs.ErrNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := fsutil.FreeSpace(tt.path(t))

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Positive(t, got)
		})
	}
}
