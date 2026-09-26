package converter

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"

	"github.com/na2na-p/mnemonic/internal/charset"
)

// ErrUnsupportedEncoding は未対応のエンコーディング名（変換先として指定したUTF-16を含む）を
// 受け取った場合のエラー。
var ErrUnsupportedEncoding = errors.New("サポートされていないエンコーディングです")

// ErrEncodingFileNotFound はEncodingDetectorの検出対象ファイルが存在しない場合のエラー。
var ErrEncodingFileNotFound = errors.New("ファイルが見つかりません")

// ErrEncodingConversionFailed はEncodingConverter.Convertが変換元の内容を
// 目的のエンコーディングへ変換できなかった場合のエラー。原因のエラーを%wで保持する。
var ErrEncodingConversionFailed = errors.New("エンコーディング変換に失敗しました")

// ErrUndecodableSource は変換元の内容が、試した変換元エンコーディングのいずれでも
// 置換文字(U+FFFD)を生じずに復号できなかった場合のエラー。
var ErrUndecodableSource = errors.New("変換元エンコーディングとして復号できないバイト列が含まれています")

// utf8BOM はUTF-8のバイトオーダーマーク。
var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// replacementCharacter はU+FFFD(REPLACEMENT CHARACTER)のUTF-8表現。
var replacementCharacter = []byte("\uFFFD")

// utf16LEBOM / utf16BEBOM はUTF-16のバイトオーダーマーク。utf32LEBOMはutf16LEBOMと
// 先頭2バイトが一致するため、UTF-16LEと取り違えないよう区別に使う。
var (
	utf16LEBOM = []byte{0xff, 0xfe}
	utf16BEBOM = []byte{0xfe, 0xff}
	utf32LEBOM = []byte{0xff, 0xfe, 0x00, 0x00}
)

// SupportedEncodings はEncodingConverterが変換元として認識するエンコーディング名の一覧。
// utf-16le/utf-16beは変換元としてのみ受け付け、変換先には指定できない。
var SupportedEncodings = []string{"shift_jis", "euc-jp", "utf-8", "gb2312", "gb18030", "big5", "cp949", "utf-16le", "utf-16be"}

// encodingAliases はchardetが返すエンコーディング名とSupportedEncodingsの対応マッピング。
//
// why not: "utf-8-sig"のエイリアスはBOM付きUTF-8を示す一般的なエンコーディング
// 名を吸収するために存在する。github.com/saintfish/chardetはBOMの有無に
// 関わらず常に"UTF-8"を返すため、Go側でこのエイリアスが実際に引かれることは
// ないが、将来chardet実装が変わった場合の防御として残す。
var encodingAliases = map[string]string{
	"shift-jis": "shift_jis",
	"shiftjis":  "shift_jis",
	"sjis":      "shift_jis",
	"euc_jp":    "euc-jp",
	"eucjp":     "euc-jp",
	// why not: 別名が無いと、chardetが返す"GB-18030"と"EUC-KR"（saintfish/chardet
	// multi_byte.go）がSupportedEncodingsのどれにも一致せず、gb18030とcp949は
	// 自動検出で選ばれない。
	"gb-18030":  "gb18030",
	"euc-kr":    "cp949",
	"euckr":     "cp949",
	"utf8":      "utf-8",
	"utf-8-sig": "utf-8",
	"ascii":     "utf-8",
	"utf16le":   "utf-16le",
	"utf16be":   "utf-16be",
	// "utf-16"はバイト順を名前に持たないが、復号器はBOMがあればBOMのバイト順に従う。
	"utf-16": "utf-16le",
	"utf16":  "utf-16le",
}

// normalizeEncoding はエンコーディング名を正規化する。
func normalizeEncoding(enc string) string {
	lower := strings.ToLower(strings.ReplaceAll(enc, "_", "-"))
	if alias, ok := encodingAliases[lower]; ok {
		return alias
	}

	return strings.ToLower(enc)
}

// isSupportedEncoding はencがSupportedEncodingsに含まれるかを確認する。
func isSupportedEncoding(enc string) bool {
	if enc == "" {
		return false
	}

	normalized := strings.ToLower(strings.ReplaceAll(normalizeEncoding(enc), "_", "-"))
	return slices.ContainsFunc(SupportedEncodings, func(supported string) bool {
		return normalized == strings.ToLower(strings.ReplaceAll(supported, "_", "-"))
	})
}

