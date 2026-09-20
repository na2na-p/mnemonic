package builder_test

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestPluginFetcher_GetPlugins(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		cachedExists    bool
		expectCallCount int32
	}{
		{name: "正常系: キャッシュが無い場合は全プラグインをダウンロードする", cachedExists: false, expectCallCount: int32(len(builder.SupportedABIs))},
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
			var callCount atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callCount.Add(1)
				_, _ = w.Write(zipContent)
			}))
			t.Cleanup(server.Close)

			f := builder.NewPluginFetcher(cacheDir, server.Client())
			f.PluginConfigs = singleExtransConfig(server.URL)

			result, err := f.GetPlugins()

			require.NoError(t, err)
			extrans, ok := result.Plugins["extrans"]
			require.True(t, ok)
			assert.Equal(t, "extrans", extrans.Name)
			assert.Len(t, extrans.Paths, len(builder.SupportedABIs))
			for abi, path := range extrans.Paths {
				assert.FileExists(t, path, "%sのプラグインが存在するはず", abi)
			}
			assert.Equal(t, tc.expectCallCount, callCount.Load())
		})
	}
}

func TestPluginFetcher_DownloadAllPlugins(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 単一プラグインのダウンロードが成功する", func(t *testing.T) {
		t.Parallel()

		cacheDir := t.TempDir()
		zipContent := buildTestPluginZip(t, "extrans.so", []byte("fake so content"))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(zipContent)
		}))
		t.Cleanup(server.Close)

		f := builder.NewPluginFetcher(cacheDir, server.Client())
		f.PluginConfigs = singleExtransConfig(server.URL)

		result, err := f.DownloadAllPlugins()

		require.NoError(t, err)
		extrans, ok := result.Plugins["extrans"]
		require.True(t, ok)
		assert.Equal(t, "extrans", extrans.Name)
		assert.Len(t, extrans.Paths, len(builder.SupportedABIs))
		for _, path := range extrans.Paths {
			assert.Equal(t, ".so", filepath.Ext(path))
			assert.FileExists(t, path)
		}
	})

	t.Run("異常系: ネットワークエラー時にErrPluginDownload", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		client := server.Client()
		serverURL := server.URL
		server.Close()

		f := builder.NewPluginFetcher(t.TempDir(), client)
		f.PluginConfigs = singleExtransConfig(serverURL)

		_, err := f.DownloadAllPlugins()

		require.ErrorIs(t, err, builder.ErrPluginDownload)
	})

	testCases := []struct {
		name         string
		statusCode   int
		zipFilename  string
		responseBody []byte
		wantError    string
	}{
		{name: "異常系: HTTPエラー時にErrPluginDownload", statusCode: http.StatusNotFound, wantError: "404"},
		{name: "異常系: ZIP内に対象ファイルが見つからない場合にErrPluginDownload", statusCode: http.StatusOK, zipFilename: "unrelated.so", wantError: "見つかりません"},
		{name: "異常系: 不正なZIPファイルの場合にErrPluginDownload", statusCode: http.StatusOK, responseBody: []byte("not a zip file"), wantError: "無効なZIP"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			responseBody := tc.responseBody
			if tc.zipFilename != "" {
				responseBody = buildTestPluginZip(t, tc.zipFilename, []byte("fake so content"))
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write(responseBody)
			}))
			t.Cleanup(server.Close)

			f := builder.NewPluginFetcher(t.TempDir(), server.Client())
			f.PluginConfigs = singleExtransConfig(server.URL)

			_, err := f.DownloadAllPlugins()

			require.ErrorIs(t, err, builder.ErrPluginDownload)
			assert.ErrorContains(t, err, tc.wantError)
		})
	}

	t.Run("正常系: HTTPクライアント未設定のゼロ値でもダウンロードできる", func(t *testing.T) {
		t.Parallel()

		zipContent := buildTestPluginZip(t, "extrans.so", []byte("fake so content"))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(zipContent)
		}))
		t.Cleanup(server.Close)

		f := &builder.PluginFetcher{CacheDir: t.TempDir(), PluginConfigs: singleExtransConfig(server.URL)}

		assert.NotPanics(t, func() {
			_, err := f.DownloadAllPlugins()
			require.NoError(t, err)
		})
	})
}

func TestPluginFetcher_IsAllCacheValid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		abis []string
		want bool
	}{
		{name: "正常系: 全ABIのプラグインファイルが存在する場合はtrueを返す", abis: builder.SupportedABIs, want: true},
		{name: "正常系: 一部ABIのプラグインファイルが欠けている場合はfalseを返す", abis: []string{"arm64-v8a", "armeabi-v7a"}, want: false},
		{name: "正常系: プラグインファイルが存在しない場合はfalseを返す", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			writeCachedPluginFiles(t, cacheDir, tc.abis)

			f := builder.NewPluginFetcher(cacheDir, nil)
			f.PluginConfigs = singleExtransConfig("http://example.invalid")

			assert.Equal(t, tc.want, f.IsAllCacheValid())
		})
	}
}

func TestPluginFetcher_GetAllCachedPlugins(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		cachedExists bool
	}{
		{name: "正常系: キャッシュされた全プラグインのパスを返す", cachedExists: true},
		{name: "正常系: キャッシュが無い場合はokがfalse", cachedExists: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			if tc.cachedExists {
				writeCachedPluginFiles(t, cacheDir, builder.SupportedABIs)
			}

			f := builder.NewPluginFetcher(cacheDir, nil)
			f.PluginConfigs = singleExtransConfig("http://example.invalid")

			result, ok := f.GetAllCachedPlugins()

			assert.Equal(t, tc.cachedExists, ok)
			if !tc.cachedExists {
				assert.Nil(t, result.Plugins)

				return
			}

			extrans, exists := result.Plugins["extrans"]
			require.True(t, exists)
			assert.Equal(t, "extrans", extrans.Name)
			assert.Len(t, extrans.Paths, len(builder.SupportedABIs))
			for _, path := range extrans.Paths {
				assert.FileExists(t, path)
			}
		})
	}
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
