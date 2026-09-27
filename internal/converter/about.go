package converter

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// AboutDialog はKAG3標準の「このソフトについて」で表示する内容。
//
// ゼロ値はabout.ksが見つからない状態を表し、本文・キャプションとも
// System.titleで表示する。
type AboutDialog struct {
	// Text はabout.ksから抜き出した本文。空なら代わりにキャプションを表示する。
	Text string
	// Caption は[title name="..."]の値。空ならSystem.titleを使う。
	Caption string
}

// ExtractAboutDialog はUTF-8のKAGシナリオscenarioから、表示される文字を
// 近似したプレーンテキストの本文とキャプションを抜き出す。
//
// 行・コメント・ラベル・タグ・エスケープの扱いはKAGParser
// (internal/resources/system_polyfill/KAGParser.tjs)に合わせ、[s]より後ろは
// 読まない。画像・フォント・配置などのタグは捨てる。実行時の評価が要るものは
// 再現しない: [emb]とマクロ呼び出しは捨て、[if]/[ignore]の条件は評価せず
// どの分岐の文字も残し、[macro]〜[endmacro]の中の文字もそのまま残す。
func ExtractAboutDialog(scenario string) AboutDialog {
	scenario = strings.TrimPrefix(scenario, string(utf8BOM))
	// why not: CRLFのまま行末を判定しない。エンジンは行の読み込みで改行
	// コードを取り除いてから行末の"\"や[p]を見るため、"\r"が残っていると
	// 改行を打ち消す"\"を見落とす。
	scenario = strings.ReplaceAll(scenario, "\r\n", "\n")
	scenario = strings.ReplaceAll(scenario, "\r", "\n")

	e := aboutExtractor{}
	inScript := false

	for line := range strings.SplitSeq(scenario, "\n") {
		line = strings.TrimLeft(line, "\t")

		if inScript {
			if line == "[endscript]" || line == "[endscript]\\" || line == "@endscript" {
				inScript = false
			}

			continue
		}

		if line == "[iscript]" || line == "[iscript]\\" || line == "@iscript" {
			inScript = true

			continue
		}

		if strings.HasPrefix(line, ";") || strings.HasPrefix(line, "*") {
			continue
		}

		if e.consumeLine(line) {
			break
		}
	}

	return AboutDialog{Text: strings.TrimSpace(e.text.String()), Caption: e.caption}
}

type aboutExtractor struct {
	text    strings.Builder
	caption string
}

// consumeLine はlineの描画内容をtextへ足し、[s]に達したらtrueを返す。
func (e *aboutExtractor) consumeLine(line string) bool {
	if rest, ok := strings.CutPrefix(line, "@"); ok {
		tag, _, ok := parseKAGTag(rest, 0, true)

		return ok && e.handleTag(tag)
	}

	lineBreak := true
	if trimmed, ok := strings.CutSuffix(line, "\\"); ok {
		line = trimmed
		lineBreak = false
	} else if strings.HasSuffix(line, "[p]") {
		lineBreak = false
	}

	for pos := 0; pos < len(line); {
		switch {
		case strings.HasPrefix(line[pos:], "[["):
			e.text.WriteByte('[')
			pos += 2
		case line[pos] == '[':
			tag, end, ok := parseKAGTag(line, pos+1, false)
			// why not: 閉じていないタグで例外にしない。エンジンは文法エラーで
			// 止まるが、ビルドを止めるほどの問題ではないため、その行の残りを
			// 表示しないだけにする。
			if !ok {
				pos = len(line)

				continue
			}

			if e.handleTag(tag) {
				return true
			}

			pos = end
		case line[pos] == '\t':
			pos++
		default:
			e.text.WriteByte(line[pos])
			pos++
		}
	}

	if lineBreak {
		e.text.WriteByte('\n')
	}

	return false
}

// kagTag はKAGのタグ1つ分。
type kagTag struct {
	name  string
	attrs map[string]kagAttr
}

