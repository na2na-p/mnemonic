package builder_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/builder"
)

func newTestDownloader(t *testing.T, cacheDir string, handler http.HandlerFunc) *builder.TemplateDownloader {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	d := builder.NewTemplateDownloader(cacheDir, server.Client())
	d.APIBaseURL = server.URL

	return d
}

func writeJSON(t *testing.T, w http.ResponseWriter, body any) {
	t.Helper()

	require.NoError(t, json.NewEncoder(w).Encode(body))
}

func TestTemplateDownloader_GetLatestVersion(t *testing.T) {
	t.Parallel()

	t.Run("正常系: GitHub APIから最新バージョンを取得", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		d := newTestDownloader(t, t.TempDir(), func(w http.ResponseWriter, r *http.Request) {
			callCount++
			assert.Equal(t, "/releases/latest", r.URL.Path)
			writeJSON(t, w, map[string]string{"tag_name": "template-2026.01.31"})
		})

		version, err := d.GetLatestVersion()

		require.NoError(t, err)
		assert.Equal(t, "template-2026.01.31", version)
		assert.Equal(t, 1, callCount)
	})

	t.Run("異常系: ネットワークエラー時にErrNetwork", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		client := server.Client()
		server.Close() // クローズ済みサーバーへのリクエストは接続エラーになる

		d := builder.NewTemplateDownloader(t.TempDir(), client)
		d.APIBaseURL = server.URL

		_, err := d.GetLatestVersion()

		assert.ErrorIs(t, err, builder.ErrNetwork)
	})

	t.Run("正常系: /releases/latestが404の場合、/releasesにフォールバック", func(t *testing.T) {
		t.Parallel()

		d := newTestDownloader(t, t.TempDir(), func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/releases/latest":
				w.WriteHeader(http.StatusNotFound)
			case "/releases":
				writeJSON(t, w, []map[string]string{
					{"tag_name": "template-2026.01.15"},
					{"tag_name": "template-2026.01.10"},
				})
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
		})

		version, err := d.GetLatestVersion()

		require.NoError(t, err)
		assert.Equal(t, "template-2026.01.15", version)
	})

	t.Run("異常系: フォールバック先でリリースが空の場合、ErrNetwork", func(t *testing.T) {
		t.Parallel()

		d := newTestDownloader(t, t.TempDir(), func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/releases/latest":
				w.WriteHeader(http.StatusNotFound)
			case "/releases":
				writeJSON(t, w, []map[string]string{})
			}
		})

		_, err := d.GetLatestVersion()

		require.ErrorIs(t, err, builder.ErrNetwork)
		assert.ErrorContains(t, err, "リリースが見つかりません")
	})
}

func TestTemplateDownloader_GetDownloadURL(t *testing.T) {
	t.Parallel()

	t.Run("正常系: ダウンロードURLが正しく構築される", func(t *testing.T) {
		t.Parallel()

		d := builder.NewTemplateDownloader("", nil)

		url, err := d.GetDownloadURL("template-2026.01.31")

		require.NoError(t, err)
		assert.Contains(t, url, "template-2026.01.31")
		assert.Contains(t, url, "mnemonic")
		assert.Contains(t, url, "github.com")
	})

	t.Run("正常系: 連番サフィックス付きバージョンでもダウンロードURLが構築される", func(t *testing.T) {
		t.Parallel()

		d := builder.NewTemplateDownloader("", nil)

		url, err := d.GetDownloadURL("template-2026.07.28-12")

		require.NoError(t, err)
		assert.Contains(t, url, "template-2026.07.28-12")
	})

	t.Run("異常系: 不正なバージョン形式でErrInvalidVersion", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name    string
			version string
		}{
			{name: "異常系: 空文字のバージョン", version: ""},
			{name: "異常系: 不正なバージョン形式", version: "invalid"},
			{name: "異常系: vのみのバージョン", version: "v"},
			{name: "異常系: 連番サフィックスが非数値", version: "template-2026.07.28-abc"},
			{name: "異常系: 連番サフィックスのハイフンが二重", version: "template-2026.07.28--12"},
			{name: "異常系: 連番サフィックスがハイフンのみで数値がない", version: "template-2026.07.28-"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				d := builder.NewTemplateDownloader("", nil)

				_, err := d.GetDownloadURL(tc.version)

				assert.ErrorIs(t, err, builder.ErrInvalidVersion)
			})
		}
	})
}