// EncodingDetectionResult は文字コード検出結果を表す不変値。
//
// Encodingが空文字列の場合、検出できなかったことを表す。
type EncodingDetectionResult struct {
	Encoding    string
	Confidence  float64
	IsSupported bool
}

// EncodingDetector は文字コード検出を行う。
//
// github.com/saintfish/chardetを使用して検出を行う。
type EncodingDetector struct{}

// NewEncodingDetector はEncodingDetectorを初期化する。
func NewEncodingDetector() *EncodingDetector {
	return &EncodingDetector{}
}

// Detect はfilePathの文字コードを検出する。
func (d *EncodingDetector) Detect(filePath string) (EncodingDetectionResult, error) {
	data, err := os.ReadFile(filePath) //nolint:gosec // 呼び出し側が指定したアセットパスを読む用途のため妥当
	if err != nil {
		return EncodingDetectionResult{}, fmt.Errorf("%w: %s", ErrEncodingFileNotFound, filePath)
	}

	return d.DetectBytes(data), nil
}

// DetectBytes はバイトデータの文字コードを検出する。
//
// UTF-16はBOMで始まる場合に限り"utf-16le"/"utf-16be"として検出する。吉里吉里の
// simple crypt形式は復号するとUTF-16LEになるため、復号せずに"utf-16le"として検出する。
func (d *EncodingDetector) DetectBytes(data []byte) EncodingDetectionResult {
	if len(data) == 0 {
		return EncodingDetectionResult{Encoding: "", Confidence: 0.0, IsSupported: false}
	}

	if _, ok := isSimpleCrypt(data); ok {
		return EncodingDetectionResult{Encoding: "utf-16le", Confidence: 1.0, IsSupported: true}
	}

	if enc := utf16EncodingByBOM(data); enc != "" {
		return EncodingDetectionResult{Encoding: enc, Confidence: 1.0, IsSupported: true}
	}

	// why not: 純ASCIIをchardetの推定名のまま扱うと、ASCIIのみの.ini/.txt/.csv/.ksが
	// SupportedEncodingsに含まれないエンコーディング名として検出され、本来SKIPPED
	// （.ksならBOM付与のSUCCESS）が適切なケースでConvertがエンコーディング変換失敗のエラーを返してしまう。
	// そのためASCIIはUTF-8のサブセットとして"utf-8"（対応済み）に確定させる。
	if charset.IsASCII(data) {
		return EncodingDetectionResult{Encoding: "utf-8", Confidence: 1.0, IsSupported: true}
	}

	var (
		rawEncoding string
		confidence  float64
	)

	// why not: DetectBestは同じ信頼度の候補から実行ごとに異なる候補を返すため使わない
	// （理由はcharset.Detectを参照）。信頼度も要るため、全候補からcharset.BestResultで選ぶ。
	if results, err := chardet.NewTextDetector().DetectAll(data); err == nil {
		if best, ok := charset.BestResult(results); ok {
			rawEncoding = best.Charset
			confidence = float64(best.Confidence) / 100.0
		}
	}

	normalized := ""
	if rawEncoding != "" {
		normalized = normalizeEncoding(rawEncoding)
	}

	return EncodingDetectionResult{
		Encoding:    normalized,
		Confidence:  confidence,
		IsSupported: isSupportedEncoding(rawEncoding),
	}
}

// IsTextFile はfilePathがテキストファイルかどうかを判定する。
//
// 空ファイルはテキストファイルとして扱う。
func (d *EncodingDetector) IsTextFile(filePath string) (bool, error) {
	data, err := os.ReadFile(filePath) //nolint:gosec // 呼び出し側が指定したアセットパスを読む用途のため妥当
	if err != nil {
		return false, fmt.Errorf("%w: %s", ErrEncodingFileNotFound, filePath)
	}

	if len(data) == 0 {
		return true, nil
	}

	// UTF-16のテキストやsimple crypt形式はNULを含むため、NULによるバイナリ判定より
	// 先に判定する。
	if _, ok := isSimpleCrypt(data); ok || utf16EncodingByBOM(data) != "" {
		return true, nil
	}

	if bytes.Contains(data, []byte{0x00}) {
		return false, nil
	}

	result := d.DetectBytes(data)

	return result.Encoding != "", nil
}