// kagAttr はタグの属性値。evaluatedは値が"&"(TJS式)か"%"(マクロ引数)で
// 始まり、実行時にしか値が決まらないことを表す。
type kagAttr struct {
	value     string
	evaluated bool
}

// parseKAGTag はline[pos:]をタグ名から始まるタグとして読み、(タグ, タグ直後の
// 位置, 成否)を返す。lineCommandがtrueなら"@"行として行末までを1つのタグと
// みなす。属性値の読み方はKAGParser.tjsの_getNextTagに合わせる。
func parseKAGTag(line string, pos int, lineCommand bool) (kagTag, int, bool) {
	isWS := func(c byte) bool { return c == ' ' || c == '\t' }
	isDelim := func(c byte) bool { return !lineCommand && c == ']' }
	skipWS := func() {
		for pos < len(line) && isWS(line[pos]) {
			pos++
		}
	}

	skipWS()

	nameStart := pos
	for pos < len(line) && !isWS(line[pos]) && !isDelim(line[pos]) {
		pos++
	}

	if nameStart == pos {
		return kagTag{}, 0, false
	}

	tag := kagTag{name: strings.ToLower(line[nameStart:pos]), attrs: map[string]kagAttr{}}

	for {
		skipWS()

		if pos == len(line) {
			return tag, pos, lineCommand
		}

		if isDelim(line[pos]) {
			return tag, pos + 1, true
		}

		if line[pos] == '*' {
			pos++

			continue
		}

		attrStart := pos
		for pos < len(line) && !isWS(line[pos]) && line[pos] != '=' && !isDelim(line[pos]) {
			pos++
		}

		attrName := line[attrStart:pos]

		skipWS()

		if pos == len(line) || line[pos] != '=' {
			tag.attrs[attrName] = kagAttr{value: "true"}

			continue
		}

		pos++

		skipWS()

		if pos == len(line) {
			return kagTag{}, 0, false
		}

		var (
			attr kagAttr
			ok   bool
		)

		attr, pos, ok = parseKAGAttrValue(line, pos, lineCommand)
		if !ok {
			return kagTag{}, 0, false
		}

		tag.attrs[attrName] = attr
	}
}

// parseKAGAttrValue はline[pos:]を属性値として読み、(値, 値の直後の位置, 成否)を
// 返す。"`"は次の1文字をそのまま値に含める。
func parseKAGAttrValue(line string, pos int, lineCommand bool) (kagAttr, int, bool) {
	evaluated := false
	if line[pos] == '&' || line[pos] == '%' {
		evaluated = true
		pos++
	}

	var quote byte
	if pos < len(line) && (line[pos] == '"' || line[pos] == '\'') {
		quote = line[pos]
		pos++
	}

	var value strings.Builder

	for pos < len(line) {
		c := line[pos]
		if quote != 0 && c == quote {
			break
		}

		if quote == 0 && (c == ' ' || c == '\t' || (!lineCommand && c == ']')) {
			break
		}

		if c == '`' {
			pos++
			if pos == len(line) {
				return kagAttr{}, 0, false
			}

			c = line[pos]
		}

		value.WriteByte(c)
		pos++
	}

	if !lineCommand && pos == len(line) {
		return kagAttr{}, 0, false
	}

	if quote != 0 && pos < len(line) {
		pos++
	}

	v := value.String()
	if !evaluated && (strings.HasPrefix(v, "&") || strings.HasPrefix(v, "%")) {
		evaluated = true
		v = v[1:]
	}

	return kagAttr{value: v, evaluated: evaluated}, pos, true
}

// handleTag はtagを処理し、[s]ならtrueを返す。
func (e *aboutExtractor) handleTag(tag kagTag) bool {
	switch tag.name {
	case "s":
		return true
	case "r":
		e.text.WriteByte('\n')
	case "title":
		// why not: "&"や"%"で始まる値はKAGが実行時に評価するTJS式やマクロ
		// 引数であり、ビルド時には値が分からない。式の文字列をそのまま出すと
		// 意味の無い表示になるため、キャプションを空にしてSystem.titleへ任せる。
		name := tag.attrs["name"]
		if name.evaluated {
			e.caption = ""
		} else {
			e.caption = name.value
		}
	}

	return false
}

