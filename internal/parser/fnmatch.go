package parser

import (
	"regexp"
	"runtime"
	"strings"
)

// matchGlob はUnix fnmatch(3)相当のマッチングを行う（"*"がパス区切りを含む
// 任意の文字列にマッチする）。
//
// why not: Go標準の path.Match / filepath.Match は"*"が"/"を越えてマッチしない
// （シェルのグロブ相当）が、exclude/conversion_rulesパターン
// （例: "voice/*.ogg"）は"*"がパス区切りを含む任意の文字列にマッチする
// （正規表現の".*"相当）性質に依存している。そのためfnmatch相当の変換
// ロジックを実装したうえでregexpに変換して判定する。
func matchGlob(name, pattern string) bool {
	if runtime.GOOS == "windows" {
		// Windowsのファイルシステムは大文字小文字を区別しないため、
		// パターン・対象文字列とも小文字化してから照合する。
		name = strings.ToLower(name)
		pattern = strings.ToLower(pattern)
	}

	re, err := regexp.Compile(translateFnmatch(pattern))
	if err != nil {
		// why not: translateFnmatchが生成した正規表現がコンパイルできない場合
		// （下記の既知の差分など）、設定ファイルの記述ミス1件でスキャン全体を
		// 失敗させたくないため、「不正なパターンは常に不一致」として
		// 安全側（除外/上書きなしに倒れる）にフォールバックする。
		return false
	}

	return re.MatchString(name)
}

// translateFnmatch はfnmatchパターンをGo正規表現ソースへ変換する。
//
// fnmatch相当の変換規則を実装したものだが、以下の2点は意味を保存しない
// 既知の差分として残っている（拡張子・ディレクトリ名によるglobという
// 現実的な設定パターンでは踏まないため許容している。いずれもregexp.Compile
// がエラーを返し、結果としてmatchGlobはfalseを返す）:
//   - 文字クラス内のバックスラッシュ（例: "[a\b]"）: "-"を含まない文字クラス
//     の中身のバックスラッシュを"\\\\"へエスケープしリテラル扱いにする
//     実装も考えられるが、本実装は素通しするため生成される正規表現の意味が
//     変わりうる。
//   - 逆順・空レンジ（例: "[b-a]"）: 無効なレンジを検出して除去・結合する
//     実装も考えられるが、本実装は素通しするため、Goの正規表現エンジンが
//     受理しない（開始>終了のレンジをコンパイルエラーにする）。
func translateFnmatch(pattern string) string {
	var b strings.Builder
	b.WriteString("^")

	runes := []rune(pattern)
	n := len(runes)
	i := 0

	for i < n {
		c := runes[i]
		i++

		switch c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '[':
			j := i
			if j < n && runes[j] == '!' {
				j++
			}
			if j < n && runes[j] == ']' {
				j++
			}
			for j < n && runes[j] != ']' {
				j++
			}

			if j >= n {
				// 対応する ']' がない場合は '[' をリテラルとして扱う。
				b.WriteString(`\[`)

				continue
			}

			stuff := string(runes[i:j])
			if strings.HasPrefix(stuff, "!") {
				stuff = "^" + stuff[1:]
			} else if strings.HasPrefix(stuff, "^") || strings.HasPrefix(stuff, "[") {
				stuff = `\` + stuff
			}
			b.WriteString("[")
			b.WriteString(stuff)
			b.WriteString("]")

			i = j + 1
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}

	b.WriteString("$")

	// "?"/"*" は改行を含む任意の文字にマッチさせる必要があるため(?s)を付与する。
	return "(?s)" + b.String()
}
