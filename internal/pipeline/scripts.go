package pipeline

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/na2na-p/mnemonic/internal/converter"
)

// adjustScripts はdirectory配下の全.ks/.tjsファイルにScriptAdjusterを適用する。
//
// why not: 大文字小文字のバリエーション（.ks/.KS/.Ks/.tjs/.TJS/.Tjs）ごとに
// 走査を繰り返す方式も考えられるが、大文字小文字を区別しないファイル
// システム（例: macOS既定）では同一ファイルに複数回ヒットしうる。1回の
// WalkDirで拡張子をstrings.EqualFoldで比較することで、大文字小文字を
// 区別するファイルシステムでも1回の走査で漏れなく捕捉でき、かつ大文字
// 小文字を区別しないファイルシステムでの二重適用も起こらない。
//
// why not: --skip-video時はDefaultRulesWithoutVideoExtensions（動画拡張子
// 書き換えルールを除いたルール集合）を使う。SkipVideo時はVideoConverterが
// 登録されず動画ファイルは無変換のまま(拡張子も実体も元のまま)なので、
// DefaultRulesのまま適用すると参照だけが.mpgへ書き換わり、実体の無い.mpgを
// 指す参照が残る不具合になる。
func (b *BuildPipeline) adjustScripts(directory string) error {
	var rules []converter.AdjustmentRule
	if b.config.SkipVideo {
		rules = converter.DefaultRulesWithoutVideoExtensions()
	}

	adjuster := converter.NewScriptAdjuster(rules, true)

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

		// why not: ConversionManagerを経由させない。ScriptAdjusterは外部コマンドを
		// 起動せず、読み込んだ内容を置換してその場へ書き戻すだけなので、失敗は
		// 変換直後の一時ツリーに対するローカルなファイルI/Oか内容起因の恒久的な
		// 失敗に限られ、数秒のバックオフで解消する類ではなく、リトライが役に立たない。
		if _, err := adjuster.Convert(path, path); err != nil {
			return fmt.Errorf("スクリプトの調整に失敗しました: %w", err)
		}

		return nil
	})
}
