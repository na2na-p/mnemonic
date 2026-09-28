package builder_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/na2na-p/mnemonic/internal/builder"
	"github.com/na2na-p/mnemonic/internal/cmdrun"
)

func TestNewGradleBuilder(t *testing.T) {
	t.Parallel()

	t.Run("正常系: タイムアウト未指定でも初期化でき、wrapperの無いプロジェクトのBuildはErrGradleWrapperNotFoundを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		b, err := builder.NewGradleBuilder(dir, 0, nil)
		require.NoError(t, err)

		_, err = b.Build("release")
		assert.ErrorIs(t, err, builder.ErrGradleWrapperNotFound)
	})

	t.Run("正常系: gradle.propertiesが存在しない場合は新規作成される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		_, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(dir, "gradle.properties"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "org.gradle.caching=false")
		assert.Contains(t, string(content), "org.gradle.vfs.watch=false")
	})

	t.Run("正常系: gradle.propertiesが既に設定を含む場合は追記しない", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		propsPath := filepath.Join(dir, "gradle.properties")
		original := "org.gradle.caching=false\nother.setting=true\n"
		require.NoError(t, os.WriteFile(propsPath, []byte(original), 0o600))

		_, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		content, err := os.ReadFile(propsPath) //nolint:gosec // テストで作成した固定パスを読むだけのため妥当
		require.NoError(t, err)
		assert.Contains(t, string(content), "org.gradle.vfs.watch=false")
		assert.Equal(t, 1, countOccurrences(string(content), "org.gradle.caching=false"))
	})
}

func countOccurrences(s, substr string) int {
	count := 0
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}

	return count
}

func TestGradleBuilder_Build(t *testing.T) {
	t.Parallel()

	t.Run("異常系: gradlewが存在しない場合にErrGradleWrapperNotFound", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		b, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		_, err = b.Build("release")

		assert.ErrorIs(t, err, builder.ErrGradleWrapperNotFound)
	})

	t.Run("正常系: buildがCommandRunnerを正しい引数で呼び出す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFakeGradlew(t, dir)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), dir, gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ []string, args []string) (builder.RunResult, error) {
				assert.Contains(t, args, "assembleRelease")
				assert.Contains(t, args, "--no-daemon")
				assert.Contains(t, args, "--stacktrace")

				return builder.RunResult{ExitCode: 0, Stdout: "BUILD SUCCESSFUL"}, nil
			})

		b, err := builder.NewGradleBuilder(dir, time.Minute, runner)
		require.NoError(t, err)

		result, err := b.Build("release")

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, result.OutputLog, "BUILD SUCCESSFUL")
	})

	t.Run("異常系: ビルド失敗時にErrGradleBuildFailed", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFakeGradlew(t, dir)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(builder.RunResult{ExitCode: 1, Stderr: "BUILD FAILED"}, nil)

		b, err := builder.NewGradleBuilder(dir, time.Minute, runner)
		require.NoError(t, err)

		_, err = b.Build("release")

		require.ErrorIs(t, err, builder.ErrGradleBuildFailed)
		assert.ErrorContains(t, err, "BUILD FAILED")
	})

	t.Run("異常系: タイムアウト時にErrGradleTimeout", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFakeGradlew(t, dir)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(builder.RunResult{}, errors.Join(builder.ErrGradleTimeout, context.DeadlineExceeded))

		b, err := builder.NewGradleBuilder(dir, time.Millisecond, runner)
		require.NoError(t, err)

		_, err = b.Build("release")

		assert.ErrorIs(t, err, builder.ErrGradleTimeout)
	})

	t.Run("正常系: ビルドタイプに応じたタスクが実行される", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name         string
			buildType    string
			expectedTask string
		}{
			{name: "正常系: releaseビルド", buildType: "release", expectedTask: "assembleRelease"},
			{name: "正常系: debugビルド", buildType: "debug", expectedTask: "assembleDebug"},
			{name: "正常系: ビルドタイプが空文字列ならreleaseビルド", buildType: "", expectedTask: "assembleRelease"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				writeFakeGradlew(t, dir)

				ctrl := gomock.NewController(t)
				runner := NewMockCommandRunner(ctrl)
				runner.EXPECT().
					Run(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, _ []string, args []string) (builder.RunResult, error) {
						assert.Contains(t, args, tc.expectedTask)

						return builder.RunResult{ExitCode: 0}, nil
					})

				b, err := builder.NewGradleBuilder(dir, time.Minute, runner)
				require.NoError(t, err)

				_, err = b.Build(tc.buildType)
				require.NoError(t, err)
			})
		}
	})
}