// LoadAboutDialog はroot配下からabout.ksを探してAboutDialogを返す。
//
// KAGは'about.ks'という格納名で開くため、ディレクトリを問わずファイル名を
// 大文字小文字を区別せずに探す。見つからなければゼロ値を返す。複数あって
// 表示内容が異なる場合もゼロ値を返す。
//
// why not: 複数のうちどれかを選ばない。ビルド時にはエンジンがどれを開くか
// 判断できないため、別の画面の内容を表示するよりタイトルだけを出す。
//
// about.ksがUTF-8として妥当でない場合は変換元のパスを添えてErrScriptNotUTF8を
// 返す。
func LoadAboutDialog(root string) (AboutDialog, error) {
	var (
		found  bool
		result AboutDialog
		differ bool
	)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() || !strings.EqualFold(d.Name(), "about.ks") {
			return nil
		}

		content, err := os.ReadFile(path) //nolint:gosec // 変換済みツリー内のシナリオを読む用途のため妥当
		if err != nil {
			return fmt.Errorf("about.ksの読み込みに失敗しました: %w", err)
		}

		content = bytes.TrimPrefix(content, utf8BOM)
		if !utf8.Valid(content) {
			return fmt.Errorf("%s: %w", path, ErrScriptNotUTF8)
		}

		dialog := ExtractAboutDialog(string(content))
		if found && dialog != result {
			differ = true
		}

		found = true
		result = dialog

		return nil
	})
	if err != nil {
		return AboutDialog{}, fmt.Errorf("about.ksの探索に失敗しました: %w", err)
	}

	if differ {
		return AboutDialog{}, nil
	}

	return result, nil
}

// WithAboutDialog はMainWindow.tjsの「このソフトについて」をdの内容で
// 置き換えるScriptAdjusterを返す。aは変更しない。
func (a *ScriptAdjuster) WithAboutDialog(d AboutDialog) *ScriptAdjuster {
	adjusted := *a
	adjusted.about = d

	return &adjusted
}

// aboutHandlerSep は文の間に置ける空白とコメント。
const aboutHandlerSep = `(?:\s|//[^\n]*|/\*[\s\S]*?\*/)*`

// aboutHandlerPattern はKAG3標準のonHelpAboutMenuItemClickの定義全体に一致する。
//
// why not: 関数名だけで一致させて本体を丸ごと置き換えない。ゲームが独自の
// 処理に書き換えたハンドラまで消してしまうため、標準の5文だけから成る
// 本体に限る。
var aboutHandlerPattern = regexp.MustCompile(`(?m)^([ \t]*)function\s+onHelpAboutMenuItemClick\s*\(\s*sender\s*\)` +
	aboutHandlerSep + `\{` + aboutHandlerSep +
	`var\s+win\s*=\s*new\s+global\.KAGWindow\s*\(\s*false\s*,\s*aboutWidth\s*,\s*aboutHeight\s*\)\s*;` + aboutHandlerSep +
	`win\.setPos\s*\(\s*left\s*\+\s*\(\s*\(\s*width\s*-\s*win\.width\s*\)\s*>>\s*1\s*\)\s*,` +
	`\s*top\s*\+\s*\(\s*\(\s*height\s*-\s*win\.height\s*\)\s*>>\s*1\s*\)\s*\)\s*;` + aboutHandlerSep +
	`win\.process\s*\(\s*(?:'about\.ks'|"about\.ks")\s*,\s*,\s*,\s*true\s*\)\s*;` + aboutHandlerSep +
	`win\.showModal\s*\(\s*\)\s*;` + aboutHandlerSep +
	`invalidate\s+win\s*;` + aboutHandlerSep + `\}`)

