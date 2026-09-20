package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildPipeline_AdjustScripts はScriptAdjuster自体は別途
// internal/converter/script_test.goで検証済みのため、ここではpipeline側の再帰探索と
// 呼び出しが行われることを実際の内容変化で確認する。
func TestBuildPipeline_AdjustScripts(t *testing.T) {
	t.Parallel()

	t.Run("正常系: startup.tjsにポリフィル初期化ディレクティブを追加する", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		startupFile := filepath.Join(dir, "startup.tjs")
		require.NoError(t, os.WriteFile(startupFile, []byte("// original content"), 0o600))

		require.NoError(t, p.adjustScripts(dir))

		content, err := os.ReadFile(startupFile) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Contains(t, string(content), "// krkrsdl2 polyfill initialization")
	})

	t.Run("正常系: 大文字小文字のバリエーションを検出する", func(t *testing.T) {
		t.Parallel()

		for _, variant := range []string{"Startup.tjs", "STARTUP.TJS", "StartUp.tjs"} {
			t.Run(variant, func(t *testing.T) {
				t.Parallel()

				p := newTestPipeline(t)
				dir := t.TempDir()
				startupFile := filepath.Join(dir, variant)
				require.NoError(t, os.WriteFile(startupFile, []byte("// original content"), 0o600))

				require.NoError(t, p.adjustScripts(dir))

				content, err := os.ReadFile(startupFile) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
				require.NoError(t, err)
				assert.Contains(t, string(content), "// krkrsdl2 polyfill initialization")
			})
		}
	})

	t.Run("正常系: 全ての.ksファイルを再帰的に処理する", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		dir := t.TempDir()
		scenarioDir := filepath.Join(dir, "scenario")
		require.NoError(t, os.MkdirAll(scenarioDir, 0o750))
		firstKS := filepath.Join(scenarioDir, "first.ks")
		require.NoError(t, os.WriteFile(firstKS, []byte(`[loadplugin module="wuvorbis.dll"]`), 0o600))

		require.NoError(t, p.adjustScripts(dir))

		content, err := os.ReadFile(firstKS) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Contains(t, string(content), `libwuvorbis.so`)
	})
}

// TestBuildPipeline_AdjustScripts_SkipVideo は--skip-video時にスクリプト
// 参照だけが.mpgへ書き換わり実体の無い参照が残る不具合の修正をピン留めする。
//
// why: --skip-video時はVideoConverterがconvertersに登録されず(phases.go
// executeConvert参照)、動画ファイルは無変換のまま(実体はop.wmvのまま)
// convertDirに残る。adjustScriptsがDefaultRulesをそのまま適用すると、
// スクリプト側の参照だけが"op.mpg"へ書き換わり、実体の無いファイルを指す
// ようになる（fault-injection: 実際にはop.wmvしか存在しないディレクトリで
// adjustScriptsを実行し、参照が書き換わらないこと=danglingにならないことを
// 機械的に検証する）。
func TestBuildPipeline_AdjustScripts_SkipVideo(t *testing.T) {
	t.Parallel()

	t.Run("正常系: SkipVideo時は.ks内の動画参照が書き換わらずopファイルと整合する", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		p.config.SkipVideo = true

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "op.wmv"), []byte("raw wmv content"), 0o600))
		scriptPath := filepath.Join(dir, "first.ks")
		scriptSource := `[movie storage="op.wmv" layer=0]`
		require.NoError(t, os.WriteFile(scriptPath, []byte(scriptSource), 0o600))

		require.NoError(t, p.adjustScripts(dir))

		content, err := os.ReadFile(scriptPath) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		// スクリプトが引き続き"op.wmv"を参照し、実体のop.wmvも存在する
		// (=参照先が実在する)ことを機械的に確認する。
		assert.Contains(t, string(content), `"op.wmv"`)
		assert.NotContains(t, string(content), ".mpg")
		assert.FileExists(t, filepath.Join(dir, "op.wmv"))
	})

	t.Run("正常系: SkipVideoでない場合は従来どおり動画参照が.mpgへ書き換わる", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)

		dir := t.TempDir()
		scriptPath := filepath.Join(dir, "first.ks")
		require.NoError(t, os.WriteFile(scriptPath, []byte(`[movie storage="op.wmv" layer=0]`), 0o600))

		require.NoError(t, p.adjustScripts(dir))

		content, err := os.ReadFile(scriptPath) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Contains(t, string(content), `"op.mpg"`)
	})
}
