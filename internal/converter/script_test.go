package converter_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/converter"
)

func TestAdjustmentRule_Fields(t *testing.T) {
	t.Parallel()

	rule := converter.AdjustmentRule{
		Pattern:     "test_pattern",
		Replacement: "replaced",
		Description: "テストルール",
	}

	assert.Equal(t, "test_pattern", rule.Pattern)
	assert.Equal(t, "replaced", rule.Replacement)
	assert.Equal(t, "テストルール", rule.Description)
}

func TestNewScriptAdjuster(t *testing.T) {
	t.Parallel()

	t.Run("正常系: rulesがnilの場合DefaultRulesが使用される", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)

		assert.Len(t, adjuster.Rules(), len(converter.DefaultRules))
		assert.Equal(t, "プラグインDLL読み込みの無効化", adjuster.Rules()[0].Description)
	})

	t.Run("正常系: カスタムルールを指定できる", func(t *testing.T) {
		t.Parallel()

		customRules := []converter.AdjustmentRule{
			{Pattern: "custom", Replacement: "replaced", Description: "カスタムルール"},
		}
		adjuster := converter.NewScriptAdjuster(customRules, true)

		require.Len(t, adjuster.Rules(), 1)
		assert.Equal(t, "カスタムルール", adjuster.Rules()[0].Description)
	})

	t.Run("正常系: addEncodingDirectiveを有効化できる", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		assert.True(t, adjuster.AddEncodingDirective())
	})

	t.Run("正常系: addEncodingDirectiveを無効化できる", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, false)
		assert.False(t, adjuster.AddEncodingDirective())
	})
}

func TestScriptAdjuster_DefaultRules(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, converter.DefaultRules)

	descriptions := make([]string, 0, len(converter.DefaultRules))
	for _, r := range converter.DefaultRules {
		descriptions = append(descriptions, r.Description)
	}
	assert.Contains(t, descriptions, "プラグインDLL読み込みの無効化")
}

func TestScriptAdjuster_SupportedExtensions(t *testing.T) {
	t.Parallel()

	adjuster := converter.NewScriptAdjuster(nil, true)

	for _, ext := range []string{".ks", ".tjs"} {
		assert.Contains(t, adjuster.SupportedExtensions(), ext)
	}
	for _, ext := range []string{".txt", ".png", ".exe"} {
		assert.NotContains(t, adjuster.SupportedExtensions(), ext)
	}
}

func TestScriptAdjuster_CanConvert(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		filename string
		expected bool
	}{
		"正常系: .ksファイルは変換可能":     {"script.ks", true},
		"正常系: .tjsファイルは変換可能":    {"startup.tjs", true},
		"正常系: 大文字の.KSファイルも変換可能": {"Script.KS", true},
		"正常系: .txtファイルは変換不可":    {"readme.txt", false},
		"正常系: .pngファイルは変換不可":    {"image.png", false},
		"正常系: .exeファイルは変換不可":    {"game.exe", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			adjuster := converter.NewScriptAdjuster(nil, true)
			testFile := filepath.Join(t.TempDir(), tc.filename)
			writeFile(t, testFile, []byte("test content"))

			assert.Equal(t, tc.expected, adjuster.CanConvert(testFile))
		})
	}
}

