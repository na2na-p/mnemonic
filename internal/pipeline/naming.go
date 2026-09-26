package pipeline

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrPackageNameUndeterminable は--package-name が未指定で、パッケージ名の元になる
// 名前（ゲームタイトル。タイトルが空なら入力ファイル名）から英字で始まる名前を
// 作れない場合のエラー。
var ErrPackageNameUndeterminable = errors.New("ゲームタイトルまたは入力ファイル名から英字で始まるパッケージ名を決定できません。--package-name で指定してください")

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

// sanitizeName はnameをAndroidパッケージ名のセグメントに使える文字だけから
// なる形式に変換する。
//
// 空白は単語区切りとして保持するためアンダースコアに変換し、その他の特殊文字
// （ハイフン、記号、日本語等）はパッケージ名に使用できないため削除する。
// 英字で始まるかどうかは判定しないため、数字始まりの結果もそのまま返す。
func (b *BuildPipeline) sanitizeName(name string) string {
	sanitized := strings.ReplaceAll(name, " ", "_")
	sanitized = nonPackageCharPattern.ReplaceAllString(sanitized, "")
	sanitized = strings.ToLower(sanitized)
	// 先頭の "_"（主に空白由来）は Android の applicationId 規則（各セグメントは
	// 英字で始まる）に反するため除去する。予約語判定より前に行わないと " true" が
	// 予約語 true として扱われない。
	sanitized = strings.TrimLeft(sanitized, "_")

	if _, reserved := javaReservedWords[sanitized]; reserved {
		sanitized = "game_" + sanitized
	}

	return sanitized
}

// derivePackageName はビルドに使うAndroidパッケージ名を返す。explicit が
// 指定されていればそれを、無ければ baseName から導出した名前を返す。baseName から
// 英字で始まる名前を作れない場合はErrPackageNameUndeterminableを返す。
func (b *BuildPipeline) derivePackageName(explicit, baseName string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}

	sanitized := b.sanitizeName(baseName)
	// why not: 数字始まりや "_" 始まりのセグメントは Android の applicationId 規則に
	// 反する。固定の代替名や接頭辞を発明すると、利用者の意図と無関係な名前になり
	// 別のゲームと同じパッケージ名になって端末上で互いを上書きしうるため、利用者に
	// 指定させる。
	if sanitized == "" || sanitized[0] < 'a' || sanitized[0] > 'z' {
		return "", fmt.Errorf("%w: 名前 %q", ErrPackageNameUndeterminable, baseName)
	}

	return "com.krkr." + sanitized, nil
}
