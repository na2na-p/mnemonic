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
// regexp.MustCompileに渡す前に(?m)（複数行モード）を付与して評価される。
// Replacementは正規表現の後方参照をGoのregexp構文（$1、$2、…）で指定する。
//
// Applyが非nilの場合、Pattern/Replacementの代わりにこの関数で変換される。
//
// why not: Goのregexp(RE2)はバックトラック方式の正規表現エンジンと異なり
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
// MIDI系ルール（MIDISoundBuffer以降）は適用順序を維持する必要がある:
// MIDISoundBuffer→WaveSoundBuffer変換が先に走ることで、変換前の
// "MIDISoundBuffer.midiOut(...)" 呼び出しも次のmidiOut置換ルールで捕捉される。
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
	// VideoConverterはmpeg1video+mp2への変換後、常に.mpg拡張子で出力する
	// (video.goのGetOutputExtension参照)。.mpgは対象外のため
	// (パターンが.mpg自体にマッチせず)無変換で残り、二重変換は起きない。
	//
	// why not: 拡張子部分だけ(?i:...)で大文字小文字を無視する。Windows製作の
	// ゲームは".WMV"のような大文字拡張子を参照することがあり
	// (VideoConverter.CanConvert/adjustScriptsが拡張子を小文字化して比較する
	// のと同じ理由)、ここを素のリテラルのままにすると大文字参照だけ書き換えを
	// 素通りし、実体は常に小文字".mpg"で出力されるため参照が解決できなくなる。
	{
		Pattern:     `(["'])([^"']*?)\.(?i:wmv)(["'])`,
		Replacement: `$1$2.mpg$3`,
		Description: "動画参照をMPEGに変換（.wmv → .mpg）",
	},
	{
		Pattern:     `(["'])([^"']*?)\.(?i:avi)(["'])`,
		Replacement: `$1$2.mpg$3`,
		Description: "動画参照をMPEGに変換（.avi → .mpg）",
	},
	{
		Pattern:     `(["'])([^"']*?)\.(?i:mpeg)(["'])`,
		Replacement: `$1$2.mpg$3`,
		Description: "動画参照をMPEGに変換（.mpeg → .mpg）",
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
	// why not: extrans.dllを除外する否定先読み（module="(?!extrans")`のような
	// パターン）を検討する余地があるが、この時点で既にextrans.dllは前段の
	// ルールで"libextrans.so"へ変換済み（".dll"で終わらなくなりこのパターン
	// 自体にマッチしなくなる）ため除外は不要。RE2は否定先読みに対応しないため、
	// この不要な制約を持たないプレーンなパターンで同一の実効的な挙動を
	// 再現する。
	//
	// why not(krmovie.dllの二重コメントアウト): krmovie.dllルール適用後も
	// 置換結果の文字列中に元の`[loadplugin module="krmovie.dll"]`が置換
	// 文字列の一部として文字通り残るため、本ルールが同じ部分文字列に再度
	// マッチし二重にコメントアウトされる（";# ;# ... # Disabled for Android
	// # Disabled: not supported on krkrsdl2"）。意図的な仕様ではなく偶発的な
	// 副作用と考えられるが、DEFAULT_RULESの最終状態と順序の一致を維持する
	// ため、この挙動をそのまま許容する。
	{
		Pattern:     `(\[loadplugin\s+module="[^"]*\.dll"\])`,
		Replacement: `;# $1 # Disabled for Android`,
		Description: "その他のDLLプラグインをコメントアウト",
	},
	// レイヤー透過修正: [layopt layer=N] に type=alpha を自動追加。
	// krkrsdl2のSIMDeエミュレーション問題により、明示的なtype指定が必要。
	//
	// why not: 「type=が未指定の場合のみ追加」は否定先読みで自然に表現できるが、
	// RE2は否定先読みに対応しないため、タグ全体を先にキャプチャしGo側で
	// type=の有無を判定するapplyLayoptAlphaRuleへ委譲する。
	{
		Description: "レイヤー透過修正: type=alphaを自動追加（krkrsdl2対応）",
		Apply:       applyLayoptAlphaRule,
	},
}

// videoExtensionRuleDescriptions はDefaultRules中の動画拡張子書き換えルール
// (.wmv/.avi/.mpeg → .mpg)を識別するDescriptionの集合。AdjustmentRuleに
// カテゴリを表すフィールドが無いため、TestScriptAdjuster_DefaultRulesOrderと
// 同様にDescriptionを識別子として扱う。
var videoExtensionRuleDescriptions = map[string]bool{
	"動画参照をMPEGに変換（.wmv → .mpg）":  true,
	"動画参照をMPEGに変換（.avi → .mpg）":  true,
	"動画参照をMPEGに変換（.mpeg → .mpg）": true,
}

