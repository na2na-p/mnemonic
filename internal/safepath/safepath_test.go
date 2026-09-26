package safepath_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/safepath"
)

func TestJoin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		entryName string
		wantPath  func(root, base string) string
		wantErr   bool
	}{
		{
			name:      "正常系: 入れ子の相対パスを結合する",
			entryName: "nested/file.txt",
			wantPath:  func(_ string, base string) string { return filepath.Join(base, "nested", "file.txt") },
		},
		{
			name:      "異常系: 一階層上への脱出を拒否する",
			entryName: "../evil.txt",
			wantErr:   true,
		},
		{
			name:      "異常系: 二階層上への脱出を拒否する",
			entryName: "../../evil.txt",
			wantErr:   true,
		},
		{
			name:      "異常系: 子ディレクトリ経由の脱出を拒否する",
			entryName: "a/../../evil.txt",
			wantErr:   true,
		},
		{
			name:      "正常系: 絶対パスを基準ディレクトリ配下へ無害化する",
			entryName: "/etc/evil.txt",
			wantPath:  func(_ string, base string) string { return filepath.Join(base, "etc", "evil.txt") },
		},
		{
			name:      "異常系: バックスラッシュによる脱出を拒否する",
			entryName: `..\evil.txt`,
			wantErr:   true,
		},
		{
			name:      "異常系: 前方一致する兄弟ディレクトリへの脱出を拒否する",
			entryName: "../out-evil/x",
			wantErr:   true,
		},
		{
			name:      "正常系: バックスラッシュ区切りをディレクトリ階層として結合する",
			entryName: `dir\file.txt`,
			wantPath:  func(_ string, base string) string { return filepath.Join(base, "dir", "file.txt") },
		},
		{
			name:      "正常系: 基準ディレクトリ自体を返す",
			entryName: ".",
			wantPath:  func(_ string, base string) string { return base },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			base := filepath.Join(root, "out")

			got, err := safepath.Join(base, tt.entryName)
			if tt.wantErr {
				require.ErrorIs(t, err, safepath.ErrOutsideBase)
				assert.Empty(t, got)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantPath(root, base), got)
		})
	}
}
