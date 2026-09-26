package converter

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConversionStatus_Values(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		status   ConversionStatus
		expected string
	}{
		"正常系: SUCCESSステータス": {StatusSuccess, "success"},
		"正常系: SKIPPEDステータス": {StatusSkipped, "skipped"},
		"正常系: FAILEDステータス":  {StatusFailed, "failed"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, string(tc.status))
		})
	}
}

func TestConversionResult_CompressionRatio(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 圧縮率の計算", func(t *testing.T) {
		t.Parallel()

		result := ConversionResult{BytesBefore: 100, BytesAfter: 80}
		assert.InDelta(t, 0.8, result.CompressionRatio(), 1e-9)
	})

	t.Run("異常系: BytesBeforeが0の場合は1.0", func(t *testing.T) {
		t.Parallel()

		result := ConversionResult{BytesBefore: 0, BytesAfter: 50}
		assert.InDelta(t, 1.0, result.CompressionRatio(), 1e-9)
	})
}

func TestConversionResult_BytesSaved(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 節約バイト数の計算", func(t *testing.T) {
		t.Parallel()

		result := ConversionResult{BytesBefore: 100, BytesAfter: 80}
		assert.Equal(t, int64(20), result.BytesSaved())
	})

	t.Run("正常系: サイズ増加時は負の値", func(t *testing.T) {
		t.Parallel()

		result := ConversionResult{BytesBefore: 80, BytesAfter: 100}
		assert.Equal(t, int64(-20), result.BytesSaved())
	})
}

func TestConversionResult_IsSuccess(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		status   ConversionStatus
		expected bool
	}{
		"正常系: SUCCESSの場合True":  {StatusSuccess, true},
		"正常系: SKIPPEDの場合False": {StatusSkipped, false},
		"正常系: FAILEDの場合False":  {StatusFailed, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := ConversionResult{Status: tc.status}
			assert.Equal(t, tc.expected, result.IsSuccess())
		})
	}
}

func TestValidateSource(t *testing.T) {
	t.Parallel()

	t.Run("異常系: 存在しないファイル", func(t *testing.T) {
		t.Parallel()

		err := validateSource(filepath.Join(t.TempDir(), "non_existent.txt"))

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSourceNotFound)
	})

	t.Run("異常系: ディレクトリを指定した場合", func(t *testing.T) {
		t.Parallel()

		err := validateSource(t.TempDir())

		require.Error(t, err)
		require.ErrorIs(t, err, ErrSourceIsDirectory)
		assert.ErrorIs(t, err, ErrPermanentFailure)
	})

	t.Run("正常系: 有効なファイル", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		validFile := filepath.Join(dir, "valid.txt")
		require.NoError(t, os.WriteFile(validFile, []byte("test content"), 0o644))

		assert.NoError(t, validateSource(validFile))
	})
}

// statFailureCase はos.Statが失敗する変換元と、その失敗の期待する分類の組。
type statFailureCase struct {
	name          string
	skipAsRoot    bool
	setup         func(t *testing.T) string
	wantErr       error
	notWantErr    error
	wantOSErr     error
	wantPermanent bool
	wantPathErr   bool
}

func statFailureCases() []statFailureCase {
	return []statFailureCase{
		{
			name: "異常系: 存在しない変換元は再試行不要なErrSourceNotFoundになる",
			setup: func(t *testing.T) string {
				t.Helper()

				return filepath.Join(t.TempDir(), "missing.txt")
			},
			wantErr:       ErrSourceNotFound,
			notWantErr:    ErrSourceUnreadable,
			wantOSErr:     nil,
			wantPermanent: true,
			wantPathErr:   false,
		},
		{
			name:       "異常系: 探索権限の無いディレクトリ配下の変換元は再試行不要なErrSourceUnreadableになる",
			skipAsRoot: true,
			setup: func(t *testing.T) string {
				t.Helper()

				locked := filepath.Join(t.TempDir(), "locked")
				require.NoError(t, os.Mkdir(locked, 0o700))
				path := filepath.Join(locked, "source.txt")
				require.NoError(t, os.WriteFile(path, []byte("content"), 0o600))
				// t.TempDir()のクリーンアップは探索権限の無いディレクトリを削除できないため、
				// 権限を落とす前に戻す処理を登録する。
				t.Cleanup(func() { _ = os.Chmod(locked, 0o700) }) //nolint:gosec // テスト用の一時ディレクトリの権限を戻す用途のため妥当
				require.NoError(t, os.Chmod(locked, 0o000))

				return path
			},
			wantErr:       ErrSourceUnreadable,
			notWantErr:    ErrSourceNotFound,
			wantOSErr:     fs.ErrPermission,
			wantPermanent: true,
			wantPathErr:   true,
		},
		{
			name: "異常系: 親がファイルの変換元は再試行対象のErrSourceUnreadableになる",
			setup: func(t *testing.T) string {
				t.Helper()

				parent := filepath.Join(t.TempDir(), "parent.txt")
				require.NoError(t, os.WriteFile(parent, []byte("content"), 0o600))

				return filepath.Join(parent, "source.txt")
			},
			wantErr:       ErrSourceUnreadable,
			notWantErr:    ErrSourceNotFound,
			wantOSErr:     syscall.ENOTDIR,
			wantPermanent: false,
			wantPathErr:   true,
		},
	}
}

func assertStatFailure(t *testing.T, tt statFailureCase, path string, err error) {
	t.Helper()

	require.Error(t, err)
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
	_, ok := errors.AsType[*fs.PathError](err)
	assert.Equal(t, tt.wantPathErr, ok, "OSのエラー(*fs.PathError)を包むかどうか")
}

func runStatFailureCases(t *testing.T, check func(path string) error) {
	t.Helper()

	for _, tt := range statFailureCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.skipAsRoot && os.Geteuid() == 0 {
				t.Skip("rootはパーミッションに関係なく読み込めるため再現できない")
			}

			path := tt.setup(t)

			assertStatFailure(t, tt, path, check(path))
		})
	}
}

func TestClassifyStatError(t *testing.T) {
	t.Parallel()

	runStatFailureCases(t, func(path string) error {
		_, statErr := os.Stat(path)
		if statErr == nil {
			return nil
		}

		return classifyStatError(path, statErr)
	})
}

func TestValidateSource_StatFailure(t *testing.T) {
	t.Parallel()

	runStatFailureCases(t, validateSource)
}

func TestEnsureSourceExists(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 存在するファイルはnilを返す", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "source.txt")
		require.NoError(t, os.WriteFile(path, []byte("content"), 0o600))

		assert.NoError(t, ensureSourceExists(path))
	})

	t.Run("正常系: ディレクトリは後続の処理に判断を委ねてnilを返す", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, ensureSourceExists(t.TempDir()))
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		runStatFailureCases(t, ensureSourceExists)
	})
}

func TestGetFileSize(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 存在するファイルのサイズ取得", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		testFile := filepath.Join(dir, "test.txt")
		content := "Hello, World!"
		require.NoError(t, os.WriteFile(testFile, []byte(content), 0o644))

		assert.Equal(t, int64(len(content)), getFileSize(testFile))
	})

	t.Run("正常系: 存在しないファイルは0を返す", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, int64(0), getFileSize(filepath.Join(t.TempDir(), "non_existent.txt")))
	})
}
