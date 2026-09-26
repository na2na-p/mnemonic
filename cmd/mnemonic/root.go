package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/na2na-p/mnemonic/internal/apperr"
	"github.com/na2na-p/mnemonic/internal/cache"
	"github.com/na2na-p/mnemonic/internal/version"
)

// cliExitError はコマンド実行の終了コードを保持するsentinelエラー。
//
// why not: cobraはRunEが返したエラーを既定でstderrへ"Error: <message>"の
// 形式で出力してしまうが、本CLIの各コマンドは既にメッセージを自前で
// OutOrStdout/ErrOrStderrへ出力済みである。cobraによる二重出力を避けるため、
// Errorメッセージを空にしたsentinelでExitCodeのみをrun()へ伝え、
// rootCmd.SilenceErrors/SilenceUsageと組み合わせてcobra自身の出力を抑止する。
type cliExitError struct {
	code apperr.ExitCode
}

func (e *cliExitError) Error() string { return "" }

// exitWith はcodeがExitSuccessでない場合にcliExitErrorを返す
// （codeがExitSuccessの場合はnil、すなわちコマンド成功を表す）。
//
// why not: build/info/cacheのRunE内の失敗経路は原因ごとに終了コードを分けず、
// 一律apperr.ExitErrorとする。原因別の終了コードはCI等が機械的に分岐する価値が
// ある場合にだけ導入する方針で、doctorだけは必須ツール不足を
// apperr.ExitDependencyErrorで返し、「実行環境が未整備」を他の失敗と区別できる
// ようにしている。
func exitWith(code apperr.ExitCode) error {
	if code == apperr.ExitSuccess {
		return nil
	}

	return &cliExitError{code: code}
}

// NewRootCmd はmnemonic CLIのルートコマンドを構築する。
// cacheサブコマンドは実環境のキャッシュディレクトリ（cache.Dir、通常$HOME配下）
// を使用する。
func NewRootCmd() *cobra.Command {
	return newRootCmd(cache.Dir)
}

// newRootCmd はcacheディレクトリ解決関数を注入してルートコマンドを構築する。
//
// why: cache clean / cache infoは実際のキャッシュディレクトリを操作・削除する。
// テストが本番同様cache.Dir（$HOME配下）を使うと、開発者がダウンロード済みの
// テンプレートキャッシュを`go test`実行のたびに消去しかねない。コマンドツリー
// 構築時に解決関数を引数として渡す設計にすることで、テストはt.TempDir()を
// 返す関数を注入した独立したコマンドツリーを都度生成でき、newBuildPipeline
// のようなパッケージ変数の差し替えと異なりグローバル状態を共有しないため、
// t.Parallel()配下でも安全に使える。
func newRootCmd(cacheDir func() (string, error)) *cobra.Command {
	var showVersion bool

	root := &cobra.Command{
		Use:           "mnemonic",
		Short:         "吉里吉里ゲームをAndroid APKに変換するCLIツール",
		Long:          "Mnemonic CLI - 吉里吉里ゲームをAndroid APKに変換",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				fmt.Fprintf(cmd.OutOrStdout(), "mnemonic %s\n", version.String()) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

				return nil
			}

			return cmd.Help()
		},
	}

	root.Flags().BoolVar(&showVersion, "version", false, "バージョンを表示する")

	root.AddCommand(newBuildCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newInfoCmd())
	root.AddCommand(newCacheCmd(cacheDir))

	return root
}

// run はCLIを実行し、終了コード（apperr.ExitCode相当の整数値）を返す。
//
// main()とテストの双方から呼び出せるよう、os.Exitを直接呼ばずコードを返す
// 設計にする。
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runWithRoot(NewRootCmd(), args, stdin, stdout, stderr)
}

// runWithRoot はrootを実行し、終了コードを返す。rootの構築方法を呼び出し元に
// 委ねることで、テストがnewRootCmdへ差し替え用のcacheDir解決関数を注入した
// コマンドツリーを実行できるようにする。
func runWithRoot(root *cobra.Command, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.Execute()
	if err == nil {
		return int(apperr.ExitSuccess)
	}

	if exitErr, ok := errors.AsType[*cliExitError](err); ok {
		return int(exitErr.code)
	}

	// cobra自身が返すエラー（未知のフラグ・引数不足等）はExitInvalidInputとして扱う。
	fmt.Fprintln(stderr, err.Error()) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

	return int(apperr.ExitInvalidInput)
}
