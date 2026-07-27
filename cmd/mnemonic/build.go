package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/na2na-p/mnemonic/internal/apperr"
	"github.com/na2na-p/mnemonic/internal/logger"
	"github.com/na2na-p/mnemonic/internal/pipeline"
)

// buildRunner はbuildコマンドが必要とするBuildPipelineの振る舞いを表す
// インターフェース。テストで実際のGradle/ffmpeg/keytool実行を伴わない
// スタブに差し替えるために定義する。
type buildRunner interface {
	Validate() []string
	Run(pipeline.ProgressCallback) pipeline.Result
}

// newBuildPipeline はconfigからbuildRunnerを生成する。テストでの差し替え口
// として、パッケージ変数として保持する（internal/converter.
// ConversionManager.SleepFuncと同じ設計方針）。
var newBuildPipeline = func(config pipeline.Config) buildRunner {
	return pipeline.NewBuildPipeline(config)
}

func newBuildCmd() *cobra.Command {
	var (
		output              string
		packageName         string
		appName             string
		keystore            string
		skipVideo           bool
		verbose             int
		quality             string
		clean               bool
		logFile             string
		ffmpegTimeout       int
		gradleTimeout       int
		templateVersion     string
		templateRefreshDays int
		templateOffline     bool
	)

	cmd := &cobra.Command{
		Use:   "build <input>",
		Short: "ゲームをAndroid APKにビルドする",
		Long: "ゲームをAndroid APKにビルドする。\n\n" +
			"--keystore指定時、署名パスワードは環境変数 MNEMONIC_KEYSTORE_PASS " +
			"から読み込む（設定されていれば対話入力を求めない）。CI等の非対話 " +
			"実行では必ずこの環境変数を設定すること。",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputPath := args[0]
			if output == "" {
				output = strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".apk"
			}

			config := pipeline.NewConfig(inputPath, output)
			config.PackageName = packageName
			config.AppName = appName
			config.KeystorePath = keystore
			config.SkipVideo = skipVideo
			config.Quality = quality
			config.CleanCache = clean
			config.VerboseLevel = verbose
			config.LogFile = logFile
			config.FFmpegTimeoutSeconds = ffmpegTimeout
			config.GradleTimeoutSeconds = gradleTimeout

			if templateVersion != "" {
				config.TemplateVersion = &templateVersion
			}

			config.TemplateRefreshDays = templateRefreshDays
			config.TemplateOffline = templateOffline

			p := newBuildPipeline(config)

			if errs := p.Validate(); len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintf(cmd.OutOrStdout(), "Error: %s\n", e) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
				}

				return exitWith(apperr.ExitError)
			}

			var progressCallback pipeline.ProgressCallback
			if verbose > 0 {
				progressCallback = buildProgressCallback(cmd)
			}

			result := p.Run(progressCallback)

			if result.Success {
				// why not: 正常系のBuildPipeline.Runは常にOutputPathを設定するが、
				// buildRunnerはテストではstubBuildRunnerに差し替え可能であり、
				// Success:trueかつOutputPath:nilという（本来あり得ない）組み合わせを
				// 返すスタブが来てもpanicしないよう防御的にnilチェックする。
				outputPath := ""
				if result.OutputPath != nil {
					outputPath = *result.OutputPath
				}

				fmt.Fprintf(cmd.OutOrStdout(), "ビルド完了: %s\n", outputPath) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "ビルド失敗: %s\n", result.ErrorMessage) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

			return exitWith(apperr.ExitError)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "出力APKパス")
	cmd.Flags().StringVar(&packageName, "package-name", "", "Androidパッケージ名")
	cmd.Flags().StringVar(&appName, "app-name", "", "アプリ表示名")
	cmd.Flags().StringVar(&keystore, "keystore", "", "署名用キーストア")
	cmd.Flags().BoolVar(&skipVideo, "skip-video", false, "動画変換をスキップ")
	cmd.Flags().CountVarP(&verbose, "verbose", "v", "詳細ログ出力")
	cmd.Flags().StringVar(&quality, "quality", pipeline.DefaultQuality, "画像品質プリセット")
	cmd.Flags().BoolVar(&clean, "clean", false, "キャッシュをクリア")
	cmd.Flags().StringVar(&logFile, "log-file", "", "ログファイル出力先")
	cmd.Flags().IntVar(&ffmpegTimeout, "ffmpeg-timeout", pipeline.DefaultFFmpegTimeoutSecs, "FFmpegタイムアウト（秒）")
	cmd.Flags().IntVar(&gradleTimeout, "gradle-timeout", pipeline.DefaultGradleTimeoutSecs, "Gradleタイムアウト（秒）")
	cmd.Flags().StringVar(&templateVersion, "template-version", "", "テンプレートバージョン固定")
	cmd.Flags().IntVar(
		&templateRefreshDays, "template-refresh-days", pipeline.DefaultTemplateRefreshDays, "テンプレートキャッシュ期限（日）",
	)
	cmd.Flags().BoolVar(&templateOffline, "template-offline", false, "オフラインモード")

	return cmd
}

// buildProgressCallback はinternal/logger.ProgressDisplayへ委譲する
// pipeline.ProgressCallbackを構築する（-v指定時のみ使用）。
//
// Config.Runは各フェーズについて必ずCurrent=0（開始）→Current=Total
// （完了）の順で2回コールバックを呼ぶため、Current==0をStart、それ以外を
// Finish(成功)として扱えばよい（失敗時はRunがそのフェーズのFinish相当の
// コールバックを呼ばないため、ここでもFinish(false, ...)は呼ばれない）。
func buildProgressCallback(cmd *cobra.Command) pipeline.ProgressCallback {
	display := logger.NewConsoleProgressDisplayWithWriter(false, true, cmd.OutOrStdout())

	return func(p pipeline.Progress) {
		if p.Current == 0 {
			display.Start(p.Phase, p.Total)

			return
		}

		display.Finish(true, "")
	}
}
