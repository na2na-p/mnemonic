package converter

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
)

func TestQuoteTJSString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "空文字列は空のリテラルになる", in: "", want: `""`},
		{name: "ASCIIはそのまま囲む", in: "Ver.1.05", want: `"Ver.1.05"`},
		{name: "二重引用符をエスケープする", in: `say "hi"`, want: `"say \"hi\""`},
		{name: "バックスラッシュをエスケープする", in: `C:\game\`, want: `"C:\\game\\"`},
		{name: "改行を\\nにする", in: "a\nb", want: `"a\nb"`},
		{name: "タブを\\tにする", in: "a\tb", want: `"a\tb"`},
		{name: "日本語はそのまま残す", in: "バージョン情報", want: `"バージョン情報"`},
		{name: "U+FFFFまでの文字はそのまま残す", in: "�！", want: "\"�！\""},
		{name: "U+20022はサロゲートペアの\\xにする", in: "\U00020022", want: `"\xD840\xDC22"`},
		{name: "U+2005Cはサロゲートペアの\\xにする", in: "\U0002005C", want: `"\xD840\xDC5C"`},
		{name: "U+20000はサロゲートペアの\\xにする", in: "\U00020000", want: `"\xD840\xDC00"`},
		{name: "絵文字はサロゲートペアの\\xにする", in: "a\U0001F600b", want: `"a\xD83D\xDE00b"`},
		{name: "\\xの直後の16進数字は4桁の後ろに続く別の文字として残す", in: "\U0001F600A0", want: `"\xD83D\xDE00A0"`},
		{name: "単一引用符とドル記号とアンパサンドはそのまま残す", in: "it's $1 & ${x}", want: `"it's $1 & ${x}"`},
		{name: "改行とタブ以外の制御文字は取り除く", in: "a\rb\x00c\x1fd\x7fe", want: `"abcde"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, quoteTJSString(tt.in))
		})
	}
}

// decodeTJSLiteralForTest はTJS2の字句解析(tjsLex.cppのTJSInternalParseString)が
// 二重引用符文字列を読む手順のうち、quoteTJSStringが出力するエスケープだけを
// 再現してUTF-16の符号単位列へ戻す。tjs_charがchar16_tのビルド(Android)と
// 同じく、UTF-8の4バイト文字は読み込み時(CharacterSet.cppのTVPUtf8ToWideChar)に
// 下位16ビットへ切り詰め、\xは16進数字を最大4桁まで読む。
func decodeTJSLiteralForTest(t *testing.T, literal string) []uint16 {
	t.Helper()

	var units []uint16
	for _, r := range literal {
		units = append(units, uint16(r)) //nolint:gosec // エンジンの切り詰めを再現する
	}
	if len(units) < 2 || units[0] != '"' || units[len(units)-1] != '"' {
		t.Fatalf("二重引用符で囲まれていない: %q", literal)
	}

	units = units[1 : len(units)-1]

	var out []uint16

	for i := 0; i < len(units); i++ {
		if units[i] != '\\' {
			if units[i] == '"' {
				t.Fatalf("エスケープされていない二重引用符がある: %q", literal)
			}

			out = append(out, units[i])

			continue
		}

		i++
		if i == len(units) {
			t.Fatalf("\\で終わっている: %q", literal)
		}

		switch units[i] {
		case 'x':
			code, count := uint16(0), 0
			for i+1 < len(units) && count < 4 {
				d := strings.IndexRune("0123456789abcdef", unicode.ToLower(rune(units[i+1])))
				if d < 0 {
					break
				}

				code = code*16 + uint16(d)
				count++
				i++
			}

			out = append(out, code)
		case 'n':
			out = append(out, '\n')
		case 't':
			out = append(out, '\t')
		default:
			out = append(out, units[i])
		}
	}

	return out
}

func TestQuoteTJSString_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "サロゲートペアの直後に16進数字が続いても元の文字列に戻る", in: "\U0001F600A0\U00020022F"},
		{name: "U+20022の直後の二重引用符も元の文字列に戻る", in: "\U00020022\"\\\n\t"},
		{name: "日本語と記号が混ざっても元の文字列に戻る", in: "バージョン情報 $1 & \U0002005C"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := decodeTJSLiteralForTest(t, quoteTJSString(tt.in))
			assert.Equal(t, tt.in, string(utf16.Decode(got)))
		})
	}
}
