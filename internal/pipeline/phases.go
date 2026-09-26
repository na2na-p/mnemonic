package pipeline

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/na2na-p/mnemonic/internal/builder"
	"github.com/na2na-p/mnemonic/internal/cache"
	"github.com/na2na-p/mnemonic/internal/converter"
	"github.com/na2na-p/mnemonic/internal/parser"
	"github.com/na2na-p/mnemonic/internal/signer"
)

// ErrGradleAPKMissing はGradleビルドが成功終了コードを返したにもかかわらず、
// 期待される場所にAPKファイルが生成されなかった場合のエラー。
var ErrGradleAPKMissing = errors.New("Gradleビルド後にAPKファイルが見つかりません")

// ErrAssetConversionFailed はCONVERTフェーズで1つ以上のアセットの変換に
// 失敗した場合のエラー。
var ErrAssetConversionFailed = errors.New("アセットの変換に失敗しました")

// ErrTemplateUnavailable はテンプレートをキャッシュから解決できなかった場合
// （オフラインモードで未取得の場合など）のエラー。
var ErrTemplateUnavailable = errors.New("テンプレートが利用できません。オンラインモードで再実行してください。")

// executeAnalyze はANALYZEフェーズを実行する: 入力ファイルの形式を確認し、
// 必要に応じて暗号化チェックを行う。
func (b *BuildPipeline) executeAnalyze(a buildArtifacts) (buildArtifacts, error) {
	suffix := strings.ToLower(filepath.Ext(b.config.InputPath))

	switch suffix {
	case ".exe":
		extractor, err := parser.NewEmbeddedXP3Extractor(b.config.InputPath)
		if err != nil {
			return a, err
		}

		xp3List, err := extractor.FindEmbeddedXP3()
		if err != nil {
			return a, err
		}
		if len(xp3List) == 0 {
			return a, fmt.Errorf("EXEファイル内にXP3アーカイブが見つかりません: %s", b.config.InputPath)
		}
	case ".xp3":
		checker := parser.NewXP3EncryptionChecker(b.config.InputPath)
		if err := checker.RaiseIfEncrypted(); err != nil {
			return a, err
		}
	}

	return a, nil
}

// executeExtract はEXTRACTフェーズを実行する: XP3アーカイブを展開し、
// ゲーム構造を解析する。EXEファイルの場合は埋め込みXP3を抽出してから展開する。
func (b *BuildPipeline) executeExtract(a buildArtifacts) (buildArtifacts, error) {
	extractDir, err := b.newTempDir("mnemonic_extract_")
	if err != nil {
		return a, err
	}
	a.extractDir = extractDir

	suffix := strings.ToLower(filepath.Ext(b.config.InputPath))

	if suffix == ".exe" {
		extractor, err := parser.NewEmbeddedXP3Extractor(b.config.InputPath)
		if err != nil {
			return a, err
		}

		xp3Files, err := extractor.ExtractAll(extractDir)
		if err != nil {
			return a, err
		}

		for _, xp3File := range xp3Files {
			archive, err := parser.NewXP3Archive(xp3File)
			if err != nil {
				return a, err
			}
			if err := archive.ExtractAll(extractDir); err != nil {
				return a, err
			}
		}
	} else {
		archive, err := parser.NewXP3Archive(b.config.InputPath)
		if err != nil {
			return a, err
		}
		if err := archive.ExtractAll(extractDir); err != nil {
			return a, err
		}
	}

	detector, err := parser.NewGameDetector(extractDir)
	if err != nil {
		return a, err
	}

	structure, err := detector.Detect()
	if err != nil {
		return a, err
	}
	a.gameStructure = &structure

	return a, nil
}

