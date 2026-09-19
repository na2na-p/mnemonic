package signer

import (
	"context"
	"fmt"
	"os"
)

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

// Align はinputPathのAPKファイルにアラインメント最適化を適用し、
// outputPathへ出力する。成功時はoutputPathを返す。
// zipalign -p -f 4 <input> <output> を実行する。
// 入力ファイルが存在しない場合はErrZipalignFileNotFound、
// zipalignコマンドが見つからない場合はErrZipalignNotFound、
// 実行に失敗した場合はErrZipalignFailedを返す。
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
// 見つからない場合は空文字列とfalseを返す。
func (r *DefaultZipalignRunner) FindZipalign() (string, bool) {
	return findAndroidBuildTool("zipalign")
}
