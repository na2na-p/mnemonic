package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/na2na-p/mnemonic/internal/builder"
)

// removePluginDirectory はdirectory直下のプラグインディレクトリを削除する。
//
// Windows用の.dllプラグインはAndroidで使用できないため、プラグイン
// ディレクトリを削除する。krkrsdl2は多くの機能をビルトインで持っているため
// プラグインDLLは不要（extrans/wuvorbisのようにkrkrsdl2が必要とするネイティブ
// プラグインはjniLibs経由で別途配置されるため、削除対象のプラグイン
// ディレクトリには含まれない）。
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
