package converter

import (
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isSupportedEncodingはパッケージ非公開ヘルパーであり、chardetの推定結果に依存せず
// 任意のエンコーディング名を与えて判定を検証するため、ホワイトボックステストとする。
func TestIsSupportedEncoding(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		enc  string
		want bool
	}{
		"正常系: SupportedEncodingsに無い名前は非対応と判定する": {"klingon", false},
		"正常系: 大文字のEUC-JPは対応と判定する":               {"EUC-JP", true},
		"正常系: 大文字のUTF-16LEは対応と判定する":             {"UTF-16LE", true},
		"正常系: 大文字小文字の混ざったBig5は対応と判定する":          {"Big5", true},
		"異常系: 正式名に無い区切りを入れたbig-5は非対応と判定する":      {"big-5", false},
		"異常系: 正式名に無い区切りを入れたgb-2312は非対応と判定する":    {"gb-2312", false},
		"異常系: 正式名に無い区切りを入れたcp-949は非対応と判定する":     {"cp-949", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isSupportedEncoding(tc.enc))
		})
	}
}

// 検証が受け付ける別名が、正式名と綴りの違いしかないものを除いてどれも
// DescribeSelectableSourceEncodingsの一覧で、解決先の正式名の括弧内に載ることを
// 確かめる。別名の値の綴りが正式名と違っても載ることを含む。
func TestDescribeSelectableSourceEncodings_ListsEveryAcceptedAlias(t *testing.T) {
	t.Parallel()

	listed := make(map[string][]string)
	group := regexp.MustCompile(`([^,\s（）]+)(?:（([^）]*)）)?`)
	for _, m := range group.FindAllStringSubmatch(DescribeSelectableSourceEncodings(), -1) {
		listed[m[1]] = nil
		if m[2] != "" {
			listed[m[1]] = strings.Split(m[2], ", ")
		}
	}

	for _, alias := range slices.Sorted(maps.Keys(encodingAliases)) {
		if !IsSelectableSourceEncoding(alias) {
			continue
		}

		canonical, ok := canonicalEncoding(alias)
		require.True(t, ok)
		if encodingKey(alias) == encodingKey(canonical) {
			continue
		}

		t.Run("正常系: 別名"+alias+"は"+canonical+"の別名として一覧に載る", func(t *testing.T) {
			t.Parallel()

			require.Contains(t, listed, canonical)
			assert.Contains(t, listed[canonical], alias)
		})
	}
}

// 一覧に載せる別名がどれも実際に同じ文字コードとして復号されることを、
// 非公開のencodingAliasesから選んだ結果そのものに対して確かめる。
func TestSourceEncodingAliases(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, sourceEncodingAliases("shift_jis"))

	for _, canonical := range SelectableSourceEncodings {
		for _, alias := range sourceEncodingAliases(canonical) {
			t.Run("正常系: "+canonical+"の別名"+alias+"は同じ文字コードとして復号できる", func(t *testing.T) {
				t.Parallel()

				got, ok := canonicalEncoding(alias)
				require.True(t, ok)
				assert.Equal(t, canonical, got)
				assert.True(t, IsSelectableSourceEncoding(alias))
				_, err := decodeToUTF8([]byte("title=abc"), alias)
				require.NoError(t, err)
			})
		}
	}
}

// 選べる文字コードの正式名と、それを指す別名のそれぞれについて、"-"と"_"の
// 入れ替えと大文字化で作った綴りがどれも受け付けられ、復号にも使えることを
// 確かめる。別名は非公開のencodingAliasesから直接取るため、ホワイトボックス
// テストとする。
func TestIsSelectableSourceEncoding_AcceptedSpellingsDecode(t *testing.T) {
	t.Parallel()

	var bases []string
	bases = append(bases, SelectableSourceEncodings...)
	for alias, target := range encodingAliases {
		if IsSelectableSourceEncoding(target) {
			bases = append(bases, alias)
		}
	}

	var spellings []string
	for _, base := range bases {
		for _, variant := range []string{base, strings.ReplaceAll(base, "-", "_"), strings.ReplaceAll(base, "_", "-")} {
			spellings = append(spellings, variant, strings.ToUpper(variant))
		}
	}
	slices.Sort(spellings)
	spellings = slices.Compact(spellings)

	for _, name := range spellings {
		t.Run("正常系: "+name+"は指定でき、復号にも使える", func(t *testing.T) {
			t.Parallel()

			require.True(t, IsSelectableSourceEncoding(name))
			_, err := decodeToUTF8([]byte("title=abc"), name)
			require.NoError(t, err)
		})
	}
}