func TestScriptAdjuster_AdjustContent(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 単一のPlugins.link()呼び出しを無効化する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `Plugins.link("something.dll");`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, `Plugins.link("something.dll")`)
		assert.Contains(t, adjusted, "// Disabled for Android")
		assert.True(t, strings.HasPrefix(adjusted, "// "))
		assert.Equal(t, 1, count)
	})

	t.Run("正常系: 複数のPlugins.link()呼び出しを無効化する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "// some comment\nPlugins.link(\"plugin1.dll\");\nvar x = 1;\nPlugins.link('plugin2.dll');\n"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Equal(t, 2, strings.Count(adjusted, "// Disabled for Android"))
		assert.Equal(t, 2, count)
	})

	t.Run("正常系: プラグイン以外のコードを保持する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "var x = 1;\nfunction test() {\n    return x * 2;\n}\n// This is a comment\n"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Equal(t, content, adjusted)
		assert.Equal(t, 0, count)
	})

	t.Run("正常系: インデントを保持しながらプラグインを無効化する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `    Plugins.link("test.dll");`

		adjusted, count := adjuster.AdjustContent(content)

		assert.True(t, strings.HasPrefix(adjusted, "    // "))
		assert.Equal(t, 1, count)
	})

	t.Run("正常系: 調整回数を正しく返す", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "Plugins.link(\"a.dll\");\nPlugins.link(\"b.dll\");\nPlugins.link(\"c.dll\");\n"

		_, count := adjuster.AdjustContent(content)
		assert.Equal(t, 3, count)
	})

	t.Run("正常系: 空の内容を処理できる", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		adjusted, count := adjuster.AdjustContent("")

		assert.Empty(t, adjusted)
		assert.Equal(t, 0, count)
	})

	t.Run("正常系: カスタムルールを適用できる", func(t *testing.T) {
		t.Parallel()

		customRules := []converter.AdjustmentRule{
			{Pattern: `OLD_FUNCTION\(\)`, Replacement: "NEW_FUNCTION()", Description: "関数名変更"},
		}
		adjuster := converter.NewScriptAdjuster(customRules, true)
		content := "var result = OLD_FUNCTION();"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, "NEW_FUNCTION()")
		assert.NotContains(t, adjusted, "OLD_FUNCTION()")
		assert.Equal(t, 1, count)
	})

	t.Run("正常系: 日本語コメントを保持する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "// これは日本語のコメントです\nvar x = 1;  // 変数xを初期化\n/* 複数行の\n   日本語コメント */\n"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, "これは日本語のコメントです")
		assert.Contains(t, adjusted, "変数xを初期化")
		assert.Contains(t, adjusted, "複数行の")
		assert.Contains(t, adjusted, "日本語コメント")
		assert.Equal(t, 0, count)
	})
}

func TestScriptAdjuster_AddStartupDirective(t *testing.T) {
	t.Parallel()

	t.Run("正常系: startup.tjsにポリフィル初期化ディレクティブを追加できる", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "// Original script\nvar initialized = false;\n"

		result := adjuster.AddStartupDirective(content)

		assert.Contains(t, result, "// krkrsdl2 polyfill initialization")
		assert.Contains(t, result, `Scripts.execStorage("system/polyfillinitialize.tjs");`)
		assert.Contains(t, result, "// Original script")
	})

	t.Run("正常系: ディレクティブがファイル先頭に追加される", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		result := adjuster.AddStartupDirective("var x = 1;")

		assert.True(t, strings.HasPrefix(result, "// krkrsdl2 polyfill initialization"))
	})

	t.Run("正常系: 元の内容が保持される", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "function initialize() {\n    System.inform(\"Hello\");\n}\n"

		result := adjuster.AddStartupDirective(content)

		assert.Contains(t, result, content)
	})
}

