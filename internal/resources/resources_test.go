package resources_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/resources"
)

// utf8BOM はUTF-8のバイトオーダーマーク。
var utf8BOM = []byte{0xef, 0xbb, 0xbf}

func TestSystemPolyfillFS_ContainsAllEmbeddedFiles(t *testing.T) {
	t.Parallel()

	allFiles := []string{
		"PolyfillInitialize.tjs",
		"MenuItem_stub.tjs",
		"MenuOpener.tjs",
		"KAGParser.tjs",
		"MIDISoundBuffer_stub.tjs",
		"VideoOverlay_stub.tjs",
		"SaveDataPath_patch.tjs",
	}

	for _, name := range allFiles {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data, err := resources.SystemPolyfillFS.ReadFile("system_polyfill/" + name)

			require.NoError(t, err)
			assert.NotEmpty(t, data)
		})
	}
}

func TestSystemPolyfillFiles_CopyList(t *testing.T) {
	t.Parallel()

	// why: コピー対象ファイルの一覧・順序をピン留めする。SaveDataPath_patch.tjs
	// は同梱されているがコピー対象には含まれない（resources.goのwhy not
	// コメント参照）。MenuOpener.tjsはmnemonic独自の追加であり、この一覧
	// そのものをピン留めするテストとして扱う。
	want := []string{
		"PolyfillInitialize.tjs",
		"MenuItem_stub.tjs",
		"MenuOpener.tjs",
		"KAGParser.tjs",
		"MIDISoundBuffer_stub.tjs",
		"VideoOverlay_stub.tjs",
	}

	assert.Equal(t, want, resources.SystemPolyfillFiles)
	assert.NotContains(t, resources.SystemPolyfillFiles, "SaveDataPath_patch.tjs")
}

func TestSystemPolyfillFiles_AllReadableFromFS(t *testing.T) {
	t.Parallel()

	for _, name := range resources.SystemPolyfillFiles {
		data, err := resources.SystemPolyfillFS.ReadFile("system_polyfill/" + name)

		require.NoError(t, err)
		assert.NotEmpty(t, data)
	}
}

// TestSystemPolyfillFS_MenuFilesBraceBalance はMenuItemツリーの本実装化で
// 書き下ろしたTJSファイルに対する最小限の静的チェック。
//
// why: TJSインタプリタをこのテスト環境で実行できないため、波括弧・丸括弧の
// 対応崩れのような明白な構文破損だけを検出する。文字列・正規表現リテラル内の
// 括弧は区別できないため厳密な構文検証ではないが、コピー&ペーストや
// 編集時の閉じ忘れは検出できる。
func TestSystemPolyfillFS_MenuFilesBraceBalance(t *testing.T) {
	t.Parallel()

	names := []string{"MenuItem_stub.tjs", "MenuOpener.tjs"}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data, err := resources.SystemPolyfillFS.ReadFile("system_polyfill/" + name)
			require.NoError(t, err)

			content := string(data)
			assert.Equal(t, strings.Count(content, "{"), strings.Count(content, "}"), "波括弧の対応が崩れています")
			assert.Equal(t, strings.Count(content, "("), strings.Count(content, ")"), "丸括弧の対応が崩れています")
		})
	}
}

// TestSystemPolyfillFS_MenuFilesHaveUTF8BOM は、メニューポリフィル一式
// （PolyfillInitialize.tjs/MenuItem_stub.tjs/MenuOpener.tjs）が先頭に
// UTF-8のBOMを持つことをピン留めする。
//
// why: copyPolyfillFilesUsingは埋め込みリソースを生バイトのままコピーし、
// ScriptAdjusterはこれらのファイルの内容がDefaultRulesのどのパターンにも
// 一致しないためStatusSkippedとなりBOMを付与しない。したがってBOMは
// embed元のファイル自体が持っていない限り実機に載らない。3ファイルは
// いずれも日本語コメントや、画面に表示される日本語文字列(MenuOpener.tjsの
// 「✓」「次へ...」)をUTF-8で含む。吉里吉里がBOM無しテキストを読む文字コードは
// ビルドで決まり（TVP_TEXT_READ_ANSI_MBCSありならShift_JIS、なしなら厳格な
// UTF-8。krkrsdl2 external/krkrz/base/TextStream.cpp:33-37, :205-227）、起動
// オプション-readencodingでも変わる（同base/ScriptMgnIntf.cpp:141-147）。
// Shift_JISで読む設定ではBOM無しのUTF-8を正しく読めない。UTF-8 BOMがあれば
// 設定によらずUTF-8として読まれる（同TextStream.cpp:181-193）ため、BOMで
// ファイル自身に文字コードを示させる。
//
// why not(対象を3ファイルに限定する理由): 同じ一覧に含まれる
// MIDISoundBuffer_stub.tjs/VideoOverlay_stub.tjsは、この3ファイルとは別に
// 元々BOM無しのまま同梱されており、この事実自体を変更する判断は本テストの
// スコープ外とする。判定基準を「非ASCIIバイトを含むかどうか」のような
// 計算値にすると、後から日本語コメントが1行増えただけの無関係な変更で
// このテストが新たに赤くなってしまうため、対象ファイル名を明示的に列挙する。
func TestSystemPolyfillFS_MenuFilesHaveUTF8BOM(t *testing.T) {
	t.Parallel()

	names := []string{"PolyfillInitialize.tjs", "MenuItem_stub.tjs", "MenuOpener.tjs"}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data, err := resources.SystemPolyfillFS.ReadFile("system_polyfill/" + name)
			require.NoError(t, err)

			assert.True(t, bytes.HasPrefix(data, utf8BOM), "UTF-8のBOMが先頭にありません")
		})
	}
}
