package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/na2na-p/mnemonic/internal/builder"
	"github.com/na2na-p/mnemonic/internal/resources"
)

// copyPolyfillFiles はkrkrsdl2/kag3 polyfillファイルをdirectory/system/へ
// コピーする。これによりMenuItemやKAGParserなどの不足クラスが提供される。
// また、Koruriフォントをsystem/font.ttfとしてコピーする。
func (b *BuildPipeline) copyPolyfillFiles(directory string) error {
	return copyPolyfillFilesUsing(directory, builder.NewFontFetcher("", nil), b.log())
}

func copyPolyfillFilesUsing(directory string, fontFetcher *builder.FontFetcher, logger Logger) error {
	systemDir := filepath.Join(directory, "system")
	if err := os.MkdirAll(systemDir, 0o750); err != nil {
		return fmt.Errorf("systemディレクトリの作成に失敗しました: %w", err)
	}

	for _, name := range resources.SystemPolyfillFiles {
		data, err := resources.SystemPolyfillFS.ReadFile("system_polyfill/" + name)
		if err != nil {
			// why: リソースは埋め込みFSでビルド時に固定されるため通常発生しない。
			// 見つからない場合はスキップする防御的な実装とする。
			continue
		}

		if err := os.WriteFile(filepath.Join(systemDir, name), data, 0o644); err != nil { //nolint:gosec // ビルド成果物の出力用途のため妥当な権限
			return fmt.Errorf("polyfillファイルの書き込みに失敗しました: %s: %w", name, err)
		}
	}

	// why not: フォントを用意できなくてもビルドは止めず警告に留める。
	// PolyfillInitialize.tjsはsystem/font.ttf・font.otfがどちらも無ければ
	// -deffontを設定せずに初期化を続けるため、フォントの欠落でAPKの初期化
	// スクリプトが失敗することはない。
	if err := copyFontFile(systemDir, fontFetcher); err != nil {
		logger.Warning(fmt.Sprintf("フォントを用意できなかったため既定フォントのままビルドを続けます: %v", err))
	}

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
		return fmt.Errorf("フォントの取得に失敗しました: %w", err)
	}

	return copyFile(fontInfo.Path, fontDest)
}