func TestScriptAdjuster_Convert(t *testing.T) {
	t.Parallel()

	t.Run("異常系: Shift_JISのままの非UTF-8ファイルはFAILEDを返す", func(t *testing.T) {
		t.Parallel()

		// why: レビュー指摘の回帰防止。Python版はsource.read_text(encoding="utf-8")
		// がUnicodeDecodeErrorを送出しtry/exceptでFAILEDになる。Go版がstring()への
		// 変換前にUTF-8妥当性を検証していないと、不正なバイト列がそのまま
		// SUCCESSとして書き出されてしまう（文字化けの温存）ため、この失敗パスを
		// ピン留めする。
		dir := t.TempDir()
		source := filepath.Join(dir, "sjis.ks")
		dest := filepath.Join(dir, "output", "sjis.ks")

		writeSJIS(t, source, "Plugins.link(\"test.dll\");\n[message text=\"日本語テスト\"]\n")

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusFailed, result.Status)
		assert.NoFileExists(t, dest)
	})

	t.Run("正常系: プラグイン呼び出しを含む.ksファイルを変換できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.ks")
		dest := filepath.Join(dir, "output", "test.ks")

		content := "Plugins.link(\"test.dll\");\n[message text=\"Hello\"]\n"
		writeFile(t, source, []byte(content))

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		converted := readFile(t, dest)
		assert.Contains(t, string(converted), "// Disabled for Android")
		assert.Contains(t, string(converted), "[message")
	})

	t.Run("正常系: プラグイン呼び出しを含む.tjsファイルを変換できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.tjs")
		dest := filepath.Join(dir, "output", "test.tjs")

		content := "Plugins.link('extrans.dll');\nvar transition = new ExtTransition();\n"
		writeFile(t, source, []byte(content))

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		converted := readFile(t, dest)
		assert.Contains(t, string(converted), "// Disabled for Android")
	})

	t.Run("正常系: startup.tjsにエンコーディングディレクティブを追加する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "startup.tjs")
		dest := filepath.Join(dir, "output", "startup.tjs")

		content := "// Game startup script\nvar kag = new KAGWindow();\n"
		writeFile(t, source, []byte(content))

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		converted := readFile(t, dest)
		assert.Contains(t, string(converted), "// krkrsdl2 polyfill initialization")
		assert.Contains(t, string(converted), `Scripts.execStorage("system/polyfillinitialize.tjs");`)
	})

	t.Run("正常系: addEncodingDirective無効時はディレクティブを追加しない", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "startup.tjs")
		dest := filepath.Join(dir, "output", "startup.tjs")

		content := "Plugins.link(\"test.dll\");\nvar x = 1;\n"
		writeFile(t, source, []byte(content))

		adjuster := converter.NewScriptAdjuster(nil, false)
		result, err := adjuster.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		converted := readFile(t, dest)
		assert.NotContains(t, string(converted), "// krkrsdl2 polyfill initialization")
		assert.Contains(t, string(converted), "// Disabled for Android")
	})

	t.Run("正常系: 調整が不要な場合はSKIPPEDを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "clean.ks")
		dest := filepath.Join(dir, "output", "clean.ks")

		content := "[message text=\"No plugins here\"]\n[wait time=1000]\n"
		writeFile(t, source, []byte(content))

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSkipped, result.Status)
	})

	t.Run("正常系: 変換前後のバイト数を記録する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.ks")
		dest := filepath.Join(dir, "output", "test.ks")

		writeFile(t, source, []byte(`Plugins.link("test.dll");`))

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, dest)

		require.NoError(t, err)
		assert.Positive(t, result.BytesBefore)
		assert.Positive(t, result.BytesAfter)
		assert.Greater(t, result.BytesAfter, result.BytesBefore)
	})

	t.Run("異常系: 存在しないファイルはFAILEDを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(filepath.Join(dir, "nonexistent.ks"), filepath.Join(dir, "output.ks"))

		require.NoError(t, err)
		assert.Equal(t, converter.StatusFailed, result.Status)
	})

	t.Run("正常系: 変換先ディレクトリが存在しない場合は作成する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.ks")
		dest := filepath.Join(dir, "nested", "dir", "output.ks")

		writeFile(t, source, []byte(`Plugins.link("test.dll");`))

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("正常系: UTF-8で読み書きする", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "japanese.ks")
		dest := filepath.Join(dir, "output", "japanese.ks")

		content := "Plugins.link(\"test.dll\");\n[message text=\"日本語テスト\"]\n// 日本語コメント\n"
		writeFile(t, source, []byte(content))

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		converted := readFile(t, dest)
		assert.Contains(t, string(converted), "日本語テスト")
		assert.Contains(t, string(converted), "日本語コメント")
	})

	t.Run("正常系: 変換後のファイルにはUTF-8 BOMが付与される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.ks")
		dest := filepath.Join(dir, "output", "test.ks")

		writeFile(t, source, []byte(`Plugins.link("test.dll");`))

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		converted := readFile(t, dest)
		assert.True(t, bytes.HasPrefix(converted, utf8BOMBytes), "UTF-8 BOMが付与されているべき")
	})

	t.Run("正常系: 入力のUTF-8 BOMは読み込み時に自動除去される", func(t *testing.T) {
		t.Parallel()

		// why: Python版はsource.read_text(encoding="utf-8-sig")でBOMを自動除去する。
		// 入力側に既にBOMが付いていても、出力に二重付与されないことを確認する。
		dir := t.TempDir()
		source := filepath.Join(dir, "test.ks")
		dest := filepath.Join(dir, "output", "test.ks")

		content := append(append([]byte{}, utf8BOMBytes...), []byte("Plugins.link(\"test.dll\");\n[message text=\"Hello\"]\n")...)
		writeFile(t, source, content)

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		converted := readFile(t, dest)
		assert.True(t, bytes.HasPrefix(converted, utf8BOMBytes))
		// BOMが本文中に二重に残っていないことを確認する
		assert.Equal(t, 1, bytes.Count(converted, utf8BOMBytes))
	})
}

var utf8BOMBytes = []byte{0xef, 0xbb, 0xbf}

