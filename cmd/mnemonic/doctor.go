package main

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/na2na-p/mnemonic/internal/apperr"
	"github.com/na2na-p/mnemonic/internal/doctor"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "依存ツールをチェックする",
		RunE: func(cmd *cobra.Command, _ []string) error {
			results := doctor.CheckAllDependencies()

			printDependencyTable(cmd.OutOrStdout(), results)

			hasMissingRequired := false
			for _, r := range results {
				if r.Required && !r.Found {
					hasMissingRequired = true
				}
			}

			if hasMissingRequired {
				fmt.Fprintln(cmd.OutOrStdout())                       //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
				fmt.Fprintln(cmd.OutOrStdout(), "エラー: 必須ツールが不足しています") //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

				return exitWith(apperr.ExitError)
			}

			fmt.Fprintln(cmd.OutOrStdout())                     //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
			fmt.Fprintln(cmd.OutOrStdout(), "すべての必須ツールが利用可能です") //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

			return nil
		},
	}
}

// printDependencyTable は依存ツールチェック結果を表形式で出力する。
//
// why not: Python版はrich.Table（ANSI装飾・罫線付き）で描画するが、CLI
// テストがANSIエスケープの有無に依存しないよう、Go版はtext/tabwriterによる
// プレーンテキストの整形のみを行う。
func printDependencyTable(w io.Writer, results []doctor.CheckResult) {
	fmt.Fprintln(w, "依存ツールチェック結果") //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ステータス\tツール名\tバージョン\t必須\tメッセージ") //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

	for _, r := range results {
		status := "OK"
		if !r.Found {
			status = "NG"
		}

		required := "オプション"
		if r.Required {
			required = "必須"
		}

		version := r.Version
		if version == "" {
			version = "-"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", status, r.Name, version, required, r.Message) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
	}

	_ = tw.Flush()
}
