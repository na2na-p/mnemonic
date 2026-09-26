package pipeline

import (
	"path/filepath"
	"strings"

	"github.com/na2na-p/mnemonic/internal/parser"
)

// gameIconNames は優先順位の高いアイコンファイル名（krkr/吉里吉里ゲームで
// よく使われる）。
var gameIconNames = []string{"icon.png", "icon.ico", "icon.bmp"}

// findGameIcon はゲームアイコンを検索する。
//
// 以下の優先順位でアイコンを検索する:
//  1. 抽出ディレクトリからアイコンファイルを検索
//  2. EXEファイルから埋め込みアイコンを抽出（入力がEXEの場合のみ）
//
// 見つからない場合は空文字列を返す。
func (b *BuildPipeline) findGameIcon(extractDir string) string {
	return b.findGameIconUsing(extractDir, parser.NewExeIconExtractor())
}

// findGameIconUsing はfindGameIconの実装本体。テストからExeIconExtractor
// をparser.IconExtractorインターフェース経由で差し替え可能にする。
func (b *BuildPipeline) findGameIconUsing(extractDir string, iconExtractor parser.IconExtractor) string {
	if extractDir == "" {
		return ""
	}

	for _, name := range gameIconNames {
		candidate := filepath.Join(extractDir, name)
		if fileExists(candidate) {
			return candidate
		}
	}

	matches, err := filepath.Glob(filepath.Join(extractDir, "*.ico"))
	if err == nil && len(matches) > 0 {
		return matches[0]
	}

	// 抽出ディレクトリにアイコンが無い場合、入力がEXEであればEXEに
	// 埋め込まれたアイコンの抽出を試みる。
	if strings.ToLower(filepath.Ext(b.config.InputPath)) == ".exe" {
		extracted, extractErr := iconExtractor.Extract(b.config.InputPath, extractDir)
		if extractErr == nil {
			return extracted
		}
	}

	return ""
}
