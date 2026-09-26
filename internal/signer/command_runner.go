package signer

import (
	"context"
	"errors"
	"fmt"

	"github.com/na2na-p/mnemonic/internal/cmdrun"
)

// RunResult は外部コマンドの実行結果を表す。
//
// why not: 非ゼロ終了をerrorへ畳み込むと、呼び出し元(Align/Sign)がツールの
// stderrを自身のセンチネルエラーへ添えて返せなくなる。そのため終了コード・
// stdout・stderrをそのまま呼び出し元へ返し、プロセスの実行自体に失敗した場合
// （コマンド未検出、コンテキストの期限超過・キャンセル等）のみerrorを返す
// （internal/builder.RunResultと同じ設計方針）。
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
	// 終了コードにかかわらずRunResultを返す。コマンド未検出など
	// プロセスの実行自体に失敗した場合と、コンテキストの期限超過・
	// キャンセルでプロセスが強制終了された場合にerrorを返す。
	Run(ctx context.Context, args []string) (RunResult, error)
}

// execCommandRunner はos/execを使った既定のCommandRunner実装。
type execCommandRunner struct{}

// NewExecCommandRunner はos/execベースのCommandRunnerを返す。
func NewExecCommandRunner() CommandRunner {
	return execCommandRunner{}
}

func (execCommandRunner) Run(ctx context.Context, args []string) (RunResult, error) {
	res, err := cmdrun.Run(ctx, cmdrun.Options{}, args...)
	if errors.Is(err, cmdrun.ErrNoCommand) {
		return RunResult{}, err
	}
	// why not: コンテキストで強制終了されたプロセスはcmdrunからResult{ExitCode: -1}と
	// nil errorで返るため、errだけを見るとタイムアウトがstderrのほぼ空なツールの失敗として
	// 報告される。一方、成功時にctx.Err()を見ると、期限直前に正常終了したコマンドが
	// タイムアウト扱いになるため、失敗した場合に限って確認する。
	if err != nil || res.ExitCode != 0 {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return RunResult{}, fmt.Errorf("コマンドの実行に失敗しました: %w", ctxErr)
		}
	}
	if err != nil {
		return RunResult{}, fmt.Errorf("コマンドの実行に失敗しました: %w", err)
	}

	return RunResult{ExitCode: res.ExitCode, Stdout: res.Stdout, Stderr: res.Stderr}, nil
}
