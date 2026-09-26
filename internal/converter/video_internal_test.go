package converter

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// copyFileはos.Createでdestを作成するためモードを保持しない。他のcopyFile
// 実装（builder/pipeline）と異なりモードの一致は検証しない。
func TestCopyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		content       []byte
		setup         func(t *testing.T, dir string, content []byte) (src, dst string)
		wantStatError bool
	}{
		{
			name:    "同一パスへのコピーは内容を保持する",
			content: []byte("original content"),
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				path := filepath.Join(dir, "src.mpg")
				require.NoError(t, os.WriteFile(path, content, 0o644))

				return path, path
			},
		},
		{
			name:    "ハードリンク経由のコピーは内容を保持する",
			content: []byte("hard-linked content"),
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.mpg")
				dst := filepath.Join(dir, "linked.mpg")
				require.NoError(t, os.WriteFile(src, content, 0o644))

				if err := os.Link(src, dst); err != nil {
					t.Skipf("このファイルシステムはハードリンクをサポートしません: %v", err)
				}

				return src, dst
			},
		},
		{
			name:    "既存の別ファイルへのコピーは内容が上書きされる",
			content: []byte("distinct content"),
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.mpg")
				dst := filepath.Join(dir, "dst.mpg")
				require.NoError(t, os.WriteFile(src, content, 0o644))
				require.NoError(t, os.WriteFile(dst, []byte("stale content that is much longer than the new one"), 0o644))

				return src, dst
			},
		},
		{
			name:    "dstが存在しない場合は新規作成する",
			content: []byte("new file content"),
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.mpg")
				dst := filepath.Join(dir, "nested", "dst.mpg")
				require.NoError(t, os.WriteFile(src, content, 0o644))
				require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o750))

				return src, dst
			},
		},
		{
			name:          "dstの親が通常ファイルの場合はstatエラーを伝播する",
			wantStatError: true,
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.mpg")
				require.NoError(t, os.WriteFile(src, content, 0o644))

				notADir := filepath.Join(dir, "not-a-dir")
				require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o644))

				return src, filepath.Join(notADir, "dst.mpg")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			src, dst := tt.setup(t, dir, tt.content)

			err := copyFile(src, dst)

			if tt.wantStatError {
				var pathErr *fs.PathError
				require.ErrorAs(t, err, &pathErr)
				assert.Equal(t, "stat", pathErr.Op)

				return
			}

			require.NoError(t, err)
			got, readErr := os.ReadFile(dst)
			require.NoError(t, readErr)
			assert.Equal(t, tt.content, got)
		})
	}
}
