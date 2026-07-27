package converter

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// ErrUnsupportedEncoding はencodingByNameが未対応のエンコーディング名を受け取った場合のエラー。
var ErrUnsupportedEncoding = errors.New("サポートされていないエンコーディングです")

// ErrEncodingFileNotFound はEncodingDetectorの検出対象ファイルが存在しない場合のエラー。
var ErrEncodingFileNotFound = errors.New("ファイルが見つかりません")

// utf8BOM はUTF-8のバイトオーダーマーク。
var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// SupportedEncodings はEncodingConverterが変換元として認識するエンコーディング名の一覧。
var SupportedEncodings = []string{"shift_jis", "euc-jp", "utf-8", "gb2312", "big5", "cp949"}

// encodingAliases はchardetが返すエンコーディング名とSupportedEncodingsの対応マッピング。
//
// why not: "utf-8-sig"のエイリアスはPython版chardetがBOM付きUTF-8に対して
// 返す名称を吸収するために存在する。github.com/saintfish/chardetはBOMの
// 有無に関わらず常に"UTF-8"を返す（PR3のchardet移植と同様の語彙差）ため
// Go側でこのエイリアスが実際に引かれることはないが、Python版との対応表を
// 明示するため、また将来chardet実装が変わった場合の防御として残す。
var encodingAliases = map[string]string{
	"shift-jis": "shift_jis",
	"shiftjis":  "shift_jis",
	"sjis":      "shift_jis",
	"euc_jp":    "euc-jp",
	"eucjp":     "euc-jp",
	"utf8":      "utf-8",
	"utf-8-sig": "utf-8",
	"ascii":     "utf-8",
}

// isASCII はdataが7ビットASCII（0x00〜0x7F）のみで構成されているかを判定する。
//
// why not: ESC(0x1B)はISO-2022-JP等7bitエンコーディングの制御文字であり、
// これを含む入力をASCIIと即断するとchardetの正しいISO-2022-JP判定を
// 潰してしまうため除外する（internal/parser/detector.goのisASCIIと同じ理由）。
func isASCII(data []byte) bool {
	for _, b := range data {
		if b >= 0x80 || b == 0x1b {
			return false
		}
	}

	return true
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
	for _, supported := range SupportedEncodings {
		if normalized == strings.ToLower(strings.ReplaceAll(supported, "_", "-")) {
			return true
		}
	}

	return false
}

