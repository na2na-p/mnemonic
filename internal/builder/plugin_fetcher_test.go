package builder_test

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/builder"
)

// buildTestPluginZip はfilename単一エントリを含むZIPバイト列を生成する。
func buildTestPluginZip(t *testing.T, filename string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	w, err := zw.Create(filename)
	require.NoError(t, err)
	_, err = w.Write(content)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return buf.Bytes()
}

// singleExtransConfig はテスト用の単一プラグイン設定（テストサーバーURLを埋め込む）。
func singleExtransConfig(serverURL string) []builder.PluginConfig {
	return []builder.PluginConfig{
		{
			Name:           "extrans",
			SourceFilename: "extrans.so",
			OutputFilename: "libextrans.so",
			URLTemplate:    serverURL + "/{abi}/plugin.zip",
		},
	}
}

func writeCachedPluginFiles(t *testing.T, cacheDir string, abis []string) {
	t.Helper()

	for _, abi := range abis {
		abiDir := filepath.Join(cacheDir, abi)
		require.NoError(t, os.MkdirAll(abiDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(abiDir, "libextrans.so"), []byte("cached so content"), 0o600))
	}
}

func TestPluginCacheDir(t *testing.T) {
	t.Parallel()

	t.Run("正常系: キャッシュディレクトリのパスを返す", func(t *testing.T) {
		t.Parallel()

		dir, err := builder.PluginCacheDir()

		require.NoError(t, err)
		assert.Contains(t, dir, "mnemonic")
		assert.Contains(t, dir, "plugins")
	})
}

func TestPluginsInfo_GetAllPathsForABI(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 指定ABIの全プラグインパスを取得する", func(t *testing.T) {
		t.Parallel()

		info := builder.PluginsInfo{
			Plugins: map[string]builder.PluginInfo{
				"extrans": {
					Name: "extrans",
					Paths: map[string]string{
						"arm64-v8a":   "/cache/plugins/arm64-v8a/libextrans.so",
						"armeabi-v7a": "/cache/plugins/armeabi-v7a/libextrans.so",
					},
				},
				"wuvorbis": {
					Name: "wuvorbis",
					Paths: map[string]string{
						"arm64-v8a":   "/cache/plugins/arm64-v8a/libwuvorbis.so",
						"armeabi-v7a": "/cache/plugins/armeabi-v7a/libwuvorbis.so",
					},
				},
			},
		}

		paths := info.GetAllPathsForABI("arm64-v8a")

		assert.Len(t, paths, 2)
		assert.Contains(t, paths, "extrans")
		assert.Contains(t, paths, "wuvorbis")
	})
}

func TestPluginFetcher_GetPlugin(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		cachedExists    bool
		expectCallCount int
	}{
		{name: "正常系: キャッシュが無い場合はダウンロードする", cachedExists: false, expectCallCount: 4},
		{name: "正常系: キャッシュが有効な場合はダウンロードしない", cachedExists: true, expectCallCount: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			if tc.cachedExists {
				writeCachedPluginFiles(t, cacheDir, builder.SupportedABIs)
			}

			zipContent := buildTestPluginZip(t, "extrans.so", []byte("fake so content"))
			callCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callCount++
				_, _ = w.Write(zipContent)
			}))
			t.Cleanup(server.Close)

			f := builder.NewPluginFetcher(cacheDir, server.Client())
			f.PluginConfigs = singleExtransConfig(server.URL)

			result, err := f.GetPlugin()

			require.NoError(t, err)
			assert.Equal(t, "extrans", result.Name)
			assert.Len(t, result.Paths, 4)

			for abi, path := range result.Paths {
				assert.FileExists(t, path, "%sのプラグインが存在するはず", abi)
			}

			assert.Equal(t, tc.expectCallCount, callCount)
		})
	}
}