// executeConvert はCONVERTフェーズを実行する: 抽出されたアセットをAndroid
// 互換形式に変換する。まず全ファイルをコピーし（ゲームコアファイルを含む）、
// その後変換対象ファイルを変換（上書き）する。
func (b *BuildPipeline) executeConvert(a buildArtifacts) (buildArtifacts, error) {
	if a.extractDir == "" {
		return a, errors.New("抽出フェーズが完了していません")
	}

	convertDir, err := b.newTempDir("mnemonic_convert_")
	if err != nil {
		return a, err
	}
	a.convertDir = convertDir

	if err := copyTree(a.extractDir, a.convertDir); err != nil {
		return a, err
	}

	converters := []converter.Converter{
		converter.NewEncodingConverter("", b.config.SourceEncoding),
		converter.NewImageConverter(),
	}
	if !b.config.SkipVideo {
		timeout := time.Duration(b.config.FFmpegTimeoutSeconds) * time.Second
		converters = append(converters, converter.NewVideoConverter(timeout, nil))
	}

	manager := converter.NewConversionManager(converters, nil, 0, nil)

	// why not: converter.ConvertDirectoryはsourceDir（ここではextractDir）が
	// 存在しない場合にerrorを返す。extractDirは直前のEXTRACTフェーズが
	// os.MkdirTempで必ず作成しているため、通常の実行経路でこのエラー分岐に
	// 到達することはない。到達するとすればextractDirが実行中に消失した異常系
	// であり、そのケースを空サマリーで握りつぶさず、CONVERTフェーズの失敗
	// として明示的に報告する（「エラーはerrorとして呼び出し元へ伝播する」
	// という他フェーズと同じ契約に沿うほうが、黙って空の変換結果を返すより
	// 安全なため）。
	summary, err := manager.ConvertDirectory(a.extractDir, a.convertDir, true)
	if err != nil {
		return a, fmt.Errorf("アセット変換に失敗しました: %w", err)
	}

	b.log().Info(fmt.Sprintf(
		"アセット変換: 成功 %d件 / 失敗 %d件 / スキップ %d件",
		summary.Success, summary.Failed, summary.Skipped,
	))

	// why not: 変換に失敗したアセットがあってもビルドを続けると、素材が欠けた
	// り未変換のまま残ったりしたAPKができ、原因は後段の別エラー（スクリプト
	// 調整の失敗など）として現れて特定しにくい。後処理に進む前に、失敗した
	// 全ファイルとその原因を報告して止める。
	if err := conversionFailureError(summary); err != nil {
		return a, err
	}

	return a, b.finalizeConvertedTree(a.convertDir, summary, b.newMidiConverter())
}

// conversionFailureError はsummaryに失敗した結果が含まれる場合、
// ErrAssetConversionFailedをラップしたエラーを返す。
//
// why not: StatusFailedとの一致だけで判定しない。ConversionManagerは
// StatusSuccess/StatusSkipped以外の状態をすべてFailedとして集計するため、
// 一致判定だとsummary.Failedに数えられた失敗を見逃しうる。
func conversionFailureError(summary converter.ConversionSummary) error {
	var failed []converter.ConversionResult
	for _, result := range summary.Results {
		if result.Status != converter.StatusSuccess && result.Status != converter.StatusSkipped {
			failed = append(failed, result)
		}
	}

	if len(failed) == 0 {
		return nil
	}

	// why not: ConvertDirectoryの結果は並列ワーカーの完了順に並び実行ごとに
	// 変わるため、そのまま報告すると同じ失敗でもエラー文の並びが揺れる。
	// 変換元パス順に並べ替えて報告を決定的にする。
	slices.SortFunc(failed, func(a, b converter.ConversionResult) int {
		return cmp.Compare(a.SourcePath, b.SourcePath)
	})

	failures := make([]string, 0, len(failed))
	for _, result := range failed {
		failures = append(failures, fmt.Sprintf("%s: %s", result.SourcePath, result.Message))
	}

	return fmt.Errorf("%w: %s", ErrAssetConversionFailed, strings.Join(failures, " / "))
}

