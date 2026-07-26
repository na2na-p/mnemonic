package converter

import (
	"os"
	"path/filepath"
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
		assert.ErrorIs(t, err, ErrSourceIsDirectory)
	})

	t.Run("正常系: 有効なファイル", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		validFile := filepath.Join(dir, "valid.txt")
		require.NoError(t, os.WriteFile(validFile, []byte("test content"), 0o644))

		assert.NoError(t, validateSource(validFile))
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
