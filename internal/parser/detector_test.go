package parser_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/japanese"

	"github.com/na2na-p/mnemonic/internal/parser"
)

// fixturesDir はGameDetectorテスト用の静的フィクスチャディレクトリを返す。
//
// GameDetectorのテスト用に用意したダミーゲームディレクトリ
// （testdata配下）。
func fixturesDir(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err)

	return filepath.Join(wd, "testdata", "game_samples")
}

func TestEngineType_Values(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		engineType parser.EngineType
		expected   string
	}{
		"正常系: KIRIKIRI2の値が正しい":      {parser.EngineKirikiri2, "kirikiri2"},
		"正常系: KIRIKIRI2_KAG3の値が正しい": {parser.EngineKirikiri2KAG3, "kirikiri2_kag3"},
		"正常系: UNKNOWNの値が正しい":        {parser.EngineUnknown, "unknown"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, string(tc.engineType))
		})
	}
}

func TestNewGameDetector(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 有効なディレクトリで初期化できる", func(t *testing.T) {
		t.Parallel()

		gameDir := filepath.Join(fixturesDir(t), "kirikiri2_game")
		detector, err := parser.NewGameDetector(gameDir)

		require.NoError(t, err)
		assert.NotNil(t, detector)
	})

	t.Run("異常系: 存在しないディレクトリでErrGameDirNotFound", func(t *testing.T) {
		t.Parallel()

		_, err := parser.NewGameDetector("/nonexistent/path/to/game")

		require.ErrorIs(t, err, parser.ErrGameDirNotFound)
	})

	t.Run("異常系: ファイルパスを指定するとErrGameDirNotADirectory", func(t *testing.T) {
		t.Parallel()

		filePath := filepath.Join(t.TempDir(), "not_a_dir.txt")
		writeFile(t, filePath, []byte("test"))

		_, err := parser.NewGameDetector(filePath)

		require.ErrorIs(t, err, parser.ErrGameDirNotADirectory)
	})
}

func TestGameDetector_Detect(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 吉里吉里2ゲームを正しく検出できる", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
		require.NoError(t, err)

		result, err := detector.Detect()

		require.NoError(t, err)
		assert.Contains(t, []parser.EngineType{parser.EngineKirikiri2, parser.EngineKirikiri2KAG3}, result.Engine)
	})

	t.Run("正常系: KAG3スクリプト(.ks)を検出できる", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
		require.NoError(t, err)

		result, err := detector.Detect()
		require.NoError(t, err)

		ksCount := 0
		for _, s := range result.Scripts {
			if filepath.Ext(s) == ".ks" {
				ksCount++
			}
		}
		assert.Positive(t, ksCount)
	})

	t.Run("正常系: TJSスクリプト(.tjs)を検出できる", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
		require.NoError(t, err)

		result, err := detector.Detect()
		require.NoError(t, err)

		tjsCount := 0
		for _, s := range result.Scripts {
			if filepath.Ext(s) == ".tjs" {
				tjsCount++
			}
		}
		assert.Positive(t, tjsCount)
	})

	t.Run("正常系: 画像ファイルを検出できる", func(t *testing.T) {
		t.Parallel()

		for _, ext := range []string{".tlg", ".bmp"} {
			t.Run(ext, func(t *testing.T) {
				t.Parallel()

				detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
				require.NoError(t, err)

				result, err := detector.Detect()
				require.NoError(t, err)

				count := 0
				for _, img := range result.Images {
					if filepath.Ext(img) == ext {
						count++
					}
				}
				assert.Positive(t, count)
			})
		}
	})

	t.Run("正常系: 音声ファイルを検出できる", func(t *testing.T) {
		t.Parallel()

		for _, ext := range []string{".ogg", ".wav"} {
			t.Run(ext, func(t *testing.T) {
				t.Parallel()

				detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
				require.NoError(t, err)

				result, err := detector.Detect()
				require.NoError(t, err)

				count := 0
				for _, a := range result.Audio {
					if filepath.Ext(a) == ext {
						count++
					}
				}
				assert.Positive(t, count)
			})
		}
	})

	t.Run("正常系: 動画ファイル(.mpg)を検出できる", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
		require.NoError(t, err)

		result, err := detector.Detect()
		require.NoError(t, err)
		assert.NotEmpty(t, result.Video)
	})

	t.Run("正常系: プラグイン(.dll)を検出できる", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
		require.NoError(t, err)

		result, err := detector.Detect()
		require.NoError(t, err)
		assert.NotEmpty(t, result.Plugins)
	})

	t.Run("異常系: 空のディレクトリでErrEmptyGameDir", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "empty_dir"))
		require.NoError(t, err)

		_, err = detector.Detect()

		require.ErrorIs(t, err, parser.ErrEmptyGameDir)
	})

	t.Run("正常系: ゲームでないディレクトリはUNKNOWN判定", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "non_game_dir"))
		require.NoError(t, err)

		result, err := detector.Detect()

		require.NoError(t, err)
		assert.Equal(t, parser.EngineUnknown, result.Engine)
	})
}

func TestGameDetector_GetSummary(t *testing.T) {
	t.Parallel()

	t.Run("正常系: サマリーが文字列として取得できる", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
		require.NoError(t, err)

		summary, err := detector.GetSummary()

		require.NoError(t, err)
		assert.NotEmpty(t, summary)
	})

	t.Run("正常系: サマリーにエンジン情報が含まれる", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
		require.NoError(t, err)

		summary, err := detector.GetSummary()

		require.NoError(t, err)
		assert.Contains(t, strings.ToLower(summary), "kirikiri")
	})

	t.Run("正常系: サマリーにファイル数（数字）が含まれる", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
		require.NoError(t, err)

		summary, err := detector.GetSummary()

		require.NoError(t, err)
		assert.Regexp(t, `[0-9]`, summary)
	})

	t.Run("正常系: 各リソース種別の情報が含まれる", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
		require.NoError(t, err)

		summary, err := detector.GetSummary()

		require.NoError(t, err)
		assert.Contains(t, summary, "Scripts")
		assert.Contains(t, summary, "Images")
		assert.Contains(t, summary, "Audio")
	})
}