func TestGradleBuilder_GetAPKPath(t *testing.T) {
	t.Parallel()

	t.Run("正常系: APKが存在する場合にパスを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		apkDir := filepath.Join(dir, "app", "build", "outputs", "apk", "release")
		require.NoError(t, os.MkdirAll(apkDir, 0o750))
		apkFile := filepath.Join(apkDir, "app-release-unsigned.apk")
		require.NoError(t, os.WriteFile(apkFile, []byte(""), 0o600))

		b, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		result := b.GetAPKPath("release")

		require.NotNil(t, result)
		assert.Equal(t, apkFile, *result)
	})

	t.Run("正常系: APKが存在しない場合にnilを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		b, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		assert.Nil(t, b.GetAPKPath("release"))
	})

	t.Run("正常系: ビルドタイプに応じたパスが返される", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name         string
			buildType    string
			relativePath string
		}{
			{
				name:         "正常系: releaseビルドのパス",
				buildType:    "release",
				relativePath: filepath.Join("app", "build", "outputs", "apk", "release", "app-release-unsigned.apk"),
			},
			{
				name:         "正常系: debugビルドのパス",
				buildType:    "debug",
				relativePath: filepath.Join("app", "build", "outputs", "apk", "debug", "app-debug.apk"),
			},
			{
				name:         "正常系: ビルドタイプが空文字列ならreleaseビルドのパス",
				buildType:    "",
				relativePath: filepath.Join("app", "build", "outputs", "apk", "release", "app-release-unsigned.apk"),
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				apkPath := filepath.Join(dir, tc.relativePath)
				require.NoError(t, os.MkdirAll(filepath.Dir(apkPath), 0o750))
				require.NoError(t, os.WriteFile(apkPath, []byte(""), 0o600))

				b, err := builder.NewGradleBuilder(dir, time.Minute, nil)
				require.NoError(t, err)

				result := b.GetAPKPath(tc.buildType)

				require.NotNil(t, result)
				assert.Equal(t, apkPath, *result)
			})
		}
	})

	t.Run("正常系: app-release.apk(unsigned接尾辞なし)が存在する場合にパスを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		apkDir := filepath.Join(dir, "app", "build", "outputs", "apk", "release")
		require.NoError(t, os.MkdirAll(apkDir, 0o750))
		apkFile := filepath.Join(apkDir, "app-release.apk")
		require.NoError(t, os.WriteFile(apkFile, []byte(""), 0o600))

		b, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		result := b.GetAPKPath("release")

		require.NotNil(t, result)
		assert.Equal(t, apkFile, *result)
	})

	t.Run("正常系: krkrsdl2テンプレートのカスタムファイル名しか無い場合はglobフォールバックで見つかる", func(t *testing.T) {
		t.Parallel()

		// why not: krkrsdl2テンプレートのapp/build.gradleはoutputFileNameを
		// "${app_name}_${architecture}.apk"へカスタマイズしており、標準名の
		// APKが生成されないことがある。
		dir := t.TempDir()
		apkDir := filepath.Join(dir, "app", "build", "outputs", "apk", "release")
		require.NoError(t, os.MkdirAll(apkDir, 0o750))
		apkFile := filepath.Join(apkDir, "krkrsdl2_universal.apk")
		require.NoError(t, os.WriteFile(apkFile, []byte(""), 0o600))

		b, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		result := b.GetAPKPath("release")

		require.NotNil(t, result)
		assert.Equal(t, apkFile, *result)
	})

	t.Run("正常系: outputsディレクトリが存在しない場合にnilを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		b, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		assert.Nil(t, b.GetAPKPath("release"))
	})

	t.Run("正常系: ディレクトリは存在するがAPKファイルが無い場合にnilを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		apkDir := filepath.Join(dir, "app", "build", "outputs", "apk", "release")
		require.NoError(t, os.MkdirAll(apkDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(apkDir, "readme.txt"), []byte(""), 0o600))

		b, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		assert.Nil(t, b.GetAPKPath("release"))
	})

	t.Run("正常系: 標準名とカスタム名の両方が存在する場合は標準名が優先される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		apkDir := filepath.Join(dir, "app", "build", "outputs", "apk", "release")
		require.NoError(t, os.MkdirAll(apkDir, 0o750))
		standardFile := filepath.Join(apkDir, "app-release-unsigned.apk")
		require.NoError(t, os.WriteFile(standardFile, []byte(""), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(apkDir, "krkrsdl2_universal.apk"), []byte(""), 0o600))

		b, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		result := b.GetAPKPath("release")

		require.NotNil(t, result)
		assert.Equal(t, standardFile, *result)
	})
}

