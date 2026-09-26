package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/apperr"
	"github.com/na2na-p/mnemonic/internal/pipeline"
)

// cliResult はtyper.testing.CliRunnerのResultに相当するテスト用の結果値。
type cliResult struct {
	exitCode int
	stdout   string
	stderr   string
}

// invoke はCLIを標準入力なしで実行する。cache clean/infoのように標準入力や
// 実キャッシュディレクトリを必要とするテストはinvokeWithCacheDirを使う。
func invoke(t *testing.T, args []string) cliResult {
	t.Helper()

	var stdout, stderr bytes.Buffer

	code := run(args, strings.NewReader(""), &stdout, &stderr)

	return cliResult{exitCode: code, stdout: stdout.String(), stderr: stderr.String()}
}

// invokeWithCacheDir はcacheDirを注入した独立したコマンドツリーでCLIを実行する。
//
// cache clean / cache infoはキャッシュディレクトリを実際に読み書き・削除する
// ため、cacheテストは必ずこのヘルパーでt.TempDir()等のテスト専用ディレクトリを
// 指定する（cache.Dir、すなわち開発者の実際の$HOME配下のキャッシュを
// 誤って削除しないため）。newRootCmdはコマンドツリー構築のたびに独立した
// クロージャを生成するだけでパッケージ変数を共有しないため、invoke()と違い
// t.Parallel()配下でも安全に呼び出せる。
func invokeWithCacheDir(t *testing.T, args []string, stdin, cacheDir string) cliResult {
	t.Helper()

	var stdout, stderr bytes.Buffer

	root := newRootCmd(func() (string, error) { return cacheDir, nil })
	code := runWithRoot(root, args, strings.NewReader(stdin), &stdout, &stderr)

	return cliResult{exitCode: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestMainCommand_Options(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantSubstr string
	}{
		{name: "正常系: ヘルプ表示", args: []string{"--help"}, wantSubstr: "吉里吉里ゲーム"},
		// why not: バージョン文字列をversion.String()参照で自己言及的に検証すると、
		// version.valueが書き換わっても常に一致してしまいテストが実質何も
		// ピン留めしない。既定値"0.1.0-dev"（internal/version/version.go）を
		// リテラルでピン留めする。
		{name: "正常系: バージョン表示", args: []string{"--version"}, wantSubstr: "0.1.0-dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := invoke(t, tt.args)

			assert.Equal(t, 0, result.exitCode)
			assert.Contains(t, result.stdout, tt.wantSubstr)
		})
	}
}

func TestBuildCommand_Help(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"build", "--help"})

	assert.Equal(t, 0, result.exitCode)
	lower := strings.ToLower(result.stdout)
	assert.True(t, strings.Contains(result.stdout, "ビルド") || strings.Contains(lower, "build"))
}

// TestBuildCommand_MissingInput / TestBuildCommand_InvalidInputType は
// newBuildPipeline（buildコマンドが参照するパッケージ変数）を実際に読み出す。
// TestBuildCommand_Success等が同じ変数へスタブを書き込むため、t.Parallel()を
// 呼ばずシーケンシャルに実行しデータ競合を避ける。
func TestBuildCommand_MissingInput(t *testing.T) {
	dir := t.TempDir()
	nonexistent := filepath.Join(dir, "nonexistent.exe")

	result := invoke(t, []string{"build", nonexistent})

	assert.Equal(t, 1, result.exitCode)
	assert.True(t, strings.Contains(result.stdout, "Error") || strings.Contains(result.stdout, "エラー"))
}

func TestBuildCommand_InvalidInputType(t *testing.T) {
	dir := t.TempDir()
	invalidFile := filepath.Join(dir, "invalid.txt")
	require.NoError(t, os.WriteFile(invalidFile, []byte("invalid content"), 0o600))

	result := invoke(t, []string{"build", invalidFile})

	assert.Equal(t, 1, result.exitCode)
}

// stubBuildRunner はbuildRunnerのテスト用スタブ。
type stubBuildRunner struct {
	validateErrs []string
	runResult    pipeline.Result
}

func (s *stubBuildRunner) Validate() []string { return s.validateErrs }

func (s *stubBuildRunner) Run(pipeline.ProgressCallback) pipeline.Result { return s.runResult }

func withStubBuildPipeline(t *testing.T, stub *stubBuildRunner) {
	t.Helper()

	original := newBuildPipeline
	newBuildPipeline = func(pipeline.Config, pipeline.Logger) buildRunner { return stub }
	t.Cleanup(func() { newBuildPipeline = original })
}