// replaceAboutHandler はcontent内のKAG3標準の「このソフトについて」ハンドラを
// System.informで本文を表示する実装へ置き換え、(置換後の内容, 置換回数)を返す。
//
// why not(サブウィンドウを使わない理由): Androidのkrkrsdl2はウィンドウを1つしか
// 作れず、標準のハンドラはKAGWindowの生成で「Cannot create SDL window」例外に
// なる。同期的に表示できる代替はネイティブのメッセージボックスだけであり、
// about.ksの画像や配置は再現できないため本文だけを表示する。
func (a *ScriptAdjuster) replaceAboutHandler(content string) (string, int) {
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}

	caption := "System.title"
	if a.about.Caption != "" {
		caption = quoteTJSString(a.about.Caption)
	}

	text := caption
	if a.about.Text != "" {
		text = quoteTJSString(a.about.Text)
	}

	count := 0

	// why not: ReplaceAllStringで置き換えない。置換文字列の"$"が後方参照として
	// 展開されるため、本文に"$"を含むゲームで表示内容が壊れる。
	result := aboutHandlerPattern.ReplaceAllStringFunc(content, func(match string) string {
		count++

		indent := aboutHandlerPattern.FindStringSubmatch(match)[1]

		return aboutHandlerReplacement(indent, newline, strings.Count(match, "\n"), text, caption)
	})

	return result, count
}

// aboutHandlerReplacement は置き換え後のonHelpAboutMenuItemClickを、元の定義と
// 同じ改行数lineBreaksで組み立てる。
//
// why not: 置き換えで行数を変えない。TJSの例外メッセージはスクリプトの行番号を
// 示すため、ハンドラより後ろの行番号がずれると元のMainWindow.tjsと突き合わせて
// 調べられなくなる。改行が足りない書き方の定義は1行にまとめ、残りの改行を
// 後ろに置く。
func aboutHandlerReplacement(indent, newline string, lineBreaks int, text, caption string) string {
	inform := "System.inform(" + text + ", " + caption + ");"

	lines := []string{
		indent + "function onHelpAboutMenuItemClick(sender)",
		indent + "{",
		indent + "\t// Androidではウィンドウを1つしか作れないため、about.ksの本文を",
		indent + "\t// メッセージボックスで表示する(mnemonicが生成)",
		indent + "\t" + inform,
	}
	if lineBreaks < len(lines) {
		return indent + "function onHelpAboutMenuItemClick(sender) { " + inform + " }" +
			strings.Repeat(newline, lineBreaks)
	}

	for len(lines) < lineBreaks {
		lines = append(lines, "")
	}

	return strings.Join(append(lines, indent+"}"), newline)
}

// quoteTJSString はsをTJS2の二重引用符文字列リテラルにする。
//
// why not(U+FFFFを超える文字をそのまま書かない理由): Androidのkrkrsdl2は
// tjs_charがchar16_tであり、BOM付きUTF-8のスクリプトを読む際
// (TextStream.cppからCharacterSet.cppのTVPUtf8ToWideChar)に4バイト文字を
// 下位16ビットへ切り詰める。例えばU+20022は'"'になりリテラルが壊れて
// スクリプト全体が読めなくなる。そこでUTF-16のサロゲートペアを\xで書く。
// TJS2の字句解析(tjsLex.cppのTJSInternalParseString)は\xの後ろの16進数字を
// sizeof(tjs_char)*2桁(char16_tでは4桁)まで読むため、常に4桁で書けば続く
// 文字が16進数字でも取り込まれない。
//
// 改行とタブ以外の制御文字はメッセージボックスの本文として意味を持たない
// ため取り除く。"$"と"&"は@"..."形式でだけ特別な意味を持つため、そのまま
// 残す。
func quoteTJSString(s string) string {
	var b strings.Builder

	b.WriteByte('"')

	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
		case r > 0xFFFF:
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&b, `\x%04X\x%04X`, hi, lo)
		default:
			b.WriteRune(r)
		}
	}

	b.WriteByte('"')

	return b.String()
}
