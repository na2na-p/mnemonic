package converter

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdjustmentRule_CategoryInvariant_AllRuleSlices(t *testing.T) {
	t.Parallel()

	validCategories := []RuleCategory{
		RuleCategoryPlugin,
		RuleCategorySavePath,
		RuleCategoryMidiAsset,
		RuleCategoryVideoAsset,
		RuleCategoryLayerAlpha,
		RuleCategoryFont,
		RuleCategoryFinalizeGuard,
	}
	tests := []struct {
		name             string
		rules            []AdjustmentRule
		expectedCategory RuleCategory
	}{
		{name: "DefaultRulesのカテゴリを検証する", rules: DefaultRules},
		{
			name:             "MessageLayer互換ルールのカテゴリを検証する",
			rules:            messageLayerCompatRules,
			expectedCategory: RuleCategoryFont,
		},
		{
			name:             "MainWindow互換ルールのカテゴリを検証する",
			rules:            mainWindowCompatRules,
			expectedCategory: RuleCategoryFinalizeGuard,
		},
	}

	totalRules := 0
	for _, tt := range tests {
		totalRules += len(tt.rules)

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, rule := range tt.rules {
				assert.Truef(t, slices.Contains(validCategories, rule.Category), "ルール %q のカテゴリ %q は未定義です", rule.Description, rule.Category)

				if tt.expectedCategory != "" {
					assert.Equalf(t, tt.expectedCategory, rule.Category, "ルール %q のカテゴリ", rule.Description)
				}
			}
		})
	}

	// why not: 総数をテーブルから動的に導出すると、新しいルールスライスを
	// テーブルへ載せ忘れても検出できない。script.goの全ルール数を固定値で持ち、
	// 検証対象の漏れをテスト失敗として表面化させる。
	assert.Equal(t, 23, totalRules)
}