// DefaultRulesWithoutVideoExtensions はDefaultRulesから動画拡張子書き換え
// ルールを除いたルール集合のコピーを返す。
//
// why not: 呼び出し元(pipelineパッケージのadjustScripts)は--skip-video時に
// この戻り値をScriptAdjusterへ渡す。SkipVideo時はVideoConverterが登録され
// ず動画ファイルは無変換のまま(拡張子も実体も元のまま)convertDirに残るため、
// DefaultRulesをそのまま適用すると参照だけが.mpgへ書き換わり、実体の無い
// .mpgを指す不整合が生じる（変換対象アセットの拡張子だけを書き換え、実体の
// 変換自体は別条件でスキップされるケース全般に共通する不具合のクラス）。
func DefaultRulesWithoutVideoExtensions() []AdjustmentRule {
	filtered := make([]AdjustmentRule, 0, len(DefaultRules))

	for _, rule := range DefaultRules {
		if videoExtensionRuleDescriptions[rule.Description] {
			continue
		}

		filtered = append(filtered, rule)
	}

	return filtered
}

var (
	layoptTagPattern = regexp.MustCompile(`\[layopt\b([^\]]*)\]`)
	// why not: layer=[0-9]+の後ろに\bを付けると"layer=12abc"のような後続
	// 文字がある場合にマッチしなくなる。末尾に\bを付けず、後続文字の有無に
	// 関わらず数字部分でマッチさせる。
	layoptNumericLayerCheck = regexp.MustCompile(`\blayer=[0-9]+`)
	// why not: strings.Contains(attrs, "type=")は"hittype="や"subtype="の
	// ような、"type="の直前が単語構成文字であるため実際にはtype属性ではない
	// 部分文字列にも誤って一致してしまう。\btype=で単語境界付きの判定にする。
	layoptTypeCheck = regexp.MustCompile(`\btype=`)
)