// utf16EncodingByBOM はdataがUTF-16のBOMで始まる場合にそのエンコーディング名
// （"utf-16le"または"utf-16be"）を、それ以外の場合は空文字列を返す。
//
// why not: BOMだけで判定し、BOM無しのUTF-16を推定するヒューリスティックは使わない。
// NULを多く含むバイナリをUTF-16テキストと誤検出するリスクがあり、吉里吉里の
// UTF-16スクリプトはBOM付きで保存するのが慣例のため。
func utf16EncodingByBOM(data []byte) string {
	switch {
	case bytes.HasPrefix(data, utf32LEBOM):
		return ""
	case bytes.HasPrefix(data, utf16LEBOM):
		return "utf-16le"
	case bytes.HasPrefix(data, utf16BEBOM):
		return "utf-16be"
	default:
		return ""
	}
}

// EncodingConverter はテキストファイルの文字コードを変換するConverter。
//
// SourceEncodingが空文字列の場合は自動検出を行う。
type EncodingConverter struct {
	targetEncoding string
	sourceEncoding string
	detector       *EncodingDetector
}

// NewEncodingConverter はEncodingConverterを初期化する。
// targetEncodingが空文字列の場合は"utf-8"を使用する。sourceEncodingが空文字列の
// 場合は自動検出を行う。
func NewEncodingConverter(targetEncoding, sourceEncoding string) *EncodingConverter {
	targetEncoding = cmp.Or(targetEncoding, "utf-8")

	return &EncodingConverter{
		targetEncoding: targetEncoding,
		sourceEncoding: sourceEncoding,
		detector:       NewEncodingDetector(),
	}
}

// TargetEncoding は変換先の文字コードを返す。
func (c *EncodingConverter) TargetEncoding() string { return c.targetEncoding }

// SourceEncoding は変換元の文字コードを返す（空文字列の場合は自動検出）。
func (c *EncodingConverter) SourceEncoding() string { return c.sourceEncoding }

// SupportedExtensions は対応する拡張子の一覧を返す。
func (c *EncodingConverter) SupportedExtensions() []string {
	return []string{".ks", ".tjs", ".txt", ".csv", ".ini", ".asd"}
}

// kirikiriScriptExtensions は吉里吉里スクリプトファイルの拡張子一覧。
//
// why not: 変換先がUTF-8の場合は常にBOMを付与し、BOM無しUTF-8にはしない（ASCIIのみの
// 内容でも例外にしない。8a188fa）。吉里吉里がBOM無しテキストを読む文字コードは
// ビルドで決まり、TVP_TEXT_READ_ANSI_MBCSを定義したビルドはShift_JISで、定義しない
// ビルドは厳格なUTF-8で読んで不正なバイト列では例外を投げる（krkrsdl2
// external/krkrz/base/TextStream.cpp:33-37, :205-227）。UTF-8 BOMがあればどちらの
// 既定でもUTF-8として読まれる（同:181-184）。
var kirikiriScriptExtensions = []string{".ks", ".tjs", ".asd"}

func isKirikiriScriptExtension(ext string) bool {
	return containsString(kirikiriScriptExtensions, strings.ToLower(ext))
}

// GetOutputExtension は出力ファイルの拡張子を変更しないため常に空文字列を
// 返す（文字コード変換は拡張子を保持する）。
func (c *EncodingConverter) GetOutputExtension(_ string) string { return "" }

// CanConvert はfilePathが変換可能かを判定する。
// 拡張子がサポート対象であり、かつテキストファイルである場合にtrueを返す。
func (c *EncodingConverter) CanConvert(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	if !containsString(c.SupportedExtensions(), ext) {
		return false
	}

	if _, err := os.Stat(filePath); err != nil {
		return false
	}

	isText, err := c.detector.IsTextFile(filePath)
	if err != nil {
		return false
	}

	return isText
}

