package pipeline

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// convertMidiFiles はdirectory配下のMIDIファイルをOGG Vorbis形式に変換する。
//
// krkrsdl2はMIDI再生未対応のため、MIDIファイルをOGG Vorbisに変換する。
// スクリプト書き換え（ScriptAdjuster）で参照が.oggに変更されるため、
// 出力ファイル名は.mid/.midiを.oggに置換した形式にする
// （例: bgm/sinone.mid → bgm/sinone.ogg）。変換成功後、元のMIDIファイルは
// 削除する。
func (b *BuildPipeline) convertMidiFiles(directory string) error {
	timeout := time.Duration(b.config.FFmpegTimeoutSeconds) * time.Second
	midiConverter := converter.NewMidiConverter("", 0, "", 0, timeout, nil)

	return convertMidiFilesUsing(directory, midiConverter)
}

func convertMidiFilesUsing(directory string, midiConverter *converter.MidiConverter) error {
	if !midiConverter.IsFluidsynthAvailable() {
		return nil
	}

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
		return fmt.Errorf("MIDIファイルの走査に失敗しました: %w", err)
	}

	for _, midiFile := range midiFiles {
		oggFile := withSuffix(midiFile, ".ogg")

		result, _ := midiConverter.Convert(midiFile, oggFile)
		if result.Status == converter.StatusSuccess {
			_ = os.Remove(midiFile)
		}
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
