package cache_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/cache"
)

func TestDir(t *testing.T) {
	t.Parallel()

	dir, err := cache.Dir()

	require.NoError(t, err)
	assert.Contains(t, dir, "mnemonic")
}

// unsetenvForTest はkeyを未設定にし、テスト終了後に元の状態（存在した場合は元の値、
// 存在しなかった場合は未設定）へ復元する。
// t.Setenv("")は空文字列を設定するだけで「未設定」にはならないため、この用途には使えない。
func unsetenvForTest(t *testing.T, key string) {
	t.Helper()

	original, existed := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, original)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

// 環境変数を書き換えるサブテストがt.Setenvを使うため、本テスト自体は
// t.Parallel()にできない（t.Setenvは並行祖先を持つテストから呼べない）。
//
//nolint:tparallel // 理由は上記コメントの通り
func TestDirForOS(t *testing.T) {
	t.Run("正常系: Linuxでデフォルトキャッシュディレクトリ", func(t *testing.T) {
		unsetenvForTest(t, "XDG_CACHE_HOME")

		dir, err := cache.DirForOS("linux")

		require.NoError(t, err)
		assert.Contains(t, dir, filepath.Join(".cache", "mnemonic"))
	})

	t.Run("正常系: LinuxでXDG_CACHE_HOME設定時", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "/custom/cache")

		dir, err := cache.DirForOS("linux")

		require.NoError(t, err)
		assert.Equal(t, filepath.Join("/custom/cache", "mnemonic"), dir)
	})

	t.Run("正常系: macOSのキャッシュディレクトリ", func(t *testing.T) {
		t.Parallel()

		dir, err := cache.DirForOS("darwin")

		require.NoError(t, err)
		assert.Contains(t, dir, filepath.Join("Library", "Caches", "mnemonic"))
	})

	t.Run("正常系: Windowsのキャッシュディレクトリ（LOCALAPPDATA設定時）", func(t *testing.T) {
		t.Setenv("LOCALAPPDATA", `C:\Users\Test\AppData\Local`)

		dir, err := cache.DirForOS("windows")

		require.NoError(t, err)
		assert.Contains(t, dir, "mnemonic")
		assert.Contains(t, dir, "cache")
	})

	t.Run("正常系: Windowsのキャッシュディレクトリ（LOCALAPPDATA未設定時）", func(t *testing.T) {
		unsetenvForTest(t, "LOCALAPPDATA")

		dir, err := cache.DirForOS("windows")

		require.NoError(t, err)
		assert.Contains(t, dir, filepath.Join("AppData", "Local", "mnemonic", "cache"))
	})

	t.Run("正常系: 未知のOSはLinux相当のフォールバック", func(t *testing.T) {
		t.Parallel()

		dir, err := cache.DirForOS("plan9")

		require.NoError(t, err)
		assert.Contains(t, dir, filepath.Join(".cache", "mnemonic"))
	})
}

func TestTemplateCachePath(t *testing.T) {
	t.Parallel()

	t.Run("正常系: テンプレートキャッシュパスにバージョンが含まれる", func(t *testing.T) {
		t.Parallel()

		path, err := cache.TemplateCachePath("1.0.0")

		require.NoError(t, err)
		assert.Contains(t, path, "templates")
		assert.Contains(t, path, "1.0.0")
	})

	t.Run("正常系: 異なるバージョンで異なるパスを返す", func(t *testing.T) {
		t.Parallel()

		v1, err := cache.TemplateCachePath("1.0.0")
		require.NoError(t, err)
		v2, err := cache.TemplateCachePath("2.0.0")
		require.NoError(t, err)

		assert.NotEqual(t, v1, v2)
		assert.Contains(t, v1, "1.0.0")
		assert.Contains(t, v2, "2.0.0")
	})
}

func TestIsValid(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 存在しないファイルは無効", func(t *testing.T) {
		t.Parallel()

		result := cache.IsValid(filepath.Join(t.TempDir(), "nonexistent"), cache.DefaultMaxAgeDays)

		assert.False(t, result)
	})

	t.Run("正常系: 新しいファイルは有効", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("test"), 0o600))

		result := cache.IsValid(path, cache.DefaultMaxAgeDays)

		assert.True(t, result)
	})

	t.Run("正常系: カスタムmax_ageを指定できる", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("test"), 0o600))

		result := cache.IsValid(path, 1)

		assert.True(t, result)
	})

	t.Run("異常系: 古いファイルは無効", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("test"), 0o600))
		oldTime := time.Now().Add(-10 * 24 * time.Hour)
		require.NoError(t, os.Chtimes(path, oldTime, oldTime))

		result := cache.IsValid(path, 7)

		assert.False(t, result)
	})
}

