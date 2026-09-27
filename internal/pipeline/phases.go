package pipeline

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/na2na-p/mnemonic/internal/builder"
	"github.com/na2na-p/mnemonic/internal/cache"
	"github.com/na2na-p/mnemonic/internal/converter"
	"github.com/na2na-p/mnemonic/internal/fsutil"
	"github.com/na2na-p/mnemonic/internal/parser"
	"github.com/na2na-p/mnemonic/internal/signer"
)

// ErrGradleAPKMissing はGradleビルドが成功終了コードを返したにもかかわらず、
// 期待される場所にAPKファイルが生成されなかった場合のエラー。
var ErrGradleAPKMissing = errors.New("Gradleビルド後にAPKファイルが見つかりません")

// ErrAssetConversionFailed はCONVERTフェーズで1つ以上のアセットの変換に
// 失敗した場合のエラー。
var ErrAssetConversionFailed = errors.New("アセットの変換に失敗しました")

// ErrInsufficientDiskSpace はEXTRACTフェーズの展開に必要な容量が、一時ディレクトリの
// ファイルシステムの空き容量を超える場合のエラー。
var ErrInsufficientDiskSpace = errors.New("一時ディレクトリの空き容量が不足しています")

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
// 展開の前に、必要な容量が一時ディレクトリの空き容量に収まるかを確認する
// （checkExtractSpace）。
func (b *BuildPipeline) executeExtract(a buildArtifacts) (buildArtifacts, error) {
	extractDir, err := b.newTempDir("mnemonic_extract_")
	if err != nil {
		return a, err
	}
	a.extractDir = extractDir

	archivePaths := []string{b.config.InputPath}
	var embeddedSizes []int64

	if strings.ToLower(filepath.Ext(b.config.InputPath)) == ".exe" {
		extractor, err := parser.NewEmbeddedXP3Extractor(b.config.InputPath)
		if err != nil {
			return a, err
		}

		archivePaths, err = extractor.ExtractAll(extractDir)
		if err != nil {
			return a, err
		}

		embeddedSizes, err = fileSizes(archivePaths)
		if err != nil {
			return a, err
		}
	}

	// why not: アーカイブごとに容量を確認しない。展開結果はすべて同じextractDirに
	// 並んで残るため、1件ずつ同じ空き容量と比べると合計の不足を見逃す。
	archives := make([]*parser.XP3Archive, 0, len(archivePaths))
	planned := make([]int64, 0, len(archivePaths))
	for _, path := range archivePaths {
		archive, err := parser.NewXP3Archive(path)
		if err != nil {
			return a, err
		}
		archives = append(archives, archive)
		planned = append(planned, archive.PlannedOutputSize())
	}

	if err := b.checkExtractSpace(extractDir, requiredExtractSpace(planned, embeddedSizes)); err != nil {
		return a, err
	}

	for _, archive := range archives {
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

// extractFootprintCopies は、展開結果がRunの終了まで一時ディレクトリの
// ファイルシステム上に同時に置かれる数。extractDir、そのcopyTreeによる複製で
// あるconvertDir、convertDirをBUILDフェーズでprojectDirのassetsへ写した複製の
// 3つで、いずれもRun終了時のcleanupTempDirsまで削除されない。3つのディレクトリは
// どれもnewTempDirがos.MkdirTemp("", ...)で作るため、同じos.TempDir()の下、
// つまり容量を確認するextractDirと同じファイルシステムに置かれる。
//
// why not: Gradleのビルド中間生成物とAPK、projectDirへ展開するテンプレートと
// ダウンロードするSDL2のソースは数えない。前者の量はGradleとAndroid Gradle
// Pluginの挙動で、後者の量はテンプレートとSDL2の版で決まり、展開するアーカイブ
// からは導けないため。変換によるサイズの増減も変換前には分からないため数えない。
const extractFootprintCopies = 3

// requiredExtractSpace は、アーカイブごとの展開結果の見積もりplannedと、容量確認の
// 時点で一時ディレクトリへ書き出し済みのファイルのサイズwrittenから、確認時点以降に
// 必要な空き容量を返す。writtenは展開結果と同じくextractFootprintCopies個に
// 複製されるが、1つ目は既に空き容量から差し引かれている。int64を超える場合は
// math.MaxInt64を返す。
func requiredExtractSpace(planned, written []int64) int64 {
	writtenTotal := saturatingSum(written)
	total := saturatingAdd(saturatingSum(planned), writtenTotal)
	if total > math.MaxInt64/extractFootprintCopies {
		return math.MaxInt64
	}

	return total*extractFootprintCopies - writtenTotal
}

// saturatingSum は非負の値valuesの和を返す。和がint64を超える場合はmath.MaxInt64を返す。
func saturatingSum(values []int64) int64 {
	var total int64
	for _, v := range values {
		total = saturatingAdd(total, v)
	}

	return total
}

// saturatingAdd は非負のa、bの和を返す。和がint64を超える場合はmath.MaxInt64を返す。
func saturatingAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}

	return a + b
}

