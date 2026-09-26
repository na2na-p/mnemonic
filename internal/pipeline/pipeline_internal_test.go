package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/parser"
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
	p.executePhase = func(_ Phase, a buildArtifacts) (buildArtifacts, error) { return a, nil }

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
	p.executePhase = func(phase Phase, a buildArtifacts) (buildArtifacts, error) {
		if phase == PhaseConvert {
			return a, assert.AnError
		}

		return a, nil
	}

	result := p.Run(nil)

	assert.False(t, result.Success)
	assert.Nil(t, result.OutputPath)
	assert.NotEmpty(t, result.ErrorMessage)
	assert.Equal(t, []Phase{PhaseAnalyze, PhaseExtract}, result.PhasesCompleted)
}

func TestBuildPipeline_Run_ThreadsArtifactsBetweenPhases(t *testing.T) {
	t.Parallel()

	p := newValidPipelineForOrchestration(t)

	received := map[Phase]buildArtifacts{}
	p.executePhase = func(phase Phase, a buildArtifacts) (buildArtifacts, error) {
		received[phase] = a

		switch phase {
		case PhaseExtract:
			a.extractDir = "E"
		case PhaseConvert:
			a.convertDir = "C"
		case PhaseBuild:
			a.unsignedAPK = "A"
		}

		return a, nil
	}

	result := p.Run(nil)

	require.True(t, result.Success)
	require.Len(t, received, len(AllPhases()))
	assert.Equal(t, buildArtifacts{}, received[PhaseAnalyze])
	assert.Equal(t, "E", received[PhaseConvert].extractDir)
	assert.Equal(t, "C", received[PhaseBuild].convertDir)
	assert.Equal(t, "A", received[PhaseSign].unsignedAPK)
}

func TestBuildPipeline_NewTempDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prefix  string
		wantErr string
	}{
		{name: "正常系: 抽出用プレフィックスで一時ディレクトリを作成して登録する", prefix: "mnemonic_extract_"},
		{name: "正常系: 変換用プレフィックスで一時ディレクトリを作成して登録する", prefix: "mnemonic_convert_"},
		{
			name:    "異常系: パス区切りを含むプレフィックスはエラーを返し登録しない",
			prefix:  "bad" + string(filepath.Separator) + "prefix_",
			wantErr: "一時ディレクトリの作成に失敗しました",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPipeline(t)

			dir, err := p.newTempDir(tt.prefix)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Empty(t, dir)
				assert.Empty(t, p.tempDirs)

				return
			}

			require.NoError(t, err)
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			assert.Equal(t, filepath.Clean(os.TempDir()), filepath.Dir(dir))
			assert.True(t, strings.HasPrefix(filepath.Base(dir), tt.prefix))
			assert.DirExists(t, dir)
			assert.Equal(t, []string{dir}, p.tempDirs)

			p.cleanupTempDirs()

			assert.NoDirExists(t, dir)
			assert.Empty(t, p.tempDirs)
		})
	}
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
		{name: "正常系: 全角文字のみは空文字列になる", input: "全角だけの題名", want: ""},
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

func TestBuildPipeline_ExecuteBuild_ErrorsWhenPackageNameUndeterminable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		title       string
		packageName string
		wantErr     bool
	}{
		{name: "異常系: タイトルが全角のみでパッケージ名未指定ならエラーを返す", title: "全角だけの題名", packageName: "", wantErr: true},
		{name: "異常系: タイトルが空でファイル名も全角のみならエラーを返す", title: "", packageName: "", wantErr: true},
		{name: "正常系: パッケージ名を指定すればタイトルが全角のみでもエラーにしない", title: "全角だけの題名", packageName: "com.example.x", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPipeline(t)
			input := filepath.Join(t.TempDir(), "ゲーム.exe")
			require.NoError(t, os.WriteFile(input, make([]byte, 100), 0o600))
			p.config.InputPath = input
			p.config.PackageName = tt.packageName
			p.config.TemplateOffline = true
			// ホストのテンプレートキャッシュの有無に左右されず、パッケージ名の判定より
			// 先へ進んだ場合は必ずテンプレート未取得で失敗させる。
			p.config.TemplateVersion = new("v0.0.0-mnemonic-test-nonexistent")
			t.Cleanup(p.cleanupTempDirs)

			a := buildArtifacts{
				convertDir:    t.TempDir(),
				gameStructure: &parser.GameStructure{Title: tt.title},
			}

			_, err := p.executeBuild(a)

			require.Error(t, err)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrPackageNameUndeterminable)

				return
			}

			require.NotErrorIs(t, err, ErrPackageNameUndeterminable)
			assert.ErrorContains(t, err, "テンプレートが利用できません")
		})
	}
}