// finalizeConvertedTree はアセット変換済みのdirectoryへ後処理を順に適用する。
//
// midiConverterを引数で受け取るのは、テストが実プロセスのfluidsynthに触れずに
// 各ステップの順序を検証できるようにするため（converter.MidiConverterの
// CommandRunner注入口を使う）。summaryも同じ理由で引数として受け取る
// （removeStaleVideoSourceFilesは実プロセスのffmpegに触れない純粋な後処理
// だが、実行順序の検証にはexecuteConvertを経由せずこの関数を直接呼べる
// 必要がある）。
//
// why not: 各ステップの順序を入れ替えてはならない。
//   - removeStaleVideoSourceFilesはnormalizeCriticalFilenames（末尾の
//     小文字化）より前でなければならない。normalizeCriticalFilenamesが先に
//     走ると、大文字拡張子の旧ファイル(例: OP.WMV)が小文字にリネームされ、
//     summaryが記録した元のケース(.WMV)でパスを再構築しても既にリネーム済み
//     のため削除に失敗し(best-effortで握りつぶされ)、旧ファイルが永続的に
//     残ってしまう。
//   - convertMidiFilesUsingの呼び出しをadjustScriptsより後ろへ動かしては
//     ならない。ScriptAdjusterは.mid/.midi参照を無条件に.oggへ書き換えるため、
//     MIDI変換が先に失敗してCONVERTフェーズを中断できないと、実体の無い.oggを
//     指すスクリプトのままAPKが完成し、BGMが無音になる（実機で確認済み）。
//
// これらの順序不変条件はphases_internal_test.goのテストで固定している。
// 順序は MIDI変換→プラグインdllディレクトリ削除→polyfillコピー→
// スクリプト調整→ファイル名正規化 とする（動画の残留ファイル削除は
// この一連の処理に独自に追加したステップ）。
func (b *BuildPipeline) finalizeConvertedTree(
	directory string,
	summary converter.ConversionSummary,
	midiConverter *converter.MidiConverter,
) error {
	removeStaleVideoSourceFiles(summary)

	if err := convertMidiFilesUsing(directory, midiConverter, b.log()); err != nil {
		return fmt.Errorf("MIDI変換に失敗しました: %w", err)
	}

	// プラグインディレクトリを削除（Windows DLLはAndroidで使用不可。
	// extrans/wuvorbisはBUILDフェーズでjniLibs経由の.soとして別途配置される）
	if err := b.removePluginDirectory(directory); err != nil {
		return err
	}

	// krkrsdl2 polyfillファイルをコピー
	if err := b.copyPolyfillFiles(directory); err != nil {
		return err
	}

	// スクリプト調整（startup.tjsへのpolyfill読み込み追加、loadplugin書き換え等）
	if err := b.adjustScripts(directory); err != nil {
		return err
	}

	// Androidのファイルシステムは大文字小文字を区別するため、重要な
	// ファイル名を正規化（小文字化）する。変換処理の後に行う必要がある
	// （変換が元のケースでファイルを作成するため）。
	return b.normalizeCriticalFilenames(directory)
}

// executeBuild はBUILDフェーズを実行する: Gradleビルドを使用してAPKを
// 生成する。テンプレートを展開し、ゲームファイルをassetsに配置してビルドする。
func (b *BuildPipeline) executeBuild(a buildArtifacts) (buildArtifacts, error) {
	if a.convertDir == "" {
		return a, errors.New("変換フェーズが完了していません")
	}

	baseName := strings.TrimSuffix(filepath.Base(b.config.InputPath), filepath.Ext(b.config.InputPath))
	if a.gameStructure != nil && a.gameStructure.Title != "" {
		baseName = a.gameStructure.Title
	}

	packageName, err := b.derivePackageName(b.config.PackageName, baseName)
	if err != nil {
		return a, err
	}

	appName := cmp.Or(b.config.AppName, baseName)

	projectDir, err := b.newTempDir("mnemonic_project_")
	if err != nil {
		return a, err
	}
	a.projectDir = projectDir

	templatePath, err := b.resolveTemplate()
	if err != nil {
		return a, err
	}

	if err := extractTemplateZip(templatePath, projectDir); err != nil {
		return a, err
	}

	plugins := b.fetchPlugins()

	preparer := b.newTemplatePreparer(projectDir)
	if err := preparer.Prepare(packageName, appName, a.convertDir, b.findGameIcon(a.extractDir), plugins); err != nil {
		return a, err
	}

	gradleTimeout := time.Duration(b.config.GradleTimeoutSeconds) * time.Second

	gradleBuilder, err := builder.NewGradleBuilder(projectDir, gradleTimeout, nil)
	if err != nil {
		return a, err
	}

	result, err := gradleBuilder.Build("release")
	if err != nil {
		return a, err
	}
	if !result.Success || result.APKPath == nil {
		return a, fmt.Errorf("%w: %s", ErrGradleAPKMissing, result.OutputLog)
	}

	a.unsignedAPK = *result.APKPath

	return a, nil
}

// newTemplatePreparer はprojectDirのテンプレートを準備するTemplatePreparerを返す。
// SDL2ソースキャッシュの復元・保存の失敗はパイプラインのLoggerへ警告として報告する。
func (b *BuildPipeline) newTemplatePreparer(projectDir string) *builder.TemplatePreparer {
	preparer := builder.NewTemplatePreparer(projectDir, newSDL2SourceCache())
	preparer.Warn = b.log().Warning

	return preparer
}

