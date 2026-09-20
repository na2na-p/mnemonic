package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPipeline_NormalizeCriticalFilenames(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 大文字ファイル名を小文字に変換", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		skipIfCaseInsensitiveFS(t, dir)

		require.NoError(t, os.WriteFile(filepath.Join(dir, "Data.XP3"), []byte("data"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Config.TJS"), []byte("config"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "README.TXT"), []byte("readme"), 0o600))

		require.NoError(t, p.normalizeCriticalFilenames(dir))

		assert.FileExists(t, filepath.Join(dir, "data.xp3"))
		assert.FileExists(t, filepath.Join(dir, "config.tjs"))
		assert.FileExists(t, filepath.Join(dir, "readme.txt"))
		assert.NoFileExists(t, filepath.Join(dir, "Data.XP3"))
		assert.NoFileExists(t, filepath.Join(dir, "Config.TJS"))
		assert.NoFileExists(t, filepath.Join(dir, "README.TXT"))
	})

	t.Run("正常系: 小文字ファイル名はそのまま", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()

		require.NoError(t, os.WriteFile(filepath.Join(dir, "data.xp3"), []byte("data"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config.tjs"), []byte("config"), 0o600))

		require.NoError(t, p.normalizeCriticalFilenames(dir))

		assert.FileExists(t, filepath.Join(dir, "data.xp3"))
		assert.FileExists(t, filepath.Join(dir, "config.tjs"))
	})

	t.Run("正常系: ネストしたディレクトリ内のファイルも正規化", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		skipIfCaseInsensitiveFS(t, dir)
		nestedDir := filepath.Join(dir, "system")
		deepDir := filepath.Join(nestedDir, "plugins")
		require.NoError(t, os.MkdirAll(deepDir, 0o750))

		require.NoError(t, os.WriteFile(filepath.Join(dir, "Data.XP3"), []byte("data"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(nestedDir, "MainWindow.TJS"), []byte("mainwindow"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(deepDir, "Plugin.DLL"), []byte("plugin"), 0o600))

		require.NoError(t, p.normalizeCriticalFilenames(dir))

		assert.FileExists(t, filepath.Join(dir, "data.xp3"))
		assert.FileExists(t, filepath.Join(nestedDir, "mainwindow.tjs"))
		assert.FileExists(t, filepath.Join(deepDir, "plugin.dll"))
		assert.NoFileExists(t, filepath.Join(dir, "Data.XP3"))
		assert.NoFileExists(t, filepath.Join(nestedDir, "MainWindow.TJS"))
		assert.NoFileExists(t, filepath.Join(deepDir, "Plugin.DLL"))
	})

	t.Run("正常系: 正規化後の名前が既に存在する場合はスキップする", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()

		require.NoError(t, os.WriteFile(filepath.Join(dir, "data.xp3"), []byte("lower"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Data.XP3"), []byte("upper"), 0o600))

		require.NoError(t, p.normalizeCriticalFilenames(dir))

		assert.FileExists(t, filepath.Join(dir, "data.xp3"))
		assert.FileExists(t, filepath.Join(dir, "Data.XP3"))
	})
}
