package signer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// RunResult は外部コマンドの実行結果を表す。
//
// why not: zipalignのisAligned/apksignerのverifyは非ゼロ終了コードを
// 「コマンド失敗」ではなく「未アラインメント/検証失敗」という正常な戻り値
// として扱う（Python版のsubprocess.run(check=False)と同じ設計）。そのため
// 終了コード・stdout・stderrをそのまま呼び出し元へ返し、プロセスの起動自体に
// 失敗した場合のみerrorを返す（internal/builder.RunResultと同じ設計方針）。
type RunResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// CommandRunner はzipalign/apksignerコマンドの実行を抽象化する。
//
// why not: os/exec.Cmdを直接呼ぶとユニットテストが実際のzipalign/apksigner
// バイナリを要求し、CI環境依存かつAndroid SDKのインストールが前提になる。
// 実行結果を差し替え可能にするためインターフェース化し、go.uber.org/mock
// (gomock)でモックする（internal/builder.CommandRunnerと同じ設計方針）。
type CommandRunner interface {
	// Run はargsのコマンドを実行する。プロセスが起動し完了した場合は
	// 終了コードにかかわらずRunResultを返す。コマンド未検出など、
	// プロセスの実行自体に失敗した場合にerrorを返す。
	Run(ctx context.Context, args []string) (RunResult, error)
}

// execCommandRunner はos/execを使った既定のCommandRunner実装。
type execCommandRunner struct{}

// NewExecCommandRunner はos/execベースのCommandRunnerを返す。
func NewExecCommandRunner() CommandRunner {
	return execCommandRunner{}
}

func (execCommandRunner) Run(ctx context.Context, args []string) (RunResult, error) {
	if len(args) == 0 {
		return RunResult{}, errors.New("実行するコマンドが指定されていません")
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // zipalign/apksignerを呼び出す用途のため妥当

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return RunResult{
				ExitCode: exitErr.ExitCode(),
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
			}, nil
		}

		return RunResult{}, fmt.Errorf("コマンドの実行に失敗しました: %w", err)
	}

	return RunResult{ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String()}, nil
}
