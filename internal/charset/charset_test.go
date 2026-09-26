package charset_test

import (
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
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, ok := charset.Detect(chardet.NewTextDetector(), tc.data)

			assert.Equal(t, tc.expectedOK, ok)
			assert.Equal(t, tc.expectedName, got)
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
