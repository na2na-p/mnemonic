package pipeline

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/cache"
	"github.com/na2na-p/mnemonic/internal/parser"
)

// stubExecutePhase を差し込み、実際のフェーズ処理をスキップしてRun()の
// オーケストレーション（進捗コールバック・phasesCompleted集計・統計収集）
// のみを検証する。キャッシュディレクトリもt.TempDir()配下へ差し替え、CleanCacheを
// 指定したテストが開発者の実キャッシュを消さないようにする。
func newValidPipelineForOrchestration(t *testing.T) *BuildPipeline {
	t.Helper()

	dir := t.TempDir()
	input := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(input, make([]byte, 100), 0o600))

	config := NewConfig(input, filepath.Join(dir, "output.apk"))
	p := NewBuildPipeline(config)
	p.executePhase = func(_ Phase, a buildArtifacts) (buildArtifacts, error) { return a, nil }
	cacheDir := filepath.Join(dir, "cache")
	p.cacheDir = func() (string, error) { return cacheDir, nil }

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

// seedCacheDir はキャッシュディレクトリ直下の各キャッシュ（テンプレート・フォント・
// プラグイン・SDL2ソース・署名鍵）を模したファイルをcacheDir配下に作成する。
// 戻り値は署名鍵以外のキャッシュのファイルパスと、署名鍵のファイルパス。
func seedCacheDir(t *testing.T, cacheDir string) (others []string, keystore string) {
	t.Helper()

	others = []string{
		filepath.Join(cacheDir, "templates", "latest", "x"),
		filepath.Join(cacheDir, "fonts", "Koruri-Regular.ttf"),
		filepath.Join(cacheDir, "plugins", "arm64-v8a", "libextrans.so"),
		filepath.Join(cacheDir, "sdl2_sources", "marker"),
	}
	keystore = filepath.Join(cacheDir, cache.KeystoreDirName, "debug.keystore")

	for _, path := range append(slices.Clone(others), keystore) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte("cached"), 0o600))
	}

	return others, keystore
}

func TestBuildPipeline_Run_CleanCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		cleanCache      bool
		templateOffline bool
		invalidInput    bool
		wantSuccess     bool
		wantRemoved     bool
		wantErrContains string
	}{
		{
			name:        "正常系: CleanCache指定時は署名鍵以外のキャッシュを全て削除し署名鍵は残す",
			cleanCache:  true,
			wantSuccess: true,
			wantRemoved: true,
		},
		{
			name:        "正常系: CleanCache未指定時はキャッシュに触れない",
			cleanCache:  false,
			wantSuccess: true,
			wantRemoved: false,
		},
		{
			name:         "異常系: 入力の検証に失敗した場合はCleanCache指定時もキャッシュに触れない",
			cleanCache:   true,
			invalidInput: true,
			wantSuccess:  false,
			wantRemoved:  false,
		},
		{
			name:            "異常系: TemplateOfflineと同時指定の場合は検証で失敗しキャッシュに触れない",
			cleanCache:      true,
			templateOffline: true,
			wantSuccess:     false,
			wantRemoved:     false,
			wantErrContains: "--template-offline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newValidPipelineForOrchestration(t)
			p.config.CleanCache = tt.cleanCache
			p.config.TemplateOffline = tt.templateOffline
			if tt.invalidInput {
				p.config.InputPath = filepath.Join(t.TempDir(), "missing.exe")
			}

			cacheDir, err := p.cacheDir()
			require.NoError(t, err)
			others, keystore := seedCacheDir(t, cacheDir)

			result := p.Run(nil)

			assert.Equal(t, tt.wantSuccess, result.Success)
			assert.Contains(t, result.ErrorMessage, tt.wantErrContains)
			for _, path := range others {
				if tt.wantRemoved {
					assert.NoFileExists(t, path)
				} else {
					assert.FileExists(t, path)
				}
			}
			assert.FileExists(t, keystore)
		})
	}
}

