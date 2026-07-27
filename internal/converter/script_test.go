package converter_test

import (
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

	t.Run("正常系: startup.tjsにエンコーディングディレクティブを追加できる", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		content := "// Original script\nvar initialized = false;\n"

		result := adjuster.AddStartupDirective(content)

		assert.Contains(t, result, "@if (kirikiriz)")
		assert.Contains(t, result, `System.setArgument("-readencoding", "UTF-8");`)
		assert.Contains(t, result, "@endif")
		assert.Contains(t, result, "// Original script")
	})

	t.Run("正常系: ディレクティブがファイル先頭に追加される", func(t *testing.T) {
		t.Parallel()

		adjuster := converter.NewScriptAdjuster(nil, true)
		result := adjuster.AddStartupDirective("var x = 1;")

		assert.True(t, strings.HasPrefix(result, "@if (kirikiriz)"))
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
		assert.Contains(t, string(converted), "@if (kirikiriz)")
		assert.Contains(t, string(converted), `System.setArgument("-readencoding", "UTF-8");`)
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
		assert.NotContains(t, string(converted), "@if (kirikiriz)")
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
}

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