// 検証が復号より緩くならないことを確かめる。正式名と別名のそれぞれから、区切り
// ("-"/"_")をすべて除いた綴り、区切りを1文字ずつ各位置に挟んだ綴り、それらの
// 大文字化を作り、そのうち検証が受け付けたものがどれも復号に使えることを確かめる。
// 検証が受け付けない綴りについては何も確かめない。
func TestIsSelectableSourceEncoding_NearMissSpellingsDecodeIfAccepted(t *testing.T) {
	t.Parallel()

	bases := slices.Concat(SupportedEncodings, slices.Collect(maps.Keys(encodingAliases)))

	var spellings []string
	for _, base := range bases {
		stripped := strings.NewReplacer("-", "", "_", "").Replace(base)
		candidates := []string{stripped}
		for _, word := range []string{base, stripped} {
			for i := 1; i < len(word); i++ {
				candidates = append(candidates, word[:i]+"-"+word[i:], word[:i]+"_"+word[i:])
			}
		}
		for _, candidate := range candidates {
			spellings = append(spellings, candidate, strings.ToUpper(candidate))
		}
	}
	slices.Sort(spellings)
	spellings = slices.Compact(spellings)

	accepted := slices.DeleteFunc(spellings, func(name string) bool { return !IsSelectableSourceEncoding(name) })
	require.NotEmpty(t, accepted)

	for _, name := range accepted {
		t.Run("正常系: 検証が受け付けた"+name+"は復号にも使える", func(t *testing.T) {
			t.Parallel()

			_, err := decodeToUTF8([]byte("title=abc"), name)
			require.NoError(t, err)
		})
	}
}

// encodingAliasesの各別名が、正式名や他の別名と大文字小文字と"-"/"_"の違いだけで
// 重ならないこと（重なる別名は解決に使われない死んだ項目になる）を確かめる。
func TestEncodingAliases_NoRedundantSpelling(t *testing.T) {
	t.Parallel()

	seen := make(map[string]string)
	for _, supported := range SupportedEncodings {
		seen[encodingKey(supported)] = supported
	}

	for _, alias := range slices.Sorted(maps.Keys(encodingAliases)) {
		key := encodingKey(alias)
		other, dup := seen[key]
		assert.False(t, dup, "別名%sは%sと綴りの違いしかない", alias, other)
		seen[key] = alias
	}
}

// 推定を使って候補を決めた経路では候補が推定名そのものか正式名になるため、別名と
// 推定名の組み合わせは公開APIから作れない。sourcePlanを直接組み立てて確かめる。
func TestSourcePlan_OverrideMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		used string
		want string
	}{
		{name: "正常系: 推定と同じ名前で復号した場合は示さない", used: "utf-8", want: ""},
		{name: "正常系: 推定の別名utf8で復号した場合も同じ文字コードなので示さない", used: "utf8", want: ""},
		{name: "正常系: 推定の大文字とアンダースコア区切りUTF_8で復号した場合も示さない", used: "UTF_8", want: ""},
		{name: "正常系: 推定と異なる文字コードで復号した場合は両方を示す", used: "shift_jis", want: "推定 utf-8（信頼度 0.80）を shift_jis として復号"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := sourcePlan{
				candidates: []string{tt.used},
				detection:  &EncodingDetectionResult{Encoding: "utf-8", Confidence: 0.8, IsSupported: true},
			}

			assert.Equal(t, tt.want, plan.overrideMessage(tt.used))
		})
	}
}

func TestEncodingKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "正常系: 大文字のUTF-8は小文字のutf-8と同じキーになる", a: "UTF-8", b: "utf-8", want: true},
		{name: "正常系: アンダースコア区切りのutf_8はハイフン区切りのutf-8と同じキーになる", a: "utf_8", b: "utf-8", want: true},
		{name: "正常系: 大文字とハイフン区切りのShift-JISはshift_jisと同じキーになる", a: "Shift-JIS", b: "shift_jis", want: true},
		{name: "異常系: 区切り文字の無いutf8はutf-8と別のキーになる", a: "utf8", b: "utf-8", want: false},
		{name: "異常系: 別名のsjisはshift_jisと別のキーになる", a: "sjis", b: "shift_jis", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, encodingKey(tt.a) == encodingKey(tt.b))
		})
	}
}
