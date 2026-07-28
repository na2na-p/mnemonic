package pipeline

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/na2na-p/mnemonic/internal/builder"
	"github.com/na2na-p/mnemonic/internal/converter"
	"github.com/na2na-p/mnemonic/internal/parser"
	"github.com/na2na-p/mnemonic/internal/resources"
)

// normalizeCriticalFilenames はdirectory配下の全ファイル名を正規化
// （小文字化）する。
//
// Windowsはファイル名の大文字小文字を区別しないが、Androidは区別する。
// 変換フェーズの最後（他の変換処理が元のケースでファイルを作成した後）に
// 実行する必要がある。
func (b *BuildPipeline) normalizeCriticalFilenames(directory string) error {
	var allPaths []string

	err := filepath.WalkDir(directory, func(path string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == directory {
			return nil
		}
		allPaths = append(allPaths, path)

		return nil
	})
	if err != nil {
		return fmt.Errorf("ファイル名正規化のための走査に失敗しました: %w", err)
	}

	// 深い階層から処理する（Python版のkey=lambda p: len(p.parts), reverse=True
	// と同じ方針）。
	sort.Slice(allPaths, func(i, j int) bool {
		return len(strings.Split(allPaths[i], string(filepath.Separator))) >
			len(strings.Split(allPaths[j], string(filepath.Separator)))
	})

	for _, path := range allPaths {
		info, statErr := os.Stat(path)
		if statErr != nil || info.IsDir() {
			continue
		}

		name := filepath.Base(path)
		lower := strings.ToLower(name)
		if name == lower {
			continue
		}

		newPath := filepath.Join(filepath.Dir(path), lower)
		if fileExists(newPath) {
			continue
		}

		if err := os.Rename(path, newPath); err != nil {
			return fmt.Errorf("ファイル名の正規化に失敗しました: %s: %w", path, err)
		}
	}

	return nil
}

// removePluginDirectory はdirectory直下のプラグインディレクトリを削除する。
//
// Windows用の.dllプラグインはAndroidで使用できないため、プラグイン
// ディレクトリを削除する。krkrsdl2は多くの機能をビルトインで持っているため
// プラグインDLLは不要（Goal 7で導入されるextrans/wuvorbisはjniLibs経由で
// 別途配置されるため対象外）。
var pluginDirNames = []string{"plugin", "Plugin", "PLUGIN", "Plugins", "plugins", "PLUGINS"}

func (b *BuildPipeline) removePluginDirectory(directory string) error {
	for _, name := range pluginDirNames {
		pluginDir := filepath.Join(directory, name)

		info, err := os.Stat(pluginDir)
		if err != nil || !info.IsDir() {
			continue
		}

		if err := os.RemoveAll(pluginDir); err != nil {
			return fmt.Errorf("プラグインディレクトリの削除に失敗しました: %s: %w", pluginDir, err)
		}
	}

	return nil
}

// removeStaleVideoSourceFiles はVideoConverterが拡張子を変更して変換に成功した
// ファイルについて、copyTree(実行済みのexecuteConvert冒頭)がconvertDirへ
// 複製した変換前拡張子の生ファイルを削除する。
//
// why: VideoConverter.GetOutputExtensionは常に".mpg"を返すため、.wmv/.avi/
// .mpeg入力はConversionManagerによって新しい拡張子のファイルとして書き出され、
// copyTreeが複製した旧拡張子ファイルはconvertDir内に残ったままになる。
// ScriptAdjusterは動画参照を無条件に.mpgへ書き換えるため、旧ファイルを
// 残しても参照は解決できる（MIDIがT-220で踏んだ「実体の無いファイルを指す」
// 不具合とは異なる）。削除失敗をエラーにしないのは、convertMidiFileListの
// os.Remove(midiFile)と同じ理由: 変換自体は成功しスクリプト参照も解決できる
// ため、残留は死蔵アセットとしてAPKサイズが増えるだけで実害が無い。
func removeStaleVideoSourceFiles(summary converter.ConversionSummary) {
	videoExts := converter.NewVideoConverter(0, nil).SupportedExtensions()

	for _, result := range summary.Results {
		if result.Status != converter.StatusSuccess {
			continue
		}

		sourceExt := filepath.Ext(result.SourcePath)
		if strings.EqualFold(sourceExt, filepath.Ext(result.DestPath)) {
			continue
		}
		if !slices.Contains(videoExts, strings.ToLower(sourceExt)) {
			continue
		}

		_ = os.Remove(withSuffix(result.DestPath, sourceExt))
	}
}

