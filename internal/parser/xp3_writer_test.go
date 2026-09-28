package parser_test

import (
	"hash/adler32"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/parser"
)

// writeTree はfiles（キーはsrcDirからのスラッシュ区切りの相対パス）をsrcDir配下に作る。
func writeTree(t *testing.T, srcDir string, files map[string][]byte) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(srcDir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, content, 0o600))
	}
}

// 期待バイト列はkrkrz base/XP3Archive.cpp（以下XA）の読み取りコードから手で
// 起こしたもので、mnemonicの読み取り器やテスト用ビルダーからは生成しない。
// 読み取り器の誤読を写した期待値ではテストが誤りを検出できないため。
func TestWriteXP3Archive_Bytes(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	// "sub" < "sub.txt" なのでディレクトリ走査では sub/日本.txt が先に見つかるが、
	// バイト順では '.'(0x2E) < '/'(0x2F) なので sub.txt が先に書かれる。
	writeTree(t, srcDir, map[string][]byte{
		"sub.txt":    []byte("Wikipedia"),
		"sub/日本.txt": []byte("ok\n"),
	})

	// "Wikipedia" のAdler-32を手計算した値: A = 1+87+105+107+105+112+101+100+105+97
	// = 920 (0x398)、B = 88+193+300+405+517+618+718+823+920 = 4582 (0x11E6)、
	// B<<16|A = 0x11E60398。
	require.Equal(t, uint32(0x11E60398), adler32.Checksum([]byte("Wikipedia")))
	// "ok\n" のAdler-32を手計算した値: A = 1+0x6F+0x6B+0x0A = 229 (0xE5),
	// B = 112+219+229 = 560 (0x230)、B<<16|A = 0x023000E5。
	require.Equal(t, uint32(0x023000E5), adler32.Checksum([]byte("ok\n")))

	want := []byte{
		// XA:247-251 マーク11バイト
		0x58, 0x50, 0x33, 0x0d, 0x0a, 0x20, 0x0a, 0x1a, 0x8b, 0x67, 0x01,
		// XA:362,369-370 索引位置 uint64 LE = 31（データ2件の直後）
		0x1f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// 位置19: sub.txt のデータ "Wikipedia"
		'W', 'i', 'k', 'i', 'p', 'e', 'd', 'i', 'a',
		// 位置28: sub/日本.txt のデータ "ok\n"
		'o', 'k', '\n',
		// 位置31: XA:373-374,412-413 索引フラグ 0x00 = TVP_XP3_INDEX_ENCODE_RAW、
		// 継続ビット0x80なし（XA:529-530）
		0x00,
		// XA:416,420 索引サイズ uint64 = 238 = (12+104)+(12+110)
		0xee, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,

		// 1件目 sub.txt
		// XA:438,586-589 "File" チャンク: 名前4バイト + サイズ uint64 = 104
		'F', 'i', 'l', 'e', 0x68, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:444 "info" チャンク、サイズ 36 = 22 + 名前14バイト
		'i', 'n', 'f', 'o', 0x24, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:449-451 flags uint32 = 0（bit31 TVP_XP3_FILE_PROTECTED を立てない）
		0x00, 0x00, 0x00, 0x00,
		// XA:452 元サイズ uint64 = 9
		0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:453 格納サイズ uint64 = 9
		0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:455 名前の長さ uint16 = 7（UTF-16符号単位数）
		0x07, 0x00,
		// XA:467-468 名前 UTF-16LE "sub.txt"（終端なし）
		's', 0x00, 'u', 0x00, 'b', 0x00, '.', 0x00, 't', 0x00, 'x', 0x00, 't', 0x00,
		// XA:480 "segm" チャンク、サイズ 28 = 1セグメント（XA:484）
		's', 'e', 'g', 'm', 0x1c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:490-494 flags uint32 = 0 = TVP_XP3_SEGM_ENCODE_RAW
		0x00, 0x00, 0x00, 0x00,
		// XA:501 開始位置 uint64 = 19（アーカイブ先頭から）
		0x13, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:504 元サイズ uint64 = 9
		0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:505 格納サイズ uint64 = 9
		0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:514 "adlr" チャンク、サイズ 4
		'a', 'd', 'l', 'r', 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:518 Adler-32 uint32 LE = 0x11E60398（上で手計算した値）
		0x98, 0x03, 0xe6, 0x11,

		// 2件目 sub/日本.txt
		// XA:438,586-589 "File" チャンク、サイズ 110
		'F', 'i', 'l', 'e', 0x6e, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:444 "info" チャンク、サイズ 42 = 22 + 名前20バイト
		'i', 'n', 'f', 'o', 0x2a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:449-451 flags uint32 = 0
		0x00, 0x00, 0x00, 0x00,
		// XA:452 元サイズ uint64 = 3
		0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:453 格納サイズ uint64 = 3
		0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:455 名前の長さ uint16 = 10
		0x0a, 0x00,
		// XA:467-468 名前 UTF-16LE "sub/日本.txt"（日 = U+65E5、本 = U+672C）
		's', 0x00, 'u', 0x00, 'b', 0x00, '/', 0x00, 0xe5, 0x65, 0x2c, 0x67,
		'.', 0x00, 't', 0x00, 'x', 0x00, 't', 0x00,
		// XA:480 "segm" チャンク、サイズ 28
		's', 'e', 'g', 'm', 0x1c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:490-494 flags uint32 = 0
		0x00, 0x00, 0x00, 0x00,
		// XA:501 開始位置 uint64 = 28
		0x1c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:504 元サイズ uint64 = 3
		0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:505 格納サイズ uint64 = 3
		0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:514 "adlr" チャンク、サイズ 4
		'a', 'd', 'l', 'r', 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// XA:518 Adler-32 uint32 LE = 0x023000E5（上で手計算した値）
		0xe5, 0x00, 0x30, 0x02,
	}

	dst := filepath.Join(t.TempDir(), "out.xp3")
	require.NoError(t, parser.WriteXP3Archive(dst, srcDir))

	got, err := os.ReadFile(dst) //nolint:gosec // テストが作ったパスを読む
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestWriteXP3Archive_RoundTrip(t *testing.T) {
	t.Parallel()

	binary := make([]byte, 256)
	for i := range binary {
		binary[i] = byte(i)
	}

	tests := map[string]struct {
		files     map[string][]byte
		wantNames []string
	}{
		"正常系: 1ファイルだけのツリーを書き戻せる": {
			files:     map[string][]byte{"a.txt": []byte("hello")},
			wantNames: []string{"a.txt"},
		},
		"正常系: 多階層・日本語名・空ファイル・バイナリを含むツリーを名前のバイト順で書き戻せる": {
			files: map[string][]byte{
				"z.txt":          []byte("last"),
				"日本語.txt":        []byte("日本語の本文"),
				"sub/b.tjs":      []byte("b"),
				"sub/deep/c.bin": binary,
				"sub.txt":        []byte("sub"),
				"empty.dat":      {},
			},
			wantNames: []string{
				"empty.dat",
				"sub.txt",
				"sub/b.tjs",
				"sub/deep/c.bin",
				"z.txt",
				"日本語.txt",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srcDir := t.TempDir()
			writeTree(t, srcDir, tt.files)
			dst := filepath.Join(t.TempDir(), "out.xp3")

			require.NoError(t, parser.WriteXP3Archive(dst, srcDir))

			archive, err := parser.NewXP3Archive(dst)
			require.NoError(t, err)
			assert.Equal(t, tt.wantNames, archive.ListFiles())

			outDir := t.TempDir()
			require.NoError(t, archive.ExtractAll(outDir))
			for name, content := range tt.files {
				got, err := os.ReadFile(filepath.Join(outDir, filepath.FromSlash(name))) //nolint:gosec // テストが作ったパスを読む
				require.NoError(t, err, name)
				assert.Equal(t, content, got, name)
			}
		})
	}
}

func TestWriteXP3Archive_Errors(t *testing.T) {
	t.Parallel()

	// setup はbase配下に入力を作り、WriteXP3ArchiveのsrcDirに渡すパスを返す。
	tests := map[string]struct {
		setup   func(t *testing.T, base string) string
		wantErr error
	}{
		"異常系: BMP外の文字を含む名前はErrXP3UnencodableName": {
			setup: func(t *testing.T, base string) string {
				t.Helper()
				writeTree(t, base, map[string][]byte{"ok.txt": []byte("x"), "sub/😀.txt": []byte("x")})
				return base
			},
			wantErr: parser.ErrXP3UnencodableName,
		},
		"異常系: ファイルへのシンボリックリンクはErrXP3NonRegularFile": {
			setup: func(t *testing.T, base string) string {
				t.Helper()
				writeTree(t, base, map[string][]byte{"a.txt": []byte("x")})
				require.NoError(t, os.Symlink(filepath.Join(base, "a.txt"), filepath.Join(base, "link.txt")))
				return base
			},
			wantErr: parser.ErrXP3NonRegularFile,
		},
		"異常系: ディレクトリへのシンボリックリンクはErrXP3NonRegularFile": {
			setup: func(t *testing.T, base string) string {
				t.Helper()
				writeTree(t, base, map[string][]byte{"sub/a.txt": []byte("x")})
				require.NoError(t, os.Symlink(filepath.Join(base, "sub"), filepath.Join(base, "linkdir")))
				return base
			},
			wantErr: parser.ErrXP3NonRegularFile,
		},
		"異常系: 空のディレクトリはErrXP3NoFiles": {
			setup:   func(_ *testing.T, base string) string { return base },
			wantErr: parser.ErrXP3NoFiles,
		},
		"異常系: 空のサブディレクトリしかないディレクトリはErrXP3NoFiles": {
			setup: func(t *testing.T, base string) string {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(base, "a", "b"), 0o750))
				return base
			},
			wantErr: parser.ErrXP3NoFiles,
		},
		"異常系: 存在しないsrcDirはfs.ErrNotExist": {
			setup:   func(_ *testing.T, base string) string { return filepath.Join(base, "missing") },
			wantErr: fs.ErrNotExist,
		},
		"異常系: 通常ファイルをsrcDirに渡すとErrXP3SourceNotDir": {
			setup: func(t *testing.T, base string) string {
				t.Helper()
				writeTree(t, base, map[string][]byte{"file.txt": []byte("x")})
				return filepath.Join(base, "file.txt")
			},
			wantErr: parser.ErrXP3SourceNotDir,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srcDir := tt.setup(t, t.TempDir())
			dst := filepath.Join(t.TempDir(), "out.xp3")

			err := parser.WriteXP3Archive(dst, srcDir)

			require.ErrorIs(t, err, tt.wantErr)
			_, statErr := os.Stat(dst)
			assert.ErrorIs(t, statErr, fs.ErrNotExist)
		})
	}
}