// withCapturingBuildPipeline はnewBuildPipelineへ渡されたpipeline.Configを
// 記録するスタブを差し込み、記録先を返す。CLIフラグがConfigへ正しく
// 引き渡されているか（配線されているか）を検証するために使う。
func withCapturingBuildPipeline(t *testing.T, stub *stubBuildRunner) *pipeline.Config {
	t.Helper()

	var captured pipeline.Config

	original := newBuildPipeline
	newBuildPipeline = func(config pipeline.Config, _ pipeline.Logger) buildRunner {
		captured = config

		return stub
	}

	t.Cleanup(func() { newBuildPipeline = original })

	return &captured
}

// TestBuildCommand_SoundfontFlag は--soundfontがpipeline.Config.SoundfontPathへ
// 配線されていることを検証する。
//
// why: MIDI変換のサウンドフォントはこのフラグでしか指定できない。既定の探索先は
// Linuxの絶対パスのみで、macOS等ではこの経路が唯一の指定手段になる（T-220）。
func TestBuildCommand_SoundfontFlag(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(inputFile, make([]byte, 100), 0o600))
	outputFile := filepath.Join(dir, "output.apk")
	soundfont := filepath.Join(dir, "FluidR3_GM.sf2")

	tests := []struct {
		name      string
		extraArgs []string
		want      string
	}{
		{
			name:      "正常系: --soundfont指定時はそのパスがConfigへ渡る",
			extraArgs: []string{"--soundfont", soundfont},
			want:      soundfont,
		},
		{
			name:      "正常系: --soundfont未指定時は空文字列（既定の探索に委ねる）",
			extraArgs: nil,
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captured := withCapturingBuildPipeline(t, &stubBuildRunner{
				runResult: pipeline.Result{Success: true, OutputPath: &outputFile},
			})

			args := append([]string{"build", inputFile, "-o", outputFile}, tt.extraArgs...)
			result := invoke(t, args)

			require.Equal(t, 0, result.exitCode)
			assert.Equal(t, tt.want, captured.SoundfontPath)
		})
	}
}

func TestBuildCommand_CleanFlag(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(inputFile, make([]byte, 100), 0o600))
	outputFile := filepath.Join(dir, "output.apk")

	tests := []struct {
		name      string
		extraArgs []string
		want      bool
	}{
		{
			name:      "正常系: --clean指定時はCleanCacheがConfigへ渡る",
			extraArgs: []string{"--clean"},
			want:      true,
		},
		{
			name:      "正常系: --clean未指定時はCleanCacheがfalse",
			extraArgs: nil,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captured := withCapturingBuildPipeline(t, &stubBuildRunner{
				runResult: pipeline.Result{Success: true, OutputPath: &outputFile},
			})

			args := append([]string{"build", inputFile, "-o", outputFile}, tt.extraArgs...)
			result := invoke(t, args)

			require.Equal(t, 0, result.exitCode)
			assert.Equal(t, tt.want, captured.CleanCache)
		})
	}
}

func TestBuildCommand_Success(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(inputFile, make([]byte, 100), 0o600))
	outputFile := filepath.Join(dir, "output.apk")

	withStubBuildPipeline(t, &stubBuildRunner{
		validateErrs: nil,
		runResult:    pipeline.Result{Success: true, OutputPath: &outputFile},
	})

	result := invoke(t, []string{"build", inputFile, "-o", outputFile})

	assert.Equal(t, 0, result.exitCode)
	assert.Contains(t, result.stdout, "ビルド完了")
}

func TestBuildCommand_WithVerbose(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(inputFile, make([]byte, 100), 0o600))
	outputFile := filepath.Join(dir, "output.apk")

	withStubBuildPipeline(t, &stubBuildRunner{
		validateErrs: nil,
		runResult:    pipeline.Result{Success: true, OutputPath: &outputFile},
	})

	result := invoke(t, []string{"build", inputFile, "-o", outputFile, "-v"})

	assert.Equal(t, 0, result.exitCode)
}

// TestBuildCommand_SuccessWithNilOutputPath は、buildRunnerがSuccess:trueかつ
// OutputPath:nilという（実際のBuildPipelineでは起こらない）結果を返しても
// buildコマンドがpanicしないことをピン留めする（build.goの防御的nilチェック）。
func TestBuildCommand_SuccessWithNilOutputPath(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(inputFile, make([]byte, 100), 0o600))

	withStubBuildPipeline(t, &stubBuildRunner{
		validateErrs: nil,
		runResult:    pipeline.Result{Success: true, OutputPath: nil},
	})

	result := invoke(t, []string{"build", inputFile})

	assert.Equal(t, 0, result.exitCode)
	assert.Contains(t, result.stdout, "ビルド完了")
}

