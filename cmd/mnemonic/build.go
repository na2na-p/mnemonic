package main

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/na2na-p/mnemonic/internal/apperr"
	"github.com/na2na-p/mnemonic/internal/converter"
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

// newBuildPipeline はconfigとloggerからbuildRunnerを生成する。テストでの差し替え口
// として、パッケージ変数として保持する（internal/converter.
// ConversionManager.SleepFuncと同じ設計方針）。
var newBuildPipeline = func(config pipeline.Config, log pipeline.Logger) buildRunner {
	p := pipeline.NewBuildPipeline(config)
	p.SetLogger(log)

	return p
}

// openLogFile は--log-fileのパスを追記用に開く。テストで書き込みに失敗する出力先へ
// 差し替えるために、newBuildPipelineと同じくパッケージ変数として保持する。
var openLogFile = func(path string) (io.WriteCloser, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // 利用者が--log-fileで明示したパスへ書き込む用途のため妥当
}

func newBuildCmd() *cobra.Command {
	var (
		output              string
		packageName         string
		appName             string
		keystore            string
		soundfont           string
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
		sourceEncoding      string
	)

	cmd := &cobra.Command{
		Use:   "build <input>",
		Short: "ゲームをAndroid APKにビルドする",
		Long: "ゲームをAndroid APKにビルドする。\n\n" +
			"--keystore指定時、署名パスワードは環境変数 MNEMONIC_KEYSTORE_PASS " +
			"から読み込む（設定されていれば対話入力を求めない）。CI等の非対話" +
			"実行では必ずこの環境変数を設定すること。",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputPath := args[0]
			output = cmp.Or(output, strings.TrimSuffix(inputPath, filepath.Ext(inputPath))+".apk")

			config := pipeline.NewConfig(inputPath, output)
			config.PackageName = packageName
			config.AppName = appName
			config.KeystorePath = keystore
			config.SoundfontPath = soundfont
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
			config.SourceEncoding = sourceEncoding

			var logWriter io.Writer
			if logFile != "" {
				f, err := openLogFile(logFile)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "ログファイルを開けません: %v\n", err) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要

					return exitWith(apperr.ExitInvalidInput)
				}
				defer func() { _ = f.Close() }()
				logWriter = f
			}

			// why not: ロガーの標準エラー出力先を端末にしない。buildコマンドは入力検証の
			// エラーとビルド失敗を自前で標準出力へ書く（root_test.goがこの出力を固定
			// している）ため、ロガーのErrorはログファイルへの記録だけに使い、端末に
			// 同じエラーが二重に出るのを避ける。
			log := logger.New(verboseLevel(verbose), cmd.OutOrStdout(), io.Discard, logWriter)
			// why not: --log-file未指定時は書き込みエラーを確かめない。この確認はログファイルの
			// 障害を知らせるためのもので、ファイルが無ければ残る失敗は標準出力への書き込み
			// だけである。CLIの他の標準出力への書き込みも失敗を扱わない。
			if logWriter != nil {
				defer warnLogWriteError(cmd, log)
			}

			p := newBuildPipeline(config, log)

			if errs := p.Validate(); len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintf(cmd.OutOrStdout(), "Error: %s\n", e) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
					log.Error(e)
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

				log.Info("ビルド完了: " + outputPath)

				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "ビルド失敗: %s\n", result.ErrorMessage) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
			log.Error(result.ErrorMessage)

			return exitWith(apperr.ExitError)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "出力APKパス")
	cmd.Flags().StringVar(&packageName, "package-name", "", "Androidパッケージ名。例: com.example.game（英字始まりの 2 セグメント以上）")
	cmd.Flags().StringVar(&appName, "app-name", "", "アプリ表示名")
	cmd.Flags().StringVar(&keystore, "keystore", "", "署名用キーストア")
	cmd.Flags().StringVar(
		&soundfont, "soundfont", "",
		"MIDI変換に使うサウンドフォント(.sf2/.sf3)のパス（未指定時は既定のシステムパスを探索）",
	)
	cmd.Flags().BoolVar(&skipVideo, "skip-video", false, "動画変換をスキップ")
	cmd.Flags().CountVarP(&verbose, "verbose", "v", "詳細ログ出力")
	cmd.Flags().StringVar(&quality, "quality", pipeline.DefaultQuality, "画像品質プリセット")
	cmd.Flags().BoolVar(&clean, "clean", false, "署名鍵以外のキャッシュ（テンプレート・フォント・プラグイン・SDL2 ソース）をクリアしてからビルドする")
	cmd.Flags().StringVar(&logFile, "log-file", "", "ログファイル出力先")
	cmd.Flags().IntVar(&ffmpegTimeout, "ffmpeg-timeout", pipeline.DefaultFFmpegTimeoutSecs, "FFmpegタイムアウト（秒）")
	cmd.Flags().IntVar(&gradleTimeout, "gradle-timeout", pipeline.DefaultGradleTimeoutSecs, "Gradleタイムアウト（秒）")
	cmd.Flags().StringVar(&templateVersion, "template-version", "", "テンプレートバージョン固定")
	cmd.Flags().IntVar(
		&templateRefreshDays, "template-refresh-days", pipeline.DefaultTemplateRefreshDays, "テンプレートキャッシュ期限（日）",
	)
	cmd.Flags().BoolVar(&templateOffline, "template-offline", false, "オフラインモード")
	// why not: ファイルごとの指定は受け付けない。ファイルと文字コードの対応を渡すには
	// そのための入力形式が別途必要になるため、1つの名前をすべてのテキストアセットに
	// 適用する。
	cmd.Flags().StringVar(
		&sourceEncoding, "source-encoding", "",
		"テキストアセットの変換元文字コード（"+strings.Join(converter.SelectableSourceEncodings, ", ")+
			"、未指定時はファイルごとに自動検出）。指定すると、BOMで始まるファイルと吉里吉里の"+
			"simple crypt形式を除くすべてのテキストアセットを、BOM無しのUTF-8も含めてこの文字コードとして読む",
	)

	return cmd
}

// warnLogWriteError はログへの書き込みに失敗していれば標準エラー出力へ警告する。
//
// why not: 警告の文言でログファイルの失敗と決めつけない。Errはログファイルと
// 標準出力のうち最初に失敗した書き込みのエラーで、その文言が失敗したことを示す。
//
// why not: 書き込みの失敗でbuildコマンドの終了コードを変えない。ビルドの成否は
// APKを作れたかどうかで決まっており、ログの障害で失敗の終了コードを返すと、
// 終了コードで成否を判定する呼び出し側（CI等）に、APKができているのに失敗と
// 判定させてしまう。失敗したビルドの終了コードは元から失敗を示している。
func warnLogWriteError(cmd *cobra.Command, log *logger.BuildLogger) {
	if err := log.Err(); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "警告: %v\n", err) //nolint:errcheck // CLI出力の書き込み失敗は実用上ハンドリング不要
	}
}

// verboseLevel は-vの指定回数をlogger.VerboseLevelへ変換する。-vvより多い指定はDebugとして、
// 負の値はNormalとして扱う。
//
// why not: 負の値をそのままQuietにしない。pflagのカウントフラグは--verbose=-1のような
// 値の指定をstrconv.ParseIntでそのまま受け付ける（spf13/pflag count.go countValue.Set）。
// QuietではBuildLogger.Infoが端末へ書かず、「ビルド完了」の表示が消える。
func verboseLevel(count int) logger.VerboseLevel {
	return logger.VerboseLevel(min(max(count, 0), int(logger.Debug)))
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