// TestScriptAdjuster_MidiRules はMIDIファイル参照の変換ルール
// （MIDISoundBuffer→WaveSoundBuffer、拡張子.mid/.midi→.ogg）をテストする。
//
// Python版 TestScriptAdjusterMidiRules の移植。
func TestScriptAdjuster_MidiRules(t *testing.T) {
	t.Parallel()

	t.Run("正常系: MIDISoundBufferをWaveSoundBufferに変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `var bgm = new MIDISoundBuffer("bgm/title.mid");`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, "WaveSoundBuffer")
		assert.NotContains(t, adjusted, "MIDISoundBuffer")
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: .mid参照をダブルクォートで.oggに変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `var bgm = new WaveSoundBuffer("bgm/title.mid");`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, `"bgm/title.ogg"`)
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: .mid参照をシングルクォートで.oggに変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "var bgm = new WaveSoundBuffer('bgm/title.mid');"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, "'bgm/title.ogg'")
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: .midi参照を.oggに変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `var bgm = new WaveSoundBuffer("bgm/title.midi");`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, `"bgm/title.ogg"`)
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: MIDISoundBufferと.mid拡張子の両方を変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `var bgm = new MIDISoundBuffer("bgm/title.mid");`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, "WaveSoundBuffer")
		assert.NotContains(t, adjusted, "MIDISoundBuffer")
		assert.Contains(t, adjusted, `"bgm/title.ogg"`)
		assert.GreaterOrEqual(t, count, 2)
	})

	t.Run("正常系: 複数のMIDI参照を変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "var bgm1 = new MIDISoundBuffer(\"bgm/title.mid\");\n" +
			"var bgm2 = new MIDISoundBuffer(\"bgm/battle.midi\");\n" +
			"var se = new WaveSoundBuffer(\"se/click.wav\");\n"

		adjusted, _ := adjuster.AdjustContent(content)

		assert.Equal(t, 3, strings.Count(adjusted, "WaveSoundBuffer"))
		assert.NotContains(t, adjusted, "MIDISoundBuffer")
		assert.Contains(t, adjusted, `"bgm/title.ogg"`)
		assert.Contains(t, adjusted, `"bgm/battle.ogg"`)
		assert.Contains(t, adjusted, `"se/click.wav"`) // WAVはそのまま
	})

	t.Run("正常系: クォートされていないコンテキストではMIDが変換されない", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "var midpoint = calculateMidpoint();"

		adjusted, _ := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, "midpoint")
	})

	t.Run("正常系: storageのMIDI検索パターンを修正する（.mid.ogg→.ogg）", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `var path = storage + ".mid.ogg";`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, `storage + ".ogg"`)
		assert.NotContains(t, adjusted, ".mid.ogg")
		assert.GreaterOrEqual(t, count, 1)
	})
}

// TestScriptAdjuster_VideoRules はVideoConverterが対象とする入力拡張子
// (.wmv/.avi/.mpeg)の参照を.mpgへ書き換えるルールをテストする。
// VideoConverterは変換後、常にMPEG-PS(.mpg)を出力するため（video.goの
// GetOutputExtension参照）、スクリプト側の参照も同じ拡張子を指す必要がある。
func TestScriptAdjuster_VideoRules(t *testing.T) {
	t.Parallel()

	t.Run("正常系: .wmv参照をダブルクォートで.mpgに変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `[movie storage="op.wmv" layer=0]`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, `"op.mpg"`)
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: .avi参照をシングルクォートで.mpgに変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[movie storage='op.avi' layer=0]"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, "'op.mpg'")
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: .mpeg参照を.mpgに変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `[movie storage="op.mpeg" layer=0]`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, `"op.mpg"`)
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: 大文字拡張子(.WMV)の参照も小文字.mpgに変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `[movie storage="OP.WMV" layer=0]`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, `"OP.mpg"`)
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: 既に.mpg参照の場合は無変換で変化しない", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `[movie storage="op.mpg" layer=0]`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Equal(t, content, adjusted)
		assert.Equal(t, 0, count)
	})

	t.Run("正常系: 複数の動画参照を変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[movie storage=\"op.wmv\" layer=0]\n" +
			"[movie storage=\"ed.avi\" layer=0]\n" +
			"[movie storage=\"insert.mpeg\" layer=0]\n"

		adjusted, _ := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, `"op.mpg"`)
		assert.Contains(t, adjusted, `"ed.mpg"`)
		assert.Contains(t, adjusted, `"insert.mpg"`)
	})
}

