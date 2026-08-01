package builder

import (
	"fmt"
	"os"
	"path/filepath"
)

// pluginPlacer はkrkrsdl2プラグイン(.so)をjniLibsディレクトリに配置する。
type pluginPlacer struct {
	projectDir string
}

// newPluginPlacer はpluginPlacerを初期化する。
func newPluginPlacer(projectDir string) *pluginPlacer {
	return &pluginPlacer{projectDir: projectDir}
}

// Place はkrkrsdl2プラグインをjniLibsディレクトリにコピーする。
//
// プラグイン(.so)を各ABI用のjniLibsディレクトリに配置する。jniLibsに配置
// されたプラグインはAPKビルド時に自動的にlib/{abi}/配下に含まれ、
// System.loadLibraryで読み込み可能になる。スクリプト変換時にlibプレフィックス
// 付きのフルファイル名を指定するため、libプレフィックス付きのファイルのみ
// 配置すれば良い。pluginsInfoがnilの場合は何も行わない（TemplatePreparer.Prepare
// のdocコメント参照）。プラグインファイルが見つからない場合はスキップする
// （ベストエフォートとしてエラーを握りつぶす）。
func (p *pluginPlacer) Place(pluginsInfo *PluginsInfo) error {
	if pluginsInfo == nil {
		return nil
	}

	jniLibsDir := filepath.Join(p.projectDir, "app", "src", "main", "jniLibs")

	for _, abi := range SupportedABIs {
		abiDir := filepath.Join(jniLibsDir, abi)
		if err := os.MkdirAll(abiDir, 0o750); err != nil {
			return fmt.Errorf("%w: %w", ErrTemplatePreparer, err)
		}

		for _, srcPath := range pluginsInfo.GetAllPathsForABI(abi) {
			if _, err := os.Stat(srcPath); err != nil {
				continue
			}

			destPath := filepath.Join(abiDir, filepath.Base(srcPath))
			if err := copyFile(srcPath, destPath); err != nil {
				return fmt.Errorf("%w: プラグインのコピーに失敗しました: %s: %w", ErrTemplatePreparer, srcPath, err)
			}
		}
	}

	return nil
}