func TestBuildPipeline_FindGameIcon_ReturnsEmptyWhenExtractDirIsUnset(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)

	assert.Empty(t, p.findGameIcon(""))
}

func TestBuildPipeline_FindGameIcon_ReturnsPrioritizedIcon(t *testing.T) {
	t.Parallel()

	tests := []string{"icon.png", "icon.ico", "icon.bmp"}

	for _, iconName := range tests {
		t.Run("正常系: "+iconName+"を検出", func(t *testing.T) {
			t.Parallel()

			p := newTestPipeline(t)
			extractDir := t.TempDir()

			iconPath := filepath.Join(extractDir, iconName)
			require.NoError(t, os.WriteFile(iconPath, []byte("\x89PNG\r\n\x1a\n"), 0o600))

			assert.Equal(t, iconPath, p.findGameIcon(extractDir))
		})
	}
}

func TestBuildPipeline_FindGameIcon_PrefersPNGOverICO(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)
	extractDir := t.TempDir()

	pngPath := filepath.Join(extractDir, "icon.png")
	require.NoError(t, os.WriteFile(pngPath, []byte("\x89PNG\r\n\x1a\n"), 0o600))
	icoPath := filepath.Join(extractDir, "icon.ico")
	require.NoError(t, os.WriteFile(icoPath, []byte("\x00\x00\x01\x00"), 0o600))

	assert.Equal(t, pngPath, p.findGameIcon(extractDir))
}

func TestBuildPipeline_FindGameIcon_FallsBackToAnyICO(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)
	extractDir := t.TempDir()

	customICO := filepath.Join(extractDir, "game_icon.ico")
	require.NoError(t, os.WriteFile(customICO, []byte("\x00\x00\x01\x00"), 0o600))

	assert.Equal(t, customICO, p.findGameIcon(extractDir))
}

func TestBuildPipeline_FindGameIcon_ReturnsEmptyWhenNoIcon(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)
	extractDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(extractDir, "data.xp3"), []byte("XP3"), 0o600))

	assert.Empty(t, p.findGameIcon(extractDir))
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
	t.Cleanup(p.cleanupTempDirs)

	_, err := p.executeConvert(buildArtifacts{extractDir: filepath.Join(t.TempDir(), "does-not-exist")})

	require.Error(t, err)
}

func TestBuildPipeline_ExecuteConvert_ReturnsErrorWhenExtractPhaseNotDone(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)

	_, err := p.executeConvert(buildArtifacts{})

	require.Error(t, err)
	assert.EqualError(t, err, "抽出フェーズが完了していません")
}

func TestBuildPipeline_ExecuteBuild_ReturnsErrorWhenConvertPhaseNotDone(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)

	_, err := p.executeBuild(buildArtifacts{})

	require.Error(t, err)
	assert.EqualError(t, err, "変換フェーズが完了していません")
}

func TestBuildPipeline_ExecuteSign_ReturnsErrorWhenBuildPhaseNotDone(t *testing.T) {
	t.Parallel()

	p := newTestPipeline(t)

	_, err := p.executeSign(buildArtifacts{})

	require.Error(t, err)
	assert.EqualError(t, err, "ビルドフェーズが完了していません")
}
