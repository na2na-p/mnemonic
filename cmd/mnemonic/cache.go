package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/na2na-p/mnemonic/internal/apperr"
	"github.com/na2na-p/mnemonic/internal/cache"
)

func newCacheCmd(cacheDir func() (string, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "キャッシュ管理",
	}

	cmd.AddCommand(newCacheCleanCmd(cacheDir))
	cmd.AddCommand(newCacheInfoCmd(cacheDir))

	return cmd
}

func newCacheCleanCmd(cacheDir func() (string, error)) *cobra.Command {
	var (
		force        bool
		templateOnly bool
	)

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "キャッシュを削除する",
		RunE: func(cmd *cobra.Command, _ []string) error {
			target := "すべてのキャッシュ"
			if templateOnly {
				target = "テンプレートキャッシュ"
			}

			if !force {
				confirmed := confirm(cmd, fmt.Sprintf("%sを削除しますか? [y/N]: ", target))
				if !confirmed {
					fmt.Fprintln(cmd.OutOrStdout(), "キャンセルしました") //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

					return nil
				}
			}

			dir, err := cacheDir()
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Error: %s\n", err) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

				return exitWith(apperr.ExitError)
			}

			if err := cache.ClearCacheDir(dir, templateOnly); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Error: %s\n", err) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

				return exitWith(apperr.ExitError)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%sを削除しました\n", target) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "確認なしで削除")
	cmd.Flags().BoolVar(&templateOnly, "template-only", false, "テンプレートのみ削除")

	return cmd
}

// confirm はpromptを表示し、標準入力からy/yesの応答を読み取る。
//
// why not: Python版はclick.confirm（typer.confirm）を使い、無効な入力への
// 再プロンプトや既定値（Enterキーのみでの応答）等の高機能な対話UIを提供する。
// CLIテストが送るのは"y\n"/"n\n"の単純な応答のみであり、Go版はテストされる
// 範囲の挙動（y/yesで真、それ以外は偽）のみを素朴な一行読み取りで実装する。
func confirm(cmd *cobra.Command, prompt string) bool {
	fmt.Fprint(cmd.OutOrStdout(), prompt) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

	reader := bufio.NewReader(cmd.InOrStdin())

	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false
	}

	answer := strings.ToLower(strings.TrimSpace(line))

	return answer == "y" || answer == "yes"
}

func newCacheInfoCmd(cacheDir func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "キャッシュ情報を表示する",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := cacheDir()
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Error: %s\n", err) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

				return exitWith(apperr.ExitError)
			}

			ci, err := cache.InfoForDir(dir)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Error: %s\n", err) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

				return exitWith(apperr.ExitError)
			}

			printCacheInfo(cmd.OutOrStdout(), ci)

			return nil
		},
	}
}

func printCacheInfo(w io.Writer, ci cache.Info) {
	fmt.Fprintln(w, "キャッシュ情報")                            //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
	fmt.Fprintf(w, "ディレクトリ: %s\n", ci.Directory)          //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
	fmt.Fprintf(w, "サイズ: %s\n", formatSize(ci.SizeBytes)) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

	if ci.TemplateVersion == nil {
		fmt.Fprintln(w, "テンプレート: なし") //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

		return
	}

	fmt.Fprintf(w, "テンプレートバージョン: %s\n", *ci.TemplateVersion) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

	if ci.TemplateExpiresInDays == nil {
		return
	}

	if *ci.TemplateExpiresInDays > 0 {
		fmt.Fprintf(w, "有効期限: %d日後\n", *ci.TemplateExpiresInDays) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
	} else {
		fmt.Fprintln(w, "有効期限: 期限切れ") //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
	}
}
