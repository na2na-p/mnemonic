package converter

import (
	"bytes"
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
//
// Applyが非nilの場合、Pattern/Replacementの代わりにこの関数で変換される。
//
// why not: Goのregexp(RE2)はPythonのre（バックトラック方式）と異なり
// 否定先読み((?!...))に対応しない。[layopt]へのtype=alpha自動追加ルール
// （「type=が未指定の場合のみ追加」）はこの制約により単純なPattern文字列
// だけでは表現できないため、この関数で同じ意味論を実現する。
type AdjustmentRule struct {
	Pattern     string
	Replacement string
	Description string
	Apply       func(content string) (string, int)
}

// DefaultRules はScriptAdjusterのデフォルト調整ルール。
//
// MIDI系ルール（MIDISoundBuffer以降）はPR11'（T-211）でmidi.py+script.pyの
// フィーチャーブランチ差分から移植した。適用順序はPython版と同一に保つ必要が
// ある: MIDISoundBuffer→WaveSoundBuffer変換が先に走ることで、変換前の
// "MIDISoundBuffer.midiOut(...)" 呼び出しも次のmidiOut置換ルールで捕捉される
// （4e83a5eのtest_midi_out_converted_from_midi_sound_bufferが検証する挙動）。
var DefaultRules = []AdjustmentRule{
	{
		Pattern:     `^(\s*)(Plugins\.link\(["'].*?\.dll["']\);)`,
		Replacement: `$1// $2 // Disabled for Android`,
		Description: "プラグインDLL読み込みの無効化",
	},
	{
		Pattern:     `saveDataLocation\s*=\s*System\.exePath\s*\+\s*saveDataLocation`,
		Replacement: `saveDataLocation = System.dataPath`,
		Description: "セーブデータパスをdataPathに変更（Android対応）",
	},
	{
		Pattern:     `MIDISoundBuffer`,
		Replacement: `WaveSoundBuffer`,
		Description: "MIDISoundBufferをWaveSoundBufferに変換（krkrsdl2対応）",
	},
	{
		Pattern:     `^(\s*)(WaveSoundBuffer\.midiOut\([^)\n]*\);)`,
		Replacement: `$1; // $2 // Disabled: midiOut not available in WaveSoundBuffer`,
		Description: "WaveSoundBuffer.midiOut呼び出しを空文に置換（krkrsdl2対応）",
	},
	{
		Pattern:     `(["'])([^"']*?)\.mid(["'])`,
		Replacement: `$1$2.ogg$3`,
		Description: "MIDI参照をOGGに変換（.mid → .ogg）",
	},
	{
		Pattern:     `(["'])([^"']*?)\.midi(["'])`,
		Replacement: `$1$2.ogg$3`,
		Description: "MIDI参照をOGGに変換（.midi → .ogg）",
	},
	{
		Pattern:     `storage \+ "\.mid\.ogg"`,
		Replacement: `storage + ".ogg"`,
		Description: "MIDI検索パターンを修正（.mid.ogg → .ogg）",
	},
	// loadpluginタグのDLL参照をlibプレフィックス付き.soに変換（extrans.dll → libextrans.so）。
	// krkrsdl2はTVPLocatePluginで.dll→.so変換のみ行い、libプレフィックスは付与しない。
	// Androidのネイティブライブラリ規約でlibプレフィックスが必要なため、フルネームを指定する。
	{
		Pattern:     `\[loadplugin\s+module="extrans\.dll"\]`,
		Replacement: `[loadplugin module="libextrans.so"]`,
		Description: "extrans.dllをlibextrans.soに変換（Android krkrsdl2対応）",
	},
	// wuvorbis.dllをlibwuvorbis.soに変換（Ogg Vorbis再生に必要）。
	{
		Pattern:     `\[loadplugin\s+module="wuvorbis\.dll"\]`,
		Replacement: `[loadplugin module="libwuvorbis.so"]`,
		Description: "wuvorbis.dllをlibwuvorbis.soに変換（Android krkrsdl2対応）",
	},
	// krmovie.dllはkrkrsdl2で未実装のためコメントアウト。
	{
		Pattern:     `(\[loadplugin\s+module="krmovie\.dll"\])`,
		Replacement: `;# $1 # Disabled: not supported on krkrsdl2`,
		Description: "krmovie.dllをコメントアウト（krkrsdl2未対応）",
	},
	// その他のDLLプラグインをコメントアウト（extrans以外）。
	//
	// why not: Python版パターンは`module="(?!extrans")`という否定先読みを
	// 持つが、".dll\"に一致する現実的な値（例: "extrans.dll"）に対しては
	// 常にlookaheadが素通りする（"extrans"直後は"."であり、リテラル
	// `extrans"`には一致しないため）。つまりこの先読みはextrans.dllを
	// 実質的に除外しない死んだ制約であり、この時点で既にextrans.dllは
	// 前段のルールで"libextrans.so"へ変換済み（".dll"で終わらなくなり
	// このパターン自体にマッチしなくなる）のため除外は不要。RE2は
	// 否定先読みに対応しないため、この不要な制約を持たないプレーンな
	// パターンで同一の実効的な挙動を再現する。
	//
	// why not(krmovie.dllの二重コメントアウト): この副作用としてPython版
	// では、krmovie.dllルール適用後も置換結果の文字列中に元の
	// `[loadplugin module="krmovie.dll"]`が置換文字列の一部として
	// 文字通り残るため、本ルールが同じ部分文字列に再度マッチし
	// 二重にコメントアウトされる（";# ;# ... # Disabled for Android
	// # Disabled: not supported on krkrsdl2"）。これはPython版の実際の
	// 挙動であり、意図的な仕様ではなく偶発的な副作用と考えられるが、
	// 本チケットはPython版DEFAULT_RULESの最終状態と順序の一致を要求する
	// ため、Go版でも忠実に同じ挙動を再現する。
	{
		Pattern:     `(\[loadplugin\s+module="[^"]*\.dll"\])`,
		Replacement: `;# $1 # Disabled for Android`,
		Description: "その他のDLLプラグインをコメントアウト",
	},
	// レイヤー透過修正: [layopt layer=N] に type=alpha を自動追加。
	// krkrsdl2のSIMDeエミュレーション問題により、明示的なtype指定が必要。
	//
	// why not: Python版パターン`\[layopt\b(?![^\]]*\btype=)([^\]]*\blayer=
	// [0-9]+[^\]]*)\]`は「type=が未指定の場合のみ追加」を否定先読みで
	// 表現するが、RE2は否定先読みに対応しないため、タグ全体を先に
	// キャプチャしGo側でtype=の有無を判定するapplyLayoptAlphaRuleへ委譲する。
	{
		Description: "レイヤー透過修正: type=alphaを自動追加（krkrsdl2対応）",
		Apply:       applyLayoptAlphaRule,
	},
}

var (
	layoptTagPattern        = regexp.MustCompile(`\[layopt\b([^\]]*)\]`)
	layoptNumericLayerCheck = regexp.MustCompile(`\blayer=[0-9]+\b`)
)

// applyLayoptAlphaRule は「type=が未指定かつlayer=数値を持つ[layopt]タグに
// type=alphaを追加する」ルールを実装する。Pattern文字列で表現できない
// （AdjustmentRule.Applyのdocコメント参照）ためこの関数で実現する。
func applyLayoptAlphaRule(content string) (string, int) {
	count := 0

	result := layoptTagPattern.ReplaceAllStringFunc(content, func(tag string) string {
		attrs := layoptTagPattern.FindStringSubmatch(tag)[1]

		if strings.Contains(attrs, "type=") || !layoptNumericLayerCheck.MatchString(attrs) {
			return tag
		}

		count++

		return "[layopt" + attrs + " type=alpha]"
	})

	return result, count
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

// GetOutputExtension は出力ファイルの拡張子を変更しないため常に空文字列を
// 返す（スクリプト調整は拡張子を保持する）。
func (a *ScriptAdjuster) GetOutputExtension(_ string) string { return "" }

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

	// Python版はsource.read_text(encoding="utf-8-sig")でBOMを自動除去してから
	// 読み込む。入力に既にBOMが付いていても二重付与しないよう、出力時に
	// 付与し直す前提でここで一旦取り除く。
	content = bytes.TrimPrefix(content, utf8BOM)

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

	// 吉里吉里(KiriKiriZ)はBOM無しUTF-8をShift_JISとして誤解釈するため、
	// ScriptAdjusterが扱う.ks/.tjsは常にBOM付きUTF-8で書き出す
	// （Python版のdest.write_text(encoding="utf-8-sig")に相当）。
	adjustedBytes := append(append([]byte{}, utf8BOM...), []byte(adjusted)...)
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
		if rule.Apply != nil {
			newResult, count := rule.Apply(result)
			result = newResult
			total += count

			continue
		}

		re := regexp.MustCompile("(?m)" + rule.Pattern)
		matches := re.FindAllStringIndex(result, -1)
		result = re.ReplaceAllString(result, rule.Replacement)
		total += len(matches)
	}

	return result, total
}

// AddStartupDirective はstartup.tjs向けのポリフィル初期化ディレクティブを
// contentの先頭に追加する。
//
// why not: 以前はKiriKiriZ用のエンコーディング指定ディレクティブ
// （@if (kirikiriz) { System.setArgument(...) } @endif）を追加していたが、
// krkrsdl2 polyfill導入（Goal 1）に伴い、system/polyfillinitialize.tjs
// （MenuItem等の欠落クラスのスタブ読み込みを行う）の実行呼び出しに置き換えた。
func (a *ScriptAdjuster) AddStartupDirective(content string) string {
	const directive = "// krkrsdl2 polyfill initialization\n" +
		"Scripts.execStorage(\"system/polyfillinitialize.tjs\");\n\n"

	return directive + content
}
