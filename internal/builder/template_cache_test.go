package builder_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/na2na-p/mnemonic/internal/builder"
)

const metadataTimeLayout = "2006-01-02T15:04:05Z"

func writeCacheMetadata(t *testing.T, cachePath string, version string, downloadedAt, expiresAt time.Time) {
	t.Helper()

	require.NoError(t, os.MkdirAll(cachePath, 0o750))

	metadata := map[string]string{
		"version":       version,
		"downloaded_at": downloadedAt.UTC().Format(metadataTimeLayout),
		"expires_at":    expiresAt.UTC().Format(metadataTimeLayout),
	}
	data, err := json.Marshal(metadata)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "metadata.json"), data, 0o600))
}

func TestNewTemplateCache(t *testing.T) {
	t.Parallel()

	t.Run("正常系: デフォルトのリフレッシュ日数で初期化", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockManager := NewMockCacheManager(ctrl)

		c := builder.NewTemplateCache(mockManager, 0)

		assert.NotNil(t, c)
	})
}

func TestTemplateCache_GetCachedTemplate(t *testing.T) {
	t.Parallel()

	t.Run("正常系: キャッシュが存在する場合にパスを返す", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "templates", "v1.0.0")
		writeCacheMetadata(t, cachePath, "v1.0.0", time.Now(), time.Now().Add(7*24*time.Hour))

		templateFile := filepath.Join(cachePath, "template.zip")
		require.NoError(t, os.WriteFile(templateFile, []byte("test content"), 0o600))

		ctrl := gomock.NewController(t)
		mockManager := NewMockCacheManager(ctrl)
		mockManager.EXPECT().GetTemplateCachePath("v1.0.0").Return(cachePath, nil).AnyTimes()
		mockManager.EXPECT().GetCacheDir().Return(tmpDir, nil).AnyTimes()

		c := builder.NewTemplateCache(mockManager, 0)
		version := "v1.0.0"

		result, ok := c.GetCachedTemplate(&version)

		require.True(t, ok)
		assert.Equal(t, templateFile, result)
	})

	t.Run("正常系: キャッシュが存在しない場合にok=falseを返す", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		ctrl := gomock.NewController(t)
		mockManager := NewMockCacheManager(ctrl)
		mockManager.EXPECT().GetCacheDir().Return(tmpDir, nil).AnyTimes()

		c := builder.NewTemplateCache(mockManager, 0)

		_, ok := c.GetCachedTemplate(nil)

		assert.False(t, ok)
	})
}

func TestTemplateCache_IsCacheValid(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 期限内のキャッシュは有効", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "templates", "v1.0.0")
		writeCacheMetadata(t, cachePath, "v1.0.0", time.Now(), time.Now().Add(3*24*time.Hour))

		ctrl := gomock.NewController(t)
		mockManager := NewMockCacheManager(ctrl)
		mockManager.EXPECT().GetTemplateCachePath("v1.0.0").Return(cachePath, nil).AnyTimes()

		c := builder.NewTemplateCache(mockManager, 7)
		version := "v1.0.0"

		assert.True(t, c.IsCacheValid(&version))
	})

	t.Run("正常系: 期限切れのキャッシュは無効", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "templates", "v1.0.0")
		writeCacheMetadata(t, cachePath, "v1.0.0", time.Now().Add(-8*24*time.Hour), time.Now().Add(-1*24*time.Hour))

		ctrl := gomock.NewController(t)
		mockManager := NewMockCacheManager(ctrl)
		mockManager.EXPECT().GetTemplateCachePath("v1.0.0").Return(cachePath, nil).AnyTimes()

		c := builder.NewTemplateCache(mockManager, 7)
		version := "v1.0.0"

		assert.False(t, c.IsCacheValid(&version))
	})

	t.Run("正常系: refreshDaysで期限を変更できる", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name        string
			refreshDays int
		}{
			{name: "正常系: デフォルト7日", refreshDays: 7},
			{name: "正常系: 14日に変更", refreshDays: 14},
			{name: "正常系: 1日に変更", refreshDays: 1},
			{name: "正常系: 30日に変更", refreshDays: 30},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctrl := gomock.NewController(t)
				mockManager := NewMockCacheManager(ctrl)

				c := builder.NewTemplateCache(mockManager, tc.refreshDays)
				assert.NotNil(t, c)
			})
		}
	})
}