// applyLayoptAlphaRule は「type=が未指定かつlayer=数値を持つ[layopt]タグに
// type=alphaを追加する」ルールを実装する。Pattern文字列で表現できない
// （AdjustmentRule.Applyのdocコメント参照）ためこの関数で実現する。
func applyLayoptAlphaRule(content string) (string, int) {
	count := 0

	result := layoptTagPattern.ReplaceAllStringFunc(content, func(tag string) string {
		attrs := layoptTagPattern.FindStringSubmatch(tag)[1]

		if layoptTypeCheck.MatchString(attrs) || !layoptNumericLayerCheck.MatchString(attrs) {
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
// 既知の失敗はConversionResult{Status: StatusFailed}へ変換するため、
// errは常にnilを返す。
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

	// 入力に既にBOMが付いていても二重付与しないよう、出力時に付与し直す
	// 前提でここで一旦取り除く。
	content = bytes.TrimPrefix(content, utf8BOM)

	// why: Goのstring(content)はUTF-8を検証しないため、この検証を省略すると
	// 不正なバイト列（例: Shift_JISのままのファイル）を含む内容がそのまま
	// SUCCESSとして書き出されてしまう（文字化けの温存）。utf8.Validで
	// Status: FAILEDとなる失敗パスを設ける。
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

	base := filepath.Base(source)
	for _, sfa := range specialFileAdjustments {
		if !strings.EqualFold(base, sfa.fileName) {
			continue
		}

		newAdjusted, addCount := sfa.apply(a, adjusted)
		adjusted = newAdjusted
		count += addCount
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
	// ScriptAdjusterが扱う.ks/.tjsは常にBOM付きUTF-8で書き出す。
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

// specialFileAdjustment はKAG3標準の特定ファイル(ファイル名は大文字小文字を
// 無視して比較)に対し、DefaultRules適用後に追加で行う調整を表す。
type specialFileAdjustment struct {
	// fileName は対象ファイルのベース名(拡張子含む)。
	fileName string
	// apply はDefaultRules適用後の内容contentを受け取り、
	// (調整後の内容, 調整回数)を返す。
	apply func(a *ScriptAdjuster, content string) (string, int)
}

// specialFileAdjustments はConvert()がDefaultRules適用後に追加で行う
// ファイル名ベースの調整の一覧。各エントリのfileNameはKAG3標準クラスの
// ファイル名(startup.tjs/messagelayer.tjs/yesnodialog.tjs/mainwindow.tjs)と
// 1対1であり互いに排他的なため、Convert()側は単純な順次ループでよい。
var specialFileAdjustments = []specialFileAdjustment{
	{
		fileName: "startup.tjs",
		apply: func(a *ScriptAdjuster, content string) (string, int) {
			if !a.addEncodingDirective {
				return content, 0
			}

			return a.AddStartupDirective(content), 1
		},
	},
	{
		fileName: "messagelayer.tjs",
		apply: func(a *ScriptAdjuster, content string) (string, int) {
			return a.ApplyMessageLayerCompat(content)
		},
	},
	{
		fileName: "yesnodialog.tjs",
		apply: func(_ *ScriptAdjuster, _ string) (string, int) {
			return yesNoDialogReplacement, 1
		},
	},
	{
		fileName: "mainwindow.tjs",
		apply: func(a *ScriptAdjuster, content string) (string, int) {
			return a.ApplyMainWindowCompat(content)
		},
	},
}

// AdjustContent はcontentに調整ルールを適用し、(調整後の内容, 調整回数)を返す。
//
// why not: startup.tjs判定はConvert()側でsource.nameを直接見て行うため、
// この関数にfilename引数は不要である。使われない引数を持たせず、この関数の
// シグネチャをそのユースケースに合わせて単純化する。
func (a *ScriptAdjuster) AdjustContent(content string) (string, int) {
	return applyRules(content, a.rules)
}

// applyRules はcontentへrulesを順に適用し、(調整後の内容, 調整回数)を返す。
func applyRules(content string, rules []AdjustmentRule) (string, int) {
	total := 0
	result := content

	for _, rule := range rules {
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

// messageLayerCompatRules はKAG3標準のMessageLayer.tjsに適用する
// krkrsdl2互換ルール。フォント名保持用メンバ`var face`を`fontFace`へ
// リネームする（上流krkrsdl2/kag3が行っているのと同じ修正）。
//
// why not(リネーム以外の選択肢): KAG3のMessageLayerクラスはフォント名を
// 保持するメンバ`var face;`を宣言しており、krkrz系TJS2ではこのメンバが
// Layerネイティブの描画面プロパティ`face`をシャドーイングする。その結果
// clearLayer()の`face = dfProvince; colorRect(...)`が描画面を切り替えられず、
// 当たり判定領域のクリアがメイン画像への不透明黒ブレンドとして誤実行され、
// メッセージレイヤ全体が不透明黒になる（Androidでの全画面真っ黒の根本原因）。
// krkr2のTJS2はネイティブプロパティを優先するためWindowsでは顕在化しない。
// エンジン側のメンバ解決順序の変更は影響範囲が広すぎるため、上流と同じ
// スクリプト側リネームで対処する。
var messageLayerCompatRules = []AdjustmentRule{
	{
		Pattern:     `\bvar face;`,
		Replacement: `var fontFace;`,
		Description: "フォント用faceメンバ宣言をfontFaceにリネーム（krkrsdl2対応）",
	},
	{
		Pattern:     `font\.face = face =`,
		Replacement: `font.face = fontFace =`,
		Description: "font.faceへの連鎖代入をfontFaceにリネーム（krkrsdl2対応）",
	},
	{
		Pattern:     `'@' \+ \(face =`,
		Replacement: `'@' + (fontFace =`,
		Description: "アットマーク付きフォント名代入をfontFaceにリネーム（krkrsdl2対応）",
	},
	{
		// why not(\bのみにしない理由): \bはドット直後にも成立するため
		// `X.face = src.face;`のようなプロパティ代入にも一致してしまう。
		// 行頭または非ワード・非ドット文字に限定して素のメンバ参照のみ捕捉する。
		Pattern:     `(^|[^.\w])face = src\.face;`,
		Replacement: `${1}fontFace = src.fontFace;`,
		Description: "assignからのフォント名コピーをfontFaceにリネーム（krkrsdl2対応）",
	},
}

// mainWindowCompatRules はKAG3標準のMainWindow.tjsに適用するkrkrsdl2互換
// ルール。finalize()内でfore/backのlayers・messagesを走査する4箇所に、
// フィールドが未初期化のまま走査されないようtypeofガードを追加する。
//
// why not(KAGWindowコンストラクタやfinalize自体を書き換えない理由):
// Androidのkrkrsdl2は同時に1つのネイティブウィンドウしか作れないため、
// KAGWindowコンストラクタがsuper.Window()でサブウィンドウ生成を試みると
// （例: 「このソフトについて」）例外を投げる。この時点でfore/backは
// クラスフィールド初期化により空の辞書配列(%[])になっているが、
// fore.layers・fore.messages等はsuper.Window()より後のコンストラクタ
// 本体で代入されるため未設定のままで残る。呼び出し元のfireClick()は
// try/catchで保護されていても、後にGCがこの中途半端なオブジェクトの
// finalize()を呼ぶ経路はtry/catchの外側でありポリフィルでは捕捉できず、
// 未初期化フィールドへの.countアクセスが二次例外を送出してネイティブ
// クラッシュ(SIGABRT)に至る。KAGWindow自体はKAG3標準クラスであり
// ゲーム固有の内容を含まないため、MessageLayer.tjs/YesNoDialog.tjsと
// 同様にファイル名で特定して直接調整する。
var mainWindowCompatRules = []AdjustmentRule{
	{
		Pattern:     `for\(var i = 0; i< fore\.layers\.count; i\+\+\) invalidate fore\.layers\[i\];`,
		Replacement: `if(typeof fore.layers != "undefined") for(var i = 0; i< fore.layers.count; i++) invalidate fore.layers[i];`,
		Description: "finalizeでのfore.layers未初期化アクセスをガード（krkrsdl2対応）",
	},
	{
		Pattern:     `for\(var i = 0; i< back\.layers\.count; i\+\+\) invalidate back\.layers\[i\];`,
		Replacement: `if(typeof back.layers != "undefined") for(var i = 0; i< back.layers.count; i++) invalidate back.layers[i];`,
		Description: "finalizeでのback.layers未初期化アクセスをガード（krkrsdl2対応）",
	},
	{
		Pattern:     `for\(var i = 0; i< fore\.messages\.count; i\+\+\) invalidate fore\.messages\[i\];`,
		Replacement: `if(typeof fore.messages != "undefined") for(var i = 0; i< fore.messages.count; i++) invalidate fore.messages[i];`,
		Description: "finalizeでのfore.messages未初期化アクセスをガード（krkrsdl2対応）",
	},
	{
		Pattern:     `for\(var i = 0; i< back\.messages\.count; i\+\+\) invalidate back\.messages\[i\];`,
		Replacement: `if(typeof back.messages != "undefined") for(var i = 0; i< back.messages.count; i++) invalidate back.messages[i];`,
		Description: "finalizeでのback.messages未初期化アクセスをガード（krkrsdl2対応）",
	},
}

// yesNoDialogReplacement はKAG3標準のYesNoDialog.tjsを置き換える
// 単一ウィンドウ実装。
//
// why not(元実装を残さない理由): KAG3のaskYesNoはYesNoDialogWindow
// (サブWindow)をshowModalで表示するが、AndroidのSDLは1ウィンドウしか
// サポートせず「Cannot create SDL window」例外でセーブ/ロード確認等が
// 全て失敗する。同期的に結果を返せる代替はネイティブのメッセージ
// ボックスだけであるため、System.showYesNoMessageBox(fork版krkrsdl2で
// 追加)を呼ぶ実装へ全置換する。未対応エンジンでは確認なしの肯定として
// 続行する(確認できない環境で操作を全て塞ぐより安全側)。
const yesNoDialogReplacement = `// YesNoDialog.tjs - krkrsdl2単一ウィンドウ向け置換実装(mnemonicが生成)
// 元実装はサブWindowをshowModalで表示するが、Androidでは
// 「Cannot create SDL window」となるためネイティブダイアログを使う。

function askYesNo(message, caption = "確認")
{
	if (typeof(global.System.showYesNoMessageBox) != "undefined")
		return +global.System.showYesNoMessageBox(message, caption) == 1;
	return true;
}
`

// ApplyMessageLayerCompat はMessageLayer.tjs向けのkrkrsdl2互換調整を
// contentへ適用し、(調整後の内容, 調整回数)を返す。
func (a *ScriptAdjuster) ApplyMessageLayerCompat(content string) (string, int) {
	return applyRules(content, messageLayerCompatRules)
}

// ApplyMainWindowCompat はMainWindow.tjs向けのkrkrsdl2互換調整を
// contentへ適用し、(調整後の内容, 調整回数)を返す。
func (a *ScriptAdjuster) ApplyMainWindowCompat(content string) (string, int) {
	return applyRules(content, mainWindowCompatRules)
}

// AddStartupDirective はstartup.tjs向けのポリフィル初期化ディレクティブを
// contentの先頭に追加する。
//
// why not: krkrsdl2で欠落するMenuItem等のKAG3標準クラスごとに、startup.tjs
// 側で個別のif分岐やexecStorage呼び出しを増やす方法もある。しかしその方式
// では対応クラスを追加するたびにstartup.tjs自体への変更が必要になる。
// 読み込みをsystem/polyfillinitialize.tjs（欠落クラスのスタブ読み込みを行う）
// 1箇所に集約し、その実行呼び出しだけをcontent先頭に追加することで、対応
// クラスの追加・変更をpolyfillinitialize.tjs側に閉じ込める。
func (a *ScriptAdjuster) AddStartupDirective(content string) string {
	const directive = "// krkrsdl2 polyfill initialization\n" +
		"Scripts.execStorage(\"system/polyfillinitialize.tjs\");\n\n"

	return directive + content
}
