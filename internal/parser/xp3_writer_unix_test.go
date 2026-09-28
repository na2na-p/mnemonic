//go:build unix

package parser_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/parser"
)

func TestWriteXP3Archive_NamedPipe(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	writeTree(t, srcDir, map[string][]byte{"a.txt": []byte("x")})
	require.NoError(t, syscall.Mkfifo(filepath.Join(srcDir, "pipe"), 0o600))
	dst := filepath.Join(t.TempDir(), "out.xp3")

	err := parser.WriteXP3Archive(dst, srcDir)

	require.ErrorIs(t, err, parser.ErrXP3NonRegularFile)
	_, statErr := os.Stat(dst)
	assert.ErrorIs(t, statErr, fs.ErrNotExist)
}

const dstInSrcChildEnv = "MNEMONIC_TEST_XP3_DST_IN_SRC_CHILD"

// dstがsrcDirの中にあると、既存のdstがエントリとして走査され、書き込み中の
// dst自身を読み続けてファイルが際限なく伸びうる。ガードが壊れてもディスクを
// 埋めないよう、ファイルサイズ上限（RLIMIT_FSIZE）を1MiBに絞った子プロセスで
// 本体を実行する。
func TestWriteXP3Archive_DstInsideSrc(t *testing.T) {
	t.Parallel()
	if os.Getenv(dstInSrcChildEnv) == "1" {
		const limit = 1 << 20
		require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: limit, Max: limit}))
		runDstInsideSrcCases(t)
		return
	}

	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWriteXP3Archive_DstInsideSrc$", "-test.count=1", "-test.v") //nolint:gosec // 自分自身のテストバイナリを子プロセスとして起動する
	cmd.Env = append(os.Environ(), dstInSrcChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func runDstInsideSrcCases(t *testing.T) {
	t.Helper()

	tests := map[string]struct {
		// setup はbase配下に入力を作り、srcDirとdstを返す。
		setup   func(t *testing.T, base string) (srcDir, dst string)
		wantErr error
		// notErr はerrのチェーンに含まれてはならないエラー。
		notErr error
	}{
		"異常系: srcDir直下に既に存在するdstはErrXP3DestinationInSource": {
			setup: func(t *testing.T, base string) (string, string) {
				t.Helper()
				writeTree(t, base, map[string][]byte{"a.txt": []byte("a"), "out.xp3": []byte("old")})
				return base, filepath.Join(base, "out.xp3")
			},
			wantErr: parser.ErrXP3DestinationInSource,
		},
		"異常系: srcDirのサブディレクトリ内のまだ無いdstもErrXP3DestinationInSource": {
			setup: func(t *testing.T, base string) (string, string) {
				t.Helper()
				writeTree(t, base, map[string][]byte{"a.txt": []byte("a"), "sub/b.txt": []byte("b")})
				return base, filepath.Join(base, "sub", "out.xp3")
			},
			wantErr: parser.ErrXP3DestinationInSource,
		},
		"異常系: srcDirを指すシンボリックリンク経由のdstもErrXP3DestinationInSource": {
			setup: func(t *testing.T, base string) (string, string) {
				t.Helper()
				srcDir := filepath.Join(base, "src")
				writeTree(t, srcDir, map[string][]byte{"a.txt": []byte("a")})
				link := filepath.Join(base, "link")
				require.NoError(t, os.Symlink(srcDir, link))
				return srcDir, filepath.Join(link, "out.xp3")
			},
			wantErr: parser.ErrXP3DestinationInSource,
		},
		"異常系: srcDir内のファイルへのハードリンクであるdstもErrXP3DestinationInSource": {
			setup: func(t *testing.T, base string) (string, string) {
				t.Helper()
				srcDir := filepath.Join(base, "src")
				writeTree(t, srcDir, map[string][]byte{"a.txt": []byte("a")})
				dst := filepath.Join(base, "out.xp3")
				require.NoError(t, os.Link(filepath.Join(srcDir, "a.txt"), dst))
				return srcDir, dst
			},
			wantErr: parser.ErrXP3DestinationInSource,
		},
		"異常系: srcDir内の \"..\" で始まる名前のサブディレクトリにあるまだ無いdstもErrXP3DestinationInSource": {
			setup: func(t *testing.T, base string) (string, string) {
				t.Helper()
				writeTree(t, base, map[string][]byte{"a.txt": []byte("a"), "..foo/b.txt": []byte("b")})
				return base, filepath.Join(base, "..foo", "out.xp3")
			},
			wantErr: parser.ErrXP3DestinationInSource,
		},
		"異常系: srcDir内の既存ファイルを指すシンボリックリンクのdstはErrXP3DestinationInSource": {
			setup: func(t *testing.T, base string) (string, string) {
				t.Helper()
				srcDir := filepath.Join(base, "src")
				writeTree(t, srcDir, map[string][]byte{"a.txt": []byte("a")})
				dst := filepath.Join(base, "out.xp3")
				require.NoError(t, os.Symlink(filepath.Join(srcDir, "a.txt"), dst))
				return srcDir, dst
			},
			wantErr: parser.ErrXP3DestinationInSource,
		},
		"異常系: リンク先が無かったシンボリックリンクのdstは1度書かれたあとの呼び出しでErrXP3DestinationInSource": {
			setup: func(t *testing.T, base string) (string, string) {
				t.Helper()
				srcDir := filepath.Join(base, "src")
				writeTree(t, srcDir, map[string][]byte{"a.txt": []byte("a")})
				dst := filepath.Join(base, "out.xp3")
				require.NoError(t, os.Symlink(filepath.Join(srcDir, "new.xp3"), dst))
				require.NoError(t, parser.WriteXP3Archive(dst, srcDir))
				return srcDir, dst
			},
			wantErr: parser.ErrXP3DestinationInSource,
		},
		"異常系: ハードリンクのdstより先に走査される入力エラーがあればそのエラーを返し何も書かない": {
			setup: func(t *testing.T, base string) (string, string) {
				t.Helper()
				srcDir := filepath.Join(base, "src")
				writeTree(t, srcDir, map[string][]byte{"a.txt": []byte("a")})
				// "0😀.txt" は "a.txt" より前に走査されるので、名前の検査が
				// ハードリンクの照合より先に失敗する。
				if err := os.WriteFile(filepath.Join(srcDir, "0😀.txt"), []byte("x"), 0o600); err != nil {
					t.Skipf("このファイルシステムにはBMP外の文字を含む名前を作れない: %v", err)
				}
				dst := filepath.Join(base, "out.xp3")
				require.NoError(t, os.Link(filepath.Join(srcDir, "a.txt"), dst))
				return srcDir, dst
			},
			wantErr: parser.ErrXP3UnencodableName,
			notErr:  parser.ErrXP3DestinationInSource,
		},
		"異常系: シンボリックリンクのdstは走査前のパス判定で見つかるので先に走査される入力エラーがあってもErrXP3DestinationInSource": {
			setup: func(t *testing.T, base string) (string, string) {
				t.Helper()
				srcDir := filepath.Join(base, "src")
				writeTree(t, srcDir, map[string][]byte{"a.txt": []byte("a")})
				if err := os.WriteFile(filepath.Join(srcDir, "0😀.txt"), []byte("x"), 0o600); err != nil {
					t.Skipf("このファイルシステムにはBMP外の文字を含む名前を作れない: %v", err)
				}
				dst := filepath.Join(base, "out.xp3")
				require.NoError(t, os.Symlink(filepath.Join(srcDir, "a.txt"), dst))
				return srcDir, dst
			},
			wantErr: parser.ErrXP3DestinationInSource,
			notErr:  parser.ErrXP3UnencodableName,
		},
		"正常系: srcDirと名前の先頭が同じ隣のディレクトリのdstは書ける": {
			setup: func(t *testing.T, base string) (string, string) {
				t.Helper()
				srcDir := filepath.Join(base, "src")
				writeTree(t, srcDir, map[string][]byte{"a.txt": []byte("a")})
				require.NoError(t, os.Mkdir(filepath.Join(base, "src2"), 0o750))
				return srcDir, filepath.Join(base, "src2", "out.xp3")
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srcDir, dst := tt.setup(t, t.TempDir())
			before := snapshotTree(t, srcDir)
			dstBefore, dstStatErr := os.Stat(dst)
			var dstContent []byte
			if dstStatErr == nil {
				var err error
				dstContent, err = os.ReadFile(dst) //nolint:gosec // テストが作ったパスを読む
				require.NoError(t, err)
			}

			err := parser.WriteXP3Archive(dst, srcDir)

			assert.Equal(t, before, snapshotTree(t, srcDir), "srcDirの中身は変わらない")
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
			if tt.notErr != nil {
				require.NotErrorIs(t, err, tt.notErr)
			}
			if dstStatErr != nil {
				_, statErr := os.Stat(dst)
				assert.ErrorIs(t, statErr, fs.ErrNotExist, "無かったdstは作らない")
				return
			}
			dstAfter, err := os.Stat(dst)
			require.NoError(t, err)
			got, err := os.ReadFile(dst) //nolint:gosec // テストが作ったパスを読む
			require.NoError(t, err)
			assert.Equal(t, dstContent, got, "既存のdstの中身は変わらない")
			assert.Equal(t, linkCount(t, dstBefore), linkCount(t, dstAfter), "既存のdstのリンク数は変わらない")
		})
	}
}

func linkCount(t *testing.T, info fs.FileInfo) uint64 {
	t.Helper()
	st, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	return uint64(st.Nlink) //nolint:unconvert // Nlinkの型はOSごとに違う（darwinはuint16、linuxはuint64）
}

// snapshotTree はdir配下の通常ファイルの内容を相対パスをキーにして返す。
func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		content, err := os.ReadFile(path) //nolint:gosec // テストが作ったパスを読む
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(content)
		return nil
	})
	require.NoError(t, err)
	return files
}