func TestNewBuildPipeline_DefaultCacheDirIsCacheDir(t *testing.T) {
	t.Parallel()

	p := NewBuildPipeline(NewConfig("game.exe", "game.apk"))

	got, gotErr := p.cacheDir()
	want, wantErr := cache.Dir()

	require.NoError(t, wantErr)
	require.NoError(t, gotErr)
	assert.Equal(t, want, got)
}

func TestBuildPipeline_Run_CleanCacheFailure(t *testing.T) {
	t.Parallel()

	p := newValidPipelineForOrchestration(t)
	p.config.CleanCache = true
	p.cacheDir = func() (string, error) { return "", assert.AnError }

	var phases []Phase
	p.executePhase = func(phase Phase, a buildArtifacts) (buildArtifacts, error) {
		phases = append(phases, phase)

		return a, nil
	}

	result := p.Run(nil)

	assert.False(t, result.Success)
	assert.Contains(t, result.ErrorMessage, ErrCacheClean.Error())
	assert.Empty(t, phases)
}

func TestBuildPipeline_CleanCache_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T) func() (string, error)
	}{
		{
			name: "異常系: キャッシュディレクトリを解決できない",
			setup: func(*testing.T) func() (string, error) {
				return func() (string, error) { return "", assert.AnError }
			},
		},
		{
			name: "異常系: キャッシュを削除できない",
			setup: func(t *testing.T) func() (string, error) {
				t.Helper()

				if os.Geteuid() == 0 {
					t.Skip("rootはパーミッションに関係なく削除できるためスキップ")
				}

				cacheDir := t.TempDir()
				others, _ := seedCacheDir(t, cacheDir)
				lockedDir := filepath.Dir(others[0])
				require.NoError(t, os.Chmod(lockedDir, 0o500))
				t.Cleanup(func() { _ = os.Chmod(lockedDir, 0o700) })

				return func() (string, error) { return cacheDir, nil }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newValidPipelineForOrchestration(t)
			p.cacheDir = tt.setup(t)

			err := p.cleanCache()

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrCacheClean)
		})
	}
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

func TestBuildPipeline_Run_LogsPhases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		failAt      Phase
		wantVerbose []string
	}{
		{
			name: "正常系: 全フェーズの開始と完了を実行順にVerboseで報告する",
			wantVerbose: []string{
				"analyzeフェーズを開始します", "analyzeフェーズが完了しました",
				"extractフェーズを開始します", "extractフェーズが完了しました",
				"convertフェーズを開始します", "convertフェーズが完了しました",
				"buildフェーズを開始します", "buildフェーズが完了しました",
				"signフェーズを開始します", "signフェーズが完了しました",
			},
		},
		{
			name:   "異常系: 失敗したフェーズは開始のみ報告し以降のフェーズは報告しない",
			failAt: PhaseConvert,
			wantVerbose: []string{
				"analyzeフェーズを開始します", "analyzeフェーズが完了しました",
				"extractフェーズを開始します", "extractフェーズが完了しました",
				"convertフェーズを開始します",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newValidPipelineForOrchestration(t)
			p.executePhase = func(phase Phase, a buildArtifacts) (buildArtifacts, error) {
				if phase == tt.failAt {
					return a, assert.AnError
				}

				return a, nil
			}
			logger := &recordingLogger{}
			p.SetLogger(logger)

			p.Run(nil)

			assert.Equal(t, tt.wantVerbose, logger.messages("VERBOSE"))
		})
	}
}