// checkExtractSpace はdirを含むファイルシステムの空き容量がrequiredバイトに
// 満たなければErrInsufficientDiskSpaceを返す。エラー文にはdirの親ディレクトリと、
// 一時ディレクトリを変える方法を示す。
//
// why not: dir自体ではなく親ディレクトリを示す。dirはRunの終了時に削除される
// 使い捨てのディレクトリで、利用者が変えられるのは、newTempDirがdirを作る
// os.TempDir()（Unix系ではTMPDIR、WindowsではTMP、TEMP、USERPROFILEの順に
// 最初に空でないもの）の方であるため。
//
// why not: 空き容量を取得できない場合はビルドを止めず、警告して続ける。確認は
// 容量不足を展開前に知らせるためのもので、対応していないファイルシステムの
// 利用者のビルドまで妨げる理由にはならない。
func (b *BuildPipeline) checkExtractSpace(dir string, required int64) error {
	free, err := b.freeSpace(dir)
	if err != nil {
		b.log().Warning(fmt.Sprintf("一時ディレクトリの空き容量を取得できないため、展開前の容量確認を省略します: %v", err))

		return nil
	}

	if uint64(required) > free { //nolint:gosec // requiredは非負のサイズの和と積から求めた非負値
		return fmt.Errorf("%w: 展開に必要な容量 %s が一時ディレクトリ %s の空き容量 %s を超えています。"+
			"空き容量のあるディレクトリを環境変数TMPDIR（WindowsではTMP）に指定して再実行してください",
			ErrInsufficientDiskSpace, fsutil.FormatSize(required), filepath.Dir(dir), fsutil.FormatSize(int64(min(free, math.MaxInt64))))
	}

	return nil
}

// fileSizes はpathsの各ファイルのサイズを返す。
func fileSizes(paths []string) ([]int64, error) {
	sizes := make([]int64, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("抽出したファイルの情報を取得できません: %w", err)
		}
		sizes = append(sizes, info.Size())
	}

	return sizes, nil
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
		"アセット変換: 成功 %d 件 / 失敗 %d 件 / スキップ %d 件",
		summary.Success, summary.Failed, summary.Skipped,
	))
	logConversionNotes(b.log(), a.extractDir, summary.Results)

	logPreferredSourceSkips(b.log(), a.extractDir, summary)

	// why not: 変換に失敗したアセットがあってもビルドを続けると、素材が欠けた
	// り未変換のまま残ったりしたAPKができ、原因は後段の別エラー（スクリプト
	// 調整の失敗など）として現れて特定しにくい。後処理に進む前に、失敗した
	// 全ファイルとその原因を報告して止める。
	if err := conversionFailureError(a.extractDir, summary); err != nil {
		return a, err
	}

	return a, b.finalizeConvertedTree(a.convertDir, summary, b.newMidiConverter())
}

// maxReportedAssets はアセットごとの報告で1件1行に列挙する最大件数。
// 超えた分は件数だけを示す。
const maxReportedAssets = 20