func TestGameDetector_TitleDetection(t *testing.T) {
	t.Parallel()

	t.Run("正常系: Config.tjsからタイトルを正しく検出できる", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
		require.NoError(t, err)

		result, err := detector.Detect()

		require.NoError(t, err)
		assert.Equal(t, "テストゲーム", result.Title)
	})

	t.Run("正常系: Config.tjsにSystem.titleが存在しない場合は空文字列", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		systemDir := filepath.Join(dir, "system")
		require.NoError(t, os.Mkdir(systemDir, 0o750))
		writeFile(t, filepath.Join(systemDir, "Config.tjs"), []byte("// No title here\nclass Config {}"))
		writeFile(t, filepath.Join(dir, "dummy.txt"), []byte("dummy"))

		detector, err := parser.NewGameDetector(dir)
		require.NoError(t, err)

		result, err := detector.Detect()

		require.NoError(t, err)
		assert.Empty(t, result.Title)
	})

	t.Run("正常系: Config.tjsが存在しない場合は空文字列", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "dummy.txt"), []byte("dummy"))

		detector, err := parser.NewGameDetector(dir)
		require.NoError(t, err)

		result, err := detector.Detect()

		require.NoError(t, err)
		assert.Empty(t, result.Title)
	})
}

func TestGameDetector_Integration(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 検出からサマリー取得までの一連のワークフロー", func(t *testing.T) {
		t.Parallel()

		detector, err := parser.NewGameDetector(filepath.Join(fixturesDir(t), "kirikiri2_game"))
		require.NoError(t, err)

		structure, err := detector.Detect()
		require.NoError(t, err)

		assert.Contains(
			t,
			[]parser.EngineType{parser.EngineKirikiri2, parser.EngineKirikiri2KAG3},
			structure.Engine,
		)
		assert.Equal(t, "テストゲーム", structure.Title)
		assert.NotEmpty(t, structure.Scripts)
		assert.NotEmpty(t, structure.Images)
		assert.NotEmpty(t, structure.Audio)

		summary, err := detector.GetSummary()
		require.NoError(t, err)
		assert.NotEmpty(t, summary)
	})
}

// TestGameDetector_ScriptEncodingDetection はchardet結果の文字コード名が
// 共通の語彙（"ascii"/"utf-8"/"shift_jis"）に正規化されることを固定する
// リグレッションテスト。
//
// github.com/saintfish/chardet は純ASCII入力に対して専用の判定器を持たず
// "ISO-8859-1"（低信頼度）等を返すことがあり、"ascii"という一貫した結果に
// ならない問題がある（GetSummary出力や再エンコード判定に影響するユーザー
// 可視の差分）ため、detectCharsetのASCII優先判定を固定する。
func TestGameDetector_ScriptEncodingDetection(t *testing.T) {
	t.Parallel()

	t.Run("正常系: ASCIIスクリプトはasciiと判定される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		content := strings.Repeat("@bg storage=bg01\nvar x = 1;\n", 5)
		writeFile(t, filepath.Join(dir, "first.ks"), []byte(content))

		detector, err := parser.NewGameDetector(dir)
		require.NoError(t, err)

		result, err := detector.Detect()

		require.NoError(t, err)
		assert.Equal(t, "ascii", result.ScriptEncoding)
	})

	t.Run("正常系: ISO-2022-JPスクリプトはESCバイトによりASCII扱いされない", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// 「こんにちは」のISO-2022-JP表現。全バイトが7ビットだがESC(0x1B)を含む。
		iso2022jp := []byte("\x1b$B$3$s$K$A$O\x1b(B")
		content := bytes.Repeat(iso2022jp, 10)
		writeFile(t, filepath.Join(dir, "first.ks"), content)

		detector, err := parser.NewGameDetector(dir)
		require.NoError(t, err)

		result, err := detector.Detect()

		require.NoError(t, err)
		assert.NotEqual(t, "ascii", result.ScriptEncoding)
	})

	t.Run("正常系: UTF-8スクリプトはutf-8と判定される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		content := strings.Repeat(
			"これはKirikiriのシナリオスクリプトのサンプルです。日本語のテキストを含みます。", 3,
		)
		writeFile(t, filepath.Join(dir, "first.ks"), []byte(content))

		detector, err := parser.NewGameDetector(dir)
		require.NoError(t, err)

		result, err := detector.Detect()

		require.NoError(t, err)
		assert.Equal(t, "utf-8", result.ScriptEncoding)
	})

	t.Run("正常系: Shift_JISスクリプトはshift_jisと判定される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		text := strings.Repeat(
			"これはKirikiriのシナリオスクリプトのサンプルです。日本語のテキストを含みます。", 3,
		)
		encoded, err := japanese.ShiftJIS.NewEncoder().String(text)
		require.NoError(t, err)
		writeFile(t, filepath.Join(dir, "first.ks"), []byte(encoded))

		detector, err := parser.NewGameDetector(dir)
		require.NoError(t, err)

		result, err := detector.Detect()

		require.NoError(t, err)
		assert.Equal(t, "shift_jis", result.ScriptEncoding)
	})
}
