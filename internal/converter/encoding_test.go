package converter_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/converter"
)

func fixturesDir(t *testing.T) string {
	t.Helper()

	return filepath.Join("testdata", "encoding")
}

func TestSupportedEncodings(t *testing.T) {
	t.Parallel()

	for _, want := range []string{"shift_jis", "euc-jp", "utf-8", "gb2312", "big5", "cp949"} {
		assert.Contains(t, converter.SupportedEncodings, want)
	}
}

func TestEncodingDetector_Detect(t *testing.T) {
	t.Parallel()

	detector := converter.NewEncodingDetector()

	cases := map[string]struct {
		filename string
		expected string
	}{
		"正常系: Shift_JISファイルの検出":  {"shift_jis.txt", "shift_jis"},
		"正常系: UTF-8ファイルの検出":      {"utf8.txt", "utf-8"},
		"正常系: BOM付きUTF-8ファイルの検出": {"utf8_bom.txt", "utf-8"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result, err := detector.Detect(filepath.Join(fixturesDir(t), tc.filename))

			require.NoError(t, err)
			assert.NotEmpty(t, result.Encoding)
			detectedLower := strings.ReplaceAll(strings.ToLower(result.Encoding), "-", "_")
			expectedLower := strings.ReplaceAll(strings.ToLower(tc.expected), "-", "_")
			assert.True(t, detectedLower == expectedLower || strings.Contains(detectedLower, expectedLower))
			assert.Greater(t, result.Confidence, 0.5)
			assert.True(t, result.IsSupported)
		})
	}

	t.Run("申し送り: BOM付きUTF-8はutf-8-sigではなくutf-8として検出される", func(t *testing.T) {
		t.Parallel()

		// why: PythonのchardetはBOM付き入力に"utf-8-sig"を返すが、
		// Go側(saintfish/chardet)は常に"UTF-8"を返す。EncodingConverter.Convert
		// のスキップ判定は生バイト列のBOM有無で独立して行うため、この語彙差は
		// 検出結果の名称に現れないことをここでピン留めする。
		result, err := detector.Detect(filepath.Join(fixturesDir(t), "utf8_bom.txt"))

		require.NoError(t, err)
		assert.Equal(t, "utf-8", result.Encoding)
	})

	t.Run("異常系: 存在しないファイル", func(t *testing.T) {
		t.Parallel()

		_, err := detector.Detect(filepath.Join(fixturesDir(t), "nonexistent.txt"))

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrEncodingFileNotFound)
	})

	t.Run("正常系: 空ファイルでも結果を返す", func(t *testing.T) {
		t.Parallel()

		result, err := detector.Detect(filepath.Join(fixturesDir(t), "empty.txt"))

		require.NoError(t, err)
		assert.Empty(t, result.Encoding)
	})
}

func TestEncodingDetector_DetectBytes(t *testing.T) {
	t.Parallel()

	detector := converter.NewEncodingDetector()

	t.Run("正常系: UTF-8のバイトデータを検出", func(t *testing.T) {
		t.Parallel()

		result := detector.DetectBytes([]byte("これはテストです"))

		assert.Contains(t, strings.ToLower(result.Encoding), "utf")
		assert.Greater(t, result.Confidence, 0.5)
		assert.True(t, result.IsSupported)
	})

	t.Run("正常系: 空のバイトデータでも結果を返す", func(t *testing.T) {
		t.Parallel()

		result := detector.DetectBytes([]byte{})

		assert.Empty(t, result.Encoding)
		assert.False(t, result.IsSupported)
	})
}

