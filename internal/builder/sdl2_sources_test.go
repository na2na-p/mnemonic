package builder_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/builder"
)

func writeValidSDL2Cache(t *testing.T, cache *builder.SDL2SourceCache) {
	t.Helper()

	require.NoError(t, os.MkdirAll(cache.CachePath(), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(cache.CachePath(), builder.SDL2CacheMarkerFile),
		[]byte(time.Now().Format(time.RFC3339Nano)),
		0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(cache.CachePath(), builder.SDL2CacheVersionFile),
		[]byte(builder.SDL2CacheCurrentVersion),
		0o600,
	))
}

func TestSDL2SourceCache_New(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		dirName string
	}{
		{name: "正常系: 通常のディレクトリ名", dirName: "cache"},
		{name: "正常系: ハイフン付きディレクトリ名", dirName: "my-cache"},
		{name: "正常系: 数字付きディレクトリ名", dirName: "cache_123"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := filepath.Join(t.TempDir(), tc.dirName)
			cache := builder.NewSDL2SourceCache(base)

			assert.Equal(t, filepath.Join(base, "sdl2_sources"), cache.CachePath())
		})
	}
}

func TestSDL2SourceCache_IsValid(t *testing.T) {
	t.Parallel()

	t.Run("正常系: キャッシュが存在しない場合はfalse", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(t.TempDir())

		assert.False(t, cache.IsValid())
	})

	t.Run("正常系: マーカーファイルがない場合はfalse", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(t.TempDir())
		require.NoError(t, os.MkdirAll(cache.CachePath(), 0o750))

		assert.False(t, cache.IsValid())
	})

	t.Run("正常系: 有効期限内かつバージョンが一致する場合はtrue", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(t.TempDir())
		writeValidSDL2Cache(t, cache)

		assert.True(t, cache.IsValid())
	})

	t.Run("正常系: 有効期限切れの場合はfalse", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(t.TempDir())
		require.NoError(t, os.MkdirAll(cache.CachePath(), 0o750))

		expired := time.Now().AddDate(0, 0, -(builder.SDL2CacheValidityDays + 1))
		require.NoError(t, os.WriteFile(
			filepath.Join(cache.CachePath(), builder.SDL2CacheMarkerFile),
			[]byte(expired.Format(time.RFC3339Nano)),
			0o600,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(cache.CachePath(), builder.SDL2CacheVersionFile),
			[]byte(builder.SDL2CacheCurrentVersion),
			0o600,
		))

		assert.False(t, cache.IsValid())
	})

	t.Run("正常系: バージョンが不一致の場合はfalse", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(t.TempDir())
		require.NoError(t, os.MkdirAll(cache.CachePath(), 0o750))
		require.NoError(t, os.WriteFile(
			filepath.Join(cache.CachePath(), builder.SDL2CacheMarkerFile),
			[]byte(time.Now().Format(time.RFC3339Nano)),
			0o600,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(cache.CachePath(), builder.SDL2CacheVersionFile),
			[]byte("old_version"),
			0o600,
		))

		assert.False(t, cache.IsValid())
	})

	t.Run("正常系: バージョンファイルがない場合はfalse", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(t.TempDir())
		require.NoError(t, os.MkdirAll(cache.CachePath(), 0o750))
		require.NoError(t, os.WriteFile(
			filepath.Join(cache.CachePath(), builder.SDL2CacheMarkerFile),
			[]byte(time.Now().Format(time.RFC3339Nano)),
			0o600,
		))

		assert.False(t, cache.IsValid())
	})

	t.Run("正常系: マーカーファイルの内容が不正な場合はfalse", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(t.TempDir())
		require.NoError(t, os.MkdirAll(cache.CachePath(), 0o750))
		require.NoError(t, os.WriteFile(
			filepath.Join(cache.CachePath(), builder.SDL2CacheMarkerFile),
			[]byte("invalid date format"),
			0o600,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(cache.CachePath(), builder.SDL2CacheVersionFile),
			[]byte(builder.SDL2CacheCurrentVersion),
			0o600,
		))

		assert.False(t, cache.IsValid())
	})
}