// TestScriptAdjuster_MidiOutRule はWaveSoundBuffer.midiOut呼び出しの
// 空文置換ルールをテストする。
//
// Python版 TestScriptAdjusterMidiOutRule の移植。
func TestScriptAdjuster_MidiOutRule(t *testing.T) {
	t.Parallel()

	t.Run("正常系: WaveSoundBuffer.midiOut呼び出しを空文に置換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "WaveSoundBuffer.midiOut(midiInitialMessage);"

		adjusted, count := adjuster.AdjustContent(content)

		assert.True(t, strings.HasPrefix(adjusted, "; // "))
		assert.Contains(t, adjusted, "// WaveSoundBuffer.midiOut(midiInitialMessage);")
		assert.Contains(t, adjusted, "Disabled: midiOut not available in WaveSoundBuffer")
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: インデント付きのWaveSoundBuffer.midiOut呼び出しを空文に置換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "    WaveSoundBuffer.midiOut(midiInitialMessage);"

		adjusted, count := adjuster.AdjustContent(content)

		assert.True(t, strings.HasPrefix(adjusted, "    ; // "))
		assert.Contains(t, adjusted, "Disabled: midiOut not available in WaveSoundBuffer")
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: 異なる引数のWaveSoundBuffer.midiOut呼び出しを空文に置換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `WaveSoundBuffer.midiOut("some_message");`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, "; // ")
		assert.Contains(t, adjusted, "Disabled: midiOut not available in WaveSoundBuffer")
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: MIDISoundBuffer.midiOutが変換後に空文に置換される", func(t *testing.T) {
		t.Parallel()

		// MIDISoundBuffer → WaveSoundBuffer 変換後に midiOut が空文に置換されるべき
		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "MIDISoundBuffer.midiOut(midiInitialMessage);"

		adjusted, count := adjuster.AdjustContent(content)

		assert.NotContains(t, adjusted, "MIDISoundBuffer")
		assert.Contains(t, adjusted, "; // WaveSoundBuffer.midiOut(midiInitialMessage);")
		assert.Contains(t, adjusted, "Disabled: midiOut not available in WaveSoundBuffer")
		assert.GreaterOrEqual(t, count, 2) // MIDISoundBuffer変換 + midiOut置換
	})

	t.Run("正常系: 複数のmidiOut呼び出しを空文に置換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "WaveSoundBuffer.midiOut(msg1);\n    WaveSoundBuffer.midiOut(msg2);\nWaveSoundBuffer.midiOut(msg3);\n"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Equal(t, 3, strings.Count(adjusted, "Disabled: midiOut not available in WaveSoundBuffer"))

		for _, line := range strings.Split(adjusted, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			assert.True(t, strings.HasPrefix(strings.TrimLeft(line, " \t"), "; // WaveSoundBuffer.midiOut"))
		}
		assert.GreaterOrEqual(t, count, 3)
	})
}

// TestScriptAdjuster_SaveDataLocationRule はセーブデータパスをdataPathに
// 変更するルール（Android対応）をテストする。
func TestScriptAdjuster_SaveDataLocationRule(t *testing.T) {
	t.Parallel()

	t.Run("正常系: saveDataLocationをSystem.dataPathに変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `saveDataLocation = System.exePath + saveDataLocation;`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, "saveDataLocation = System.dataPath")
		assert.NotContains(t, adjusted, "System.exePath")
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: 一致しない場合は変更しない", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `var x = System.exePath;`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Equal(t, content, adjusted)
		assert.Equal(t, 0, count)
	})
}

// TestScriptAdjuster_LoadpluginRules はloadpluginタグのDLL参照を
// krkrsdl2向けの.soへ変換する、または未対応プラグインをコメントアウトする
// ルール群をテストする。Python版 TestScriptAdjusterLoadpluginRules の移植。
func TestScriptAdjuster_LoadpluginRules(t *testing.T) {
	t.Parallel()

	t.Run("正常系: extrans.dllをlibextrans.soに変換する（Android krkrsdl2対応）", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `[loadplugin module="extrans.dll"]`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, `[loadplugin module="libextrans.so"]`)
		assert.NotContains(t, adjusted, "extrans.dll")
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: wuvorbis.dllをlibwuvorbis.soに変換する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `[loadplugin module="wuvorbis.dll"]`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, `[loadplugin module="libwuvorbis.so"]`)
		assert.NotContains(t, adjusted, "wuvorbis.dll")
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: krmovie.dllをコメントアウトする", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `[loadplugin module="krmovie.dll"]`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, ";#")
		assert.Contains(t, adjusted, "not supported on krkrsdl2")
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: その他のDLLプラグインをコメントアウトする", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `[loadplugin module="layerexdraw.dll"]`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, ";#")
		assert.Contains(t, adjusted, "Disabled for Android")
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: 複数のloadpluginタグを処理する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[loadplugin module=\"wuvorbis.dll\"]\n" +
			"[loadplugin module=\"extrans.dll\"]\n" +
			"[loadplugin module=\"krmovie.dll\"]\n" +
			"[loadplugin module=\"something.dll\"]\n"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, `[loadplugin module="libextrans.so"]`)
		assert.Contains(t, adjusted, `[loadplugin module="libwuvorbis.so"]`)
		assert.Contains(t, adjusted, "not supported on krkrsdl2")
		assert.Contains(t, adjusted, "Disabled for Android")
		assert.GreaterOrEqual(t, count, 4)
	})

	t.Run("正常系: 変換後のlibextrans.soタグは再変換されない", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `[loadplugin module="libextrans.so"]`

		adjusted, count := adjuster.AdjustContent(content)

		assert.Equal(t, content, adjusted)
		assert.NotContains(t, adjusted, ";#")
		assert.Equal(t, 0, count)
	})
}

