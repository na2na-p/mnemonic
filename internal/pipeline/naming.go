package pipeline

import (
	"path/filepath"
	"regexp"
	"strings"
)

// javaReservedWords はパッケージ名生成時のフォールバック用Java予約語リスト。
var javaReservedWords = map[string]struct{}{
	"abstract": {}, "assert": {}, "boolean": {}, "break": {}, "byte": {},
	"case": {}, "catch": {}, "char": {}, "class": {}, "const": {},
	"continue": {}, "default": {}, "do": {}, "double": {}, "else": {},
	"enum": {}, "extends": {}, "false": {}, "final": {}, "finally": {},
	"float": {}, "for": {}, "goto": {}, "if": {}, "implements": {},
	"import": {}, "instanceof": {}, "int": {}, "interface": {}, "long": {},
	"native": {}, "new": {}, "null": {}, "package": {}, "private": {},
	"protected": {}, "public": {}, "return": {}, "short": {}, "static": {},
	"strictfp": {}, "super": {}, "switch": {}, "synchronized": {}, "this": {},
	"throw": {}, "throws": {}, "transient": {}, "true": {}, "try": {},
	"void": {}, "volatile": {}, "while": {},
}

var nonPackageCharPattern = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// sanitizeName はnameをAndroidパッケージ名のセグメントとして使用できる
// 形式に変換する。
//
// 空白は単語区切りとして保持するためアンダースコアに変換し、その他の特殊文字
// （ハイフン、記号、日本語等）はパッケージ名に使用できないため削除する。
func (b *BuildPipeline) sanitizeName(name string) string {
	sanitized := strings.ReplaceAll(name, " ", "_")
	sanitized = nonPackageCharPattern.ReplaceAllString(sanitized, "")

	if sanitized != "" && sanitized[0] >= '0' && sanitized[0] <= '9' {
		sanitized = "_" + sanitized
	}

	sanitized = strings.ToLower(sanitized)

	if _, reserved := javaReservedWords[sanitized]; reserved {
		sanitized = "game_" + sanitized
	}

	return sanitized
}

// gameIconNames は優先順位の高いアイコンファイル名（krkr/吉里吉里ゲームで
// よく使われる）。
var gameIconNames = []string{"icon.png", "icon.ico", "icon.bmp"}

// findGameIcon はゲームアイコンを検索する。抽出ディレクトリからアイコン
// ファイルを検索し、見つからない場合は空文字列を返す。
func (b *BuildPipeline) findGameIcon() string {
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

	return ""
}