func TestTemplateCache_GetCachedVersion(t *testing.T) {
	t.Parallel()

	t.Run("正常系: キャッシュされているバージョンを取得できる", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "templates", "v1.0.0")
		writeCacheMetadata(t, cachePath, "v1.0.0", time.Now(), time.Now().Add(7*24*time.Hour))

		ctrl := gomock.NewController(t)
		mockManager := NewMockCacheManager(ctrl)
		mockManager.EXPECT().GetCacheDir().Return(tmpDir, nil).AnyTimes()
		mockManager.EXPECT().GetTemplateCachePath("v1.0.0").Return(cachePath, nil).AnyTimes()

		c := builder.NewTemplateCache(mockManager, 0)

		result, ok := c.GetCachedVersion()

		require.True(t, ok)
		assert.Equal(t, "v1.0.0", result)
	})

	t.Run("正常系: 複数バージョンがある場合は最新のdownloaded_atを返す", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		oldPath := filepath.Join(tmpDir, "templates", "v1.0.0")
		writeCacheMetadata(t, oldPath, "v1.0.0", time.Now().Add(-2*24*time.Hour), time.Now().Add(5*24*time.Hour))
		newPath := filepath.Join(tmpDir, "templates", "v2.0.0")
		writeCacheMetadata(t, newPath, "v2.0.0", time.Now(), time.Now().Add(7*24*time.Hour))

		ctrl := gomock.NewController(t)
		mockManager := NewMockCacheManager(ctrl)
		mockManager.EXPECT().GetCacheDir().Return(tmpDir, nil).AnyTimes()
		mockManager.EXPECT().GetTemplateCachePath("v1.0.0").Return(oldPath, nil).AnyTimes()
		mockManager.EXPECT().GetTemplateCachePath("v2.0.0").Return(newPath, nil).AnyTimes()

		c := builder.NewTemplateCache(mockManager, 0)

		result, ok := c.GetCachedVersion()

		require.True(t, ok)
		assert.Equal(t, "v2.0.0", result)
	})

	t.Run("正常系: キャッシュが存在しない場合はok=falseを返す", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockManager := NewMockCacheManager(ctrl)
		mockManager.EXPECT().GetCacheDir().Return(filepath.Join(t.TempDir(), "nonexistent"), nil).AnyTimes()

		c := builder.NewTemplateCache(mockManager, 0)

		_, ok := c.GetCachedVersion()

		assert.False(t, ok)
	})
}

func TestTemplateCache_SaveTemplate(t *testing.T) {
	t.Parallel()

	t.Run("正常系: テンプレートがキャッシュディレクトリに保存される", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "cache", "templates", "v1.0.0")

		templateFile := filepath.Join(tmpDir, "template.zip")
		require.NoError(t, os.WriteFile(templateFile, []byte("test content"), 0o600))

		ctrl := gomock.NewController(t)
		mockManager := NewMockCacheManager(ctrl)
		mockManager.EXPECT().GetTemplateCachePath("v1.0.0").Return(cachePath, nil).AnyTimes()

		c := builder.NewTemplateCache(mockManager, 0)

		result, err := c.SaveTemplate(templateFile, "v1.0.0")

		require.NoError(t, err)
		assert.FileExists(t, result)
		content, err := os.ReadFile(result) //nolint:gosec // テストで作成した固定パス
		require.NoError(t, err)
		assert.Equal(t, "test content", string(content))
		assert.Equal(t, cachePath, filepath.Dir(result))
	})

	t.Run("正常系: バージョン情報が記録される", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "cache", "templates", "v2.0.0")

		templateFile := filepath.Join(tmpDir, "template.zip")
		require.NoError(t, os.WriteFile(templateFile, []byte("test content"), 0o600))

		ctrl := gomock.NewController(t)
		mockManager := NewMockCacheManager(ctrl)
		mockManager.EXPECT().GetTemplateCachePath("v2.0.0").Return(cachePath, nil).AnyTimes()

		c := builder.NewTemplateCache(mockManager, 0)

		_, err := c.SaveTemplate(templateFile, "v2.0.0")
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(cachePath, "metadata.json"))
		require.NoError(t, err)

		var metadata map[string]string
		require.NoError(t, json.Unmarshal(data, &metadata))
		assert.Equal(t, "v2.0.0", metadata["version"])
		assert.NotEmpty(t, metadata["downloaded_at"])
		assert.NotEmpty(t, metadata["expires_at"])
	})

	t.Run("異常系: 指定されたテンプレートファイルが存在しない場合", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockManager := NewMockCacheManager(ctrl)

		c := builder.NewTemplateCache(mockManager, 0)

		_, err := c.SaveTemplate(filepath.Join(t.TempDir(), "nonexistent.zip"), "v1.0.0")

		require.Error(t, err)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestTemplateCache_ClearCache(t *testing.T) {
	t.Parallel()

	t.Run("正常系: CacheManager.ClearCacheがtemplateOnly=trueで呼び出される", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockManager := NewMockCacheManager(ctrl)
		mockManager.EXPECT().ClearCache(true).Return(nil)

		c := builder.NewTemplateCache(mockManager, 0)

		require.NoError(t, c.ClearCache())
	})
}