// newSDL2SourceCache はSDL2ソースキャッシュを返す。キャッシュディレクトリを
// 解決できない場合はnilを返す。
//
// why not: キャッシュは最適化に過ぎないため、キャッシュディレクトリの解決に
// 失敗しても、ソースをダウンロードできるビルドまで失敗させない。
func newSDL2SourceCache() *builder.SDL2SourceCache {
	dir, err := cache.Dir()
	if err != nil {
		return nil
	}

	return builder.NewSDL2SourceCache(dir)
}

// resolveTemplate はキャッシュ済みテンプレートを解決する。キャッシュが無く
// オフラインモードでない場合はダウンロードしてキャッシュへ保存する。
func (b *BuildPipeline) resolveTemplate() (string, error) {
	cacheManager := builder.NewDefaultCacheManager()
	templateCache := builder.NewTemplateCache(cacheManager, b.config.TemplateRefreshDays)

	templatePath, ok := templateCache.GetCachedTemplate(b.config.TemplateVersion)

	if !ok && !b.config.TemplateOffline {
		// why not: ダウンロードしたZIPはSaveTemplateがキャッシュへコピーする中間物に
		// すぎない。キャッシュ配下へ直接落とすとコピー先と同一ファイルになり、他の
		// 永続ディレクトリへ落とすとcache cleanの届かない重複が残るため、Run終了時に
		// 消える一時ディレクトリへ置く。
		downloadDir, err := b.newTempDir("mnemonic_template_")
		if err != nil {
			return "", err
		}

		downloader := builder.NewTemplateDownloader(downloadDir, nil)

		downloaded, err := downloader.Download(b.config.TemplateVersion)
		if err != nil {
			return "", err
		}

		version := "latest"
		if b.config.TemplateVersion != nil {
			version = *b.config.TemplateVersion
		}

		if _, err := templateCache.SaveTemplate(downloaded, version); err != nil {
			return "", err
		}

		templatePath, ok = templateCache.GetCachedTemplate(b.config.TemplateVersion)
	}

	if !ok {
		return "", ErrTemplateUnavailable
	}

	return templatePath, nil
}

// executeSign はSIGNフェーズを実行する: ビルドされたAPKにzipalignを適用し、
// キーストア指定時は署名鍵で、未指定時はデバッグ鍵で署名を行う。
func (b *BuildPipeline) executeSign(a buildArtifacts) (buildArtifacts, error) {
	if a.unsignedAPK == "" {
		return a, errors.New("ビルドフェーズが完了していません")
	}

	outputPath := b.config.OutputPath
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return a, fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	zipaligner := signer.NewDefaultZipalignRunner(nil)
	alignedAPK := withSuffix(outputPath, ".aligned.apk")

	if _, err := zipaligner.Align(a.unsignedAPK, alignedAPK); err != nil {
		return a, err
	}

	keystoreConfig, err := b.resolveKeystoreConfig()
	if err != nil {
		return a, err
	}

	if err := copyFile(alignedAPK, outputPath); err != nil {
		return a, fmt.Errorf("署名前APKのコピーに失敗しました: %w", err)
	}

	apkSigner := signer.NewDefaultApkSignerRunner(nil)
	if _, err := apkSigner.Sign(outputPath, keystoreConfig); err != nil {
		return a, err
	}

	_ = os.Remove(alignedAPK)

	return a, nil
}

// resolveKeystoreConfig は署名に使うキーストア設定を決定する。
//
// KeystorePath指定時は環境変数MNEMONIC_KEYSTORE_PASS（未設定なら対話的
// 入力）からパスワードを取得する。未指定時はデバッグ用キーストアを生成する
// （PR6の申し送り: 非対話用途は環境変数、対話用途は端末入力という現行方針を
// そのままCLI経由の実行にも適用する）。
func (b *BuildPipeline) resolveKeystoreConfig() (signer.KeystoreConfig, error) {
	if b.config.KeystorePath == "" {
		debugKeystore, err := b.createDebugKeystore()
		if err != nil {
			return signer.KeystoreConfig{}, err
		}

		return signer.KeystoreConfig{
			KeystorePath:     debugKeystore,
			KeyAlias:         "debug",
			KeystorePassword: "android",
		}, nil
	}

	provider := signer.DefaultPasswordProvider{}

	password, ok := provider.GetPasswordFromEnv("")
	if !ok {
		pw, err := provider.GetPassword("")
		if err != nil {
			return signer.KeystoreConfig{}, err
		}
		password = pw
	}

	return signer.KeystoreConfig{
		KeystorePath:     b.config.KeystorePath,
		KeyAlias:         "key",
		KeystorePassword: password,
	}, nil
}
