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
	"cmp"
	"slices"
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

	// why not: DetectBestは使わない。chardetのDetectAll（detector.go）は各判定器を
	// goroutineで並行に走らせて到着順に結果を集め、安定でないsort.Sortで信頼度の
	// 降順に並べるだけなので、同じ信頼度の候補のどれが先頭になるかは実行ごとに
	// 変わり、DetectBestはその先頭をそのまま返す。短いテキストでは多バイト系判定器が
	// 揃って信頼度10を返しやすく、同じShift_JISの"ｾｰﾌﾞ"を300回判定すると
	// gb-18030のほかbig5/euc-jp/euc-krがそれぞれ数十回ずつ返った。全候補を受け取り
	// BestResultの全順序で先頭を選ぶ。
	results, err := detector.DetectAll(data)
	if err != nil {
		return "", false
	}

	best, ok := BestResult(results)
	if !ok || best.Charset == "" {
		return "", false
	}

	return strings.ToLower(best.Charset), true
}

// multiByteTieBreakOrder は同じ信頼度の多バイト系候補を並べる順序で、chardetが判定器を
// 登録している順序（detector.goのrecognizers）に合わせる。
var multiByteTieBreakOrder = []string{"Shift_JIS", "GB-18030", "EUC-JP", "EUC-KR", "Big5"}

// BestResult はchardetの候補resultsから、信頼度の降順、同じ信頼度なら
// Shift_JIS・GB-18030・EUC-JP・EUC-KR・Big5の順、それ以外は名前の昇順で先頭の
// 候補を選ぶ。選ぶ候補の文字コード名と信頼度はresultsの並び順に依存しない。
// resultsが空ならok=false。
func BestResult(results []chardet.Result) (chardet.Result, bool) {
	if len(results) == 0 {
		return chardet.Result{}, false
	}

	return slices.MinFunc(results, compareCandidates), true
}

func compareCandidates(a, b chardet.Result) int {
	return cmp.Or(
		cmp.Compare(b.Confidence, a.Confidence),
		cmp.Compare(tieBreakRank(a.Charset), tieBreakRank(b.Charset)),
		strings.Compare(a.Charset, b.Charset),
	)
}

func tieBreakRank(charsetName string) int {
	if i := slices.Index(multiByteTieBreakOrder, charsetName); i >= 0 {
		return i
	}

	return len(multiByteTieBreakOrder)
}