func TestSDL2SourceCache_GetCachedAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系: マーカーが存在しない場合はokがfalse", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(t.TempDir())

		_, ok := cache.GetCachedAt()

		assert.False(t, ok)
	})

	t.Run("正常系: マーカーが存在する場合は日時を返す", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(t.TempDir())
		require.NoError(t, os.MkdirAll(cache.CachePath(), 0o750))

		now := time.Now()
		require.NoError(t, os.WriteFile(
			filepath.Join(cache.CachePath(), builder.SDL2CacheMarkerFile),
			[]byte(now.Format(time.RFC3339Nano)),
			0o600,
		))

		result, ok := cache.GetCachedAt()

		require.True(t, ok)
		assert.WithinDuration(t, now, result, time.Second)
	})
}

func TestSDL2SourceCache_GetCacheInfo(t *testing.T) {
	t.Parallel()

	t.Run("正常系: SDL2SourceCacheInfoを返す", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(t.TempDir())
		writeValidSDL2Cache(t, cache)

		info := cache.GetCacheInfo()

		assert.True(t, info.IsValid)
		assert.Equal(t, cache.CachePath(), info.CachePath)
		require.NotNil(t, info.CachedAt)
	})
}

func TestSDL2SourceCache_Save(t *testing.T) {
	t.Parallel()

	t.Run("正常系: orgディレクトリをキャッシュにコピーする", func(t *testing.T) {
		t.Parallel()

		base := t.TempDir()
		cache := builder.NewSDL2SourceCache(filepath.Join(base, "cache"))

		sourceDir := filepath.Join(base, "source")
		orgDir := filepath.Join(sourceDir, "org", "libsdl", "app")
		require.NoError(t, os.MkdirAll(orgDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(orgDir, "SDLActivity.java"), []byte("test content"), 0o600))

		require.NoError(t, cache.Save(sourceDir))

		cachedFile := filepath.Join(cache.GetSourceFilesPath(), "SDLActivity.java")
		content, err := os.ReadFile(cachedFile) //nolint:gosec // テストで生成した固定パス
		require.NoError(t, err)
		assert.Equal(t, "test content", string(content))
	})

	t.Run("正常系: キャッシュマーカーファイルを作成する", func(t *testing.T) {
		t.Parallel()

		base := t.TempDir()
		cache := builder.NewSDL2SourceCache(filepath.Join(base, "cache"))

		sourceDir := filepath.Join(base, "source")
		orgDir := filepath.Join(sourceDir, "org", "libsdl", "app")
		require.NoError(t, os.MkdirAll(orgDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(orgDir, "SDLActivity.java"), []byte("test content"), 0o600))

		require.NoError(t, cache.Save(sourceDir))

		assert.FileExists(t, filepath.Join(cache.CachePath(), builder.SDL2CacheMarkerFile))
	})

	t.Run("正常系: 既存のキャッシュを上書きする", func(t *testing.T) {
		t.Parallel()

		base := t.TempDir()
		cache := builder.NewSDL2SourceCache(filepath.Join(base, "cache"))

		source1 := filepath.Join(base, "source1")
		orgDir1 := filepath.Join(source1, "org", "libsdl", "app")
		require.NoError(t, os.MkdirAll(orgDir1, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(orgDir1, "SDLActivity.java"), []byte("old content"), 0o600))
		require.NoError(t, cache.Save(source1))

		source2 := filepath.Join(base, "source2")
		orgDir2 := filepath.Join(source2, "org", "libsdl", "app")
		require.NoError(t, os.MkdirAll(orgDir2, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(orgDir2, "SDLActivity.java"), []byte("new content"), 0o600))
		require.NoError(t, cache.Save(source2))

		content, err := os.ReadFile(filepath.Join(cache.GetSourceFilesPath(), "SDLActivity.java")) //nolint:gosec // テストで生成した固定パス
		require.NoError(t, err)
		assert.Equal(t, "new content", string(content))
	})
}

