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
	"github.com/na2na-p/mnemonic/internal/version"
)

// cliResult はtyper.testing.CliRunnerのResultに相当するテスト用の結果値。
type cliResult struct {
	exitCode int
	stdout   string
	stderr   string
}

func invoke(t *testing.T, args []string, stdin string) cliResult {
	t.Helper()

	var stdout, stderr bytes.Buffer

	code := run(args, strings.NewReader(stdin), &stdout, &stderr)

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
		{name: "正常系: バージョン表示", args: []string{"--version"}, wantSubstr: version.String()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := invoke(t, tt.args, "")

			assert.Equal(t, 0, result.exitCode)
			assert.Contains(t, result.stdout, tt.wantSubstr)
		})
	}
}

func TestBuildCommand_Help(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"build", "--help"}, "")

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

	result := invoke(t, []string{"build", nonexistent}, "")

	assert.Equal(t, 1, result.exitCode)
	assert.True(t, strings.Contains(result.stdout, "Error") || strings.Contains(result.stdout, "エラー"))
}

func TestBuildCommand_InvalidInputType(t *testing.T) {
	dir := t.TempDir()
	invalidFile := filepath.Join(dir, "invalid.txt")
	require.NoError(t, os.WriteFile(invalidFile, []byte("invalid content"), 0o600))

	result := invoke(t, []string{"build", invalidFile}, "")

	assert.Equal(t, 1, result.exitCode)
}

// stubBuildRunner はbuildRunnerのテスト用スタブ。Python版のMock(BuildPipeline)
// に相当する。
type stubBuildRunner struct {
	validateErrs []string
	runResult    pipeline.Result
}

func (s *stubBuildRunner) Validate() []string { return s.validateErrs }

func (s *stubBuildRunner) Run(pipeline.ProgressCallback) pipeline.Result { return s.runResult }

func withStubBuildPipeline(t *testing.T, stub *stubBuildRunner) {
	t.Helper()

	original := newBuildPipeline
	newBuildPipeline = func(pipeline.Config) buildRunner { return stub }
	t.Cleanup(func() { newBuildPipeline = original })
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

	result := invoke(t, []string{"build", inputFile, "-o", outputFile}, "")

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

	result := invoke(t, []string{"build", inputFile, "-o", outputFile, "-v"}, "")

	assert.Equal(t, 0, result.exitCode)
}

func TestBuildCommand_Failure(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "game.exe")
	require.NoError(t, os.WriteFile(inputFile, make([]byte, 100), 0o600))

	withStubBuildPipeline(t, &stubBuildRunner{
		validateErrs: nil,
		runResult:    pipeline.Result{Success: false, ErrorMessage: "Gradleビルドに失敗しました"},
	})

	result := invoke(t, []string{"build", inputFile}, "")

	assert.Equal(t, int(apperr.ExitError), result.exitCode)
	assert.Contains(t, result.stdout, "ビルド失敗")
}

func TestDoctorCommand_Runs(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"doctor"}, "")

	assert.Contains(t, []int{0, 1}, result.exitCode)
}

func TestDoctorCommand_ShowsTable(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"doctor"}, "")

	assert.Contains(t, result.stdout, "依存ツールチェック結果")
}

func TestDoctorCommand_ShowsPython(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"doctor"}, "")

	assert.Contains(t, result.stdout, "Python")
}

func TestInfoCommand_Help(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"info", "--help"}, "")

	assert.Equal(t, 0, result.exitCode)
}

func TestCacheCommand_Help(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"cache", "--help"}, "")

	assert.Equal(t, 0, result.exitCode)
	assert.Contains(t, result.stdout, "clean")
	assert.Contains(t, result.stdout, "info")
}

func TestCacheCleanCommand_Help(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"cache", "clean", "--help"}, "")

	assert.Equal(t, 0, result.exitCode)
	assert.True(t, strings.Contains(result.stdout, "--force") || strings.Contains(result.stdout, "-f"))
}

func TestCacheInfoCommand_Runs(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"cache", "info"}, "")

	assert.Equal(t, 0, result.exitCode)
}
