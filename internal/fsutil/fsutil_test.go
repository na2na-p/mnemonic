package fsutil_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/fsutil"
)

func TestCopyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   []byte
		setup     func(t *testing.T, dir string, content []byte) (src, dst string)
		checkMode bool
		wantErr   bool
		wantErrOp string
	}{
		{
			name:    "同一パスへのコピーは内容を保持する",
			content: []byte("original content"),
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				path := filepath.Join(dir, "src.txt")
				require.NoError(t, os.WriteFile(path, content, 0o644))

				return path, path
			},
		},
		{
			name:    "ハードリンク経由のコピーは内容を保持する",
			content: []byte("hard-linked content"),
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				dst := filepath.Join(dir, "linked.txt")
				require.NoError(t, os.WriteFile(src, content, 0o644))

				if err := os.Link(src, dst); err != nil {
					t.Skipf("このファイルシステムはハードリンクをサポートしません: %v", err)
				}

				return src, dst
			},
		},
		{
			// O_CREATE|O_TRUNCは既存ファイルのモードを変更しない(POSIX)ため、
			// このケースはモードを検証しない。モードの検証はdst新規作成のケースで行う。
			name:    "既存の別ファイルへのコピーは長い旧内容を残さず上書きする",
			content: []byte("distinct content"),
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				dst := filepath.Join(dir, "dst.txt")
				require.NoError(t, os.WriteFile(src, content, 0o600))
				require.NoError(t, os.WriteFile(dst, []byte("stale content that is much longer than the new one"), 0o600))

				return src, dst
			},
		},
		{
			name:      "dstが存在しない場合は新規作成しモードを保持する",
			content:   []byte("new file content"),
			checkMode: true,
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				dst := filepath.Join(dir, "nested", "dst.txt")
				// 0o644は0o666からumask 022を引いた値と一致し、パーミッションを
				// 引き継がない実装でも一致してしまうため、グループ/その他の書き込み
				// ビットを含まない別の値を使う。os.Chmodはumaskを経由せずsrcだけに
				// 効き、umaskがかかるdstと食い違うため使わない。
				require.NoError(t, os.WriteFile(src, content, 0o751))
				require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o750))

				return src, dst
			},
		},
		{
			name:      "dstの親が通常ファイルの場合はstatエラーを返す",
			wantErr:   true,
			wantErrOp: "stat",
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				require.NoError(t, os.WriteFile(src, content, 0o644))

				notADir := filepath.Join(dir, "not-a-dir")
				require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o644))

				return src, filepath.Join(notADir, "dst.txt")
			},
		},
		{
			name:      "dstが既存のディレクトリの場合はopenエラーを返す",
			wantErr:   true,
			wantErrOp: "open",
			setup: func(t *testing.T, dir string, content []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				dst := filepath.Join(dir, "dst-dir")
				require.NoError(t, os.WriteFile(src, content, 0o644))
				require.NoError(t, os.Mkdir(dst, 0o750))

				return src, dst
			},
		},
		{
			name:      "srcが存在しない場合はopenエラーを返す",
			wantErr:   true,
			wantErrOp: "open",
			setup: func(t *testing.T, dir string, _ []byte) (string, string) {
				t.Helper()

				return filepath.Join(dir, "missing.txt"), filepath.Join(dir, "dst.txt")
			},
		},
		{
			// 読み取りエラーの種別はOSごとに異なるため、エラーの有無だけを検証する。
			name:    "srcがディレクトリの場合はエラーを返す",
			wantErr: true,
			setup: func(t *testing.T, dir string, _ []byte) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src-dir")
				require.NoError(t, os.Mkdir(src, 0o750))

				return src, filepath.Join(dir, "dst.txt")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			src, dst := tt.setup(t, dir, tt.content)

			err := fsutil.CopyFile(src, dst)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrOp != "" {
					pathErr, ok := errors.AsType[*fs.PathError](err)
					require.True(t, ok, "want *fs.PathError, got %v", err)
					assert.Equal(t, tt.wantErrOp, pathErr.Op)
				}

				return
			}

			require.NoError(t, err)
			got, readErr := os.ReadFile(dst)
			require.NoError(t, readErr)
			assert.Equal(t, tt.content, got)

			if tt.checkMode {
				srcInfo, statErr := os.Stat(src)
				require.NoError(t, statErr)
				dstInfo, statErr := os.Stat(dst)
				require.NoError(t, statErr)
				assert.Equal(t, srcInfo.Mode().Perm(), dstInfo.Mode().Perm())
			}
		})
	}
}

func TestSameFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setup         func(t *testing.T, dir string) (src, dst string)
		want          bool
		wantStatError bool
	}{
		{
			name: "同一パスは同一ファイルと判定する",
			setup: func(t *testing.T, dir string) (string, string) {
				t.Helper()

				path := filepath.Join(dir, "src.txt")
				require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

				return path, path
			},
			want: true,
		},
		{
			name: "ハードリンクは同一ファイルと判定する",
			setup: func(t *testing.T, dir string) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				dst := filepath.Join(dir, "linked.txt")
				require.NoError(t, os.WriteFile(src, []byte("x"), 0o600))

				if err := os.Link(src, dst); err != nil {
					t.Skipf("このファイルシステムはハードリンクをサポートしません: %v", err)
				}

				return src, dst
			},
			want: true,
		},
		{
			name: "内容が同じでも別ファイルは同一と判定しない",
			setup: func(t *testing.T, dir string) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				dst := filepath.Join(dir, "dst.txt")
				require.NoError(t, os.WriteFile(src, []byte("x"), 0o600))
				require.NoError(t, os.WriteFile(dst, []byte("x"), 0o600))

				return src, dst
			},
			want: false,
		},
		{
			name: "dstが存在しない場合はエラーにせず同一と判定しない",
			setup: func(t *testing.T, dir string) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				require.NoError(t, os.WriteFile(src, []byte("x"), 0o600))

				return src, filepath.Join(dir, "missing.txt")
			},
			want: false,
		},
		{
			name: "dstのstatがENOENT以外で失敗した場合はエラーを返す",
			setup: func(t *testing.T, dir string) (string, string) {
				t.Helper()

				src := filepath.Join(dir, "src.txt")
				require.NoError(t, os.WriteFile(src, []byte("x"), 0o600))

				return src, filepath.Join(src, "dst.txt")
			},
			wantStatError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src, dst := tt.setup(t, t.TempDir())
			srcInfo, err := os.Stat(src)
			require.NoError(t, err)

			got, err := fsutil.SameFile(srcInfo, dst)

			if tt.wantStatError {
				pathErr, ok := errors.AsType[*fs.PathError](err)
				require.True(t, ok, "want *fs.PathError, got %v", err)
				assert.Equal(t, "stat", pathErr.Op)
				assert.False(t, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