func TestTemplateDownloader_Download(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 指定バージョンのダウンロードが成功する", func(t *testing.T) {
		t.Parallel()

		content := []byte("test content")
		cacheDir := t.TempDir()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/releases/tags/template-2026.01.31":
				downloadURL := "http://" + r.Host + "/download/android-template.zip"
				writeJSON(t, w, map[string]any{
					"tag_name": "template-2026.01.31",
					"assets": []map[string]any{
						{"name": "android-template.zip", "browser_download_url": downloadURL, "size": len(content)},
					},
				})
			case "/download/android-template.zip":
				_, _ = w.Write(content)
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
		}))
		t.Cleanup(server.Close)

		d := builder.NewTemplateDownloader(cacheDir, server.Client())
		d.APIBaseURL = server.URL
		version := "template-2026.01.31"

		result, err := d.Download(&version)

		require.NoError(t, err)
		assert.Equal(t, "android-template.zip", filepath.Base(result))
		assert.Equal(t, filepath.Join(cacheDir, "template-2026.01.31"), filepath.Dir(result))

		got, err := os.ReadFile(result) //nolint:gosec // テストで生成した固定パス
		require.NoError(t, err)
		assert.Equal(t, content, got)
	})

	t.Run("正常系: バージョン未指定時は最新版をダウンロードする", func(t *testing.T) {
		t.Parallel()

		content := []byte("latest content")
		cacheDir := t.TempDir()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/releases/latest":
				writeJSON(t, w, map[string]string{"tag_name": "template-2026.02.15"})
			case "/releases/tags/template-2026.02.15":
				downloadURL := "http://" + r.Host + "/download/android-template.zip"
				writeJSON(t, w, map[string]any{
					"tag_name": "template-2026.02.15",
					"assets": []map[string]any{
						{"name": "android-template.zip", "browser_download_url": downloadURL, "size": len(content)},
					},
				})
			case "/download/android-template.zip":
				_, _ = w.Write(content)
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
		}))
		t.Cleanup(server.Close)

		d := builder.NewTemplateDownloader(cacheDir, server.Client())
		d.APIBaseURL = server.URL

		result, err := d.Download(nil)

		require.NoError(t, err)
		assert.Contains(t, result, "template-2026.02.15")
	})

	t.Run("異常系: 存在しないバージョン指定でErrTemplateNotFound", func(t *testing.T) {
		t.Parallel()

		d := newTestDownloader(t, t.TempDir(), func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		version := "template-9999.99.99"

		_, err := d.Download(&version)

		assert.ErrorIs(t, err, builder.ErrTemplateNotFound)
	})

	t.Run("異常系: ネットワークエラー時にErrNetwork", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		client := server.Client()
		server.Close()

		d := builder.NewTemplateDownloader(t.TempDir(), client)
		d.APIBaseURL = server.URL
		version := "template-2026.01.31"

		_, err := d.Download(&version)

		assert.ErrorIs(t, err, builder.ErrNetwork)
	})
}

func TestTemplateDownloader_IntegrityCheck(t *testing.T) {
	t.Parallel()

	t.Run("正常系: ダウンロードファイルの整合性チェックが成功する", func(t *testing.T) {
		t.Parallel()

		content := []byte("test content")
		cacheDir := t.TempDir()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/releases/tags/template-2026.01.31":
				downloadURL := "http://" + r.Host + "/download/android-template.zip"
				writeJSON(t, w, map[string]any{
					"tag_name": "template-2026.01.31",
					"assets": []map[string]any{
						{"name": "android-template.zip", "browser_download_url": downloadURL, "size": len(content)},
					},
				})
			case "/download/android-template.zip":
				_, _ = w.Write(content)
			}
		}))
		t.Cleanup(server.Close)

		d := builder.NewTemplateDownloader(cacheDir, server.Client())
		d.APIBaseURL = server.URL
		version := "template-2026.01.31"

		result, err := d.Download(&version)

		require.NoError(t, err)
		info, err := os.Stat(result)
		require.NoError(t, err)
		assert.Equal(t, int64(len(content)), info.Size())
	})

	t.Run("異常系: 破損ファイルのダウンロード時にErrFileIntegrity", func(t *testing.T) {
		t.Parallel()

		content := []byte("test content")
		cacheDir := t.TempDir()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/releases/tags/template-2026.01.31":
				downloadURL := "http://" + r.Host + "/download/android-template.zip"
				writeJSON(t, w, map[string]any{
					"tag_name": "template-2026.01.31",
					"assets": []map[string]any{
						// サイズ不一致を意図的に起こす
						{"name": "android-template.zip", "browser_download_url": downloadURL, "size": 1000},
					},
				})
			case "/download/android-template.zip":
				_, _ = w.Write(content)
			}
		}))
		t.Cleanup(server.Close)

		d := builder.NewTemplateDownloader(cacheDir, server.Client())
		d.APIBaseURL = server.URL
		version := "template-2026.01.31"

		_, err := d.Download(&version)

		assert.ErrorIs(t, err, builder.ErrFileIntegrity)
	})
}

