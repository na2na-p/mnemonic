package builder_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/na2na-p/mnemonic/internal/builder"
)

func TestNewGradleBuilder(t *testing.T) {
	t.Parallel()

	t.Run("正常系: デフォルトタイムアウトで初期化", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		b, err := builder.NewGradleBuilder(dir, 0, nil)

		require.NoError(t, err)
		assert.False(t, b.CheckGradleWrapper())
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

func TestGradleBuilder_CheckGradleWrapper(t *testing.T) {
	t.Parallel()

	t.Run("正常系: gradlewが存在する場合にtrueを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		gradlewName := "gradlew"
		if runtime.GOOS == "windows" {
			gradlewName = "gradlew.bat"
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, gradlewName), []byte(""), 0o600))

		b, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		assert.True(t, b.CheckGradleWrapper())
	})

	t.Run("正常系: gradlewが存在しない場合にfalseを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		b, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		assert.False(t, b.CheckGradleWrapper())
	})
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

func TestGradleBuilder_Clean(t *testing.T) {
	t.Parallel()

	t.Run("異常系: gradlewが存在しない場合にErrGradleWrapperNotFound", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		b, err := builder.NewGradleBuilder(dir, time.Minute, nil)
		require.NoError(t, err)

		err = b.Clean()

		assert.ErrorIs(t, err, builder.ErrGradleWrapperNotFound)
	})

	t.Run("正常系: cleanがcleanタスクで呼び出される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFakeGradlew(t, dir)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ []string, args []string) (builder.RunResult, error) {
				assert.Contains(t, args, "clean")

				return builder.RunResult{ExitCode: 0}, nil
			})

		b, err := builder.NewGradleBuilder(dir, time.Minute, runner)
		require.NoError(t, err)

		require.NoError(t, b.Clean())
	})

	t.Run("異常系: クリーン失敗時にErrGradleBuildFailed", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFakeGradlew(t, dir)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(builder.RunResult{ExitCode: 1, Stderr: "CLEAN FAILED"}, nil)

		b, err := builder.NewGradleBuilder(dir, time.Minute, runner)
		require.NoError(t, err)

		err = b.Clean()

		assert.ErrorIs(t, err, builder.ErrGradleBuildFailed)
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
		// APKが生成されないことがある（T-218で判明した実ビルドでの回帰）。
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
