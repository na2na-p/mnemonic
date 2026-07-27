package converter

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// AdjustmentRule はスクリプト調整ルールを表す不変値。
//
// 正規表現パターンと置換文字列のペアを保持する。Patternは
// regexp.MustCompileに渡す前に(?m)（複数行モード）を付与して評価される
// （Python版がre.MULTILINEフラグ付きでコンパイルするのと同じ挙動）。
// Replacementは正規表現の後方参照をGoのregexp構文（$1、$2、…）で指定する。
type AdjustmentRule struct {
	Pattern     string
	Replacement string
	Description string
}

// DefaultRules はScriptAdjusterのデフォルト調整ルール。
var DefaultRules = []AdjustmentRule{
	{
		Pattern:     `^(\s*)(Plugins\.link\(["'].*?\.dll["']\);)`,
		Replacement: `$1// $2 // Disabled for Android`,
		Description: "プラグインDLL読み込みの無効化",
	},
}

// ScriptAdjuster はKiriKiriZ向けにゲームスクリプト(.ks, .tjs)を調整するConverter。
type ScriptAdjuster struct {
	rules                []AdjustmentRule
	addEncodingDirective bool
}

// NewScriptAdjuster はScriptAdjusterを初期化する。
// rulesがnilの場合はDefaultRulesのコピーを使用する。
func NewScriptAdjuster(rules []AdjustmentRule, addEncodingDirective bool) *ScriptAdjuster {
	if rules == nil {
		rules = append([]AdjustmentRule{}, DefaultRules...)
	}

	return &ScriptAdjuster{rules: rules, addEncodingDirective: addEncodingDirective}
}

// Rules は適用される調整ルールを返す。
func (a *ScriptAdjuster) Rules() []AdjustmentRule { return a.rules }

// AddEncodingDirective はエンコーディングディレクティブを追加するかどうかを返す。
func (a *ScriptAdjuster) AddEncodingDirective() bool { return a.addEncodingDirective }

// SupportedExtensions は対応する拡張子の一覧を返す。
func (a *ScriptAdjuster) SupportedExtensions() []string {
	return []string{".ks", ".tjs"}
}

// CanConvert はfilePathが.ksまたは.tjsファイルかを判定する。
func (a *ScriptAdjuster) CanConvert(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))

	return containsString(a.SupportedExtensions(), ext)
}

// Convert はスクリプトファイルに調整ルールを適用し、destへ出力する。
//
// Python版のconvert()はtry/except Exceptionで全ての失敗を捕捉して
// ConversionResult{Status: StatusFailed}へ変換するため、Go版もerrは常にnilを返す。
func (a *ScriptAdjuster) Convert(source, dest string) (ConversionResult, error) {
	if _, err := os.Stat(source); err != nil {
		return ConversionResult{
			SourcePath: source,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("変換元ファイルが見つかりません: %s", source),
		}, nil
	}

	content, err := os.ReadFile(source) //nolint:gosec // 存在確認済みの変換元ファイルを読む用途のため妥当
	if err != nil {
		return ConversionResult{SourcePath: source, Status: StatusFailed, Message: err.Error()}, nil
	}

	// why: Python版はsource.read_text(encoding="utf-8")でUTF-8として厳密に
	// デコードし、不正なバイト列（例: Shift_JISのままのファイル）は
	// UnicodeDecodeErrorとしてtry/exceptに捕捉されStatus: FAILEDになる。
	// Goのstring(content)はUTF-8を検証しないため、この検証を省略すると
	// 不正なバイト列を含む内容がそのままSUCCESSとして書き出されてしまう
	// （文字化けの温存）。utf8.Validで同じ失敗パスを再現する。
	if !utf8.Valid(content) {
		return ConversionResult{
			SourcePath:  source,
			Status:      StatusFailed,
			Message:     fmt.Sprintf("UTF-8として読み込めませんでした: %s", source),
			BytesBefore: int64(len(content)),
		}, nil
	}

	bytesBefore := int64(len(content))

	adjusted, count := a.AdjustContent(string(content))

	isStartup := strings.EqualFold(filepath.Base(source), "startup.tjs")
	if isStartup && a.addEncodingDirective {
		adjusted = a.AddStartupDirective(adjusted)
		count++
	}

	if count == 0 {
		return ConversionResult{
			SourcePath:  source,
			Status:      StatusSkipped,
			Message:     "調整が不要なファイルです",
			BytesBefore: bytesBefore,
			BytesAfter:  bytesBefore,
		}, nil
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return ConversionResult{SourcePath: source, Status: StatusFailed, Message: err.Error()}, nil
	}

	adjustedBytes := []byte(adjusted)
	if err := os.WriteFile(dest, adjustedBytes, 0o644); err != nil { //nolint:gosec // ビルド成果物の出力用途のため妥当な権限
		return ConversionResult{SourcePath: source, Status: StatusFailed, Message: err.Error()}, nil
	}

	return ConversionResult{
		SourcePath:  source,
		DestPath:    dest,
		Status:      StatusSuccess,
		Message:     fmt.Sprintf("%d箇所を調整しました", count),
		BytesBefore: bytesBefore,
		BytesAfter:  int64(len(adjustedBytes)),
	}, nil
}

// AdjustContent はcontentに調整ルールを適用し、(調整後の内容, 調整回数)を返す。
//
// why not: Python版はfilename引数を受け取るが本体では未使用（startup.tjs判定は
// convert()側でsource.nameを直接見て行う）。Go版では使われない引数を持たせず、
// この関数のシグネチャをそのユースケースに合わせて単純化する。
func (a *ScriptAdjuster) AdjustContent(content string) (string, int) {
	total := 0
	result := content

	for _, rule := range a.rules {
		re := regexp.MustCompile("(?m)" + rule.Pattern)
		matches := re.FindAllStringIndex(result, -1)
		result = re.ReplaceAllString(result, rule.Replacement)
		total += len(matches)
	}

	return result, total
}

// AddStartupDirective はstartup.tjs向けのエンコーディングディレクティブを
// contentの先頭に追加する。
func (a *ScriptAdjuster) AddStartupDirective(content string) string {
	const directive = "@if (kirikiriz)\n{\n    System.setArgument(\"-readencoding\", \"UTF-8\");\n}\n@endif\n"

	return directive + content
}