func TestBuildPipeline_SetLogger_NilDisablesOutput(t *testing.T) {
	t.Parallel()

	p := newValidPipelineForOrchestration(t)
	p.SetLogger(nil)

	result := p.Run(nil)

	assert.True(t, result.Success)
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
		{name: "正常系: 数字始まりでもプレフィックスを付けない", input: "123game", want: "123game"},
		{name: "正常系: 先頭の空白由来アンダースコアを除去してから予約語を判定する", input: " true", want: "game_true"},
		{name: "正常系: 先頭の空白由来アンダースコアを除去", input: "ひぐらし Game", want: "game"},
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

func TestBuildPipeline_DerivePackageName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		explicit string
		baseName string
		want     string
		wantErr  error
	}{
		{name: "正常系: 指定されたパッケージ名をそのまま返す", explicit: "com.example.game", baseName: "全角だけの題名", want: "com.example.game"},
		{name: "正常系: 英字のタイトルからパッケージ名を導出する", explicit: "", baseName: "TRUE REMEMBRANCE", want: "com.krkr.true_remembrance"},
		{name: "正常系: 先頭の空白由来アンダースコアを除いた英字始まりの名前を使う", explicit: "", baseName: "ひぐらし Game", want: "com.krkr.game"},
		{name: "異常系: 数字始まりのタイトルはエラーを返す", explicit: "", baseName: "123game", wantErr: ErrPackageNameUndeterminable},
		{name: "異常系: 全角のみのタイトルはエラーを返す", explicit: "", baseName: "全角だけの題名", wantErr: ErrPackageNameUndeterminable},
		{name: "異常系: 英数字が無いタイトルはエラーを返す", explicit: "", baseName: "ひぐらし のなく頃に", wantErr: ErrPackageNameUndeterminable},
		{name: "正常系: 末尾の空白由来アンダースコアは残す", explicit: "", baseName: "Game ひぐらし", want: "com.krkr.game_"},
		{name: "異常系: 空白のみのタイトルはエラーを返す", explicit: "", baseName: "   ", wantErr: ErrPackageNameUndeterminable},
		{name: "異常系: 指定されたパッケージ名が1セグメントならエラーを返す", explicit: "game", baseName: "TRUE REMEMBRANCE", wantErr: ErrInvalidPackageName},
		{name: "異常系: 指定されたパッケージ名が規則に反すればタイトルから導出せずエラーを返す", explicit: "com.9game", baseName: "TRUE REMEMBRANCE", wantErr: ErrInvalidPackageName},
	}

	p := newTestPipeline(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := p.derivePackageName(tt.explicit, tt.baseName)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidatePackageName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "正常系: 英字始まりの3セグメントは受け付ける", input: "com.example.game", wantErr: false},
		{name: "正常系: ちょうど2セグメントは受け付ける", input: "com.example", wantErr: false},
		{name: "正常系: 大文字・数字・アンダースコアと大文字始まりの予約語綴りは受け付ける", input: "Com.My_Game2.Class", wantErr: false},
		{name: "異常系: 1セグメントはエラーを返す", input: "game", wantErr: true},
		{name: "異常系: 数字始まりのセグメントはエラーを返す", input: "com.9game", wantErr: true},
		{name: "異常系: アンダースコア始まりのセグメントはエラーを返す", input: "com._game", wantErr: true},
		{name: "異常系: Java予約語のセグメントはエラーを返す", input: "com.example.class", wantErr: true},
		{name: "異常系: ハイフンを含むセグメントはエラーを返す", input: "com.exam-ple", wantErr: true},
		{name: "異常系: 空のセグメントはエラーを返す", input: "com..game", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validatePackageName(tt.input)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidPackageName)

				return
			}

			require.NoError(t, err)
		})
	}
}

func newTestPipeline(t *testing.T) *BuildPipeline {
	t.Helper()

	dir := t.TempDir()
	input := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(input, make([]byte, 100), 0o600))

	p := NewBuildPipeline(NewConfig(input, filepath.Join(dir, "output.apk")))
	// why not: 既定のcache.Dirのままにしない。CleanCacheを指定したRunに開発者の実キャッシュを消させないため。
	cacheDir := filepath.Join(dir, "cache")
	p.cacheDir = func() (string, error) { return cacheDir, nil }

	return p
}