func TestSDL2SourceCache_RestoreTo(t *testing.T) {
	t.Parallel()

	t.Run("正常系: キャッシュからソースを復元する", func(t *testing.T) {
		t.Parallel()

		base := t.TempDir()
		cache := builder.NewSDL2SourceCache(filepath.Join(base, "cache"))

		sourceDir := filepath.Join(base, "source")
		orgDir := filepath.Join(sourceDir, "org", "libsdl", "app")
		require.NoError(t, os.MkdirAll(orgDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(orgDir, "SDLActivity.java"), []byte("test content"), 0o600))
		require.NoError(t, cache.Save(sourceDir))

		destDir := filepath.Join(base, "dest")
		require.NoError(t, os.MkdirAll(destDir, 0o750))
		require.NoError(t, cache.RestoreTo(destDir))

		restored := filepath.Join(destDir, "org", "libsdl", "app", "SDLActivity.java")
		content, err := os.ReadFile(restored) //nolint:gosec // テストで生成した固定パス
		require.NoError(t, err)
		assert.Equal(t, "test content", string(content))
	})

	t.Run("異常系: キャッシュが無効な場合はErrSDL2SourceCache", func(t *testing.T) {
		t.Parallel()

		base := t.TempDir()
		cache := builder.NewSDL2SourceCache(filepath.Join(base, "cache"))
		destDir := filepath.Join(base, "dest")
		require.NoError(t, os.MkdirAll(destDir, 0o750))

		err := cache.RestoreTo(destDir)

		require.ErrorIs(t, err, builder.ErrSDL2SourceCache)
		assert.ErrorContains(t, err, "有効なキャッシュがありません")
	})
}

func TestSDL2SourceCache_Clear(t *testing.T) {
	t.Parallel()

	t.Run("正常系: キャッシュディレクトリを削除する", func(t *testing.T) {
		t.Parallel()

		base := t.TempDir()
		cache := builder.NewSDL2SourceCache(filepath.Join(base, "cache"))

		sourceDir := filepath.Join(base, "source")
		orgDir := filepath.Join(sourceDir, "org", "libsdl", "app")
		require.NoError(t, os.MkdirAll(orgDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(orgDir, "SDLActivity.java"), []byte("test content"), 0o600))
		require.NoError(t, cache.Save(sourceDir))

		require.DirExists(t, cache.CachePath())

		require.NoError(t, cache.Clear())

		assert.NoDirExists(t, cache.CachePath())
	})

	t.Run("正常系: キャッシュが存在しなくてもエラーにならない", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(filepath.Join(t.TempDir(), "cache"))

		assert.NoError(t, cache.Clear())
	})
}

func TestSDL2SourceFetcher_New(t *testing.T) {
	t.Parallel()

	t.Run("正常系: デフォルトタイムアウトが設定される", func(t *testing.T) {
		t.Parallel()

		f := builder.NewSDL2SourceFetcher(0, nil)

		assert.Equal(t, 30*time.Second, f.Timeout)
	})

	t.Run("正常系: カスタムタイムアウトを設定できる", func(t *testing.T) {
		t.Parallel()

		f := builder.NewSDL2SourceFetcher(60*time.Second, nil)

		assert.Equal(t, 60*time.Second, f.Timeout)
	})

	t.Run("正常系: キャッシュを設定できる", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(t.TempDir())
		f := builder.NewSDL2SourceFetcher(0, cache)

		assert.Same(t, cache, f.Cache)
	})
}

// sdl2FetcherHandler は8ファイル分のJavaソースを返すハンドラーを構築する。
func sdl2FetcherHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("downloaded content: " + r.URL.Path))
	}
}

