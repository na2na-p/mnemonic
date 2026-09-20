package converter

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdjustmentRule_CategoryInvariant_AllRuleSlices(t *testing.T) {
	t.Parallel()

	validCategories := []RuleCategory{
		RuleCategoryPlugin,
		RuleCategorySavePath,
		RuleCategoryMidiAsset,
		RuleCategoryMidiCompat,
		RuleCategoryVideoAsset,
		RuleCategoryLayerAlpha,
		RuleCategoryFont,
		RuleCategoryFinalizeGuard,
	}
	ruleSlices := []struct {
		name  string
		rules []AdjustmentRule
	}{
		{name: "DefaultRulesのカテゴリを検証する", rules: DefaultRules},
		{name: "MessageLayer互換ルールのカテゴリを検証する", rules: messageLayerCompatRules},
		{name: "MainWindow互換ルールのカテゴリを検証する", rules: mainWindowCompatRules},
	}
	expectedCategories := []struct {
		description string
		category    RuleCategory
	}{
		{description: "プラグインDLL読み込みの無効化", category: RuleCategoryPlugin},
		{description: "セーブデータパスをdataPathに変更（Android対応）", category: RuleCategorySavePath},
		{description: "MIDISoundBufferをWaveSoundBufferに変換（krkrsdl2対応）", category: RuleCategoryMidiCompat},
		{description: "WaveSoundBuffer.midiOut呼び出しを空文に置換（krkrsdl2対応）", category: RuleCategoryMidiCompat},
		{description: "MIDI参照をOGGに変換（.mid → .ogg）", category: RuleCategoryMidiAsset},
		{description: "MIDI参照をOGGに変換（.midi → .ogg）", category: RuleCategoryMidiAsset},
		{description: "MIDI検索パターンを修正（.mid.ogg → .ogg）", category: RuleCategoryMidiAsset},
		{description: "動画参照をMPEGに変換（.wmv → .mpg）", category: RuleCategoryVideoAsset},
		{description: "動画参照をMPEGに変換（.avi → .mpg）", category: RuleCategoryVideoAsset},
		{description: "動画参照をMPEGに変換（.mpeg → .mpg）", category: RuleCategoryVideoAsset},
		{description: "extrans.dllをlibextrans.soに変換（Android krkrsdl2対応）", category: RuleCategoryPlugin},
		{description: "wuvorbis.dllをlibwuvorbis.soに変換（Android krkrsdl2対応）", category: RuleCategoryPlugin},
		{description: "krmovie.dllをコメントアウト（krkrsdl2未対応）", category: RuleCategoryPlugin},
		{description: "その他のDLLプラグインをコメントアウト", category: RuleCategoryPlugin},
		{description: "レイヤー透過修正: type=alphaを自動追加（krkrsdl2対応）", category: RuleCategoryLayerAlpha},
		{description: "フォント用faceメンバ宣言をfontFaceにリネーム（krkrsdl2対応）", category: RuleCategoryFont},
		{description: "font.faceへの連鎖代入をfontFaceにリネーム（krkrsdl2対応）", category: RuleCategoryFont},
		{description: "アットマーク付きフォント名代入をfontFaceにリネーム（krkrsdl2対応）", category: RuleCategoryFont},
		{description: "assignからのフォント名コピーをfontFaceにリネーム（krkrsdl2対応）", category: RuleCategoryFont},
		{description: "finalizeでのfore.layers未初期化アクセスをガード（krkrsdl2対応）", category: RuleCategoryFinalizeGuard},
		{description: "finalizeでのback.layers未初期化アクセスをガード（krkrsdl2対応）", category: RuleCategoryFinalizeGuard},
		{description: "finalizeでのfore.messages未初期化アクセスをガード（krkrsdl2対応）", category: RuleCategoryFinalizeGuard},
		{description: "finalizeでのback.messages未初期化アクセスをガード（krkrsdl2対応）", category: RuleCategoryFinalizeGuard},
	}

	allRules := make([]AdjustmentRule, 0)
	for _, tt := range ruleSlices {
		allRules = append(allRules, tt.rules...)

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, rule := range tt.rules {
				assert.Truef(t, slices.Contains(validCategories, rule.Category), "ルール %q のカテゴリ %q は未定義です", rule.Description, rule.Category)
			}
		})
	}

	rulesByDescription := make(map[string]AdjustmentRule, len(allRules))
	for _, rule := range allRules {
		_, exists := rulesByDescription[rule.Description]
		assert.Falsef(t, exists, "ルール %q のDescriptionが重複しています", rule.Description)
		rulesByDescription[rule.Description] = rule
	}

	require.Len(t, allRules, len(expectedCategories))
	for _, expected := range expectedCategories {
		t.Run(expected.description, func(t *testing.T) {
			t.Parallel()

			rule, ok := rulesByDescription[expected.description]
			require.Truef(t, ok, "ルール %q が見つかりません", expected.description)
			assert.Equalf(t, expected.category, rule.Category, "ルール %q のカテゴリ", expected.description)
		})
	}

	source, err := os.ReadFile("script.go") //nolint:gosec // テスト対象と同じパッケージの固定ファイルを読む
	require.NoError(t, err)

	categoryLines := regexp.MustCompile(`(?m)^[^\n]*\bCategory:\s+RuleCategory\w+[,}][^\n]*$`).FindAll(source, -1)
	descriptionLines := regexp.MustCompile(`(?m)^[^\n]*\bDescription:\s+"[^\n]*$`).FindAll(source, -1)
	coveredRuleCount := len(allRules)
	sourceRuleCount := len(categoryLines)
	descriptionCount := len(descriptionLines)

	// why not: ハードコードした総数では、未登録のルールスライスを追加しても
	// テスト側の合計が変わらず検出できない。ソース中のルール数と照合することで、
	// ruleSlicesに載っていないスライスも検出対象にする。
	assert.Equal(t, coveredRuleCount, sourceRuleCount, "script.goのルール数とテスト対象のルール数が一致しません")
	assert.Equal(t, descriptionCount, sourceRuleCount, "Categoryがないルールリテラルがあります")
}
