// Package safepath はアーカイブのエントリ名を展開先ディレクトリへ結合する際に、
// 結合結果が展開先ディレクトリの外へ脱出しないことを保証する（zip slip対策）。
//
// エントリ名中の"\"をディレクトリ区切りとして扱う設計判断もこのパッケージが持つ
// （理由はJoinのwhy not参照）。
package safepath

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// ErrOutsideBase は結合結果が基準ディレクトリの外を指す場合のエラー。
var ErrOutsideBase = errors.New("展開先が出力ディレクトリの外を指しています")

// RelPath はJoinがentryNameを書き出す位置を、基準ディレクトリからの"/"区切りの
// 相対パスとして返す。基準ディレクトリ自体を指す場合は"."を返す。
// 基準ディレクトリの外を指す場合はErrOutsideBaseを返す。
//
// why not: 先頭の"/"を残したままpath.Cleanしない。"/"から始まるパスのCleanは
// 先頭の".."を捨てるため、"/../x"は"/x"になり、その後"/"を除くと基準ディレクトリ
// 配下の"x"として受け付けてしまう。"/../x"はbaseDirへそのまま結合すると外を
// 指す名前である。先に"/"を除いてからCleanすれば"../x"が残り、外として拒否できる。
func RelPath(entryName string) (string, error) {
	rel := path.Clean(strings.TrimLeft(strings.ReplaceAll(entryName, `\`, "/"), "/"))
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%w: %s", ErrOutsideBase, entryName)
	}

	return rel, nil
}

// Join はbaseDir配下にentryNameを結合し、絶対パスを返す。
// 結合結果がbaseDirの外を指す場合はErrOutsideBaseを返す。
//
// why not: エントリ名はアーカイブ内データに由来し外部入力として信頼できない
// ため、単純なfilepath.Joinで済ませず、展開先がbaseDir外に脱出しないことを
// 検証する。
//
// もう1点、entryName中の"\"はRelPathで"/"へ正規化してからOS区切り文字へ
// 変換するためディレクトリ階層として扱われる。"\"をパス区切りとして扱わず
// リテラルな単一ファイル名の一部として扱う実装も考えられるが、XP3アーカイブの
// エントリ名はWindows由来で"\"区切りのケースが実際にあり得るため、ディレクトリ
// 階層として正規化する挙動を意図的に選んでいる。ZIPのエントリ名は仕様上"/"
// 区切りだが、"..\"のような表記を区切りとして検査対象に含めるため同じ規則を適用する。
func Join(baseDir, entryName string) (string, error) {
	rel, err := RelPath(entryName)
	if err != nil {
		return "", err
	}

	base, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("出力ディレクトリの絶対パス解決に失敗しました: %w", err)
	}

	// why not: RelPathが外を指さないと判定しただけで済ませない。RelPathが解釈するのは
	// "/"・"\"・"."・".."だけで、Windowsのボリューム名やUNCの接頭辞は解釈しない。
	// リリースはwindows/amd64のバイナリも配布するため、結合後のパスをfilepath.Absで
	// 絶対パスにしてからbaseの配下かを確かめる。Windowsのfilepath.Absは
	// syscall.FullPath（GetFullPathName）を通すため、字句的な結合の結果ではなく
	// GetFullPathNameが解決したパスで比べることになる（Go標準ライブラリの
	// path/filepath/path_windows.goのabsで確認）。
	// Unixでは、RelPathの結果は整理済みで".."を含まないため、この比較が外と判定する
	// のはbaseがルートのとき（前方一致の相手がbase+区切り文字で、ルートでは区切り
	// 文字2つになる）に限られる。
	target, err := filepath.Abs(filepath.Join(base, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("展開先パスの絶対パス解決に失敗しました: %w", err)
	}
	if target != base && !strings.HasPrefix(target, base+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", ErrOutsideBase, entryName)
	}

	return target, nil
}