// skipVideoHint は動画の変換に失敗したときに付ける、--skip-videoの案内。
const skipVideoHint = "動画を変換しない場合は --skip-video を指定してください"

// logConversionNotes は、成功した変換のうちMessageを持つ結果を1件ずつ
// 「sourceDirからの相対パス: Message」としてVerboseで記録する。
//
// why not: resultsの順に記録しない。ConvertDirectoryの結果は並列ワーカーの
// 完了順に並び実行ごとに変わるため、変換元パス順に並べ替えてログを決定的にする。
func logConversionNotes(log Logger, sourceDir string, results []converter.ConversionResult) {
	noted := slices.DeleteFunc(slices.Clone(results), func(result converter.ConversionResult) bool {
		return result.Status != converter.StatusSuccess || result.Message == ""
	})
	slices.SortFunc(noted, func(a, b converter.ConversionResult) int {
		return cmp.Compare(a.SourcePath, b.SourcePath)
	})

	for _, result := range noted {
		path := result.SourcePath
		if rel, err := filepath.Rel(sourceDir, result.SourcePath); err == nil {
			path = rel
		}

		log.Verbose(fmt.Sprintf("%s: %s", path, result.Message))
	}
}

// conversionFailureError はsummaryに失敗した結果が含まれる場合、
// ErrAssetConversionFailedをラップしたエラーを返す。
//
// エラー文は失敗件数の見出しに続けて、変換元パス順に1件1行で
// 「extractDirからの相対パス: 原因」を最大maxReportedAssets件並べる。
// 失敗に動画が含まれる場合は末尾にskipVideoHintを1回だけ付ける。
//
// why not: StatusFailedとの一致だけで判定しない。ConversionManagerは
// StatusSuccess/StatusSkipped以外の状態をすべてFailedとして集計するため、
// 一致判定だとsummary.Failedに数えられた失敗を見逃しうる。
func conversionFailureError(extractDir string, summary converter.ConversionSummary) error {
	var failed []converter.ConversionResult
	for _, result := range summary.Results {
		if result.Status != converter.StatusSuccess && result.Status != converter.StatusSkipped {
			failed = append(failed, result)
		}
	}

	if len(failed) == 0 {
		return nil
	}

	entries, omitted := assetReportEntries(extractDir, failed)

	var report strings.Builder
	for _, entry := range entries {
		report.WriteString("\n  - ")
		report.WriteString(entry)
	}
	if omitted > 0 {
		fmt.Fprintf(&report, "\n  %s", omittedAssetsLine(omitted))
	}

	if hasVideoSource(failed) {
		report.WriteString("\n")
		report.WriteString(skipVideoHint)
	}

	return fmt.Errorf("%w（%d 件）%s", ErrAssetConversionFailed, len(failed), report.String())
}

// logPreferredSourceSkips は出力先が重複し、同名の別の変換元を優先したため
// 変換しなかった変換元を、1件1行で「extractDirからの相対パス: 理由」として
// INFOで報告する。列挙はmaxReportedAssets件までとする。
//
// why not: WARNINGにはしない。出力先には優先した変換元が変換されて入る
// （その変換に失敗すればconversionFailureErrorがビルドを止める）ため出力先が
// 欠けることは無く、利用者が対処する必要が無い。
func logPreferredSourceSkips(logger Logger, extractDir string, summary converter.ConversionSummary) {
	var skipped []converter.ConversionResult
	for _, result := range summary.Results {
		if preferredOtherSource(result) {
			skipped = append(skipped, result)
		}
	}

	entries, omitted := assetReportEntries(extractDir, skipped)
	for _, entry := range entries {
		logger.Info(entry)
	}
	if omitted > 0 {
		logger.Info(omittedAssetsLine(omitted))
	}
}