// adjustScripts はdirectory配下の全.ks/.tjsファイルにScriptAdjusterを適用する。
//
// why not: Python版は大文字小文字のバリエーション6パターン
// （.ks/.KS/.Ks/.tjs/.TJS/.Tjs）ごとにrglobを繰り返すが、これはLinuxの
// 大文字小文字を区別するファイルシステムでも確実に拾うための工夫であり、
// 大文字小文字を区別しないファイルシステム（例: macOS既定）では同一
// ファイルに複数回ヒットしうる。Go版は1回のWalkDirで拡張子を
// strings.EqualFoldで比較するため、大文字小文字を区別するファイルシステム
// でもPython版と同じ集合を1回の走査で漏れなく捕捉でき、かつ
// 大文字小文字を区別しないファイルシステムでの二重適用も起こらない。
func (b *BuildPipeline) adjustScripts(directory string) error {
	adjuster := converter.NewScriptAdjuster(nil, true)

	return filepath.WalkDir(directory, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".ks" && ext != ".tjs" {
			return nil
		}

		if _, err := adjuster.Convert(path, path); err != nil {
			return fmt.Errorf("スクリプトの調整に失敗しました: %s: %w", path, err)
		}

		return nil
	})
}

// copyPolyfillFiles はkrkrsdl2/kag3 polyfillファイルをdirectory/system/へ
// コピーする。これによりMenuItemやKAGParserなどの不足クラスが提供される。
// また、Koruriフォントをsystem/font.ttfとしてコピーする。
func (b *BuildPipeline) copyPolyfillFiles(directory string) error {
	return copyPolyfillFilesUsing(directory, builder.NewFontFetcher("", nil))
}

func copyPolyfillFilesUsing(directory string, fontFetcher *builder.FontFetcher) error {
	systemDir := filepath.Join(directory, "system")
	if err := os.MkdirAll(systemDir, 0o750); err != nil {
		return fmt.Errorf("systemディレクトリの作成に失敗しました: %w", err)
	}

	for _, name := range resources.SystemPolyfillFiles {
		data, err := resources.SystemPolyfillFS.ReadFile("system_polyfill/" + name)
		if err != nil {
			// why: リソースは埋め込みFSでビルド時に固定されるため通常発生しない。
			// Python版のimportlib.resources参照が(FileNotFoundError, TypeError)を
			// 握りつぶしてcontinueする方針を踏襲し、見つからなければスキップする。
			continue
		}

		if err := os.WriteFile(filepath.Join(systemDir, name), data, 0o644); err != nil { //nolint:gosec // ビルド成果物の出力用途のため妥当な権限
			return fmt.Errorf("polyfillファイルの書き込みに失敗しました: %s: %w", name, err)
		}
	}

	// フォント取得の失敗はビルドを継続する（Python版のFontDownloadError/
	// OSErrorをログ警告のみで握りつぶす方針を踏襲。本パッケージにロガーの
	// 注入口が無いため、ここでは静かに無視する）。
	_ = copyFontFile(systemDir, fontFetcher)

	return nil
}

// copyFontFile はKoruriフォントをsystemDir/font.ttfとしてコピーする。
// PolyfillInitialize.tjsはsystem/font.ttfまたはsystem/font.otfを探して
// デフォルトフォントとして設定する。
func copyFontFile(systemDir string, fontFetcher *builder.FontFetcher) error {
	fontDest := filepath.Join(systemDir, "font.ttf")
	if fileExists(fontDest) {
		return nil
	}

	fontInfo, err := fontFetcher.GetFont()
	if err != nil {
		return nil //nolint:nilerr // フォント取得失敗はビルドを継続させる意図的な握りつぶし（copyPolyfillFilesUsingのコメント参照）
	}

	return copyFile(fontInfo.Path, fontDest)
}

// MIDI変換に関するセンチネルエラー群。
var (
	// ErrMidiConversionUnavailable はMIDIアセットを含むゲームに対し、変換に
	// 必要なFluidSynthまたはサウンドフォントが利用できない場合のエラー。
	ErrMidiConversionUnavailable = errors.New("MIDI変換に必要な環境が利用できません")
	// ErrMidiConversionFailed は1つ以上のMIDIファイルの変換に失敗した場合のエラー。
	ErrMidiConversionFailed = errors.New("MIDIファイルの変換に失敗しました")
)