func TestEncodingDetector_IsTextFile(t *testing.T) {
	t.Parallel()

	detector := converter.NewEncodingDetector()

	cases := map[string]struct {
		filename string
		expected bool
	}{
		"正常系: UTF-8テキストファイル":      {"utf8.txt", true},
		"正常系: Shift_JISテキストファイル":  {"shift_jis.txt", true},
		"正常系: BOM付きUTF-8テキストファイル": {"utf8_bom.txt", true},
		"正常系: バイナリファイル":           {"binary.dat", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result, err := detector.IsTextFile(filepath.Join(fixturesDir(t), tc.filename))

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}

	t.Run("異常系: 存在しないファイル", func(t *testing.T) {
		t.Parallel()

		_, err := detector.IsTextFile(filepath.Join(fixturesDir(t), "nonexistent.txt"))

		require.Error(t, err)
	})

	t.Run("正常系: 空ファイルはテキストファイルとして判定", func(t *testing.T) {
		t.Parallel()

		result, err := detector.IsTextFile(filepath.Join(fixturesDir(t), "empty.txt"))

		require.NoError(t, err)
		assert.True(t, result)
	})
}

func TestNewEncodingConverter(t *testing.T) {
	t.Parallel()

	t.Run("正常系: デフォルトのターゲットエンコーディングはutf-8", func(t *testing.T) {
		t.Parallel()

		c := converter.NewEncodingConverter("", "")
		assert.Equal(t, "utf-8", c.TargetEncoding())
	})

	t.Run("正常系: デフォルトのソースエンコーディングは空文字列(自動検出)", func(t *testing.T) {
		t.Parallel()

		c := converter.NewEncodingConverter("", "")
		assert.Empty(t, c.SourceEncoding())
	})

	t.Run("正常系: カスタムターゲットエンコーディング", func(t *testing.T) {
		t.Parallel()

		c := converter.NewEncodingConverter("shift_jis", "")
		assert.Equal(t, "shift_jis", c.TargetEncoding())
	})

	t.Run("正常系: カスタムソースエンコーディング", func(t *testing.T) {
		t.Parallel()

		c := converter.NewEncodingConverter("", "shift_jis")
		assert.Equal(t, "shift_jis", c.SourceEncoding())
	})
}

func TestEncodingConverter_SupportedExtensions(t *testing.T) {
	t.Parallel()

	c := converter.NewEncodingConverter("", "")
	for _, ext := range []string{".ks", ".tjs", ".txt", ".csv", ".ini"} {
		assert.Contains(t, c.SupportedExtensions(), ext)
	}
}

func TestEncodingConverter_CanConvert(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		filename string
		expected bool
	}{
		"正常系: .ksファイルは変換可能":  {"script.ks", true},
		"正常系: .tjsファイルは変換可能": {"script.tjs", true},
		"正常系: .txtファイルは変換可能": {"readme.txt", true},
		"正常系: .csvファイルは変換可能": {"data.csv", true},
		"正常系: .iniファイルは変換可能": {"config.ini", true},
		"正常系: .pngファイルは変換不可": {"image.png", false},
		"正常系: .oggファイルは変換不可": {"audio.ogg", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := converter.NewEncodingConverter("", "")
			testFile := filepath.Join(t.TempDir(), tc.filename)
			writeFile(t, testFile, []byte("test content"))

			assert.Equal(t, tc.expected, c.CanConvert(testFile))
		})
	}

	t.Run("正常系: テキスト拡張子でもバイナリファイルはFalse", func(t *testing.T) {
		t.Parallel()

		binaryPath := filepath.Join(fixturesDir(t), "binary.dat")
		c := converter.NewEncodingConverter("", "")

		assert.False(t, c.CanConvert(binaryPath))
	})
}

func TestEncodingConverter_Convert(t *testing.T) {
	t.Parallel()

	t.Run("正常系: Shift_JISファイルをUTF-8に変換", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		dest := filepath.Join(dir, "dest.txt")

		text := "これはテストです。日本語の文章。"
		writeSJIS(t, source, text)

		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assertFileUTF8Equals(t, dest, text)
	})

	t.Run("正常系: EUC-JPファイルをUTF-8に変換", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		dest := filepath.Join(dir, "dest.txt")

		text := "これはEUC-JPエンコーディングのテストです。日本語。"
		writeEUCJP(t, source, text)

		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assertFileUTF8Equals(t, dest, text)
	})

	t.Run("正常系: BOM付きUTF-8からBOMを除去", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		dest := filepath.Join(dir, "dest.txt")

		text := "BOM付きファイルのテスト"
		bom := []byte{0xef, 0xbb, 0xbf}
		writeFile(t, source, append(append([]byte{}, bom...), []byte(text)...))

		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		destBytes := readFile(t, dest)
		assert.False(t, strings.HasPrefix(string(destBytes), string(bom)))
		assert.Equal(t, text, string(destBytes))
	})

	t.Run("正常系: 既にUTF-8のファイルはスキップされる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		dest := filepath.Join(dir, "dest.txt")

		writeFile(t, source, []byte("これは既にUTF-8のファイルです"))

		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSkipped, result.Status)
	})

	t.Run("正常系: 指定されたソースエンコーディングを使用する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		dest := filepath.Join(dir, "dest.txt")

		text := "ソースエンコーディング指定テスト"
		writeSJIS(t, source, text)

		c := converter.NewEncodingConverter("", "shift_jis")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assertFileUTF8Equals(t, dest, text)
	})

	t.Run("正常系: 変換前後のバイト数が記録される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		dest := filepath.Join(dir, "dest.txt")

		// why not: 短い文字列だとgithub.com/saintfish/chardetの検出精度が
		// Python版chardetより低く、shift_jis以外に誤検出されることがあるため
		// 十分な長さのテキストを使用する（Python版テストの他ケースと同じ対応）。
		writeSJIS(t, source, "バイトサイズテストです。日本語の文章を十分な長さにして文字コード検出の精度を確保します。")

		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Positive(t, result.BytesBefore)
		assert.Positive(t, result.BytesAfter)
	})

	t.Run("異常系: 存在しないファイルはFAILED", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(filepath.Join(dir, "nonexistent.txt"), filepath.Join(dir, "dest.txt"))

		require.NoError(t, err)
		assert.Equal(t, converter.StatusFailed, result.Status)
	})

	t.Run("正常系: 変換先ディレクトリが存在しない場合は作成する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		dest := filepath.Join(dir, "subdir", "dest.txt")

		// why not: 短い文字列だとchardetの検出精度が下がるため長めのテキストを使用する。
		writeSJIS(t, source, "ディレクトリ作成テストです。日本語の文章を十分な長さにして文字コード検出の精度を確保します。")

		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})
}

