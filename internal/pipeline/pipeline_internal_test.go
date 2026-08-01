package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubExecutePhase を差し込み、実際のフェーズ処理をスキップしてRun()の
// オーケストレーション（進捗コールバック・phasesCompleted集計・統計収集）
// のみを検証する。
func newValidPipelineForOrchestration(t *testing.T) *BuildPipeline {
	t.Helper()

	dir := t.TempDir()
	input := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(input, make([]byte, 100), 0o600))

	config := NewConfig(input, filepath.Join(dir, "output.apk"))
	p := NewBuildPipeline(config)
	p.executePhase = func(Phase) error { return nil }

	return p
}

func TestBuildPipeline_Run_FullPipeline(t *testing.T) {
	t.Parallel()

	p := newValidPipelineForOrchestration(t)

	result := p.Run(nil)

	assert.True(t, result.Success)
	require.NotNil(t, result.OutputPath)
	assert.Equal(t, AllPhases(), result.PhasesCompleted)
}

func TestBuildPipeline_Run_ProgressCallback(t *testing.T) {
	t.Parallel()

	p := newValidPipelineForOrchestration(t)

	var calledPhases []Phase

	callCount := 0
	result := p.Run(func(progress Progress) {
		callCount++
		calledPhases = append(calledPhases, progress.Phase)
	})

	assert.GreaterOrEqual(t, callCount, len(AllPhases()))
	for _, phase := range AllPhases() {
		assert.Contains(t, calledPhases, phase)
	}
	assert.True(t, result.Success)
}

func TestBuildPipeline_Run_SkipVideo(t *testing.T) {
	t.Parallel()

	p := newValidPipelineForOrchestration(t)
	p.config.SkipVideo = true

	assert.True(t, p.Config().SkipVideo)

	result := p.Run(nil)

	assert.True(t, result.Success)
}

func TestBuildPipeline_Run_CleanCache(t *testing.T) {
	t.Parallel()

	p := newValidPipelineForOrchestration(t)
	p.config.CleanCache = true

	assert.True(t, p.Config().CleanCache)

	result := p.Run(nil)

	assert.True(t, result.Success)
}

func TestBuildPipeline_Run_PhaseFailure(t *testing.T) {
	t.Parallel()

	p := newValidPipelineForOrchestration(t)
	p.executePhase = func(phase Phase) error {
		if phase == PhaseConvert {
			return assert.AnError
		}

		return nil
	}

	result := p.Run(nil)

	assert.False(t, result.Success)
	assert.Nil(t, result.OutputPath)
	assert.NotEmpty(t, result.ErrorMessage)
	assert.Equal(t, []Phase{PhaseAnalyze, PhaseExtract}, result.PhasesCompleted)
}

func TestBuildPipeline_SanitizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "正常系: スペースをアンダースコアに変換", input: "TRUE REMEMBRANCE", want: "true_remembrance"},
		{name: "正常系: Java予約語にプレフィックス追加", input: "true", want: "game_true"},
		{name: "正常系: Java予約語falseにプレフィックス追加", input: "false", want: "game_false"},
		{name: "正常系: Java予約語nullにプレフィックス追加", input: "null", want: "game_null"},
		{name: "正常系: 数字始まりにプレフィックス追加", input: "123game", want: "_123game"},
		{name: "正常系: 特殊文字を削除", input: "game!@#$%", want: "game"},
		{name: "正常系: スペースと数字の組み合わせ", input: "My Game 2", want: "my_game_2"},
	}

	dir := t.TempDir()
	input := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(input, make([]byte, 100), 0o600))
	p := NewBuildPipeline(NewConfig(input, filepath.Join(dir, "output.apk")))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, p.sanitizeName(tt.input))
		})
	}
}

func newTestPipeline(t *testing.T) *BuildPipeline {
	t.Helper()

	dir := t.TempDir()
	input := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(input, make([]byte, 100), 0o600))

	return NewBuildPipeline(NewConfig(input, filepath.Join(dir, "output.apk")))
}

func TestBuildPipeline_FindGameIcon_ReturnsEmptyWhenExtractDirIsUnset(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)

	assert.Empty(t, p.findGameIcon())
}

func TestBuildPipeline_FindGameIcon_ReturnsPrioritizedIcon(t *testing.T) {
	t.Parallel()

	tests := []string{"icon.png", "icon.ico", "icon.bmp"}

	for _, iconName := range tests {
		t.Run("正常系: "+iconName+"を検出", func(t *testing.T) {
			t.Parallel()

			p := newTestPipeline(t)
			extractDir := t.TempDir()
			p.extractDir = extractDir

			iconPath := filepath.Join(extractDir, iconName)
			require.NoError(t, os.WriteFile(iconPath, []byte("\x89PNG\r\n\x1a\n"), 0o600))

			assert.Equal(t, iconPath, p.findGameIcon())
		})
	}
}

func TestBuildPipeline_FindGameIcon_PrefersPNGOverICO(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)
	extractDir := t.TempDir()
	p.extractDir = extractDir

	pngPath := filepath.Join(extractDir, "icon.png")
	require.NoError(t, os.WriteFile(pngPath, []byte("\x89PNG\r\n\x1a\n"), 0o600))
	icoPath := filepath.Join(extractDir, "icon.ico")
	require.NoError(t, os.WriteFile(icoPath, []byte("\x00\x00\x01\x00"), 0o600))

	assert.Equal(t, pngPath, p.findGameIcon())
}

func TestBuildPipeline_FindGameIcon_FallsBackToAnyICO(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)
	extractDir := t.TempDir()
	p.extractDir = extractDir

	customICO := filepath.Join(extractDir, "game_icon.ico")
	require.NoError(t, os.WriteFile(customICO, []byte("\x00\x00\x01\x00"), 0o600))

	assert.Equal(t, customICO, p.findGameIcon())
}

func TestBuildPipeline_FindGameIcon_ReturnsEmptyWhenNoIcon(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)
	extractDir := t.TempDir()
	p.extractDir = extractDir
	require.NoError(t, os.WriteFile(filepath.Join(extractDir, "data.xp3"), []byte("XP3"), 0o600))

	assert.Empty(t, p.findGameIcon())
}

// TestBuildPipeline_ExecuteConvert_MissingExtractDir はピン留めテスト:
// converter.ConvertDirectoryはsourceDirが存在しない場合にerrorを返す。この
// テストは、CONVERTフェーズがフェーズの失敗として明示的にerrorを伝播する
// （黙って空の変換結果で成功しない）ことをピン留めする。extractDirは通常の
// 実行経路では直前のEXTRACTフェーズが必ず作成するため、ここでは異常系を
// 模擬するために直接存在しないパスを設定する。
func TestBuildPipeline_ExecuteConvert_MissingExtractDir(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)
	p.extractDir = filepath.Join(t.TempDir(), "does-not-exist")

	err := p.executeConvert()

	require.Error(t, err)
}

func TestBuildPipeline_ExecuteConvert_ReturnsErrorWhenExtractPhaseNotDone(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)

	err := p.executeConvert()

	require.Error(t, err)
}

func TestBuildPipeline_ExecuteBuild_ReturnsErrorWhenConvertPhaseNotDone(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)

	err := p.executeBuild()

	require.Error(t, err)
}

func TestBuildPipeline_ExecuteSign_ReturnsErrorWhenBuildPhaseNotDone(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)

	err := p.executeSign()

	require.Error(t, err)
}