// EncodingDetectionResult は文字コード検出結果を表す不変値。
//
// Encodingが空文字列の場合、Python版のNone（検出できなかった場合）に相当する。
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
func (d *EncodingDetector) DetectBytes(data []byte) EncodingDetectionResult {
	if len(data) == 0 {
		return EncodingDetectionResult{Encoding: "", Confidence: 0.0, IsSupported: false}
	}

	// why not: github.com/saintfish/chardetには専用のASCII判定器が無く、純ASCII
	// バイト列に対しても単バイト系のフォールバック候補（例: "ISO-8859-1"、低信頼度）
	// を返すことがある（internal/parser/detector.goのdetectCharsetと同じ既知差異）。
	// Python版chardetは全バイトが0x7F以下の場合に専用の高速パスで必ず"ascii"を
	// 返し、_ENCODING_ALIASESでutf-8に正規化される。この差を放置すると、
	// ASCIIのみの.ini/.txt/.ks/.csvがGo版ではsupportedEncodingsに含まれない
	// エンコーディング名として検出されConvertがFAILEDを返してしまう
	// （Python版はSKIPPEDになる）。そのためGo側でも同じ判定を先に行い、
	// chardetの推定より優先する。ESC(0x1B)はISO-2022-JP等7bitエンコーディングの
	// 制御文字であり、これを含む入力をASCII短絡させるとchardetの正しい
	// ISO-2022-JP判定を潰してしまうため除外する。
	if isASCII(data) {
		return EncodingDetectionResult{Encoding: "utf-8", Confidence: 1.0, IsSupported: true}
	}

	result, err := chardet.NewTextDetector().DetectBest(data)

	var (
		rawEncoding string
		confidence  float64
	)

	if err == nil && result != nil {
		rawEncoding = result.Charset
		confidence = float64(result.Confidence) / 100.0
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

	if bytes.Contains(data, []byte{0x00}) {
		return false, nil
	}

	result := d.DetectBytes(data)

	return result.Encoding != "", nil
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
	if targetEncoding == "" {
		targetEncoding = "utf-8"
	}

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
// why not: これらのファイルはUTF-8 BOMが無いとKirikiriZ側でShift_JISとして
// 誤解釈されるため、変換先がUTF-8の場合は常にBOMを付与する必要がある
// （ASCIIのみの内容でも例外にしない。8a188fa）。
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
// Python版と同様、既知の失敗（ファイル未存在・デコード/エンコード失敗）は
// ConversionResult{Status: StatusFailed}として返し、errは常にnilとなる
// （Python版がこれらをConversionResult返却で処理し例外を送出しないため）。
func (c *EncodingConverter) Convert(source, dest string) (ConversionResult, error) {
	if _, err := os.Stat(source); err != nil {
		return ConversionResult{
			SourcePath: source,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("変換元ファイルが見つかりません: %s", source),
		}, nil
	}

	bytesBefore := getFileSize(source)

	data, err := os.ReadFile(source) //nolint:gosec // 存在確認済みの変換元ファイルを読む用途のため妥当
	if err != nil {
		return ConversionResult{}, fmt.Errorf("変換元ファイルの読み込みに失敗しました: %w", err)
	}

	sourceEncoding := c.resolveSourceEncoding(data)

	targetNormalized := strings.ReplaceAll(strings.ToLower(c.targetEncoding), "-", "_")
	sourceNormalized := strings.ReplaceAll(strings.ToLower(sourceEncoding), "-", "_")
	hasBOM := bytes.HasPrefix(data, utf8BOM)
	isKirikiriScript := isKirikiriScriptExtension(filepath.Ext(source))

	if sourceNormalized == targetNormalized && !hasBOM && !isKirikiriScript {
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

	resultBytes, convErr := convertEncoding(data, sourceEncoding, c.targetEncoding)
	if convErr != nil {
		return ConversionResult{
			SourcePath:  source,
			Status:      StatusFailed,
			Message:     fmt.Sprintf("エンコーディング変換に失敗しました: %v", convErr),
			BytesBefore: bytesBefore,
		}, nil
	}

	// 吉里吉里スクリプトファイル(.ks/.tjs/.asd)はUTF-8 BOMが無いとShift_JISとして
	// 誤解釈されるため、変換先がUTF-8の場合はBOMを付与する。
	if isKirikiriScript && targetNormalized == "utf_8" {
		resultBytes = append(append([]byte{}, utf8BOM...), resultBytes...)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return ConversionResult{}, fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	if err := os.WriteFile(dest, resultBytes, 0o644); err != nil { //nolint:gosec // ビルド成果物の出力用途のため妥当な権限
		return ConversionResult{}, fmt.Errorf("出力ファイルの書き込みに失敗しました: %w", err)
	}

	return ConversionResult{
		SourcePath:  source,
		DestPath:    dest,
		Status:      StatusSuccess,
		BytesBefore: bytesBefore,
		BytesAfter:  getFileSize(dest),
	}, nil
}

// ConvertBytes はバイトデータの文字コードを変換し、(変換後バイト列, 検出されたソース
// エンコーディング)を返す。
//
// Python版のconvert_bytesと同様、デコード/エンコード失敗はerrとして伝播する
// （Convertと異なりこのメソッドはPython版でも例外を捕捉しないため）。
func (c *EncodingConverter) ConvertBytes(data []byte) ([]byte, string, error) {
	sourceEncoding := c.resolveSourceEncoding(data)

	data = bytes.TrimPrefix(data, utf8BOM)

	resultBytes, err := convertEncoding(data, sourceEncoding, c.targetEncoding)
	if err != nil {
		return nil, "", err
	}

	return resultBytes, sourceEncoding, nil
}

func (c *EncodingConverter) resolveSourceEncoding(data []byte) string {
	if c.sourceEncoding != "" {
		return c.sourceEncoding
	}

	detection := c.detector.DetectBytes(data)
	if detection.Encoding == "" {
		return "utf-8"
	}

	return detection.Encoding
}

// convertEncoding はdataをsourceEncodingからUTF-8を経由してtargetEncodingへ変換する。
func convertEncoding(data []byte, sourceEncoding, targetEncoding string) ([]byte, error) {
	utf8Bytes, err := decodeToUTF8(data, sourceEncoding)
	if err != nil {
		return nil, fmt.Errorf("ソースエンコーディング%sのデコードに失敗しました: %w", sourceEncoding, err)
	}

	encoded, err := encodeFromUTF8(utf8Bytes, targetEncoding)
	if err != nil {
		return nil, fmt.Errorf("ターゲットエンコーディング%sへのエンコードに失敗しました: %w", targetEncoding, err)
	}

	return encoded, nil
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

	return enc.NewDecoder().Bytes(data)
}

func encodeFromUTF8(data []byte, targetEncoding string) ([]byte, error) {
	if normalizeEncoding(targetEncoding) == "utf-8" {
		return data, nil
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
// バイト範囲が互換なGBK（GB2312のスーパーセット）で代替する。"cp949"はx/text側で
// EUCKRという名称だが、Code Page 949そのものを指す実装であるためcp949に直接対応する。
func encodingByName(name string) (encoding.Encoding, error) {
	switch normalizeEncoding(name) {
	case "shift_jis":
		return japanese.ShiftJIS, nil
	case "euc-jp":
		return japanese.EUCJP, nil
	case "gb2312":
		return simplifiedchinese.GBK, nil
	case "big5":
		return traditionalchinese.Big5, nil
	case "cp949":
		return korean.EUCKR, nil
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