// TestScriptAdjuster_LayerAlphaRule はレイヤー透過修正
// （[layopt layer=N]へのtype=alpha自動追加、krkrsdl2対応）をテストする。
// Python版 TestScriptAdjusterLayerAlphaRules の移植。
func TestScriptAdjuster_LayerAlphaRule(t *testing.T) {
	t.Parallel()

	t.Run("正常系: type=未指定の[layopt layer=N]にtype=alphaを追加する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[layopt layer=0 page=back visible=true]"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Equal(t, "[layopt layer=0 page=back visible=true type=alpha]", adjusted)
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: hittype=のような単語境界を伴わないtype=部分文字列は本物のtype=とみなさない", func(t *testing.T) {
		t.Parallel()

		// why: strings.Contains(attrs, "type=")は"hittype="のような、直前が
		// 単語構成文字であるため実際にはtype属性ではない部分文字列にも誤って
		// 一致してしまう。Python版の\btype=（単語境界付き）と同じ判定に
		// する必要がある（レビュー指摘）。
		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[layopt hittype=foo layer=0]"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Equal(t, "[layopt hittype=foo layer=0 type=alpha]", adjusted)
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: subtype=のような単語境界を伴わないtype=部分文字列は本物のtype=とみなさない", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[layopt layer=0 subtype=q]"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Equal(t, "[layopt layer=0 subtype=q type=alpha]", adjusted)
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: type=風の部分文字列がlayer=より前にあっても追加される", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[layopt subtype=q layer=0]"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Equal(t, "[layopt subtype=q layer=0 type=alpha]", adjusted)
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: layer=数値の後に他の文字が続いてもtype=alphaが追加される", func(t *testing.T) {
		t.Parallel()

		// why: Python版のlayer=[0-9]+には後続の単語境界(\b)要求が無いため、
		// "layer=12abc"のように数字の後に他の文字が続いてもマッチする。
		// Go版がlayer=[0-9]+\bのように末尾に\bを付けると"layer=12abc"に
		// マッチしなくなりPython版と乖離する（レビュー指摘）。
		testCases := []struct {
			name    string
			content string
			want    string
		}{
			{
				name:    "layer=12abc",
				content: "[layopt layer=12abc visible=true]",
				want:    "[layopt layer=12abc visible=true type=alpha]",
			},
			{
				name:    "layer=1x",
				content: "[layopt layer=1x visible=true]",
				want:    "[layopt layer=1x visible=true type=alpha]",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				adjuster := converter.NewScriptAdjuster(nil, true)

				adjusted, count := adjuster.AdjustContent(tc.content)

				assert.Equal(t, tc.want, adjusted)
				assert.GreaterOrEqual(t, count, 1)
			})
		}
	})

	t.Run("正常系: 既にtype=が指定されている場合は変更しない", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[layopt layer=0 type=opaque visible=true]"

		adjusted, _ := adjuster.AdjustContent(content)

		assert.Equal(t, content, adjusted)
		assert.Equal(t, 1, strings.Count(adjusted, "type="))
	})

	t.Run("正常系: 既にtype=alphaが指定されている場合は変更しない", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[layopt layer=0 type=alpha visible=true]"

		adjusted, _ := adjuster.AdjustContent(content)

		assert.Equal(t, 1, strings.Count(adjusted, "type=alpha"))
	})

	t.Run("正常系: layer=base（背景レイヤー）は変更しない", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[layopt layer=base visible=true]"

		adjusted, _ := adjuster.AdjustContent(content)

		assert.NotContains(t, adjusted, "type=alpha")
		assert.Equal(t, content, adjusted)
	})

	t.Run("正常系: layer=message（メッセージレイヤー）は変更しない", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[layopt layer=message visible=true]"

		adjusted, _ := adjuster.AdjustContent(content)

		assert.NotContains(t, adjusted, "type=alpha")
		assert.Equal(t, content, adjusted)
	})

	t.Run("正常系: 複数の数字レイヤーにtype=alphaを追加する", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[layopt layer=0 visible=true]\n[layopt layer=1 visible=true]\n[layopt layer=2 page=back visible=true]"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Equal(t, 3, strings.Count(adjusted, "type=alpha"))
		assert.GreaterOrEqual(t, count, 3)
	})

	t.Run("正常系: 実際のKAGスクリプトパターンを処理できる", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "[backlay]\n" +
			"[image storage=\"title-base\" layer=base page=back]\n" +
			"[layopt layer=0 page=back visible=true]\n" +
			"[image storage=\"title-fore\" layer=0 page=back top=0 left=0]\n" +
			"[trans method=crossfade time=2000]\n" +
			"[wt]"

		adjusted, count := adjuster.AdjustContent(content)

		assert.Contains(t, adjusted, "[layopt layer=0 page=back visible=true type=alpha]")
		assert.Contains(t, adjusted, `[image storage="title-base" layer=base page=back]`)
		assert.Contains(t, adjusted, "[trans method=crossfade time=2000]")
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("正常系: クォート付きlayer値は処理しない（数字パターンのため）", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := `[layopt layer="0" visible=true]`

		adjusted, _ := adjuster.AdjustContent(content)

		assert.Equal(t, content, adjusted)
	})
}