func TestPluginFetcher_DownloadPlugin(t *testing.T) {
	t.Parallel()

	t.Run("正常系: プラグインのダウンロードが成功する", func(t *testing.T) {
		t.Parallel()

		cacheDir := t.TempDir()
		zipContent := buildTestPluginZip(t, "extrans.so", []byte("fake so content"))

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(zipContent)
		}))
		t.Cleanup(server.Close)

		f := builder.NewPluginFetcher(cacheDir, server.Client())
		f.PluginConfigs = singleExtransConfig(server.URL)

		result, err := f.DownloadPlugin()

		require.NoError(t, err)
		assert.Equal(t, "extrans", result.Name)
		assert.Len(t, result.Paths, 4)
		for _, path := range result.Paths {
			assert.Equal(t, ".so", filepath.Ext(path))
			assert.FileExists(t, path)
		}
	})

	t.Run("異常系: ネットワークエラー時にErrPluginDownload", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		client := server.Client()
		server.Close()

		f := builder.NewPluginFetcher(t.TempDir(), client)
		f.PluginConfigs = singleExtransConfig(server.URL)

		_, err := f.DownloadPlugin()

		require.ErrorIs(t, err, builder.ErrPluginDownload)
	})

	t.Run("異常系: HTTPエラー時にErrPluginDownload", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(server.Close)

		f := builder.NewPluginFetcher(t.TempDir(), server.Client())
		f.PluginConfigs = singleExtransConfig(server.URL)

		_, err := f.DownloadPlugin()

		require.ErrorIs(t, err, builder.ErrPluginDownload)
		assert.ErrorContains(t, err, "404")
	})

	t.Run("異常系: ZIP内に対象ファイルが見つからない場合にErrPluginDownload", func(t *testing.T) {
		t.Parallel()

		zipContent := buildTestPluginZip(t, "unrelated.so", []byte("fake so content"))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(zipContent)
		}))
		t.Cleanup(server.Close)

		f := builder.NewPluginFetcher(t.TempDir(), server.Client())
		f.PluginConfigs = singleExtransConfig(server.URL)

		_, err := f.DownloadPlugin()

		require.ErrorIs(t, err, builder.ErrPluginDownload)
		assert.ErrorContains(t, err, "見つかりません")
	})

	t.Run("異常系: 不正なZIPファイルの場合にErrPluginDownload", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not a zip file"))
		}))
		t.Cleanup(server.Close)

		f := builder.NewPluginFetcher(t.TempDir(), server.Client())
		f.PluginConfigs = singleExtransConfig(server.URL)

		_, err := f.DownloadPlugin()

		require.ErrorIs(t, err, builder.ErrPluginDownload)
		assert.ErrorContains(t, err, "無効なZIP")
	})
}

func TestPluginFetcher_IsCacheValid(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 全ABIのプラグインファイルが存在する場合はtrueを返す", func(t *testing.T) {
		t.Parallel()

		cacheDir := t.TempDir()
		writeCachedPluginFiles(t, cacheDir, builder.SupportedABIs)

		f := builder.NewPluginFetcher(cacheDir, nil)
		f.PluginConfigs = singleExtransConfig("http://example.invalid")

		assert.True(t, f.IsCacheValid())
	})

	t.Run("正常系: 一部ABIのプラグインファイルが欠けている場合はfalseを返す", func(t *testing.T) {
		t.Parallel()

		cacheDir := t.TempDir()
		writeCachedPluginFiles(t, cacheDir, []string{"arm64-v8a", "armeabi-v7a"})

		f := builder.NewPluginFetcher(cacheDir, nil)
		f.PluginConfigs = singleExtransConfig("http://example.invalid")

		assert.False(t, f.IsCacheValid())
	})

	t.Run("正常系: プラグインファイルが存在しない場合はfalseを返す", func(t *testing.T) {
		t.Parallel()

		f := builder.NewPluginFetcher(t.TempDir(), nil)
		f.PluginConfigs = singleExtransConfig("http://example.invalid")

		assert.False(t, f.IsCacheValid())
	})
}