func TestSDL2SourceFetcher_Fetch(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 有効なキャッシュがある場合は復元する", func(t *testing.T) {
		t.Parallel()

		base := t.TempDir()
		cache := builder.NewSDL2SourceCache(filepath.Join(base, "cache"))

		sourceDir := filepath.Join(base, "source")
		orgDir := filepath.Join(sourceDir, "org", "libsdl", "app")
		require.NoError(t, os.MkdirAll(orgDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(orgDir, "SDLActivity.java"), []byte("cached content"), 0o600))
		require.NoError(t, cache.Save(sourceDir))

		f := builder.NewSDL2SourceFetcher(0, cache)
		destDir := filepath.Join(base, "dest")
		require.NoError(t, os.MkdirAll(destDir, 0o750))

		require.NoError(t, f.Fetch(destDir))

		restored := filepath.Join(destDir, "org", "libsdl", "app", "SDLActivity.java")
		content, err := os.ReadFile(restored) //nolint:gosec // テストで生成した固定パス
		require.NoError(t, err)
		assert.Equal(t, "cached content", string(content))
	})

	t.Run("正常系: キャッシュが無効な場合はダウンロードする", func(t *testing.T) {
		t.Parallel()

		base := t.TempDir()
		cache := builder.NewSDL2SourceCache(filepath.Join(base, "cache"))

		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			_, _ = w.Write([]byte("downloaded content: " + r.URL.Path))
		}))
		t.Cleanup(server.Close)

		f := builder.NewSDL2SourceFetcher(0, cache)
		f.BaseURL = server.URL
		destDir := filepath.Join(base, "dest")
		require.NoError(t, os.MkdirAll(destDir, 0o750))

		require.NoError(t, f.Fetch(destDir))

		sdlAppDir := filepath.Join(destDir, "org", "libsdl", "app")
		assert.DirExists(t, sdlAppDir)
		assert.FileExists(t, filepath.Join(sdlAppDir, "SDLActivity.java"))
		assert.Equal(t, len(builder.SDL2RequiredFiles), callCount)
	})

	t.Run("正常系: ダウンロード後にキャッシュに保存する", func(t *testing.T) {
		t.Parallel()

		base := t.TempDir()
		cache := builder.NewSDL2SourceCache(filepath.Join(base, "cache"))

		server := httptest.NewServer(sdl2FetcherHandler(t))
		t.Cleanup(server.Close)

		f := builder.NewSDL2SourceFetcher(0, cache)
		f.BaseURL = server.URL
		destDir := filepath.Join(base, "dest")
		require.NoError(t, os.MkdirAll(destDir, 0o750))

		require.NoError(t, f.Fetch(destDir))

		assert.True(t, cache.IsValid())
	})

	t.Run("異常系: HTTPエラーの場合はErrSDL2SourceFetchNetwork", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(server.Close)

		f := builder.NewSDL2SourceFetcher(0, nil)
		f.BaseURL = server.URL
		destDir := t.TempDir()

		err := f.Fetch(destDir)

		require.ErrorIs(t, err, builder.ErrSDL2SourceFetchNetwork)
		assert.ErrorContains(t, err, "HTTP 404")
	})

	t.Run("異常系: ヘッダー受信前のタイムアウトの場合はErrSDL2SourceFetchTimeout", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(200 * time.Millisecond)
			_, _ = w.Write([]byte("too late"))
		}))
		t.Cleanup(server.Close)

		f := builder.NewSDL2SourceFetcher(10*time.Millisecond, nil)
		f.BaseURL = server.URL
		destDir := t.TempDir()

		err := f.Fetch(destDir)

		require.ErrorIs(t, err, builder.ErrSDL2SourceFetchTimeout)
		assert.ErrorContains(t, err, "タイムアウト")
	})

	// TestSDL2SourceFetcher_Fetch/異常系:_レスポンスボディ読み込み中のタイムアウト は
	// レビュー指摘の回帰テスト: 上のケースはヘッダー受信前（client.Do自体）の
	// タイムアウトしか検証できておらず、resp.Body.Read（io.ReadAll）側での
	// タイムアウトがErrSDL2SourceFetchNetworkに誤分類される欠陥を見逃していた。
	// ハンドラーでヘッダーを明示的にflushしてclient.Doを先に成功させ、
	// ボディ転送中にタイムアウトさせることで、その分岐を検証する。
	t.Run("異常系: レスポンスボディ読み込み中のタイムアウトの場合もErrSDL2SourceFetchTimeout", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			// ヘッダー送出後、Content-Lengthの分のボディを書かずにクライアントの
			// タイムアウトを超えて待機する。
			time.Sleep(200 * time.Millisecond)
		}))
		t.Cleanup(server.Close)

		f := builder.NewSDL2SourceFetcher(10*time.Millisecond, nil)
		f.BaseURL = server.URL
		destDir := t.TempDir()

		err := f.Fetch(destDir)

		require.ErrorIs(t, err, builder.ErrSDL2SourceFetchTimeout)
		assert.ErrorContains(t, err, "タイムアウト")
	})

	t.Run("異常系: 接続エラーの場合はErrSDL2SourceFetchNetwork", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		server.Close() // クローズ済みサーバーへのリクエストは接続エラーになる

		f := builder.NewSDL2SourceFetcher(0, nil)
		f.BaseURL = server.URL
		destDir := t.TempDir()

		err := f.Fetch(destDir)

		require.ErrorIs(t, err, builder.ErrSDL2SourceFetchNetwork)
		assert.ErrorContains(t, err, "ダウンロードに失敗")
	})
}