func TestBuildCommand_Failure(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(inputFile, make([]byte, 100), 0o600))

	withStubBuildPipeline(t, &stubBuildRunner{
		validateErrs: nil,
		runResult:    pipeline.Result{Success: false, ErrorMessage: "Gradleビルドに失敗しました"},
	})

	result := invoke(t, []string{"build", inputFile})

	assert.Equal(t, int(apperr.ExitError), result.exitCode)
	assert.Contains(t, result.stdout, "ビルド失敗")
}

// TestBuildCommand_LogFile_MissingInput は実際のBuildPipelineで入力検証に
// 失敗した場合も--log-fileにエラーが記録されることを検証する。
// newBuildPipelineを読み出すためt.Parallel()を呼ばない（TestBuildCommand_MissingInputと同じ理由）。
func TestBuildCommand_LogFile_MissingInput(t *testing.T) {
	dir := t.TempDir()
	nonexistent := filepath.Join(dir, "nonexistent.exe")
	logFile := filepath.Join(dir, "build.log")

	for range 2 {
		result := invoke(t, []string{"build", nonexistent, "--log-file", logFile})

		assert.Equal(t, int(apperr.ExitError), result.exitCode)
		assert.Contains(t, result.stdout, "Error: 入力ファイルが見つかりません")
		assert.NotContains(t, result.stderr, "入力ファイルが見つかりません", "エラーを端末へ二重に出さない")
	}

	content, err := os.ReadFile(logFile) //nolint:gosec // テストが指定したログファイルを読む用途のため妥当
	require.NoError(t, err)
	assert.Contains(t, string(content), "ERROR: 入力ファイルが見つかりません: "+nonexistent)
	assert.Equal(t, 2, strings.Count(string(content), "ERROR:"), "既存のログを切り詰めず追記する")

	info, err := os.Stat(logFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// newBuildPipelineを差し替えるためt.Parallel()を呼ばない。
func TestBuildCommand_LogFile_Unwritable(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(inputFile, make([]byte, 100), 0o600))
	logFile := filepath.Join(dir, "missing", "build.log")

	withStubBuildPipeline(t, &stubBuildRunner{runResult: pipeline.Result{Success: true}})

	result := invoke(t, []string{"build", inputFile, "--log-file", logFile})

	assert.Equal(t, int(apperr.ExitInvalidInput), result.exitCode)
	assert.Contains(t, result.stderr, "ログファイルを開けません: open "+logFile+": ")
	assert.Equal(t, 1, strings.Count(result.stderr, logFile), "パスは1回だけ示す")
	assert.NotContains(t, result.stdout, "ビルド完了", "ログファイルを開けなければビルドを始めない")
}

// TestBuildCommand_LogFile_PipelineLogger はパイプラインへ渡すロガーの出力先と
// 詳細度を検証する。
// newBuildPipelineを差し替えるためt.Parallel()を呼ばない。
func TestBuildCommand_LogFile_PipelineLogger(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(inputFile, make([]byte, 100), 0o600))
	outputFile := filepath.Join(dir, "output.apk")

	tests := []struct {
		name           string
		extraArgs      []string
		runResult      pipeline.Result
		wantExitCode   int
		wantFile       []string
		wantStdout     []string
		wantStdoutNone []string
	}{
		{
			name:           "正常系: -v無しでは詳細ログを端末へ出さずファイルへは全て記録する",
			runResult:      pipeline.Result{Success: true, OutputPath: &outputFile},
			wantExitCode:   int(apperr.ExitSuccess),
			wantFile:       []string{"WARNING: パイプライン警告", "VERBOSE: パイプライン詳細"},
			wantStdout:     []string{"警告: パイプライン警告", "ビルド完了"},
			wantStdoutNone: []string{"パイプライン詳細"},
		},
		{
			name:         "正常系: -v指定時は詳細ログも端末へ出す",
			extraArgs:    []string{"-v"},
			runResult:    pipeline.Result{Success: true, OutputPath: &outputFile},
			wantExitCode: int(apperr.ExitSuccess),
			wantFile:     []string{"VERBOSE: パイプライン詳細"},
			wantStdout:   []string{"パイプライン詳細"},
		},
		{
			name:         "異常系: ビルド失敗の理由をファイルへ記録する",
			runResult:    pipeline.Result{Success: false, ErrorMessage: "Gradleビルドに失敗しました"},
			wantExitCode: int(apperr.ExitError),
			wantFile:     []string{"ERROR: Gradleビルドに失敗しました"},
			wantStdout:   []string{"ビルド失敗: Gradleビルドに失敗しました"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logFile := filepath.Join(t.TempDir(), "build.log")
			stub := &stubBuildRunner{runResult: tt.runResult}

			original := newBuildPipeline
			newBuildPipeline = func(_ pipeline.Config, log pipeline.Logger) buildRunner {
				require.NotNil(t, log, "パイプラインへロガーを渡す")
				log.Warning("パイプライン警告")
				log.Verbose("パイプライン詳細")

				return stub
			}
			t.Cleanup(func() { newBuildPipeline = original })

			args := append([]string{"build", inputFile, "-o", outputFile, "--log-file", logFile}, tt.extraArgs...)
			result := invoke(t, args)

			assert.Equal(t, tt.wantExitCode, result.exitCode)

			content, err := os.ReadFile(logFile) //nolint:gosec // テストが指定したログファイルを読む用途のため妥当
			require.NoError(t, err)
			for _, want := range tt.wantFile {
				assert.Contains(t, string(content), want)
			}
			for _, want := range tt.wantStdout {
				assert.Contains(t, result.stdout, want)
			}
			for _, unwanted := range tt.wantStdoutNone {
				assert.NotContains(t, result.stdout, unwanted)
			}
		})
	}
}