// midiRequirementGuide はMIDI変換の前提条件を満たせなかった場合に、
// 利用者が復旧するための手順を示す案内文を組み立てる。
//
// why not: macOSの案内をHomebrewのインストールコマンドだけで終わらせない。
// `brew install fluid-synth`はfluidsynthコマンドを提供するがサウンドフォントは
// 同梱せず、既定の探索先はいずれもLinuxの絶対パスであるため、コマンドだけを
// 入れた利用者は同じエラーに再突入して手詰まりになる。サウンドフォントの
// 別途入手と--soundfontでの指定まで案内する必要がある。
//
// why not: 定数ではなく関数にしているのは、既定の探索先パスを文面へ直書き
// すると converter 側の定義とずれても誰も気付けないため。SSOTである
// converterのパッケージ変数から都度組み立てる。
func midiRequirementGuide() string {
	return "ゲーム内にMIDIアセット(.mid/.midi)が含まれています。" +
		"krkrsdl2はMIDIを再生できず、スクリプト内の参照は必ず.oggへ書き換えられるため、" +
		"MIDI変換にはFluidSynthとサウンドフォントの両方が必須です。" +
		"インストール例: Debian/Ubuntu系は `apt-get install fluidsynth fluid-soundfont-gm`。" +
		"macOSは `brew install fluid-synth` でコマンドを導入したうえで、" +
		"サウンドフォント(.sf2/.sf3、例: FluidR3_GM)を別途入手し、" +
		"`--soundfont <パス>` で指定してください（既定の探索先である " +
		converter.MuseScoreSoundfontPath + " または " + converter.FluidR3SoundfontPath +
		" に配置しても構いません）。" +
		"なお--skip-videoのようなスキップ指定はMIDIには適用できません" +
		"（変換を省略するとBGMが一切鳴らないAPKが出来上がるため）。"
}

// newMidiConverter は設定値からMIDI変換器を構築する。
// Config.SoundfontPathが空文字列の場合、NewMidiConverterが
// converter.GetDefaultSoundfontPathによる既定の解決を行う。
func (b *BuildPipeline) newMidiConverter() *converter.MidiConverter {
	timeout := time.Duration(b.config.FFmpegTimeoutSeconds) * time.Second

	return converter.NewMidiConverter(b.config.SoundfontPath, 0, "", 0, timeout, nil)
}

// convertMidiFilesUsing はdirectory配下のMIDIファイルをOGG Vorbis形式に変換する。
//
// krkrsdl2はMIDI再生未対応のため、MIDIファイルをOGG Vorbisに変換する。
// スクリプト書き換え（ScriptAdjuster）で参照が.oggに変更されるため、
// 出力ファイル名は.mid/.midiを.oggに置換した形式にする
// （例: bgm/sinone.mid → bgm/sinone.ogg）。変換成功後、元のMIDIファイルは
// 削除する。
func convertMidiFilesUsing(directory string, midiConverter *converter.MidiConverter) error {
	midiFiles, err := findMidiFiles(directory)
	if err != nil {
		return err
	}

	// why not: 可用性の検査をMIDIの実在確認より先に行うと、MIDIを持たない
	// ゲームのビルドまでFluidSynthのインストールを強制することになる
	// （internal/doctorがFluidSynthをRequired=falseとしているのと同じ理由）。
	// 検査は必ずMIDIが実在する場合に限る。
	if len(midiFiles) == 0 {
		return nil
	}

	if err := ensureMidiConversionAvailable(midiConverter); err != nil {
		return err
	}

	return convertMidiFileList(midiFiles, midiConverter)
}