// Convert はsourceの文字コードを変換し、destへ出力する。
//
// 失敗はerrとして返す。変換元が存在しない・権限不足で確認できない場合は
// ErrSourceNotFound/ErrSourceUnreadableをErrPermanentFailureでラップして返し、
// それ以外の理由で確認できない場合はErrSourceUnreadableを再試行対象として返す。
// デコード/エンコードに失敗した場合（変換先にUTF-16を指定した場合を含む）は
// ErrEncodingConversionFailedをErrPermanentFailureでラップして返す。
// 読み込み・出力の失敗はOSのエラーを%wで保持し、再試行対象とする。
//
// errがnilのとき、Statusは変換元が変換先と同じエンコーディングで、UTF-8 BOMが
// 無く、吉里吉里スクリプト(.ks/.tjs/.asd)でもない場合に限りStatusSkippedとなり、
// それ以外はStatusSuccessとなる。既にUTF-8の吉里吉里スクリプトもUTF-8 BOMを
// 付与して書き出すためStatusSuccessとなる。変換先がUTF-8のとき、UTF-8 BOMは
// 吉里吉里スクリプトと、拡張子によらず変換元がUTF-16のファイルに付与する。
// 吉里吉里のsimple crypt形式は復号し、変換元UTF-16LEとして扱う。
func (c *EncodingConverter) Convert(source, dest string) (ConversionResult, error) {
	if err := ensureSourceExists(source); err != nil {
		return ConversionResult{SourcePath: source}, err
	}

	bytesBefore := getFileSize(source)

	data, err := os.ReadFile(source) //nolint:gosec // 存在確認済みの変換元ファイルを読む用途のため妥当
	if err != nil {
		return ConversionResult{SourcePath: source}, fmt.Errorf("変換元ファイルの読み込みに失敗しました: %w", err)
	}

	data, plan, err := c.prepareSource(data)
	if err != nil {
		return ConversionResult{SourcePath: source}, permanentError(fmt.Errorf("%w: %w", ErrEncodingConversionFailed, err))
	}

	targetNormalized := strings.ReplaceAll(strings.ToLower(c.targetEncoding), "-", "_")
	hasBOM := bytes.HasPrefix(data, utf8BOM)
	isKirikiriScript := isKirikiriScriptExtension(filepath.Ext(source))

	if plan.isOnly(c.targetEncoding) && !hasBOM && !isKirikiriScript {
		return ConversionResult{
			SourcePath:  source,
			DestPath:    dest,
			Status:      StatusSkipped,
			Message:     "既にターゲットエンコーディングです",
			BytesBefore: bytesBefore,
			BytesAfter:  bytesBefore,
		}, nil
	}

	if hasBOM {
		data = data[len(utf8BOM):]
	}

	resultBytes, sourceEncoding, convErr := convertEncoding(data, plan, c.targetEncoding)
	if convErr != nil {
		return ConversionResult{SourcePath: source}, permanentError(fmt.Errorf("%w: %w", ErrEncodingConversionFailed, convErr))
	}

	// 吉里吉里スクリプトファイル(.ks/.tjs/.asd)は、変換先がUTF-8の場合BOMを付与して
	// 読み手の既定エンコーディングに依存しないようにする（理由は
	// kirikiriScriptExtensionsを参照）。
	//
	// why not: UTF-16由来のファイルは拡張子によらずBOM無しにしない。元のUTF-16
	// ファイルはBOMで自己記述していたため、BOM無しUTF-8にすると読み手の既定
	// エンコーディング（forkごとに異なりうる）に依存する。BOMを保てばどの既定でも
	// UTF-8として読まれる。
	if (isKirikiriScript || isUTF16Encoding(sourceEncoding)) && targetNormalized == "utf_8" {
		resultBytes = append(append([]byte{}, utf8BOM...), resultBytes...)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return ConversionResult{SourcePath: source}, fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	if err := os.WriteFile(dest, resultBytes, 0o644); err != nil { //nolint:gosec // ビルド成果物の出力用途のため妥当な権限
		return ConversionResult{SourcePath: source}, fmt.Errorf("出力ファイルの書き込みに失敗しました: %w", err)
	}

	return ConversionResult{
		SourcePath:  source,
		DestPath:    dest,
		Status:      StatusSuccess,
		BytesBefore: bytesBefore,
		BytesAfter:  getFileSize(dest),
	}, nil
}

// ConvertBytes はバイトデータの文字コードを変換し、(変換後バイト列, 復号に使った
// ソースエンコーディング)を返す。
//
// デコード/エンコード失敗はerrとして返す。Convertと異なり
// ErrEncodingConversionFailed・ErrPermanentFailureでのラップは行わない。
func (c *EncodingConverter) ConvertBytes(data []byte) ([]byte, string, error) {
	data, plan, err := c.prepareSource(data)
	if err != nil {
		return nil, "", err
	}

	data = bytes.TrimPrefix(data, utf8BOM)

	return convertEncoding(data, plan, c.targetEncoding)
}

// prepareSource は変換元dataと、それを復号するエンコーディングの候補を返す。dataが
// 吉里吉里のsimple crypt形式であれば、復号したBOM付きUTF-16LEと"utf-16le"を返す。
func (c *EncodingConverter) prepareSource(data []byte) ([]byte, sourcePlan, error) {
	// why not: モードが対応済みかどうかに関わらず、FE FEで始まれば復号へ回し、
	// 未対応モードや壊れた形式はエラーにする。吉里吉里本体もそれらのテキストは
	// 読み込みエラーにするため、別エンコーディングとして推定し直して変換を続けても、
	// エンジンが読めないファイルを別の中身に書き換えるだけになる。
	if !bytes.HasPrefix(data, simpleCryptSignature) {
		return data, c.planSource(data), nil
	}

	decoded, err := decodeSimpleCrypt(data)
	if err != nil {
		return nil, sourcePlan{}, err
	}

	// why not: 変換元エンコーディングの指定より優先する。復号結果は常にUTF-16LEで
	// あり、指定に従うと復号済みのデータを別エンコーディングとして誤読する。
	return decoded, singleSource("utf-16le"), nil
}

// ambiguousConfidence は、多バイト系の推定をそれだけでは採用しない信頼度の上限。
//
// why not: 推定名だけで復号しない。chardetの多バイト系判定器は2バイト文字が10個
// 以下で不正な並びが無いと一律に10（0.1）を返し、頻出字が1つも無い場合も文字数に
// よらず10になる（saintfish/chardet multi_byte.go matchConfidence）。この値は
// 「判断材料が無い」ことしか示さず、短いShift_JISの"ｾｰﾌﾞ"や"name=ｱｲﾃﾑ"は
// gb-18030 0.1と推定され、GB18030で復号するとU+FFFDを含まない漢字の文字化けに
// なる。文として十分な長さのGBK・EUC-KR・Big5・EUC-JPの実測値はいずれも1.0
// だった。0.5はこの0.1と1.0の間に置いた境界で、2バイト文字20個に頻出字が1個
// だけの場合（0.48）のような根拠の薄い推定もこちらに含める。
const ambiguousConfidence = 0.5

// shiftJISLookalikeEncodings は、推定の信頼度が低いときにShift_JISとの取り違えを
// 疑う多バイト系エンコーディング。
//
// why not: utf-8は含めない。UTF-8は復号時に厳格に検証しており、正しいUTF-8の
// "猫"（chardetの信頼度0.8）もShift_JISとしてはU+FFFD無しに"迪ｫ"と復号できて
// しまうため、両方で復号してShift_JISを優先するとUTF-8のファイルを文字化けさせる。
// shift_jisも含めない。低信頼度でもshift_jisと推定されれば候補はShift_JISだけで、
// ほかの文字コードとは突き合わせない。そのため短いGBKの"中文测试"（shift_jis 0.1）は
// Shift_JISの文字化け"ﾖﾐﾎﾄｲ簗ﾔ"として成功し、"保存数据"（同0.1）は恒久的な失敗に
// なる。逆に候補が2つでもShift_JISでU+FFFDが出れば推定名を採るため、短いEUC-JPの
// "漢字"（gb18030 0.1）はGB18030の"戳机"になる。いずれも入力をCP932とみなす前提で
// 受け入れる帰結である。
var shiftJISLookalikeEncodings = []string{"euc-jp", "gb2312", "gb18030", "big5", "cp949"}

// sourcePlan は変換元を復号するエンコーディングの候補を優先順に持つ。
type sourcePlan struct {
	candidates []string
	// failure はすべての候補で復号できなかった場合のエラー文言。空文字列なら
	// 各候補の失敗だけを返す。
	failure string
}

func singleSource(enc string) sourcePlan {
	return sourcePlan{candidates: []string{enc}}
}

// isOnly は候補がencだけかを、大文字小文字と"-"/"_"の違いを無視して判定する。
func (p sourcePlan) isOnly(enc string) bool {
	key := func(name string) string { return strings.ReplaceAll(strings.ToLower(name), "-", "_") }

	return len(p.candidates) == 1 && key(p.candidates[0]) == key(enc)
}

// decode はdataを候補の順にUTF-8へ復号し、最初に復号できた結果とその候補を返す。
func (p sourcePlan) decode(data []byte) ([]byte, string, error) {
	errs := make([]error, 0, len(p.candidates))

	for _, enc := range p.candidates {
		decoded, err := decodeToUTF8(data, enc)
		if err == nil {
			return decoded, enc, nil
		}

		errs = append(errs, fmt.Errorf("ソースエンコーディング%sのデコードに失敗しました: %w", enc, err))
	}

	if p.failure == "" {
		return nil, "", errors.Join(errs...)
	}

	return nil, "", fmt.Errorf("%s: %w", p.failure, errors.Join(errs...))
}

func (c *EncodingConverter) planSource(data []byte) sourcePlan {
	if c.sourceEncoding != "" {
		return singleSource(c.sourceEncoding)
	}

	detection := c.detector.DetectBytes(data)

	switch {
	case detection.Encoding == "":
		return singleSource("utf-8")
	case !detection.IsSupported && utf8.Valid(data):
		// why not: 未対応の推定でも、有効なUTF-8はShift_JISとして読まない。chardetの
		// UTF-8判定器は非ASCIIが3文字以下だと0.8止まりで、単バイト系の推定に負ける
		// （UTF-8の"title=名前"はwindows-1252 0.9）。Shift_JISの復号器はこのバイト列を
		// U+FFFD無しに"title=蜷榊燕"と復号してしまい、正しいUTF-8のファイルを黙って
		// 文字化けさせる。
		return singleSource("utf-8")
	case !detection.IsSupported:
		// why not: chardetが対応外の文字コード名を返したBOM無しテキストは、そのまま
		// 残すことも失敗にすることもせず、Shift_JISとして復号する。
		// 残さない理由: 変換先のエンジン（krkrsdl2 android-support）は
		// TVP_TEXT_READ_ANSI_MBCSを定義せずにビルドされ、BOM無しテキストを厳格な
		// UTF-8として読み、不正なバイト列で例外を投げる（krkrsdl2
		// external/krkrz/base/TextStream.cpp:33-37, :205-227）。未変換のShift_JISは
		// 読み込み時に落ちる。
		// Shift_JISとする理由: ここに来るのは有効なUTF-8ではないBOM無しテキストで、
		// 元のWindows版エンジンがそれを読めたのはANSIコードページで読む経路だけである。
		// 吉里吉里2はBOM無しテキストを常にANSIコードページで読み（krkr2@dec49af
		// kirikiri2/branches/2.32stable/kirikiri2/src/core/base/TextStream.cpp:146-158
		// からtjs2/tjsConfig.h:99-100、tjsConfig.cpp:208-245の
		// MultiByteToWideChar(CP_ACP)）、吉里吉里ZはTVP_TEXT_READ_ANSI_MBCSを定義した
		// ビルドか-readencoding=Shift_JISの指定があるときだけShift_JISで読む（上記
		// TextStream.cpp:33-37, :220-224、同base/ScriptMgnIntf.cpp:141-147）。日本語版
		// WindowsのANSIコードページはCP932である。
		// 失敗にしない理由: 短いテキストではchardetがShift_JISをwindows-1252や
		// iso-8859-1と推定しやすく（"[config]\ntitle=猫"はwindows-1252 0.75）、
		// 失敗にすると正しいゲームがchardetの弱さだけでビルドできなくなる。
		// Shift_JISでもU+FFFDが出る内容はdecodeToUTF8が失敗にする。
		return sourcePlan{
			candidates: []string{"shift_jis"},
			failure:    fmt.Sprintf("検出結果%sは未対応で、Shift_JISとしても復号できませんでした", detection.Encoding),
		}
	case detection.Confidence < ambiguousConfidence && slices.Contains(shiftJISLookalikeEncodings, detection.Encoding):
		// 両方で復号できる場合は、上の未対応時と同じ理由（元のエンジンが有効な
		// UTF-8でないBOM無しテキストを読む経路はCP932だけ）でShift_JISを優先する。
		return sourcePlan{
			candidates: []string{"shift_jis", detection.Encoding},
			failure: fmt.Sprintf("検出結果%s（信頼度%.2f）とShift_JISのいずれとしても復号できませんでした",
				detection.Encoding, detection.Confidence),
		}
	default:
		return singleSource(detection.Encoding)
	}
}

// convertEncoding はdataをplanの候補からUTF-8を経由してtargetEncodingへ変換し、
// (変換結果, 復号に使った変換元エンコーディング)を返す。
func convertEncoding(data []byte, plan sourcePlan, targetEncoding string) ([]byte, string, error) {
	utf8Bytes, sourceEncoding, err := plan.decode(data)
	if err != nil {
		return nil, "", err
	}

	encoded, err := encodeFromUTF8(utf8Bytes, targetEncoding)
	if err != nil {
		return nil, "", fmt.Errorf("ターゲットエンコーディング%sへのエンコードに失敗しました: %w", targetEncoding, err)
	}

	return encoded, sourceEncoding, nil
}

func decodeToUTF8(data []byte, sourceEncoding string) ([]byte, error) {
	if normalizeEncoding(sourceEncoding) == "utf-8" {
		if !utf8.Valid(data) {
			return nil, errors.New("不正なUTF-8バイト列です")
		}

		return data, nil
	}

	enc, err := encodingByName(sourceEncoding)
	if err != nil {
		return nil, err
	}

	decoded, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		return nil, err
	}

	// why not: 復号器のエラーだけに頼らない。x/textの多バイト系復号器は不正な
	// バイト列でもエラーを返さずU+FFFDに置き換える（japanese/shiftjis.go、
	// simplifiedchinese/gbk.goなど）ため、別の文字コードのテキストをShift_JISや
	// GB18030として読んだ結果が黙って成功し、文字化けを書き出してしまう。変換元に
	// U+FFFDそのものが符号化されている場合（GB18030は符号化できる）も失敗になるが、
	// ゲームのテキストには現れないものとして受け入れる。
	// UTF-16は対象外: BOMかsimple crypt形式で自己記述しており、chardetの推定を
	// 経由しないため別の文字コードと取り違えることがない。
	if !isUTF16Encoding(sourceEncoding) && bytes.Contains(decoded, replacementCharacter) {
		return nil, fmt.Errorf("%w: %s", ErrUndecodableSource, sourceEncoding)
	}

	return decoded, nil
}