func TestTemplateDownloader_Download_RetriesOnTransientNetworkError(t *testing.T) {
	t.Parallel()

	content := []byte("retry content")
	cacheDir := t.TempDir()

	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/tags/template-2026.01.31":
			downloadURL := "http://" + r.Host + "/download/android-template.zip"
			writeJSON(t, w, map[string]any{
				"tag_name": "template-2026.01.31",
				"assets": []map[string]any{
					{"name": "android-template.zip", "browser_download_url": downloadURL, "size": len(content)},
				},
			})
		case "/download/android-template.zip":
			attempts++
			if attempts < 2 {
				// 接続を切断してネットワークエラーを模擬する。
				// why not: ハンドラはテストのメインgoroutineとは別で実行されるため
				// require（内部でt.FailNow）は使えない（testifylintのgo-require
				// ルールが検出する既知の制約）。Hijackerでない/失敗時はpanicで
				// 早期に気付ける形にする。
				hj, ok := w.(http.Hijacker)
				if !ok {
					panic("http.ResponseWriter does not implement http.Hijacker")
				}
				conn, _, err := hj.Hijack()
				if err != nil {
					panic(err)
				}
				_ = conn.Close()

				return
			}
			_, _ = w.Write(content)
		}
	}))
	t.Cleanup(server.Close)

	d := builder.NewTemplateDownloader(cacheDir, server.Client())
	d.APIBaseURL = server.URL
	version := "template-2026.01.31"

	result, err := d.Download(&version)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, attempts, 2)

	got, err := os.ReadFile(result) //nolint:gosec // テストで生成した固定パス
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// TestTemplateDownloader_Download_DoesNotRetryOnHTTPServerError はレビュー指摘の
// 回帰テスト: 5xxはサーバーが応答済み（接続は成功している）エラーであり、
// 再試行しても解決しないため、downloadFileはこれを再試行しない
// （errRetryableDownloadのwhy not参照）。
func TestTemplateDownloader_Download_DoesNotRetryOnHTTPServerError(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	downloadAttempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/tags/template-2026.01.31":
			downloadURL := "http://" + r.Host + "/download/android-template.zip"
			writeJSON(t, w, map[string]any{
				"tag_name": "template-2026.01.31",
				"assets": []map[string]any{
					{"name": "android-template.zip", "browser_download_url": downloadURL, "size": 4},
				},
			})
		case "/download/android-template.zip":
			downloadAttempts++
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	d := builder.NewTemplateDownloader(cacheDir, server.Client())
	d.APIBaseURL = server.URL
	version := "template-2026.01.31"

	_, err := d.Download(&version)

	require.ErrorIs(t, err, builder.ErrNetwork)
	assert.Equal(t, 1, downloadAttempts, "5xxは再試行対象外のため試行は1回のみのはず")
}

// TestTemplateDownloader_ZeroValue_DoesNotPanic はレビュー指摘の回帰テスト:
// NewTemplateDownloaderを介さずbuilder.TemplateDownloader{}のゼロ値を直接構築
// した場合でも、HTTPClientフィールドがnilのままnilポインタ参照でpanicしない
// ことを確認する。
func TestTemplateDownloader_ZeroValue_DoesNotPanic(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]string{"tag_name": "template-2026.01.31"})
	}))
	t.Cleanup(server.Close)

	d := &builder.TemplateDownloader{CacheDir: t.TempDir(), APIBaseURL: server.URL}

	assert.NotPanics(t, func() {
		_, err := d.GetLatestVersion()
		require.NoError(t, err)
	})
}