func TestSDL2SourceFetcher_RequiredFiles(t *testing.T) {
	t.Parallel()

	t.Run("正常系: すべての必要なファイルが含まれている（krkrsdl2互換コミット）", func(t *testing.T) {
		t.Parallel()

		expected := []string{
			"SDLActivity.java",
			"SDL.java",
			"SDLAudioManager.java",
			"SDLControllerManager.java",
			"HIDDevice.java",
			"HIDDeviceManager.java",
			"HIDDeviceUSB.java",
			"HIDDeviceBLESteamController.java",
		}

		assert.Equal(t, expected, builder.SDL2RequiredFiles)
	})
}

// TestSDL2CacheCurrentVersion はレビュー指摘の回帰テスト: SDL2CacheCurrentVersionが
// krkrsdl2互換コミット(53dea9830964eee8b5c2a7ee0a65d6e268dc78a1)の先頭8文字と
// 一致することをピン留めする。実装側はコメントによる手動同期ではなく
// sdlCommit[:8]からの構造的な派生に変更済みだが、その値自体が意図した
// コミットからずれていないかは外部から検証できる形で残す。
func TestSDL2CacheCurrentVersion(t *testing.T) {
	t.Parallel()

	t.Run("正常系: krkrsdl2互換SDLコミットの先頭8文字と一致する", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "53dea983", builder.SDL2CacheCurrentVersion)
		assert.Len(t, builder.SDL2CacheCurrentVersion, 8)
	})
}

// TestSDL2SourceErrorHierarchy はFetch/RestoreToが実際に返すエラーが
// ErrSDL2SourceFetcher（基底）と各具体センチネルの両方を満たすこと
// （errors.Is）を検証する。
func TestSDL2SourceErrorHierarchy(t *testing.T) {
	t.Parallel()

	t.Run("正常系: HTTPエラーはErrSDL2SourceFetcherとErrSDL2SourceFetchNetworkの両方を満たす", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(server.Close)

		f := builder.NewSDL2SourceFetcher(0, nil)
		f.BaseURL = server.URL

		err := f.Fetch(t.TempDir())

		require.ErrorIs(t, err, builder.ErrSDL2SourceFetcher)
		require.ErrorIs(t, err, builder.ErrSDL2SourceFetchNetwork)
	})

	t.Run("正常系: タイムアウトエラーはErrSDL2SourceFetcherとErrSDL2SourceFetchTimeoutの両方を満たす", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(200 * time.Millisecond)
			_, _ = w.Write([]byte("too late"))
		}))
		t.Cleanup(server.Close)

		f := builder.NewSDL2SourceFetcher(10*time.Millisecond, nil)
		f.BaseURL = server.URL

		err := f.Fetch(t.TempDir())

		require.ErrorIs(t, err, builder.ErrSDL2SourceFetcher)
		require.ErrorIs(t, err, builder.ErrSDL2SourceFetchTimeout)
	})

	t.Run("正常系: キャッシュエラーはErrSDL2SourceFetcherとErrSDL2SourceCacheの両方を満たす", func(t *testing.T) {
		t.Parallel()

		cache := builder.NewSDL2SourceCache(t.TempDir())

		err := cache.RestoreTo(t.TempDir())

		require.ErrorIs(t, err, builder.ErrSDL2SourceFetcher)
		require.ErrorIs(t, err, builder.ErrSDL2SourceCache)
	})
}

// TestSDL2SourceFetcher_ZeroValue_DoesNotPanic はレビュー指摘の回帰テスト:
// NewSDL2SourceFetcherを介さずbuilder.SDL2SourceFetcher{}のゼロ値を直接構築した
// 場合でも、HTTPClientフィールドがnilのままnilポインタ参照でpanicしないことを
// 確認する（TemplateDownloaderと同じ方針）。
func TestSDL2SourceFetcher_ZeroValue_DoesNotPanic(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(sdl2FetcherHandler(t))
	t.Cleanup(server.Close)

	f := &builder.SDL2SourceFetcher{BaseURL: server.URL}
	destDir := t.TempDir()

	assert.NotPanics(t, func() {
		require.NoError(t, f.Fetch(destDir))
	})
}
