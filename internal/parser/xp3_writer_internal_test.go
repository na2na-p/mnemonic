package parser

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// macOS（APFS）では不正なUTF-8の名前は illegal byte sequence、65535符号単位を
// 超える相対パスは file name too long で作成時に失敗し、WriteXP3Archive経由では
// 検証できないため、名前の符号化を直接検証する。
func TestEncodeXP3Name(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name    string
		want    []uint16
		wantErr error
	}{
		"正常系: ASCIIと日本語のBMP文字をそのままUTF-16符号単位にする": {
			name: "a/日.txt",
			want: []uint16{'a', '/', 0x65E5, '.', 't', 'x', 't'},
		},
		"正常系: 65535符号単位ちょうどの名前は書ける": {
			name: strings.Repeat("a", 65535),
			want: slices.Repeat([]uint16{'a'}, 65535),
		},
		"異常系: BMP外の文字はErrXP3UnencodableName": {
			name:    "😀",
			wantErr: ErrXP3UnencodableName,
		},
		"異常系: 不正なUTF-8はU+FFFDに置き換えずErrXP3UnencodableName": {
			name:    "a\xffb",
			wantErr: ErrXP3UnencodableName,
		},
		"異常系: 名前の長さ欄uint16に収まらない65536符号単位の名前はErrXP3UnencodableName": {
			name:    strings.Repeat("a", 65536),
			wantErr: ErrXP3UnencodableName,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := encodeXP3Name(tt.name)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// 期待値は krkrz base/StorageIntf.cpp の tTVPArchive::NormalizeInArchiveStorageName
// から起こした。
func TestNormalizeXP3Name(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name string
		want string
	}{
		"正常系: ASCIIの大文字だけを小文字にする": {
			name: "Sub/ABC.TXT",
			want: "sub/abc.txt",
		},
		"正常系: ASCII以外の大文字（全角Ａ・ギリシャ文字Σ）は変えない": {
			name: "Ａ/Σ.txt",
			want: "Ａ/Σ.txt",
		},
		"正常系: バックスラッシュをスラッシュにする": {
			name: `a\b.txt`,
			want: "a/b.txt",
		},
		"正常系: 連続するスラッシュを1つにまとめる": {
			name: `a\\b//c.txt`,
			want: "a/b/c.txt",
		},
		"正常系: 先頭のスラッシュは取り除く": {
			name: `\\a.txt`,
			want: "a.txt",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, normalizeXP3Name(tt.name))
		})
	}
}