// findMidiFiles はdirectory配下の.mid/.midiファイルを再帰的に列挙する。
func findMidiFiles(directory string) ([]string, error) {
	var midiFiles []string

	err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".mid" || ext == ".midi" {
			midiFiles = append(midiFiles, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("MIDIファイルの走査に失敗しました: %w", err)
	}

	return midiFiles, nil
}

// ensureMidiConversionAvailable はMIDI変換の前提条件（FluidSynthの実行可能性と
// サウンドフォントの実在）を検査する。
//
// why not: 以前は前提条件を満たさない場合に変換自体を黙ってスキップしていたが、
// スクリプトの.mid→.ogg書き換えは無条件に走るため、存在しない.oggを指す参照が
// 残りBGMが無音のAPKが完成してしまう（実機で確認済み）。前提条件の欠如は
// 「やることが無い」ではなくビルドの失敗として扱う。
//
// why not: サウンドフォントの実在確認をここで行うのは、converter.
// GetDefaultSoundfontPathがFluidR3のパスへ実在確認なしにフォールバックし、
// MidiConverter.Convertが不在をファイル単位のStatusFailedとしてしか報告
// しないため。全ファイルを試して初めて原因が判明するより、着手前に一度だけ
// 検査して単一のエラーへまとめる方が原因を特定しやすい。
func ensureMidiConversionAvailable(midiConverter *converter.MidiConverter) error {
	if !midiConverter.IsFluidsynthAvailable() {
		return fmt.Errorf(
			"%w: fluidsynthコマンドを実行できません。%s",
			ErrMidiConversionUnavailable, midiRequirementGuide(),
		)
	}

	soundfontPath := midiConverter.SoundfontPath()
	if _, err := os.Stat(soundfontPath); err != nil {
		return fmt.Errorf(
			"%w: サウンドフォントが見つかりません: %s。%s",
			ErrMidiConversionUnavailable, soundfontPath, midiRequirementGuide(),
		)
	}

	return nil
}

// convertMidiFileList はmidiFilesを順にOGGへ変換し、失敗を集約して返す。
//
// why not: 最初の失敗で打ち切らず全ファイルを試すのは、利用者が一度の実行で
// 失敗した全ファイルを把握できるようにするため。ただし1件でも失敗した場合は
// エラーを返し、変換されなかったMIDIを指す.ogg参照がAPKへ混入するのを防ぐ。
func convertMidiFileList(midiFiles []string, midiConverter *converter.MidiConverter) error {
	var failures []string

	for _, midiFile := range midiFiles {
		oggFile := withSuffix(midiFile, ".ogg")

		result, err := midiConverter.Convert(midiFile, oggFile)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %s", midiFile, err))

			continue
		}
		if result.Status != converter.StatusSuccess {
			failures = append(failures, fmt.Sprintf("%s: %s", midiFile, result.Message))

			continue
		}

		// why not: 削除失敗はビルドエラーに昇格させない。変換自体は成功して
		// おり.oggの実体が揃っているため、スクリプトの.ogg参照は解決でき無音に
		// ならない（本チケットが対象とする欠陥は発生しない）。残留した.midは
		// 再生されない死蔵アセットとしてAPKへ同梱されるだけ（サイズ増のみ）で
		// あり、これでビルド全体を落とす方が損害が大きい。本パッケージには
		// ロガーの注入口が無いため警告出力も行わない（copyPolyfillFilesUsingの
		// フォント取得失敗と同じ方針）。
		_ = os.Remove(midiFile)
	}

	if len(failures) > 0 {
		return fmt.Errorf("%w: %s", ErrMidiConversionFailed, strings.Join(failures, " / "))
	}

	return nil
}

// fetchPlugins はkrkrsdl2プラグイン(extrans/wuvorbis)を取得する。
// 取得に失敗してもビルドは継続する（呼び出し元はnilを「プラグインなし」
// として扱う）。
func (b *BuildPipeline) fetchPlugins() *builder.PluginsInfo {
	fetcher := builder.NewPluginFetcher("", nil)

	info, err := fetcher.GetPlugins()
	if err != nil {
		return nil
	}

	return &info
}

// findGameIconUsing はfindGameIconの実装本体。テストからExeIconExtractor
// をparser.IconExtractorインターフェース経由で差し替え可能にする。
func (b *BuildPipeline) findGameIconUsing(iconExtractor parser.IconExtractor) string {
	if b.extractDir == "" {
		return ""
	}

	for _, name := range gameIconNames {
		candidate := filepath.Join(b.extractDir, name)
		if fileExists(candidate) {
			return candidate
		}
	}

	matches, err := filepath.Glob(filepath.Join(b.extractDir, "*.ico"))
	if err == nil && len(matches) > 0 {
		return matches[0]
	}

	// 抽出ディレクトリにアイコンが無い場合、入力がEXEであればEXEに
	// 埋め込まれたアイコンの抽出を試みる。
	if strings.ToLower(filepath.Ext(b.config.InputPath)) == ".exe" {
		extracted, extractErr := iconExtractor.Extract(b.config.InputPath, b.extractDir)
		if extractErr == nil {
			return extracted
		}
	}

	return ""
}