// verboseRecorder はVerboseのメッセージだけを記録するpipeline.Loggerのテスト用実装。
type verboseRecorder struct {
	messages []string
}

func (*verboseRecorder) Info(string)              {}
func (*verboseRecorder) Warning(string)           {}
func (r *verboseRecorder) Verbose(message string) { r.messages = append(r.messages, message) }

// TestNewBuildPipeline_WiresLogger は既定のnewBuildPipelineが受け取ったロガーを
// 実際のBuildPipelineへ設定することを検証する。
// newBuildPipelineを読み出すためt.Parallel()を呼ばない（TestBuildCommand_MissingInputと同じ理由）。
func TestNewBuildPipeline_WiresLogger(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(input, make([]byte, 100), 0o600))
	recorder := &verboseRecorder{}

	p := newBuildPipeline(pipeline.NewConfig(input, filepath.Join(dir, "output.apk")), recorder)
	result := p.Run(nil)

	require.False(t, result.Success, "XP3を含まないEXEはANALYZEフェーズで失敗する")
	assert.Contains(t, recorder.messages, "analyzeフェーズを開始します")
}

func TestDoctorCommand_Runs(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"doctor"})

	assert.Contains(t, []int{int(apperr.ExitSuccess), int(apperr.ExitDependencyError)}, result.exitCode)
}

// why not: t.Parallel()を呼ばない。t.SetenvはPATH・ANDROID_HOMEというプロセス
// 全体の環境変数を書き換えるため、並列実行中の他テストのツール検出結果まで
// 変えてしまう（testingパッケージもt.Parallel()との併用を禁止している）。
func TestDoctorCommand_MissingRequiredToolExitsWithDependencyError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("ANDROID_HOME", t.TempDir())

	result := invoke(t, []string{"doctor"})

	assert.Equal(t, int(apperr.ExitDependencyError), result.exitCode)
	assert.Contains(t, result.stdout, "必須ツールが不足しています")
}

func TestDoctorCommand_ShowsTable(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"doctor"})

	assert.Contains(t, result.stdout, "依存ツールチェック結果")
}

func TestDoctorCommand_ShowsFFmpeg(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"doctor"})

	assert.Contains(t, result.stdout, "FFmpeg")
}

func TestInfoCommand_Help(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"info", "--help"})

	assert.Equal(t, 0, result.exitCode)
}

func TestCacheCommand_Help(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"cache", "--help"})

	assert.Equal(t, 0, result.exitCode)
	assert.Contains(t, result.stdout, "clean")
	assert.Contains(t, result.stdout, "info")
}

func TestCacheCleanCommand_Help(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"cache", "clean", "--help"})

	assert.Equal(t, 0, result.exitCode)
	assert.True(t, strings.Contains(result.stdout, "--force") || strings.Contains(result.stdout, "-f"))
}

func TestCacheInfoCommand_Runs(t *testing.T) {
	t.Parallel()

	result := invokeWithCacheDir(t, []string{"cache", "info"}, "", t.TempDir())

	assert.Equal(t, 0, result.exitCode)
}
