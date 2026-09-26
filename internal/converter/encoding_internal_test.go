package converter

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isSupportedEncodingはパッケージ非公開ヘルパーであり、chardetの推定結果に依存せず
// 任意のエンコーディング名を与えて判定を検証するため、ホワイトボックステストとする。
func TestIsSupportedEncoding(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		enc  string
		want bool
	}{
		"正常系: SupportedEncodingsに無い名前は非対応と判定する": {"klingon", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isSupportedEncoding(tc.enc))
		})
	}
}

// Convertの読み込み失敗のうち「確認後に存在しなくなった」場合はStatとReadFileの間の
// 競合でしか起きず公開APIから決定的に再現できないため、os.ReadFileの失敗を直接与える
// ホワイトボックステストとする。
func TestClassifyReadError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		skipAsRoot    bool
		setup         func(t *testing.T) string
		wantErr       error
		notWantErr    error
		wantOSErr     error
		wantPermanent bool
	}{
		{
			name:       "異常系: 読み込み権限が無い場合は再試行不要なErrSourceUnreadableになる",
			skipAsRoot: true,
			setup: func(t *testing.T) string {
				t.Helper()

				path := filepath.Join(t.TempDir(), "source.ks")
				require.NoError(t, os.WriteFile(path, []byte("content"), 0o600))
				require.NoError(t, os.Chmod(path, 0o000))

				return path
			},
			wantErr:       ErrSourceUnreadable,
			notWantErr:    ErrSourceNotFound,
			wantOSErr:     fs.ErrPermission,
			wantPermanent: true,
		},
		{
			name: "異常系: 存在しない場合は再試行対象のErrSourceNotFoundになる",
			setup: func(t *testing.T) string {
				t.Helper()

				return filepath.Join(t.TempDir(), "missing.ks")
			},
			wantErr:       ErrSourceNotFound,
			notWantErr:    ErrSourceUnreadable,
			wantOSErr:     nil,
			wantPermanent: false,
		},
		{
			name: "異常系: ディレクトリの場合は再試行対象のErrSourceUnreadableになる",
			setup: func(t *testing.T) string {
				t.Helper()

				return t.TempDir()
			},
			wantErr:       ErrSourceUnreadable,
			notWantErr:    ErrSourceNotFound,
			wantOSErr:     syscall.EISDIR,
			wantPermanent: false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.skipAsRoot && os.Geteuid() == 0 {
				t.Skip("rootはパーミッションに関係なく読み込めるため再現できない")
			}

			path := tt.setup(t)
			_, readErr := os.ReadFile(path) //nolint:gosec // テスト用の一時パスを読む用途のため妥当
			require.Error(t, readErr)

			err := classifyReadError(path, readErr)

			require.ErrorIs(t, err, tt.wantErr)
			require.NotErrorIs(t, err, tt.notWantErr)
			require.ErrorContains(t, err, path)
			if tt.wantOSErr != nil {
				require.ErrorIs(t, err, tt.wantOSErr)
			}
			if tt.wantPermanent {
				require.ErrorIs(t, err, ErrPermanentFailure)
			} else {
				require.NotErrorIs(t, err, ErrPermanentFailure)
			}
		})
	}
}
