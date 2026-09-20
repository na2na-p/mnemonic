package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