func writeFakeGradlew(t *testing.T, dir string) {
	t.Helper()

	gradlewName := "gradlew"
	if runtime.GOOS == "windows" {
		gradlewName = "gradlew.bat"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, gradlewName), []byte("#!/bin/sh\n"), 0o700)) //nolint:gosec // テスト用のフェイク実行ファイルのため妥当
}

func TestExecCommandRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 終了コード0でstdoutを返す", func(t *testing.T) {
		t.Parallel()

		runner := builder.NewExecCommandRunner()

		result, err := runner.Run(t.Context(), "", nil, []string{"echo", "-n", "ok"})

		require.NoError(t, err)
		assert.Equal(t, 0, result.ExitCode)
		assert.Equal(t, "ok", result.Stdout)
	})

	t.Run("正常系: 非ゼロ終了コードはerrorではなくRunResultで返す", func(t *testing.T) {
		t.Parallel()

		runner := builder.NewExecCommandRunner()

		result, err := runner.Run(t.Context(), "", nil, []string{"sh", "-c", "echo fail 1>&2; exit 3"})

		require.NoError(t, err)
		assert.Equal(t, 3, result.ExitCode)
		assert.Contains(t, result.Stderr, "fail")
	})

	t.Run("正常系: workDirが作業ディレクトリになる", func(t *testing.T) {
		t.Parallel()

		runner := builder.NewExecCommandRunner()
		dir := t.TempDir()

		result, err := runner.Run(t.Context(), dir, nil, []string{"pwd"})

		require.NoError(t, err)
		assert.Equal(t, 0, result.ExitCode)

		// macOSではt.TempDir()の/varが/private/varへのシンボリックリンクのため、
		// 両辺を実パスへ解決してから比較する。
		wantDir, err := filepath.EvalSymlinks(dir)
		require.NoError(t, err)
		gotDir, err := filepath.EvalSymlinks(strings.TrimRight(result.Stdout, "\n"))
		require.NoError(t, err)
		assert.Equal(t, wantDir, gotDir)
	})

	t.Run("異常系: コンテキスト期限超過で強制終了された場合はErrGradleTimeout", func(t *testing.T) {
		t.Parallel()

		runner := builder.NewExecCommandRunner()
		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()

		_, err := runner.Run(ctx, "", nil, []string{"sleep", "5"})

		require.ErrorIs(t, err, builder.ErrGradleTimeout)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("異常系: コマンドが空の場合にErrNoCommand", func(t *testing.T) {
		t.Parallel()

		runner := builder.NewExecCommandRunner()

		_, err := runner.Run(t.Context(), "", nil, nil)

		require.ErrorIs(t, err, cmdrun.ErrNoCommand)
	})

	t.Run("異常系: コマンドが見つからない場合にerror", func(t *testing.T) {
		t.Parallel()

		runner := builder.NewExecCommandRunner()

		_, err := runner.Run(t.Context(), "", nil, []string{"mnemonic-builder-nonexistent-command-xyz"})

		assert.Error(t, err)
	})
}

func TestGradleBuildError_Error(t *testing.T) {
	t.Parallel()

	aapt2Failure, err := os.ReadFile(filepath.Join("testdata", "gradle_failure_aapt2.txt"))
	require.NoError(t, err)

	longBlock := make([]string, 0, 40)
	for i := range 40 {
		longBlock = append(longBlock, fmt.Sprintf("cause%02d", i+1))
	}

	noBlock := make([]string, 0, 50)
	for i := range 25 {
		noBlock = append(noBlock, fmt.Sprintf("line%02d", i+1), "")
	}

	tests := []struct {
		name        string
		exitCode    int
		output      string
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:     "異常系: What went wrongブロックを終了コードとともに示し、ダウンロード進捗やスタックトレースは含めない",
			exitCode: 1,
			output:   string(aapt2Failure),
			wantContain: []string{
				"Gradleビルドに失敗しました: exit code 1:\n",
				"Execution failed for task ':app:mergeReleaseResources'.",
				"AAPT2 aapt2-7.4.2-8841542-linux Daemon #1: Daemon startup failed",
			},
			wantAbsent: []string{
				"....",
				"Welcome to Gradle",
				"Unexpected error output",
				"* What went wrong:",
				"* Try:",
				"ExecuteActionsTaskExecuter",
				"Caused by",
				"BUILD FAILED",
			},
		},
		{
			name:        "異常系: 失敗が複数あれば各What went wrongブロックを示す",
			exitCode:    1,
			output:      "FAILURE: Build completed with 2 failures.\n\n1: Task failed with an exception.\n-----------\n* What went wrong:\nfirst cause\n\n* Try:\n> Run with --info\n==============\n\n2: Task failed with an exception.\n-----------\n* What went wrong:\nsecond cause\n\n* Try:\n> Run with --info\n",
			wantContain: []string{"exit code 1:\nfirst cause\n\nsecond cause"},
			wantAbsent:  []string{"Run with --info"},
		},
		{
			name:        "異常系: CRLFの出力でもブロックを抽出し、CRを残さない",
			exitCode:    1,
			output:      "* What went wrong:\r\ncrlf cause\r\n\r\n* Try:\r\n> Run with --info\r\n",
			wantContain: []string{"crlf cause"},
			wantAbsent:  []string{"\r", "Run with --info"},
		},
		{
			name:        "異常系: 長いブロックは先頭30行に切り詰め、省略した行数を示す",
			exitCode:    1,
			output:      "* What went wrong:\n" + strings.Join(longBlock, "\n") + "\n\n* Try:\n",
			wantContain: []string{"cause01\n", "cause30\n", "ほか10行を省略"},
			wantAbsent:  []string{"cause31"},
		},
		{
			name:        "異常系: 出力が空白だけなら出力が無いことを示す",
			exitCode:    1,
			output:      " \n\n",
			wantContain: []string{"Gradleビルドに失敗しました: exit code 1: （Gradleの出力なし）"},
			wantAbsent:  []string{":\n"},
		},
		{
			name:        "異常系: ブロックが無ければ空行を除く末尾20行を示す",
			exitCode:    2,
			output:      strings.Join(noBlock, "\n"),
			wantContain: []string{"Gradleビルドに失敗しました: exit code 2:\n", "line06\nline07", "line25"},
			wantAbsent:  []string{"line05", "\n\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := &builder.GradleBuildError{ExitCode: tt.exitCode, Output: tt.output}

			require.ErrorIs(t, err, builder.ErrGradleBuildFailed)
			for _, want := range tt.wantContain {
				assert.Contains(t, err.Error(), want)
			}
			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, err.Error(), absent)
			}
		})
	}
}

