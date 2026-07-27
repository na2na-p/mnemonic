// cache_test.go はpackage main（root_test.goのinvoke/invokeWithCacheDirおよび
// runWithRoot/newRootCmdといった非公開の差し替え口）に依存するため、
// 外部テストパッケージ（package main_test）へは分離しない。cache.Dirを
// 参照する本番用NewRootCmdだけを使う純粋な結合テストであれば分離候補だが、
// 本ファイルの全テストは$HOME配下の実キャッシュを誤って削除しないよう
// invokeWithCacheDirでのcacheDir注入が必須であるため、その前提が成立しない。
package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCacheHelpCommand_ShowsSubcommands(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"cache", "--help"})

	assert.Equal(t, 0, result.exitCode)
	assert.Contains(t, result.stdout, "clean")
	assert.Contains(t, result.stdout, "info")
}

func TestCacheHelpCommand_ShowsDescription(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"cache", "--help"})

	assert.Equal(t, 0, result.exitCode)
	assert.Contains(t, result.stdout, "キャッシュ")
}

func TestCacheCleanCommand_BasicExecution(t *testing.T) {
	t.Parallel()

	result := invokeWithCacheDir(t, []string{"cache", "clean"}, "y\n", t.TempDir())

	assert.Equal(t, 0, result.exitCode)
}

func TestCacheCleanCommand_ForceOption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "正常系: --force オプション", args: []string{"cache", "clean", "--force"}},
		{name: "正常系: -f オプション（短縮形）", args: []string{"cache", "clean", "-f"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := invokeWithCacheDir(t, tt.args, "", t.TempDir())

			assert.Equal(t, 0, result.exitCode)
		})
	}
}

func TestCacheCleanCommand_TemplateOnlyOption(t *testing.T) {
	t.Parallel()

	result := invokeWithCacheDir(t, []string{"cache", "clean", "--template-only"}, "y\n", t.TempDir())

	assert.Equal(t, 0, result.exitCode)
}

func TestCacheCleanCommand_CombinedOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "正常系: --force と --template-only の組み合わせ",
			args: []string{"cache", "clean", "--force", "--template-only"},
		},
		{
			name: "正常系: -f と --template-only の組み合わせ",
			args: []string{"cache", "clean", "-f", "--template-only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := invokeWithCacheDir(t, tt.args, "", t.TempDir())

			assert.Equal(t, 0, result.exitCode)
		})
	}
}

func TestCacheCleanCommand_HelpShowsOptions(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"cache", "clean", "--help"})

	assert.Equal(t, 0, result.exitCode)
	assert.True(t, strings.Contains(result.stdout, "--force") || strings.Contains(result.stdout, "-f"))
	assert.Contains(t, result.stdout, "--template-only")
}

func TestCacheCleanCommand_CancelledWhenDeclined(t *testing.T) {
	t.Parallel()

	result := invokeWithCacheDir(t, []string{"cache", "clean"}, "n\n", t.TempDir())

	assert.Equal(t, 0, result.exitCode)
	assert.Contains(t, result.stdout, "キャンセル")
}

func TestCacheInfoCommand_BasicExecution(t *testing.T) {
	t.Parallel()

	result := invokeWithCacheDir(t, []string{"cache", "info"}, "", t.TempDir())

	assert.Equal(t, 0, result.exitCode)
}

func TestCacheInfoCommand_Help(t *testing.T) {
	t.Parallel()

	result := invoke(t, []string{"cache", "info", "--help"})

	assert.Equal(t, 0, result.exitCode)
	assert.Contains(t, result.stdout, "キャッシュ情報")
}