func TestClearCacheDir(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 全キャッシュをクリアする", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0o600))
		templateDir := filepath.Join(dir, "templates", "1.0.0")
		require.NoError(t, os.MkdirAll(templateDir, 0o750))
		require.NoError(
			t,
			os.WriteFile(filepath.Join(templateDir, "template.txt"), []byte("template"), 0o600),
		)

		err := cache.ClearCacheDir(dir, false)

		require.NoError(t, err)
		_, statErr := os.Stat(dir)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("正常系: テンプレートのみクリアする", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		cacheFile := filepath.Join(dir, "test.txt")
		require.NoError(t, os.WriteFile(cacheFile, []byte("test"), 0o600))
		templateDir := filepath.Join(dir, "templates", "1.0.0")
		require.NoError(t, os.MkdirAll(templateDir, 0o750))
		require.NoError(
			t,
			os.WriteFile(filepath.Join(templateDir, "template.txt"), []byte("template"), 0o600),
		)

		err := cache.ClearCacheDir(dir, true)

		require.NoError(t, err)
		_, statErr := os.Stat(cacheFile)
		require.NoError(t, statErr)
		_, statErr = os.Stat(filepath.Join(dir, "templates"))
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("正常系: 存在しないディレクトリのクリアはエラーにならない", func(t *testing.T) {
		t.Parallel()

		nonexistent := filepath.Join(t.TempDir(), "nonexistent")

		err := cache.ClearCacheDir(nonexistent, false)

		assert.NoError(t, err)
	})
}

func TestInfoForDir(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 存在しないディレクトリの情報を取得", func(t *testing.T) {
		t.Parallel()

		nonexistent := filepath.Join(t.TempDir(), "nonexistent")

		result, err := cache.InfoForDir(nonexistent)

		require.NoError(t, err)
		assert.Equal(t, nonexistent, result.Directory)
		assert.Equal(t, int64(0), result.SizeBytes)
		assert.Nil(t, result.TemplateVersion)
		assert.Nil(t, result.TemplateExpiresInDays)
	})

	t.Run("正常系: ファイルが存在する場合のキャッシュ情報", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(
			t,
			os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test content"), 0o600),
		)

		result, err := cache.InfoForDir(dir)

		require.NoError(t, err)
		assert.Equal(t, dir, result.Directory)
		assert.Positive(t, result.SizeBytes)
		assert.Nil(t, result.TemplateVersion)
	})

	t.Run("正常系: テンプレートが存在する場合のキャッシュ情報", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		templateDir := filepath.Join(dir, "templates", "1.0.0")
		require.NoError(t, os.MkdirAll(templateDir, 0o750))
		require.NoError(
			t,
			os.WriteFile(filepath.Join(templateDir, "template.txt"), []byte("template"), 0o600),
		)

		result, err := cache.InfoForDir(dir)

		require.NoError(t, err)
		assert.Equal(t, dir, result.Directory)
		assert.Positive(t, result.SizeBytes)
		require.NotNil(t, result.TemplateVersion)
		assert.Equal(t, "1.0.0", *result.TemplateVersion)
		require.NotNil(t, result.TemplateExpiresInDays)
		assert.LessOrEqual(t, *result.TemplateExpiresInDays, 7)
	})
}

func TestGetCacheInfo(t *testing.T) {
	t.Parallel()

	result, err := cache.GetCacheInfo()

	require.NoError(t, err)
	assert.Contains(t, result.Directory, "mnemonic")
}

func TestInfo_CreationAndFieldAccess(t *testing.T) {
	t.Parallel()

	version := "1.0.0"
	expires := 7
	info := cache.Info{
		Directory:             "/tmp/cache",
		SizeBytes:             1024,
		TemplateVersion:       &version,
		TemplateExpiresInDays: &expires,
	}

	assert.Equal(t, "/tmp/cache", info.Directory)
	assert.Equal(t, int64(1024), info.SizeBytes)
	require.NotNil(t, info.TemplateVersion)
	assert.Equal(t, "1.0.0", *info.TemplateVersion)
	require.NotNil(t, info.TemplateExpiresInDays)
	assert.Equal(t, 7, *info.TemplateExpiresInDays)
}
