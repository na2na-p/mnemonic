package pipeline

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/builder"
)

func TestBuildPipeline_RemovePluginDirectory(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 小文字のpluginディレクトリが削除される", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		pluginDir := filepath.Join(dir, "plugin")
		require.NoError(t, os.MkdirAll(pluginDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "test.dll"), make([]byte, 10), 0o600))

		require.NoError(t, p.removePluginDirectory(dir))

		assert.NoDirExists(t, pluginDir)
	})

	for _, dirName := range []string{"Plugin", "PLUGIN", "Plugins", "plugins", "PLUGINS"} {
		t.Run("正常系: 大文字小文字のバリエーション"+dirName+"が削除される", func(t *testing.T) {
			t.Parallel()

			p := newTestPipeline(t)
			dir := t.TempDir()
			pluginDir := filepath.Join(dir, dirName)
			require.NoError(t, os.MkdirAll(pluginDir, 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "wuvorbis.dll"), make([]byte, 10), 0o600))

			require.NoError(t, p.removePluginDirectory(dir))

			assert.NoDirExists(t, pluginDir)
		})
	}

	t.Run("正常系: pluginディレクトリがない場合は何もしない", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		otherDir := filepath.Join(dir, "other")
		require.NoError(t, os.MkdirAll(otherDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(otherDir, "test.txt"), []byte("test"), 0o600))

		require.NoError(t, p.removePluginDirectory(dir))

		assert.DirExists(t, otherDir)
		assert.FileExists(t, filepath.Join(otherDir, "test.txt"))
	})

	t.Run("正常系: ネストされたDLLファイルも含めて削除される", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		pluginDir := filepath.Join(dir, "plugin")
		subDir := filepath.Join(pluginDir, "subdir")
		require.NoError(t, os.MkdirAll(subDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "wuvorbis.dll"), make([]byte, 10), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(subDir, "other.dll"), make([]byte, 10), 0o600))

		require.NoError(t, p.removePluginDirectory(dir))

		assert.NoDirExists(t, pluginDir)
	})
}

func TestBuildPipeline_FetchPlugins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fetcher      func(t *testing.T) *builder.PluginFetcher
		wantPlugins  bool
		wantWarnings int
	}{
		{
			name:         "正常系: 取得に失敗した場合は警告を1件出してプラグイン無しを返す",
			fetcher:      failingPluginFetcher,
			wantPlugins:  false,
			wantWarnings: 1,
		},
		{
			name:         "正常系: キャッシュから取得できた場合は警告しない",
			fetcher:      offlinePluginFetcher,
			wantPlugins:  true,
			wantWarnings: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := &recordingLogger{}

			got := fetchPluginsUsing(tc.fetcher(t), logger)

			assert.Equal(t, tc.wantPlugins, got != nil)
			warnings := logger.messages("WARNING")
			require.Len(t, warnings, tc.wantWarnings)
			for _, w := range warnings {
				assert.Contains(t, w, "プラグイン")
				assert.Contains(t, w, "テストでは実ネットワークへのアクセスを許可しない", "取得失敗の原因を警告に含める")
			}
		})
	}
}

// failingPluginFetcher はキャッシュが空で、ダウンロードが必ず失敗するPluginFetcherを返す。
func failingPluginFetcher(t *testing.T) *builder.PluginFetcher {
	t.Helper()

	return builder.NewPluginFetcher(t.TempDir(), &http.Client{Transport: alwaysFailRoundTripper{}})
}

// offlinePluginFetcher は全プラグインのキャッシュを持つPluginFetcherを返す
// （実ネットワークに触れずGetPlugins()が成功する）。
func offlinePluginFetcher(t *testing.T) *builder.PluginFetcher {
	t.Helper()

	cacheDir := t.TempDir()
	for _, abi := range builder.SupportedABIs {
		require.NoError(t, os.MkdirAll(filepath.Join(cacheDir, abi), 0o750))
		for _, config := range builder.DefaultPluginConfigs {
			require.NoError(t, os.WriteFile(filepath.Join(cacheDir, abi, config.OutputFilename), []byte("fake so"), 0o600))
		}
	}

	return builder.NewPluginFetcher(cacheDir, &http.Client{Transport: alwaysFailRoundTripper{}})
}
