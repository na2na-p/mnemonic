// why not: このファイルのテストはすべてANDROID_HOME/PATH環境変数をt.Setenvで
// 変更する。t.Setenvはテスト（または親テスト）がt.Parallel()を呼んでいると
// panicするため、本ファイルでは一貫してt.Parallel()を使わない
// （lang-go.md Criterion G1「t.Parallel()は副作用が無い場合に使う」に従う）。
package signer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/na2na-p/mnemonic/internal/signer"
)

func TestNewDefaultZipalignRunner_NilRunnerFallsBackToExecCommandRunner(t *testing.T) {
	// why: 既定実装(execCommandRunner)は実プロセスの起動を要求する。zipalignが
	// 存在しない環境でも、コマンド不在の分岐に到達しパニックしないことのみを
	// 確認する（実効的なコマンド呼び出しの検証はCommandRunner注入テストで別途行う）。
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("PATH", t.TempDir())

	r := signer.NewDefaultZipalignRunner(nil)

	assert.NotPanics(t, func() {
		_, ok := r.FindZipalign()
		assert.False(t, ok)
	})
}

func TestDefaultZipalignRunner_Align(t *testing.T) {
	t.Run("正常系: アラインメント成功時に出力パスを返す", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "input.apk")
		output := filepath.Join(dir, "output.apk")
		require.NoError(t, os.WriteFile(input, []byte("unaligned"), 0o600))

		androidHome := writeFakeTool(t, "zipalign")
		t.Setenv("ANDROID_HOME", androidHome)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, args []string) (signer.RunResult, error) {
				assert.Contains(t, args[0], "zipalign")
				assert.Contains(t, args, "-f")
				assert.Contains(t, args, "4")
				assert.Contains(t, args, input)
				assert.Contains(t, args, output)

				return signer.RunResult{ExitCode: 0, Stdout: "aligned"}, nil
			})

		r := signer.NewDefaultZipalignRunner(runner)

		result, err := r.Align(input, output)

		require.NoError(t, err)
		assert.Equal(t, output, result)
	})

	t.Run("異常系: 入力ファイルが存在しない場合にErrZipalignFileNotFound", func(t *testing.T) {
		dir := t.TempDir()

		r := signer.NewDefaultZipalignRunner(nil)

		_, err := r.Align(filepath.Join(dir, "missing.apk"), filepath.Join(dir, "output.apk"))

		assert.ErrorIs(t, err, signer.ErrZipalignFileNotFound)
	})

	t.Run("異常系: zipalignコマンドが見つからない場合にErrZipalignNotFound", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "input.apk")
		require.NoError(t, os.WriteFile(input, []byte("unaligned"), 0o600))

		t.Setenv("ANDROID_HOME", "")
		t.Setenv("PATH", t.TempDir())

		r := signer.NewDefaultZipalignRunner(nil)

		_, err := r.Align(input, filepath.Join(dir, "output.apk"))

		assert.ErrorIs(t, err, signer.ErrZipalignNotFound)
	})

	t.Run("異常系: zipalignコマンドが非ゼロ終了の場合にErrZipalignFailed", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "input.apk")
		require.NoError(t, os.WriteFile(input, []byte("unaligned"), 0o600))

		androidHome := writeFakeTool(t, "zipalign")
		t.Setenv("ANDROID_HOME", androidHome)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			Return(signer.RunResult{ExitCode: 1, Stderr: "zipalign error: invalid input"}, nil)

		r := signer.NewDefaultZipalignRunner(runner)

		_, err := r.Align(input, filepath.Join(dir, "output.apk"))

		require.ErrorIs(t, err, signer.ErrZipalignFailed)
		assert.ErrorContains(t, err, "invalid input")
	})

	t.Run("異常系: コマンド実行自体に失敗した場合にErrZipalignFailed", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "input.apk")
		require.NoError(t, os.WriteFile(input, []byte("unaligned"), 0o600))

		androidHome := writeFakeTool(t, "zipalign")
		t.Setenv("ANDROID_HOME", androidHome)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			Return(signer.RunResult{}, assert.AnError)

		r := signer.NewDefaultZipalignRunner(runner)

		_, err := r.Align(input, filepath.Join(dir, "output.apk"))

		assert.ErrorIs(t, err, signer.ErrZipalignFailed)
	})
}

func TestDefaultZipalignRunner_IsAligned(t *testing.T) {
	t.Run("正常系: 終了コード0はアラインメント済みでtrue", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))

		androidHome := writeFakeTool(t, "zipalign")
		t.Setenv("ANDROID_HOME", androidHome)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, args []string) (signer.RunResult, error) {
				assert.Contains(t, args, "-c")
				assert.Contains(t, args, "4")
				assert.Contains(t, args, apk)

				return signer.RunResult{ExitCode: 0}, nil
			})

		r := signer.NewDefaultZipalignRunner(runner)

		aligned, err := r.IsAligned(apk)

		require.NoError(t, err)
		assert.True(t, aligned)
	})

	t.Run("正常系: 終了コード非ゼロは未アラインメントでfalse(エラーなし)", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))

		androidHome := writeFakeTool(t, "zipalign")
		t.Setenv("ANDROID_HOME", androidHome)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			Return(signer.RunResult{ExitCode: 1}, nil)

		r := signer.NewDefaultZipalignRunner(runner)

		aligned, err := r.IsAligned(apk)

		require.NoError(t, err)
		assert.False(t, aligned)
	})

	t.Run("異常系: ファイルが存在しない場合にErrZipalignFileNotFound", func(t *testing.T) {
		dir := t.TempDir()

		r := signer.NewDefaultZipalignRunner(nil)

		_, err := r.IsAligned(filepath.Join(dir, "missing.apk"))

		assert.ErrorIs(t, err, signer.ErrZipalignFileNotFound)
	})

	t.Run("異常系: zipalignコマンドが見つからない場合にErrZipalignNotFound", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))

		t.Setenv("ANDROID_HOME", "")
		t.Setenv("PATH", t.TempDir())

		r := signer.NewDefaultZipalignRunner(nil)

		_, err := r.IsAligned(apk)

		assert.ErrorIs(t, err, signer.ErrZipalignNotFound)
	})

	t.Run("異常系: コマンド実行自体に失敗した場合にErrZipalignFailed", func(t *testing.T) {
		dir := t.TempDir()
		apk := filepath.Join(dir, "test.apk")
		require.NoError(t, os.WriteFile(apk, []byte("apk"), 0o600))

		androidHome := writeFakeTool(t, "zipalign")
		t.Setenv("ANDROID_HOME", androidHome)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), gomock.Any()).
			Return(signer.RunResult{}, assert.AnError)

		r := signer.NewDefaultZipalignRunner(runner)

		_, err := r.IsAligned(apk)

		assert.ErrorIs(t, err, signer.ErrZipalignFailed)
	})
}

// writeFakeTool はtoolNameを含むbuild-toolsディレクトリ構造をtempディレクトリ配下に
// 作成し、ANDROID_HOMEに設定すべきパスを返す。
func writeFakeTool(t *testing.T, toolName string) string {
	t.Helper()

	androidHome := t.TempDir()
	buildTools := filepath.Join(androidHome, "build-tools", "34.0.0")
	require.NoError(t, os.MkdirAll(buildTools, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(buildTools, toolName), []byte("#!/bin/sh\n"), 0o700)) //nolint:gosec // テスト用フェイク実行ファイルのため妥当

	return androidHome
}