func TestBuildPipeline_ExecuteBuild_RejectsUnusablePackageName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		title       string
		packageName string
		wantErr     error
	}{
		{name: "異常系: タイトルが全角のみでパッケージ名未指定ならエラーを返す", title: "全角だけの題名", packageName: "", wantErr: ErrPackageNameUndeterminable},
		{name: "異常系: タイトルが空でファイル名も全角のみならエラーを返す", title: "", packageName: "", wantErr: ErrPackageNameUndeterminable},
		{name: "異常系: タイトルに英数字が無ければエラーを返す", title: "ひぐらし のなく頃に", packageName: "", wantErr: ErrPackageNameUndeterminable},
		{name: "異常系: タイトルが数字始まりならエラーを返す", title: "123game", packageName: "", wantErr: ErrPackageNameUndeterminable},
		{name: "異常系: 指定されたパッケージ名が規則に反すればテンプレート取得前にエラーを返す", title: "Game", packageName: "com.9game", wantErr: ErrInvalidPackageName},
		{name: "正常系: パッケージ名を指定すればタイトルが全角のみでもエラーにしない", title: "全角だけの題名", packageName: "com.example.x", wantErr: nil},
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

			got, err := p.executeBuild(a)

			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.NotErrorIs(t, err, ErrTemplateUnavailable)
				assert.Empty(t, got.projectDir)

				return
			}

			require.NotErrorIs(t, err, ErrPackageNameUndeterminable)
			require.NotErrorIs(t, err, ErrInvalidPackageName)
			require.ErrorIs(t, err, ErrTemplateUnavailable)
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

func TestBuildPipeline_ExecuteConvert_AssetConversionFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		files       map[string]string
		wantFailed  string
		wantSummary string
	}{
		{
			name: "異常系: TLGとして解釈できない画像があれば変換元パスと原因を含むエラーを返す",
			files: map[string]string{
				"first.ks":       "*start\n吾輩は猫である。名前はまだ無い。\n",
				"image/bg01.tlg": "not a tlg image",
			},
			wantFailed:  filepath.Join("image", "bg01.tlg"),
			wantSummary: "失敗 1件",
		},
		{
			name: "正常系: 変換可能なアセットだけならエラーを返さない",
			files: map[string]string{
				"first.ks":        "*start\n吾輩は猫である。名前はまだ無い。\n",
				"system/font.ttf": "stub font",
			},
			wantSummary: "失敗 0件",
		},
		{
			name: "正常系: chardetが未対応の文字コードと推定する短いShift_JISの.csvがあってもエラーを返さない",
			files: map[string]string{
				"first.ks":        "*start\n吾輩は猫である。名前はまだ無い。\n",
				"system/font.ttf": "stub font",
				// Shift_JISの"id,name\n1,ｱｲﾃﾑ"。chardetはiso-8859-1と推定する。
				"data/items.csv": "id,name\n1,\xb1\xb2\xc3\xd1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			extractDir := t.TempDir()
			for name, content := range tt.files {
				path := filepath.Join(extractDir, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
				require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
			}

			p := newTestPipeline(t)
			t.Cleanup(p.cleanupTempDirs)
			logger := &recordingLogger{}
			p.SetLogger(logger)

			a, err := p.executeConvert(buildArtifacts{extractDir: extractDir})

			infos := logger.messages("INFO")
			require.Len(t, infos, 1, "変換結果の集計は失敗時も含めて1回報告する")
			assert.Contains(t, infos[0], tt.wantSummary)

			if tt.wantFailed == "" {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, ErrAssetConversionFailed)
			require.ErrorContains(t, err, filepath.Join(extractDir, tt.wantFailed))
			require.ErrorContains(t, err, "TLG形式ではありません")
			// system/はcopyPolyfillFilesが作るため、これが無いことで変換失敗時に
			// finalizeConvertedTreeへ進んでいないことを確かめる。
			assert.NoDirExists(t, filepath.Join(a.convertDir, "system"))
		})
	}
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
