package converter

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "dstの親が通常ファイルの場合はコピー失敗として包んだstatエラーを返す"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			src := filepath.Join(dir, "src.mpg")
			require.NoError(t, os.WriteFile(src, []byte("content"), 0o644))
			notADir := filepath.Join(dir, "not-a-dir")
			require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o644))

			err := copyFile(src, filepath.Join(notADir, "dst.mpg"))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "ファイルのコピーに失敗しました")
			pathErr, ok := errors.AsType[*fs.PathError](err)
			require.True(t, ok, "want *fs.PathError, got %v", err)
			assert.Equal(t, "stat", pathErr.Op)
		})
	}
}