func TestGradleBuilder_Build_Failure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		result      builder.RunResult
		wantOutput  string
		wantContain string
	}{
		{
			name:        "異常系: 失敗時は標準出力と標準エラーを結合した全出力をエラーから取り出せる",
			result:      builder.RunResult{ExitCode: 1, Stdout: "> Task :app:a\n", Stderr: "* What went wrong:\nboom\n\n* Try:\n"},
			wantOutput:  "> Task :app:a\n* What went wrong:\nboom\n\n* Try:\n",
			wantContain: "boom",
		},
		{
			name:        "異常系: 標準出力が改行で終わらなくても標準エラーのブロックを抽出する",
			result:      builder.RunResult{ExitCode: 1, Stdout: "> Task :app:a FAILED", Stderr: "* What went wrong:\nboom\n\n* Try:\n"},
			wantOutput:  "> Task :app:a FAILED\n* What went wrong:\nboom\n\n* Try:\n",
			wantContain: "exit code 1:\nboom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeFakeGradlew(t, dir)

			ctrl := gomock.NewController(t)
			runner := NewMockCommandRunner(ctrl)
			runner.EXPECT().
				Run(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(tt.result, nil)

			b, err := builder.NewGradleBuilder(dir, time.Minute, runner)
			require.NoError(t, err)

			_, err = b.Build("release")

			require.ErrorIs(t, err, builder.ErrGradleBuildFailed)
			buildErr, ok := errors.AsType[*builder.GradleBuildError](err)
			require.True(t, ok)
			assert.Equal(t, tt.result.ExitCode, buildErr.ExitCode)
			assert.Equal(t, tt.wantOutput, buildErr.Output)
			assert.Contains(t, err.Error(), tt.wantContain)
		})
	}
}