func TestEncodingConverter_ConvertBytes(t *testing.T) {
	t.Parallel()

	t.Run("正常系: Shift_JISバイトデータをUTF-8に変換", func(t *testing.T) {
		t.Parallel()

		text := "これはShift_JISエンコーディングのテストファイルです。日本語の文章を含みます。吾輩は猫である。名前はまだ無い。"
		sjisBytes := encodeSJIS(t, text)

		c := converter.NewEncodingConverter("", "")
		resultBytes, detected, err := c.ConvertBytes(sjisBytes)

		require.NoError(t, err)
		assert.Equal(t, text, string(resultBytes))
		lower := strings.ToLower(detected)
		assert.True(t, strings.Contains(lower, "shift") || strings.Contains(lower, "sjis"))
	})

	t.Run("正常系: EUC-JPバイトデータをUTF-8に変換", func(t *testing.T) {
		t.Parallel()

		text := "これはEUC-JPエンコーディングのテストファイルです。日本語の文章を含みます。坊っちゃん。親譲りの無鉄砲で小供の時から損ばかりしている。"
		eucBytes := encodeEUCJP(t, text)

		c := converter.NewEncodingConverter("", "")
		resultBytes, _, err := c.ConvertBytes(eucBytes)

		require.NoError(t, err)
		assert.Equal(t, text, string(resultBytes))
	})

	t.Run("正常系: 指定されたソースエンコーディングを使用する", func(t *testing.T) {
		t.Parallel()

		text := "ソースエンコーディング指定"
		sjisBytes := encodeSJIS(t, text)

		c := converter.NewEncodingConverter("", "shift_jis")
		resultBytes, detected, err := c.ConvertBytes(sjisBytes)

		require.NoError(t, err)
		assert.Equal(t, text, string(resultBytes))
		assert.Equal(t, "shift_jis", detected)
	})

	t.Run("正常系: バイトデータからBOMを除去する", func(t *testing.T) {
		t.Parallel()

		text := "BOMテスト"
		bom := []byte{0xef, 0xbb, 0xbf}
		data := append(append([]byte{}, bom...), []byte(text)...)

		c := converter.NewEncodingConverter("", "")
		resultBytes, _, err := c.ConvertBytes(data)

		require.NoError(t, err)
		assert.False(t, strings.HasPrefix(string(resultBytes), string(bom)))
		assert.Equal(t, text, string(resultBytes))
	})
}

func TestEncodingConverter_JapanesePreservation(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		sourceEncoding string
		text           string
	}{
		"正常系: Shift_JISの日本語が保全される": {
			"shift_jis",
			"吾輩は猫である。名前はまだ無い。どこで生れたかとんと見当がつかぬ。何でも薄暗いじめじめした所でニャーニャー泣いていた事だけは記憶している。",
		},
		"正常系: EUC-JPの日本語が保全される": {
			"euc-jp",
			"坊っちゃん。親譲りの無鉄砲で小供の時から損ばかりしている。小学校に居る時分学校の二階から飛び降りて一週間ほど腰を抜かした事がある。",
		},
		"正常系: 複合文字が保全される": {
			"shift_jis",
			"漢字、ひらがな、カタカナ、ＡＢＣ、１２３。日本語のテキストファイルに含まれる様々な文字を保全することを確認します。",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			source := filepath.Join(dir, "source.txt")
			dest := filepath.Join(dir, "dest.txt")

			switch tc.sourceEncoding {
			case "shift_jis":
				writeSJIS(t, source, tc.text)
			case "euc-jp":
				writeEUCJP(t, source, tc.text)
			}

			c := converter.NewEncodingConverter("", "")
			result, err := c.Convert(source, dest)

			require.NoError(t, err)
			assert.Equal(t, converter.StatusSuccess, result.Status)
			assertFileUTF8Equals(t, dest, tc.text)
		})
	}
}