// preferredOtherSource はresultが、出力先の重複で同名の別の変換元を優先した
// ため変換しなかったスキップかどうかを返す。
//
// why not: DestPathを持つStatusSkippedだけでは判定しない。EncodingConverterは
// 既にターゲットエンコーディングのファイルを、DestPathを持つStatusSkippedとして
// 返す。ConversionManagerが優先するのは出力先と拡張子が一致する唯一の変換元で
// あり、優先されなかった変換元の拡張子は出力先と一致しない。一方EncodingConverterは
// 拡張子を変えないため、拡張子の不一致で両者を区別できる。
func preferredOtherSource(result converter.ConversionResult) bool {
	return result.Status == converter.StatusSkipped &&
		result.DestPath != "" &&
		!strings.EqualFold(filepath.Ext(result.SourcePath), filepath.Ext(result.DestPath))
}

// hasVideoSource はresultsにVideoConverterの対象とする動画を変換元とする結果が
// 含まれるかどうかを返す。
//
// why not: ConversionResultは原因をerrではなくMessageの文字列でしか持たないため、
// errors.Is(ErrVideoConversionFailed)では判定できない。拡張子で判定すれば、
// --skip-videoを指定したときに変換対象から外れるファイルとも一致する。
func hasVideoSource(results []converter.ConversionResult) bool {
	videoExts := converter.NewVideoConverter(0, nil).SupportedExtensions()

	return slices.ContainsFunc(results, func(result converter.ConversionResult) bool {
		return slices.Contains(videoExts, strings.ToLower(filepath.Ext(result.SourcePath)))
	})
}

// assetReportEntries はresultsを変換元パス順に並べ、先頭maxReportedAssets件を
// 「extractDirからの相対パス: Message」の形にしたものと、列挙しなかった件数を返す。
//
// why not: ConvertDirectoryの結果は並列ワーカーの完了順に並び実行ごとに
// 変わるため、そのまま報告すると同じ結果でも報告の並びや省略される項目が揺れる。
// 変換元パス順に並べ替えてから切り詰め、報告を決定的にする。
func assetReportEntries(extractDir string, results []converter.ConversionResult) (entries []string, omitted int) {
	sorted := slices.SortedFunc(slices.Values(results), func(a, b converter.ConversionResult) int {
		return cmp.Compare(a.SourcePath, b.SourcePath)
	})

	shown := sorted[:min(len(sorted), maxReportedAssets)]
	entries = make([]string, 0, len(shown))
	for _, result := range shown {
		entries = append(entries, fmt.Sprintf("%s: %s", relativeToExtractDir(extractDir, result.SourcePath), result.Message))
	}

	return entries, len(sorted) - len(shown)
}

// omittedAssetsLine はassetReportEntriesが列挙しなかった件数を示す行を返す。
func omittedAssetsLine(omitted int) string {
	return fmt.Sprintf("…ほか %d 件", omitted)
}

// relativeToExtractDir はpathをextractDirからの相対パスにして返す。
// 相対パスにできない場合はpathをそのまま返す。
//
// why not: 絶対パスのまま報告しない。extractDirはRunの終了時に削除される
// 一時ディレクトリであり、その絶対パスは利用者が参照できず、行を長くするだけである。
func relativeToExtractDir(extractDir, path string) string {
	rel, err := filepath.Rel(extractDir, path)
	if err != nil {
		return path
	}

	return rel
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
	preparer := builder.NewTemplatePreparer(projectDir, newSDL2SourceCache(b.log()))
	preparer.Warn = b.log().Warning

	return preparer
}

// newSDL2SourceCache はSDL2ソースキャッシュを返す。キャッシュディレクトリを
// 解決できない場合は原因をloggerへ警告してnilを返す。
//
// why not: キャッシュは最適化に過ぎないため、キャッシュディレクトリの解決に
// 失敗しても、ソースをダウンロードできるビルドまで失敗させない。
func newSDL2SourceCache(logger Logger) *builder.SDL2SourceCache {
	dir, err := cache.Dir()
	if err != nil {
		logger.Warning(fmt.Sprintf("キャッシュディレクトリを解決できないため、SDL2ソースのキャッシュを使わずに続行します: %v", err))

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
