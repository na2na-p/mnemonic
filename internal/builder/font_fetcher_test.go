package builder_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/builder"
)

func newTestFontFetcher(t *testing.T, cacheDir string, handler http.HandlerFunc) *builder.FontFetcher {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	f := builder.NewFontFetcher(cacheDir, server.Client())
	f.SourceURL = server.URL + "/Koruri-Regular.ttf"

	return f
}

func TestFontCacheDir(t *testing.T) {
	t.Parallel()

	t.Run("正常系: キャッシュディレクトリのパスを返す", func(t *testing.T) {
		t.Parallel()

		dir, err := builder.FontCacheDir()

		require.NoError(t, err)
		assert.Contains(t, dir, "mnemonic")
		assert.Contains(t, dir, "fonts")
	})
}

func TestFontFetcher_GetFont(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		cachedFontExists bool
		expectDownload   bool
	}{
		{name: "正常系: キャッシュが無い場合はダウンロードする", cachedFontExists: false, expectDownload: true},
		{name: "正常系: キャッシュが有効な場合はダウンロードしない", cachedFontExists: true, expectDownload: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			if tc.cachedFontExists {
				require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "Koruri-Regular.ttf"), []byte("cached ttf content"), 0o600))
			}

			callCount := 0
			f := newTestFontFetcher(t, cacheDir, func(w http.ResponseWriter, _ *http.Request) {
				callCount++
				_, _ = w.Write([]byte("fake ttf content"))
			})

			result, err := f.GetFont()

			require.NoError(t, err)
			assert.Equal(t, builder.KoruriFontName, result.Name)
			require.FileExists(t, result.Path)

			if tc.expectDownload {
				assert.Equal(t, 1, callCount)
			} else {
				assert.Equal(t, 0, callCount)
			}
		})
	}
}

func TestFontFetcher_DownloadFont(t *testing.T) {
	t.Parallel()

	t.Run("正常系: フォントのダウンロードが成功する", func(t *testing.T) {
		t.Parallel()

		cacheDir := t.TempDir()
		f := newTestFontFetcher(t, cacheDir, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("fake ttf content"))
		})

		result, err := f.DownloadFont()

		require.NoError(t, err)
		assert.Equal(t, builder.KoruriFontName, result.Name)
		assert.Equal(t, ".ttf", filepath.Ext(result.Path))
		require.FileExists(t, result.Path)
		assert.Equal(t, builder.KoruriVersion, result.Version)
	})

	t.Run("異常系: ネットワークエラー時にErrFontDownload", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		client := server.Client()
		server.Close() // クローズ済みサーバーへのリクエストは接続エラーになる

		f := builder.NewFontFetcher(t.TempDir(), client)
		f.SourceURL = server.URL + "/Koruri-Regular.ttf"

		_, err := f.DownloadFont()

		require.ErrorIs(t, err, builder.ErrFontDownload)
	})

	t.Run("異常系: HTTPエラー時にErrFontDownload", func(t *testing.T) {
		t.Parallel()

		cacheDir := t.TempDir()
		f := newTestFontFetcher(t, cacheDir, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		_, err := f.DownloadFont()

		require.ErrorIs(t, err, builder.ErrFontDownload)
		assert.ErrorContains(t, err, "404")
	})
}

func TestFontFetcher_IsCacheValid(t *testing.T) {
	t.Parallel()

	t.Run("正常系: フォントファイルが存在する場合はtrueを返す", func(t *testing.T) {
		t.Parallel()

		cacheDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "Koruri-Regular.ttf"), []byte("cached ttf content"), 0o600))

		f := builder.NewFontFetcher(cacheDir, nil)

		assert.True(t, f.IsCacheValid())
	})

	t.Run("正常系: フォントファイルが存在しない場合はfalseを返す", func(t *testing.T) {
		t.Parallel()

		f := builder.NewFontFetcher(t.TempDir(), nil)

		assert.False(t, f.IsCacheValid())
	})
}

func TestFontFetcher_GetCachedFontPath(t *testing.T) {
	t.Parallel()

	t.Run("正常系: キャッシュされたフォントのパスを返す", func(t *testing.T) {
		t.Parallel()

		cacheDir := t.TempDir()
		fontFile := filepath.Join(cacheDir, "Koruri-Regular.ttf")
		require.NoError(t, os.WriteFile(fontFile, []byte("cached ttf content"), 0o600))

		f := builder.NewFontFetcher(cacheDir, nil)
		result, ok := f.GetCachedFontPath()

		assert.True(t, ok)
		assert.Equal(t, fontFile, result)
	})

	t.Run("正常系: キャッシュが無い場合はokがfalse", func(t *testing.T) {
		t.Parallel()

		f := builder.NewFontFetcher(t.TempDir(), nil)
		result, ok := f.GetCachedFontPath()

		assert.False(t, ok)
		assert.Empty(t, result)
	})
}

// TestFontFetcher_ZeroValue_DoesNotPanic はレビュー指摘の回帰テスト:
// NewFontFetcherを介さずbuilder.FontFetcher{}のゼロ値を直接構築した場合でも、
// HTTPClientフィールドがnilのままnilポインタ参照でpanicしないことを確認する
// （TemplateDownloaderと同じ方針）。
func TestFontFetcher_ZeroValue_DoesNotPanic(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("fake ttf content"))
	}))
	t.Cleanup(server.Close)

	f := &builder.FontFetcher{CacheDir: t.TempDir(), SourceURL: server.URL}

	assert.NotPanics(t, func() {
		_, err := f.DownloadFont()
		require.NoError(t, err)
	})
}
