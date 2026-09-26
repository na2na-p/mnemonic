package charset_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/saintfish/chardet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/japanese"

	"github.com/na2na-p/mnemonic/internal/charset"
)

func TestIsASCII(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		data     []byte
		expected bool
	}{
		"正常系: 純ASCII文字列":    {[]byte("key=value\n"), true},
		"正常系: 空バイト列":        {[]byte{}, true},
		"異常系: 0x80以上を含む":    {[]byte("caf\xe9"), false},
		"異常系: ESC(0x1B)を含む": {append([]byte{0x1b}, []byte("$B$3$s(B")...), false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, charset.IsASCII(tc.data))
		})
	}
}

func TestDetect(t *testing.T) {
	t.Parallel()

	japaneseText := strings.Repeat(
		"これはKirikiriのシナリオスクリプトのサンプルです。日本語のテキストを含みます。", 3,
	)
	shiftJIS, err := japanese.ShiftJIS.NewEncoder().String(japaneseText)
	require.NoError(t, err)

	cases := map[string]struct {
		data         []byte
		expectedName string
		expectedOK   bool
	}{
		"正常系: 純ASCIIはchardetより優先してasciiと判定される": {
			[]byte("@bg storage=bg01\nvar x = 1;\n"), "ascii", true,
		},
		"正常系: 空バイト列は純ASCIIとしてasciiと判定される": {
			[]byte{}, "ascii", true,
		},
		"正常系: Shift_JISの日本語は小文字のshift_jisと判定される": {
			[]byte(shiftJIS), "shift_jis", true,
		},
		"正常系: UTF-8の日本語はutf-8と判定される": {
			[]byte(japaneseText), "utf-8", true,
		},
		"正常系: 多バイト系候補が同じ信頼度で並ぶ短いShift_JISのｾｰﾌﾞはgb-18030と判定される": {
			encodeShiftJIS(t, "ｾｰﾌﾞ"), "gb-18030", true,
		},
		"正常系: 多バイト系候補が同じ信頼度で並ぶ短いShift_JISのname=ｱｲﾃﾑはgb-18030と判定される": {
			encodeShiftJIS(t, "name=ｱｲﾃﾑ"), "gb-18030", true,
		},
		"正常系: 多バイト系候補が同じ信頼度で並ぶ復号不能な短いバイト列はgb-18030と判定される": {
			[]byte{0x6b, 0x3d, 0xca, 0xea, 0xba}, "gb-18030", true,
		},
		"異常系: chardetが推定できないバイト列はok=false": {
			[]byte{0xff}, "", false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			detector := chardet.NewTextDetector()

			// why: chardetは同じ信頼度の候補の順序が実行ごとに変わるため、1回の
			// 一致だけでは推定結果が決定的であることを示せない。
			for range 50 {
				got, ok := charset.Detect(detector, tc.data)

				require.Equal(t, tc.expectedOK, ok)
				require.Equal(t, tc.expectedName, got)
			}
		})
	}

	t.Run("正常系: ISO-2022-JPはESCを含むためasciiと判定されない", func(t *testing.T) {
		t.Parallel()

		// 「こんにちは」のISO-2022-JP表現。全バイトが7ビットだがESC(0x1B)を含む。
		iso2022jp := []byte("\x1b$B$3$s$K$A$O\x1b(B")

		got, ok := charset.Detect(chardet.NewTextDetector(), iso2022jp)

		assert.True(t, ok)
		assert.NotEqual(t, "ascii", got)
	})
}

func TestBestResult(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		results  []chardet.Result
		expected chardet.Result
		ok       bool
	}{
		"正常系: 信頼度が最も高い候補を選ぶ": {
			results: []chardet.Result{
				{Charset: "Shift_JIS", Confidence: 10},
				{Charset: "Big5", Confidence: 30},
				{Charset: "ISO-8859-1", Confidence: 20},
			},
			expected: chardet.Result{Charset: "Big5", Confidence: 30},
			ok:       true,
		},
		"正常系: 同じ信頼度の多バイト系候補はShift_JIS・GB-18030・EUC-JP・EUC-KR・Big5の順で先頭を選ぶ": {
			results: []chardet.Result{
				{Charset: "Big5", Confidence: 10},
				{Charset: "EUC-KR", Confidence: 10},
				{Charset: "EUC-JP", Confidence: 10},
				{Charset: "GB-18030", Confidence: 10},
				{Charset: "Shift_JIS", Confidence: 10},
			},
			expected: chardet.Result{Charset: "Shift_JIS", Confidence: 10},
			ok:       true,
		},
		"正常系: Shift_JISが無ければ同じ信頼度のGB-18030をEUC-JP・EUC-KR・Big5より優先する": {
			results: []chardet.Result{
				{Charset: "Big5", Confidence: 10},
				{Charset: "EUC-KR", Confidence: 10},
				{Charset: "EUC-JP", Confidence: 10},
				{Charset: "GB-18030", Confidence: 10},
			},
			expected: chardet.Result{Charset: "GB-18030", Confidence: 10},
			ok:       true,
		},
		"正常系: 同じ信頼度なら多バイト系候補をそれ以外の候補より優先する": {
			results: []chardet.Result{
				{Charset: "ISO-8859-1", Confidence: 10},
				{Charset: "Shift_JIS", Confidence: 10},
			},
			expected: chardet.Result{Charset: "Shift_JIS", Confidence: 10},
			ok:       true,
		},
		"正常系: 同じ信頼度の多バイト系以外の候補は名前の昇順で先頭を選ぶ": {
			results: []chardet.Result{
				{Charset: "windows-1252", Confidence: 10},
				{Charset: "ISO-8859-1", Confidence: 10},
			},
			expected: chardet.Result{Charset: "ISO-8859-1", Confidence: 10},
			ok:       true,
		},
		"異常系: 空の候補はok=falseになる": {
			results: []chardet.Result{},
		},
		"異常系: nilの候補はok=falseになる": {
			results: nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// why: chardetのDetectAllは同じ信頼度の候補を実行ごとに異なる順で返すため、
			// 入力の並びを変えても同じ候補が選ばれることを確認する。
			for _, results := range orderings(tc.results) {
				got, ok := charset.BestResult(results)

				require.Equal(t, tc.ok, ok, "入力順: %v", results)
				require.Equal(t, tc.expected, got, "入力順: %v", results)
			}
		})
	}
}

// orderings はresultsの全ローテーションとその逆順を返す。各候補が先頭と末尾の
// 両方に来る並びを含む。
func orderings(results []chardet.Result) [][]chardet.Result {
	if len(results) == 0 {
		return [][]chardet.Result{results}
	}

	reversed := slices.Clone(results)
	slices.Reverse(reversed)

	var all [][]chardet.Result
	for _, base := range [][]chardet.Result{results, reversed} {
		for i := range len(base) {
			all = append(all, slices.Concat(base[i:], base[:i]))
		}
	}

	return all
}

func encodeShiftJIS(t *testing.T, text string) []byte {
	t.Helper()

	encoded, err := japanese.ShiftJIS.NewEncoder().String(text)
	require.NoError(t, err)

	return []byte(encoded)
}
