// Package cmdrun は外部コマンドを起動し、終了コード・標準出力・標準エラーを
// 取得する共通処理を提供する。
//
// why not: 実行結果の分類（タイムアウト・コマンド未検出・非ゼロ終了）は
// このパッケージでは行わない。同じ非ゼロ終了でも、gradlewにとってはビルド失敗、
// doctorにとっては「コマンドは存在する」を意味するなど、呼び出し元ごとに解釈が
// 異なるためである。ここでは生の結果だけを返し、分類規則は各呼び出し元に残す。
package cmdrun

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// Result は外部コマンドが起動して終了した場合の結果。
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// Options はコマンド実行時の作業ディレクトリと環境変数。ゼロ値は継承。
type Options struct {
	Dir string
	Env []string
}

// ErrNoCommand はargsが空の場合のエラー。
var ErrNoCommand = errors.New("実行するコマンドが指定されていません")

// Run はargs[0]をargs[1:]で実行する。プロセスが起動して終了した場合は終了コードに
// かかわらずResultを返す（コンテキストにより強制終了された場合はExitCodeが-1）。
// プロセスを起動できなかった場合はos/execのエラーをそのまま返す。
//
// why not: コンテキストの期限切れでプロセスが強制終了された場合も、os/execは
// context.DeadlineExceededではなく*exec.ExitError（ExitCode -1, "signal: killed"）を
// 返す。これをタイムアウトとしてerrorへ変換すると、期限切れをどう扱うかという
// 呼び出し元の判断を奪うため、ctx.Err()の確認は呼び出し元に委ねる。
func Run(ctx context.Context, opts Options, args ...string) (Result, error) {
	if len(args) == 0 {
		return Result{}, ErrNoCommand
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // zipalign/apksigner/gradlew/keytool/doctorの依存ツールを呼び出す用途のため妥当
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return Result{ExitCode: exitErr.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String()}, nil
		}

		return Result{}, err
	}

	return Result{ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String()}, nil
}
