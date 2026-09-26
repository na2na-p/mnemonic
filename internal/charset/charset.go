// Package charset はテキストの文字コード推定を提供する。
//
// why not: github.com/saintfish/chardetには専用のASCII判定器がなく、純ASCII
// バイト列に対しても単バイト系のフォールバック候補（例: "ISO-8859-1"、低信頼度）
// を返す。推定された文字コード名は利用側でそのままユーザーに表示される
// （例: mnemonic infoのEncoding行）ため、純ASCIIのファイルが"iso-8859-1"の
// ような候補名で表示されてしまう。そのためchardetに渡す前に純ASCIIかを判定し、
// chardetの推定より優先して常に"ascii"を返す。
package charset

import (
	"strings"

	"github.com/saintfish/chardet"
)

// IsASCII はdataが7ビットASCII（0x00〜0x7F）のみで構成され、かつESC(0x1B)を含まないかを判定する。
//
// why not: ISO-2022-JPは7ビットのみで構成されるため、ESC(0x1B)を含む入力を
// ASCIIと即断するとchardetの正しいISO-2022-JP判定を潰してしまう。
func IsASCII(data []byte) bool {
	for _, b := range data {
		if b >= 0x80 || b == 0x1b {
			return false
		}
	}

	return true
}

// Detect はdataの文字コードを推定し、小文字の名前で返す。純ASCIIなら"ascii"。
// 推定できない場合はok=false。
func Detect(detector *chardet.Detector, data []byte) (name string, ok bool) {
	if IsASCII(data) {
		return "ascii", true
	}

	result, err := detector.DetectBest(data)
	if err != nil || result == nil || result.Charset == "" {
		return "", false
	}

	return strings.ToLower(result.Charset), true
}
