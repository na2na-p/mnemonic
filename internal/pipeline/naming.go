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

// ErrInvalidPackageName は--package-name で指定された値が Android の applicationId
// 規則を満たさない場合のエラー。
var ErrInvalidPackageName = errors.New("--package-name の値が Android の applicationId 規則を満たしません")

// javaReservedWords はパッケージ名のセグメントに使えないJava予約語の集合。
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

var (
	nonPackageCharPattern = regexp.MustCompile(`[^a-zA-Z0-9_]`)
	packageSegmentPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)
)

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
// 指定されていればそれを、無ければ baseName から導出した名前を返す。explicit が
// applicationId 規則を満たさない場合はErrInvalidPackageNameを、baseName から
// 英字で始まる名前を作れない場合はErrPackageNameUndeterminableを返す。
func (b *BuildPipeline) derivePackageName(explicit, baseName string) (string, error) {
	if explicit != "" {
		// why not: builder の Java ソース生成時の検査（generateGameActivityJava）は
		// 1 セグメントの値、"_" で始まるセグメント、2 番目以降の数字始まりのセグメント、
		// 空のセグメント、予約語のセグメントも通すため、そこに任せると規則に反する値が
		// Java ソース生成より後の段階まで進み、失敗しても --package-name を含まない
		// エラーとして遅れて現れる。導出名と同じ applicationId 規則を Validate と同じ
		// 規則でここでも課す。Validate を経ずに derivePackageName や executeBuild を
		// 呼ぶ経路（Run は Validate を通すが、テストは直接呼ぶ）でも不正な値を通さない
		// ための防御なので、Validate と重複していても消さない。
		if err := validatePackageName(explicit); err != nil {
			return "", err
		}

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

// validatePackageName は name が Android の applicationId 規則（ドット区切りで
// 2 セグメント以上、各セグメントは英字で始まり英数字と "_" だけからなる）を満たす
// ことを確かめる。予約語のセグメントも拒否するが、これは applicationId 規則ではなく
// javac の制約による: name は builder が生成する Java ソースの `package %s;` 宣言に
// そのまま入り、javac は予約語のセグメントを持つパッケージ名を受け付けない。
func validatePackageName(name string) error {
	segments := 0
	for segment := range strings.SplitSeq(name, ".") {
		segments++
		if !packageSegmentPattern.MatchString(segment) {
			return fmt.Errorf("%w: セグメント %q は英字で始まり英数字と \"_\" だけからなる必要があります（値: %q）", ErrInvalidPackageName, segment, name)
		}
		if _, reserved := javaReservedWords[segment]; reserved {
			return fmt.Errorf("%w: セグメント %q は Java の予約語です（値: %q）", ErrInvalidPackageName, segment, name)
		}
	}
	if segments < 2 {
		return fmt.Errorf("%w: ドット区切りのセグメントが 2 つ以上必要です（値: %q）", ErrInvalidPackageName, name)
	}

	return nil
}
