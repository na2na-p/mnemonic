package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/na2na-p/mnemonic/internal/apperr"
	"github.com/na2na-p/mnemonic/internal/info"
)

func newInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <path>",
		Short: "ゲーム構成を解析・表示する",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			stat, err := os.Stat(path)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Error: パスが見つかりません: %s\n", path) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

				return exitWith(apperr.ExitError)
			}

			if !stat.IsDir() {
				fmt.Fprintf(cmd.OutOrStdout(), "Error: ディレクトリを指定してください: %s\n", path) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

				return exitWith(apperr.ExitError)
			}

			printGameInfoTable(cmd.OutOrStdout(), info.AnalyzeGame(path))

			return nil
		},
	}
}

func printGameInfoTable(w io.Writer, gi info.GameInfo) {
	fmt.Fprintln(w, "Game Info") //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

	encoding := gi.DetectedEncoding
	if encoding == "" {
		encoding = "N/A"
	}

	fmt.Fprintf(w, "Engine: %s\n", gi.Engine)  //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
	fmt.Fprintf(w, "Encoding: %s\n", encoding) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

	printFileStatsSection(w, "Scripts", gi.Scripts)
	printFileStatsSection(w, "Images", gi.Images)
	printFileStatsSection(w, "Audio", gi.Audio)
	printFileStatsSection(w, "Video", gi.Video)
}

func printFileStatsSection(w io.Writer, label string, stats info.FileStats) {
	fmt.Fprintf(w, "%s: %d files\n", label, stats.Count) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
	if len(stats.Extensions) > 0 {
		fmt.Fprintf(w, "  Extensions: %s\n", strings.Join(stats.Extensions, ", ")) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
	}
	fmt.Fprintf(w, "  Total Size: %s\n", formatSize(stats.TotalSizeBytes)) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
}
