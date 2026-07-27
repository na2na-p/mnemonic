package signer

import (
	"context"
	"fmt"
	"os"
)

// ZipalignRunner はzipalignコマンドを実行するためのインターフェース。
//
// APKファイルのアラインメント最適化を行うzipalignコマンドの
// 実行機能を抽象化する。
type ZipalignRunner interface {
	// Align はinputPathのAPKファイルにアラインメント最適化を適用し、
	// outputPathへ出力する。成功時はoutputPathを返す。
	// 入力ファイルが存在しない場合はErrZipalignFileNotFound、
	// zipalignコマンドが見つからない場合はErrZipalignNotFound、
	// 実行に失敗した場合はErrZipalignFailedを返す。
	Align(inputPath, outputPath string) (string, error)

	// FindZipalign はzipalignコマンドのパスを検索する。
	// ANDROID_HOME環境変数やシステムPATHを参照する。
	// 見つからない場合は空文字列とfalseを返す。
	FindZipalign() (string, bool)

	// IsAligned はapkPathがアラインメント済みかどうかを確認する。
	// ファイルが存在しない場合はErrZipalignFileNotFound、
	// zipalignコマンドが見つからない場合はErrZipalignNotFound、
	// コマンドの実行自体に失敗した場合はErrZipalignFailedを返す。
	IsAligned(apkPath string) (bool, error)
}

// DefaultZipalignRunner はzipalignコマンドを実行する既定実装。
type DefaultZipalignRunner struct {
	runner CommandRunner
}

// NewDefaultZipalignRunner はDefaultZipalignRunnerを初期化する。
// runnerがnilの場合はos/execベースの既定実装を使用する。
func NewDefaultZipalignRunner(runner CommandRunner) *DefaultZipalignRunner {
	if runner == nil {
		runner = NewExecCommandRunner()
	}

	return &DefaultZipalignRunner{runner: runner}
}

// Align はAPKファイルにアラインメント最適化を適用する。
// zipalign -p -f 4 <input> <output> を実行する。
func (r *DefaultZipalignRunner) Align(inputPath, outputPath string) (string, error) {
	if _, err := os.Stat(inputPath); err != nil {
		return "", fmt.Errorf("%w: %s", ErrZipalignFileNotFound, inputPath)
	}

	zipalignPath, ok := r.FindZipalign()
	if !ok {
		return "", ErrZipalignNotFound
	}

	result, err := r.runner.Run(context.Background(), []string{zipalignPath, "-p", "-f", "4", inputPath, outputPath})
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrZipalignFailed, err)
	}

	if result.ExitCode != 0 {
		return "", fmt.Errorf("%w: %s", ErrZipalignFailed, result.Stderr)
	}

	return outputPath, nil
}

// FindZipalign はANDROID_HOME配下のbuild-toolsから最新バージョンのzipalignを検索し、
// 見つからない場合はシステムPATHから検索する。
func (r *DefaultZipalignRunner) FindZipalign() (string, bool) {
	return findAndroidBuildTool("zipalign")
}

// IsAligned はzipalign -c -v 4を実行してAPKファイルのアラインメント状態を確認する。
// 終了コードが0の場合にtrueを返す（非ゼロはコマンド失敗ではなく未アラインメントを表す）。
func (r *DefaultZipalignRunner) IsAligned(apkPath string) (bool, error) {
	if _, err := os.Stat(apkPath); err != nil {
		return false, fmt.Errorf("%w: %s", ErrZipalignFileNotFound, apkPath)
	}

	zipalignPath, ok := r.FindZipalign()
	if !ok {
		return false, ErrZipalignNotFound
	}

	result, err := r.runner.Run(context.Background(), []string{zipalignPath, "-c", "-v", "4", apkPath})
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrZipalignFailed, err)
	}

	return result.ExitCode == 0, nil
}