// TestScriptAdjuster_DefaultRulesOrder はDefaultRulesの並び順がPython版
// DEFAULT_RULESの最終状態と一致することをピン留めする。
//
// why: MIDISoundBuffer→WaveSoundBuffer変換ルールが先に走ることで、
// 変換前の"MIDISoundBuffer.midiOut(...)"呼び出しも次のmidiOut置換ルールで
// 捕捉される（T-211由来の順序制約）。今回追加するsaveDataLocationルールは
// Plugins.link無効化ルールの直後、MIDISoundBuffer変換ルールの直前に位置する
// 必要がある（他のルールと干渉しない独立したルールのため、この位置自体に
// 機能的な依存はないが、Python版DEFAULT_RULESの宣言順と一致させることで
// 差分レビューを容易にする）。loadplugin系・layopt系ルールは末尾に追加される。
func TestScriptAdjuster_DefaultRulesOrder(t *testing.T) {
	t.Parallel()

	wantDescriptions := []string{
		"プラグインDLL読み込みの無効化",
		"セーブデータパスをdataPathに変更（Android対応）",
		"MIDISoundBufferをWaveSoundBufferに変換（krkrsdl2対応）",
		"WaveSoundBuffer.midiOut呼び出しを空文に置換（krkrsdl2対応）",
		"MIDI参照をOGGに変換（.mid → .ogg）",
		"MIDI参照をOGGに変換（.midi → .ogg）",
		"MIDI検索パターンを修正（.mid.ogg → .ogg）",
		"動画参照をMPEGに変換（.wmv → .mpg）",
		"動画参照をMPEGに変換（.avi → .mpg）",
		"動画参照をMPEGに変換（.mpeg → .mpg）",
		"extrans.dllをlibextrans.soに変換（Android krkrsdl2対応）",
		"wuvorbis.dllをlibwuvorbis.soに変換（Android krkrsdl2対応）",
		"krmovie.dllをコメントアウト（krkrsdl2未対応）",
		"その他のDLLプラグインをコメントアウト",
		"レイヤー透過修正: type=alphaを自動追加（krkrsdl2対応）",
	}

	descriptions := make([]string, 0, len(converter.DefaultRules))
	for _, r := range converter.DefaultRules {
		descriptions = append(descriptions, r.Description)
	}

	assert.Equal(t, wantDescriptions, descriptions)
}