// isUTF16Encoding はエンコーディング名encが（別名の正規化後に）utf-16le/utf-16beかを返す。
func isUTF16Encoding(enc string) bool {
	normalized := normalizeEncoding(enc)

	return normalized == "utf-16le" || normalized == "utf-16be"
}

func encodeFromUTF8(data []byte, targetEncoding string) ([]byte, error) {
	if normalizeEncoding(targetEncoding) == "utf-8" {
		return data, nil
	}

	// why not: encodingByNameが復号用に返すUTF-16実装を符号化にも流用すると、
	// 変換先UTF-16の出力を黙って受け付けてしまう。出力はUTF-8に揃えており
	// UTF-16で書き出す用途は無いため、変換先としては未対応エラーにする。
	if isUTF16Encoding(targetEncoding) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedEncoding, targetEncoding)
	}

	enc, err := encodingByName(targetEncoding)
	if err != nil {
		return nil, err
	}

	return enc.NewEncoder().Bytes(data)
}

// encodingByName はSupportedEncodings（utf-8を除く）に対応するx/text/encoding実装を返す。
//
// why not: "gb2312"はx/text/encoding/simplifiedchineseに専用の実装が無いため、
// バイト範囲が互換なGBK（GB2312のスーパーセット）で代替する。"gb18030"をこのGBKへ
// 寄せないのは、GBKの復号器が4バイト符号を扱わずU+FFFDにするため（x/text
// simplifiedchinese/gbk.goのgb18030フラグ）。"cp949"はx/text側で
// EUCKRという名称だが、Code Page 949そのものを指す実装であるためcp949に直接対応する。
// UTF-16はUseBOMの復号器を返すため、先頭のBOMは復号時に取り除かれ、BOMがあれば
// そのバイト順が名前のバイト順より優先される。
func encodingByName(name string) (encoding.Encoding, error) {
	switch normalizeEncoding(name) {
	case "shift_jis":
		return japanese.ShiftJIS, nil
	case "euc-jp":
		return japanese.EUCJP, nil
	case "gb2312":
		return simplifiedchinese.GBK, nil
	case "gb18030":
		return simplifiedchinese.GB18030, nil
	case "big5":
		return traditionalchinese.Big5, nil
	case "cp949":
		return korean.EUCKR, nil
	case "utf-16le":
		return unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), nil
	case "utf-16be":
		return unicode.UTF16(unicode.BigEndian, unicode.UseBOM), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedEncoding, name)
	}
}

func containsString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}

	return false
}
