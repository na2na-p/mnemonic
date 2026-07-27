package signer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findAndroidBuildToolのwhite-boxテスト。
// Python版のTestDefault*RunnerFind*系のシナリオ
// （ANDROID_HOME優先探索・バージョンソート・PATHフォールバック）を
// ZipalignRunner/ApkSignerRunner両方に共通する探索ロジックとしてまとめて検証する。
func TestFindAndroidBuildTool(t *testing.T) {
	t.Run("正常系: ANDROID_HOMEのbuild-toolsから検出", func(t *testing.T) {
		androidHome := t.TempDir()
		buildTools := filepath.Join(androidHome, "build-tools", "34.0.0")
		require.NoError(t, os.MkdirAll(buildTools, 0o750))
		toolPath := filepath.Join(buildTools, "zipalign")
		require.NoError(t, os.WriteFile(toolPath, nil, 0o600))

		t.Setenv("ANDROID_HOME", androidHome)

		result, ok := findAndroidBuildTool("zipalign")

		assert.True(t, ok)
		assert.Equal(t, toolPath, result)
	})

	t.Run("正常系: 複数バージョンから最新を選択", func(t *testing.T) {
		androidHome := t.TempDir()
		for _, v := range []string{"30.0.0", "33.0.0", "34.0.0"} {
			dir := filepath.Join(androidHome, "build-tools", v)
			require.NoError(t, os.MkdirAll(dir, 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "apksigner"), nil, 0o600))
		}

		t.Setenv("ANDROID_HOME", androidHome)

		result, ok := findAndroidBuildTool("apksigner")

		assert.True(t, ok)
		assert.Equal(t, filepath.Join(androidHome, "build-tools", "34.0.0", "apksigner"), result)
	})

	t.Run("正常系: ANDROID_HOME未設定の場合はPATHから検出", func(t *testing.T) {
		t.Setenv("ANDROID_HOME", "")
		t.Setenv("PATH", "")

		binDir := t.TempDir()
		toolPath := filepath.Join(binDir, "zipalign")
		require.NoError(t, os.WriteFile(toolPath, []byte("#!/bin/sh\n"), 0o700)) //nolint:gosec // テスト用フェイク実行ファイルのため妥当
		t.Setenv("PATH", binDir)

		result, ok := findAndroidBuildTool("zipalign")

		assert.True(t, ok)
		assert.Equal(t, toolPath, result)
	})

	t.Run("正常系: ANDROID_HOMEに見つからない場合はPATHにフォールバック", func(t *testing.T) {
		androidHome := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(androidHome, "build-tools"), 0o750))
		t.Setenv("ANDROID_HOME", androidHome)

		binDir := t.TempDir()
		toolPath := filepath.Join(binDir, "apksigner")
		require.NoError(t, os.WriteFile(toolPath, []byte("#!/bin/sh\n"), 0o700)) //nolint:gosec // テスト用フェイク実行ファイルのため妥当
		t.Setenv("PATH", binDir)

		result, ok := findAndroidBuildTool("apksigner")

		assert.True(t, ok)
		assert.Equal(t, toolPath, result)
	})

	t.Run("正常系: どこにも見つからない場合はfalse", func(t *testing.T) {
		t.Setenv("ANDROID_HOME", "")
		t.Setenv("PATH", t.TempDir())

		result, ok := findAndroidBuildTool("zipalign")

		assert.False(t, ok)
		assert.Empty(t, result)
	})

	// レビュー指摘の回帰テスト: os.ReadDirのDirEntry.IsDir()はシンボリックリンク
	// 自体の種別を見るためリンクされたバージョンディレクトリを除外してしまっていた。
	// os.Stat（symlink解決）で判定するよう修正したことをピン留めする。
	t.Run("正常系: バージョンディレクトリがシンボリックリンクでも検出", func(t *testing.T) {
		androidHome := t.TempDir()
		buildToolsDir := filepath.Join(androidHome, "build-tools")
		require.NoError(t, os.MkdirAll(buildToolsDir, 0o750))

		realDir := filepath.Join(t.TempDir(), "34.0.0-real")
		require.NoError(t, os.MkdirAll(realDir, 0o750))
		toolPath := filepath.Join(realDir, "zipalign")
		require.NoError(t, os.WriteFile(toolPath, nil, 0o600))

		linkPath := filepath.Join(buildToolsDir, "34.0.0")
		require.NoError(t, os.Symlink(realDir, linkPath))

		t.Setenv("ANDROID_HOME", androidHome)

		result, ok := findAndroidBuildTool("zipalign")

		assert.True(t, ok)
		assert.Equal(t, filepath.Join(linkPath, "zipalign"), result)
	})
}
