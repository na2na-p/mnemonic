// Package safepath はアーカイブのエントリ名を展開先ディレクトリへ結合する際に、
// 結合結果が展開先ディレクトリの外へ脱出しないことを保証する（zip slip対策）。
//
// エントリ名中の"\"をディレクトリ区切りとして扱う設計判断もこのパッケージが持つ
// （理由はJoinのwhy not参照）。
package safepath

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrOutsideBase は結合結果が基準ディレクトリの外を指す場合のエラー。
var ErrOutsideBase = errors.New("展開先が出力ディレクトリの外を指しています")

// Join はbaseDir配下にentryNameを結合し、絶対パスを返す。
// 結合結果がbaseDirの外を指す場合はErrOutsideBaseを返す。
//
// why not: エントリ名はアーカイブ内データに由来し外部入力として信頼できない
// ため、単純なfilepath.Joinで済ませず、展開先がbaseDir外に脱出しないことを
// 検証する。
//
// もう1点、entryName中の"\"はここで"/"へ正規化してからOS区切り文字へ
// 変換するためディレクトリ階層として扱われる。"\"をパス区切りとして扱わず
// リテラルな単一ファイル名の一部として扱う実装も考えられるが、XP3アーカイブの
// エントリ名はWindows由来で"\"区切りのケースが実際にあり得るため、ディレクトリ
// 階層として正規化する挙動を意図的に選んでいる。ZIPのエントリ名は仕様上"/"
// 区切りだが、"..\"のような表記を区切りとして検査対象に含めるため同じ規則を適用する。
func Join(baseDir, entryName string) (string, error) {
	cleanedName := filepath.FromSlash(strings.ReplaceAll(entryName, `\`, "/"))
	joined := filepath.Join(baseDir, cleanedName)

	base, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("出力ディレクトリの絶対パス解決に失敗しました: %w", err)
	}

	target, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("展開先パスの絶対パス解決に失敗しました: %w", err)
	}

	if target != base && !strings.HasPrefix(target, base+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", ErrOutsideBase, entryName)
	}

	return target, nil
}