func TestScriptAdjuster_ApplyMessageLayerCompat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		want      string
		wantCount int
	}{
		{
			name:      "フォント用faceメンバ宣言をfontFaceにリネームする",
			content:   "\t/*C*/var face; // フォント\n",
			want:      "\t/*C*/var fontFace; // フォント\n",
			wantCount: 1,
		},
		{
			name:      "font.faceへの連鎖代入をリネームする",
			content:   "\t\t\tlineLayer.font.face = face = defaultFace == 'user' ? userFace : defaultFace;\n",
			want:      "\t\t\tlineLayer.font.face = fontFace = defaultFace == 'user' ? userFace : defaultFace;\n",
			wantCount: 1,
		},
		{
			name:      "アットマーク付きフォント名の括弧内代入をリネームする",
			content:   "\t\t\t\tvar f = '@' + (face = defaultFace);\n",
			want:      "\t\t\t\tvar f = '@' + (fontFace = defaultFace);\n",
			wantCount: 1,
		},
		{
			name:      "assignからのコピーを両辺リネームする",
			content:   "\t\tface = src.face;\n",
			want:      "\t\tfontFace = src.fontFace;\n",
			wantCount: 1,
		},
		{
			name:      "描画面切替のface代入は変更しない",
			content:   "\t\t\tface = dfProvince;\n\t\t\tface = dfAuto;\n\t\t\tll.face = dfProvince;\n",
			want:      "\t\t\tface = dfProvince;\n\t\t\tface = dfAuto;\n\t\t\tll.face = dfProvince;\n",
			wantCount: 0,
		},
		{
			name:      "Fontオブジェクトのfaceプロパティは変更しない",
			content:   "\t\tof.face = lf.face;\n\t\tvar elmface = elm.face;\n\t\tlf.face = orgfont;\n",
			want:      "\t\tof.face = lf.face;\n\t\tvar elmface = elm.face;\n\t\tlf.face = orgfont;\n",
			wantCount: 0,
		},
		{
			name: "実際のclearLayer相当のコードでは描画面切替を保持しつつ宣言のみ変更する",
			content: "\t/*C*/var face; // フォント\n" +
				"\t\t\tface = dfProvince;\n" +
				"\t\t\tcolorRect(0, 0, imageWidth, imageHeight, 0); // 領域もクリア\n" +
				"\t\t\tface = dfAuto;\n",
			want: "\t/*C*/var fontFace; // フォント\n" +
				"\t\t\tface = dfProvince;\n" +
				"\t\t\tcolorRect(0, 0, imageWidth, imageHeight, 0); // 領域もクリア\n" +
				"\t\t\tface = dfAuto;\n",
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			adjuster := converter.NewScriptAdjuster(nil, true)
			got, count := adjuster.ApplyMessageLayerCompat(tt.content)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantCount, count)
		})
	}
}

func TestScriptAdjuster_Convert_MessageLayerCompat(t *testing.T) {
	t.Parallel()

	t.Run("messagelayer.tjsにはface互換リネームが適用される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "MessageLayer.tjs")
		content := "\uFEFF\t/*C*/var face; // フォント\n\t\t\tface = dfProvince;\n"
		writeFile(t, source, []byte(content))

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, source)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)

		got := readFile(t, source)
		assert.Contains(t, string(got), "var fontFace; // フォント")
		assert.Contains(t, string(got), "face = dfProvince;")
	})

	t.Run("messagelayer.tjs以外にはface互換リネームを適用しない", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "other.tjs")
		content := "\uFEFF\t/*C*/var face; // フォント\n"
		writeFile(t, source, []byte(content))

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, source)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSkipped, result.Status)

		got := readFile(t, source)
		assert.Contains(t, string(got), "var face; // フォント")
	})
}

func TestScriptAdjuster_Convert_YesNoDialogReplacement(t *testing.T) {
	t.Parallel()

	t.Run("yesnodialog.tjsは単一ウィンドウ実装へ全置換される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "YesNoDialog.tjs")
		content := "\uFEFFclass YesNoDialogWindow extends Window\n{\n}\nfunction askYesNo(message, caption = \"確認\")\n{\n\tvar win = new YesNoDialogWindow(message, caption);\n}\n"
		writeFile(t, source, []byte(content))

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, source)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)

		got := string(readFile(t, source))
		assert.Contains(t, got, "System.showYesNoMessageBox")
		assert.Contains(t, got, "function askYesNo(message, caption = \"確認\")")
		assert.NotContains(t, got, "new YesNoDialogWindow")
	})

	t.Run("他のファイルは置換されない", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "other.tjs")
		content := "\uFEFFvar win = new YesNoDialogWindow(message, caption);\n"
		writeFile(t, source, []byte(content))

		adjuster := converter.NewScriptAdjuster(nil, true)
		result, err := adjuster.Convert(source, source)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSkipped, result.Status)
	})
}