func TestPluginFetcher_GetCachedPluginPaths(t *testing.T) {
	t.Parallel()

	t.Run("正常系: キャッシュされたプラグインのパスを返す", func(t *testing.T) {
		t.Parallel()

		cacheDir := t.TempDir()
		writeCachedPluginFiles(t, cacheDir, builder.SupportedABIs)

		f := builder.NewPluginFetcher(cacheDir, nil)
		f.PluginConfigs = singleExtransConfig("http://example.invalid")

		result, ok := f.GetCachedPluginPaths()

		require.True(t, ok)
		assert.Len(t, result, 4)
		for _, path := range result {
			assert.FileExists(t, path)
		}
	})

	t.Run("正常系: キャッシュが無い場合はokがfalse", func(t *testing.T) {
		t.Parallel()

		f := builder.NewPluginFetcher(t.TempDir(), nil)
		f.PluginConfigs = singleExtransConfig("http://example.invalid")

		result, ok := f.GetCachedPluginPaths()

		assert.False(t, ok)
		assert.Nil(t, result)
	})
}

// TestPluginFetcher_MultiPlugin はfde5205/8afffdfで追加されたマルチプラグイン対応
// （既定でextrans/wuvorbisの2種類を扱う）を検証する。
func TestPluginFetcher_MultiPlugin(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 複数プラグインを一括ダウンロードしPluginsInfoにまとめる", func(t *testing.T) {
		t.Parallel()

		cacheDir := t.TempDir()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// リクエストパス(/extrans/{abi}/... または /wuvorbis/{abi}/...)に応じて
			// 対応するプラグインのsoファイルを含むZIPを返す。
			if strings.Contains(r.URL.Path, "wuvorbis") {
				_, _ = w.Write(buildTestPluginZip(t, "wuvorbis.so", []byte("wuvorbis content")))

				return
			}
			_, _ = w.Write(buildTestPluginZip(t, "extrans.so", []byte("extrans content")))
		}))
		t.Cleanup(server.Close)

		f := builder.NewPluginFetcher(cacheDir, server.Client())
		f.PluginConfigs = []builder.PluginConfig{
			{
				Name:           "extrans",
				SourceFilename: "extrans.so",
				OutputFilename: "libextrans.so",
				URLTemplate:    server.URL + "/extrans/{abi}/plugin.zip",
			},
			{
				Name:           "wuvorbis",
				SourceFilename: "wuvorbis.so",
				OutputFilename: "libwuvorbis.so",
				URLTemplate:    server.URL + "/wuvorbis/{abi}/plugin.zip",
			},
		}

		result, err := f.DownloadAllPlugins()

		require.NoError(t, err)
		assert.Len(t, result.Plugins, 2)
		assert.Contains(t, result.Plugins, "extrans")
		assert.Contains(t, result.Plugins, "wuvorbis")

		for _, abi := range builder.SupportedABIs {
			paths := result.GetAllPathsForABI(abi)
			assert.Len(t, paths, 2)
		}

		assert.True(t, f.IsAllCacheValid())

		cached, ok := f.GetAllCachedPlugins()
		require.True(t, ok)
		assert.Len(t, cached.Plugins, 2)
	})
}

// TestPluginFetcher_ZeroValue_DoesNotPanic はレビュー指摘の回帰テスト:
// NewPluginFetcherを介さずbuilder.PluginFetcher{}のゼロ値を直接構築した場合でも、
// HTTPClientフィールドがnilのままnilポインタ参照でpanicしないことを確認する
// （TemplateDownloaderと同じ方針）。
func TestPluginFetcher_ZeroValue_DoesNotPanic(t *testing.T) {
	t.Parallel()

	zipContent := buildTestPluginZip(t, "extrans.so", []byte("fake so content"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(zipContent)
	}))
	t.Cleanup(server.Close)

	f := &builder.PluginFetcher{CacheDir: t.TempDir(), PluginConfigs: singleExtransConfig(server.URL)}

	assert.NotPanics(t, func() {
		_, err := f.DownloadPlugin()
		require.NoError(t, err)
	})
}