func TestWriteXP3Archive_ReadFailureRemovesDst(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("rootは読み取り権限のないファイルも開けるため読み取り失敗を起こせない")
	}

	srcDir := t.TempDir()
	writeTree(t, srcDir, map[string][]byte{"a.txt": []byte("a"), "b.txt": []byte("b")})
	require.NoError(t, os.Chmod(filepath.Join(srcDir, "b.txt"), 0o000))
	dst := filepath.Join(t.TempDir(), "out.xp3")

	err := parser.WriteXP3Archive(dst, srcDir)

	require.ErrorIs(t, err, fs.ErrPermission)
	_, statErr := os.Stat(dst)
	assert.ErrorIs(t, statErr, fs.ErrNotExist)
}

// krkrz は索引の名前を NormalizeInArchiveStorageName（base/StorageIntf.cpp）で
// 正規化してから引くため、正規化後に同じになる名前は区別できない。
func TestWriteXP3Archive_NameCollision(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		files     map[string][]byte
		wantNames []string
	}{
		"異常系: ASCII大文字小文字だけが違う名前はErrXP3NameCollision": {
			files:     map[string][]byte{"Sub/A.txt": []byte("1"), "sub/a.txt": []byte("2")},
			wantNames: []string{"Sub/A.txt", "sub/a.txt"},
		},
		"異常系: バックスラッシュを含む名前がスラッシュ区切りの名前と重なるとErrXP3NameCollision": {
			files:     map[string][]byte{`sub\x.txt`: []byte("1"), "sub/x.txt": []byte("2")},
			wantNames: []string{`sub\x.txt`, "sub/x.txt"},
		},
		"正常系: ASCII以外の大文字小文字（Σとσ）はkrkrzでも別の名前なので書ける": {
			files: map[string][]byte{"Σ.txt": []byte("1"), "σ.txt": []byte("2")},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srcDir := t.TempDir()
			writeTree(t, srcDir, tt.files)
			if got := snapshotNames(t, srcDir); len(got) != len(tt.files) {
				t.Skipf("このファイルシステムは入力を別々のファイルとして保持できない: %v", got)
			}
			dst := filepath.Join(t.TempDir(), "out.xp3")

			err := parser.WriteXP3Archive(dst, srcDir)

			if tt.wantNames == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, parser.ErrXP3NameCollision)
			for _, want := range tt.wantNames {
				assert.Contains(t, err.Error(), want)
			}
			_, statErr := os.Stat(dst)
			assert.ErrorIs(t, statErr, fs.ErrNotExist)
		})
	}
}

// snapshotNames はdir配下の通常ファイルの相対パス（OS区切り）を返す。
func snapshotNames(t *testing.T, dir string) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		names = append(names, rel)
		return err
	})
	require.NoError(t, err)
	return names
}
