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
			name:      "異常系: 先頭の/の後の..による脱出を拒否する",
			entryName: "/../evil.txt",
			wantErr:   true,
		},
		{
			name:      "正常系: 連続した/や.を含む名前を整理して結合する",
			entryName: "//a/./b//c.txt",
			wantPath:  func(_ string, base string) string { return filepath.Join(base, "a", "b", "c.txt") },
		},
		{
			name:      "異常系: 一度外へ出てから基準ディレクトリへ戻る名前も拒否する",
			entryName: "a/../../out/x",
			wantErr:   true,
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

func TestRelPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		entryName string
		want      string
		wantErr   bool
	}{
		{name: "正常系: 入れ子の相対パスをそのまま返す", entryName: "a/b", want: "a/b"},
		{name: "正常系: バックスラッシュを区切りとして/に置き換える", entryName: `a\b`, want: "a/b"},
		{name: "正常系: 先頭の/を除く", entryName: "/a", want: "a"},
		{name: "正常系: 先頭の連続した/を除く", entryName: "//a/b", want: "a/b"},
		{name: "正常系: 先頭の./を除く", entryName: "./a", want: "a"},
		{name: "正常系: 連続した/を1つにまとめる", entryName: "a//b", want: "a/b"},
		{name: "正常系: 途中の./を除く", entryName: "a/./b", want: "a/b"},
		{name: "正常系: 末尾の/を除く", entryName: "a/", want: "a"},
		{name: "正常系: 基準ディレクトリ内に収まる..を解決する", entryName: "a/../b", want: "b"},
		{name: "正常系: ..で始まるだけの名前は外を指さない", entryName: "..a", want: "..a"},
		{name: "正常系: 基準ディレクトリ自体は.を返す", entryName: ".", want: "."},
		{name: "異常系: 一階層上への脱出を拒否する", entryName: "../x", wantErr: true},
		{name: "異常系: ..だけの名前を拒否する", entryName: "..", wantErr: true},
		{name: "異常系: 先頭の/の後の..による脱出を拒否する", entryName: "/../x", wantErr: true},
		{name: "異常系: 子ディレクトリ経由の脱出を拒否する", entryName: "a/../../x", wantErr: true},
		{name: "異常系: バックスラッシュによる脱出を拒否する", entryName: `..\x`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := safepath.RelPath(tt.entryName)

			if tt.wantErr {
				require.ErrorIs(t, err, safepath.ErrOutsideBase)
				require.ErrorContains(t, err, tt.entryName)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestJoin_RootBase(t *testing.T) {
	t.Parallel()

	root := string(filepath.Separator)

	tests := []struct {
		name      string
		entryName string
		wantPath  string
		wantErr   bool
	}{
		// 包含の判定はbase+区切り文字との前方一致で、ルートではそれが区切り文字2つになる。
		{name: "異常系: 基準ディレクトリがルートのときは直下のエントリも外として扱う", entryName: "a", wantErr: true},
		{name: "正常系: 基準ディレクトリがルートでも基準ディレクトリ自体は返す", entryName: ".", wantPath: root},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := safepath.Join(root, tt.entryName)

			if tt.wantErr {
				require.ErrorIs(t, err, safepath.ErrOutsideBase)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, got)
		})
	}
}
