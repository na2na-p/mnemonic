package converter_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/na2na-p/mnemonic/internal/converter"
)

func fixturesDir(t *testing.T) string {
	t.Helper()

	return filepath.Join("testdata", "encoding")
}

func TestSupportedEncodings(t *testing.T) {
	t.Parallel()

	for _, want := range []string{"shift_jis", "euc-jp", "utf-8", "gb2312", "gb18030", "big5", "cp949", "utf-16le", "utf-16be"} {
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

		// why: Go側(saintfish/chardet)はBOM付き入力に対しても常に"UTF-8"を返す。
		// EncodingConverter.Convertのスキップ判定は生バイト列のBOM有無で独立して
		// 行うため、この語彙差は検出結果の名称に現れないことをここでピン留めする。
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

	t.Run("正常系: 純ASCIIバイト列はutf-8として検出される", func(t *testing.T) {
		t.Parallel()

		// why: github.com/saintfish/chardetは専用のASCII判定器を持たず、純ASCII
		// バイト列に対しても"ISO-8859-1"等の低信頼度フォールバックを返すことが
		// あるため、charset.IsASCIIによる短絡が効いていることをピン留めする。
		result := detector.DetectBytes([]byte("key=value\nname=example\n"))

		assert.Equal(t, "utf-8", result.Encoding)
		assert.InDelta(t, 1.0, result.Confidence, 1e-9)
		assert.True(t, result.IsSupported)
	})

	bomCases := []struct {
		name         string
		data         []byte
		wantEncoding string
	}{
		{
			name:         "正常系: UTF-16LEのBOMで始まるバイト列はutf-16leとして検出される",
			data:         encodeUTF16("吉里吉里スクリプト", false, true),
			wantEncoding: "utf-16le",
		},
		{
			name:         "正常系: UTF-16BEのBOMで始まるバイト列はutf-16beとして検出される",
			data:         encodeUTF16("吉里吉里スクリプト", true, true),
			wantEncoding: "utf-16be",
		},
		{
			name:         "正常系: simple crypt mode 0のバイト列はutf-16leとして検出される",
			data:         encodeSimpleCrypt(t, "吉里吉里スクリプト", 0),
			wantEncoding: "utf-16le",
		},
		{
			name:         "正常系: simple crypt mode 1のバイト列はutf-16leとして検出される",
			data:         encodeSimpleCrypt(t, "吉里吉里スクリプト", 1),
			wantEncoding: "utf-16le",
		},
		{
			name:         "正常系: simple crypt mode 2のバイト列はutf-16leとして検出される",
			data:         encodeSimpleCryptCompressed(t, "吉里吉里スクリプト"),
			wantEncoding: "utf-16le",
		},
	}

	for _, tc := range bomCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := detector.DetectBytes(tc.data)

			assert.Equal(t, tc.wantEncoding, result.Encoding)
			assert.InDelta(t, 1.0, result.Confidence, 1e-9)
			assert.True(t, result.IsSupported)
		})
	}
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

	utf16Cases := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{"正常系: BOM付きUTF-16LEはNULを含んでもテキストファイル", encodeUTF16("[playbgm storage=\"bgm.mid\"]", false, true), true},
		{"正常系: BOM付きUTF-16BEはNULを含んでもテキストファイル", encodeUTF16("[playbgm storage=\"bgm.mid\"]", true, true), true},
		{"正常系: BOM無しUTF-16LEはバイナリファイル", encodeUTF16("[playbgm storage=\"bgm.mid\"]", false, false), false},
		{"正常系: UTF-32LEのBOMで始まるデータはUTF-16扱いせずバイナリファイル", []byte{0xff, 0xfe, 0x00, 0x00, 0x41, 0x00, 0x00, 0x00}, false},
		{"正常系: simple crypt mode 0はNULを含んでもテキストファイル", encodeSimpleCrypt(t, "[playbgm storage=\"bgm.mid\"]", 0), true},
		{"正常系: simple crypt mode 1はNULを含んでもテキストファイル", encodeSimpleCrypt(t, "[playbgm storage=\"bgm.mid\"]", 1), true},
		{"正常系: simple crypt mode 2はNULを含んでもテキストファイル", encodeSimpleCryptCompressed(t, "[playbgm storage=\"bgm.mid\"]"), true},
		{"正常系: 未対応モードのsimple cryptは通常判定に落ちNULを含むためバイナリファイル", []byte{0xfe, 0xfe, 0x03, 0xff, 0xfe, 0x41, 0x00}, false},
		{"正常系: 文字コードを推定できないShift_JISの1文字もNULを含まなければテキストファイル", encodeWith(t, japanese.ShiftJIS, "猫"), true},
		{"正常系: 文字コードを推定できないバイト列もNULを含まなければテキストファイル", []byte{0x94}, true},
		{"正常系: NULを含むデータはバイナリファイル", []byte("[l]\x00[r]"), false},
	}

	for _, tc := range utf16Cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "script.ks")
			writeFile(t, path, tc.data)

			result, err := detector.IsTextFile(path)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
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
	for _, ext := range []string{".ks", ".tjs", ".txt", ".csv", ".ini", ".asd"} {
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

	t.Run("正常系: ASCIIのみの.iniファイルはSKIPPEDになる", func(t *testing.T) {
		t.Parallel()

		// why: ASCII短絡が無いと自動検出がutf-8ではないエンコーディング名を返し
		// エンコーディング変換失敗のエラーになってしまうことの回帰を防止する。
		dir := t.TempDir()
		source := filepath.Join(dir, "config.ini")
		dest := filepath.Join(dir, "dest.ini")

		writeFile(t, source, []byte("[section]\nkey=value\nname=example\n"))

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

		// why not: 短い文字列だとgithub.com/saintfish/chardetの検出精度が低く、
		// shift_jis以外に誤検出されることがあるため十分な長さのテキストを使用する。
		writeSJIS(t, source, "バイトサイズテストです。日本語の文章を十分な長さにして文字コード検出の精度を確保します。")

		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Positive(t, result.BytesBefore)
		assert.Positive(t, result.BytesAfter)
	})

	t.Run("異常系: 存在しないファイルは再試行不要なエラー", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "nonexistent.txt")
		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(source, filepath.Join(dir, "dest.txt"))

		require.ErrorIs(t, err, converter.ErrSourceNotFound)
		require.ErrorIs(t, err, converter.ErrPermanentFailure)
		assert.Contains(t, err.Error(), "変換元ファイルが見つかりません: "+source)
		assert.Equal(t, source, result.SourcePath)
	})

	t.Run("異常系: 権限不足で確認できない変換元は見つからないとは報告せず再試行不要なエラー", func(t *testing.T) {
		t.Parallel()

		if os.Geteuid() == 0 {
			t.Skip("rootはパーミッションに関係なく読み込めるため再現できない")
		}

		source := writeFileInLockedDir(t, "source.txt", []byte("plain ascii text"))
		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(source, filepath.Join(t.TempDir(), "dest.txt"))

		require.ErrorIs(t, err, converter.ErrSourceUnreadable)
		require.ErrorIs(t, err, fs.ErrPermission)
		require.ErrorIs(t, err, converter.ErrPermanentFailure)
		require.NotErrorIs(t, err, converter.ErrSourceNotFound)
		assert.NotContains(t, err.Error(), "見つかりません")
		assert.Contains(t, err.Error(), source)
		require.EqualError(t, err, "再試行しても解消しない変換失敗です: 変換元ファイルを読み込めません: stat "+source+": permission denied")
		assert.Equal(t, source, result.SourcePath)
	})

	t.Run("異常系: 未対応のソースエンコーディングは再試行不要なエラー", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		dest := filepath.Join(dir, "dest.txt")
		writeFile(t, source, []byte("plain ascii text"))

		c := converter.NewEncodingConverter("", "unknown-encoding")
		_, err := c.Convert(source, dest)

		require.ErrorIs(t, err, converter.ErrEncodingConversionFailed)
		require.ErrorIs(t, err, converter.ErrUnsupportedEncoding)
		require.ErrorIs(t, err, converter.ErrPermanentFailure)
		assert.Contains(t, err.Error(), "エンコーディング変換に失敗しました: ")
		assert.NoFileExists(t, dest)
	})

	t.Run("異常系: 出力先の親がファイルでディレクトリを作れない場合は再試行対象のエラー", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		writeFile(t, source, []byte("plain ascii text"))
		blocker := filepath.Join(dir, "blocker")
		writeFile(t, blocker, []byte("not a directory"))

		c := converter.NewEncodingConverter("", "shift_jis")
		_, err := c.Convert(source, filepath.Join(blocker, "dest.txt"))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "出力先ディレクトリの作成に失敗しました")
		require.NotErrorIs(t, err, converter.ErrPermanentFailure)
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

func TestEncodingConverter_Convert_UTF16(t *testing.T) {
	t.Parallel()

	const text = "[playbgm storage=\"bgm.mid\"]\r\n吉里吉里のUTF-16スクリプトです。"

	tests := []struct {
		name      string
		fileName  string
		bigEndian bool
		want      []byte
	}{
		{
			name:     "正常系: BOM付きUTF-16LEの.ksはUTF-8 BOM付きに変換される",
			fileName: "first.ks",
			want:     append([]byte{0xef, 0xbb, 0xbf}, text...),
		},
		{
			name:      "正常系: BOM付きUTF-16BEの.tjsはUTF-8 BOM付きに変換される",
			fileName:  "startup.tjs",
			bigEndian: true,
			want:      append([]byte{0xef, 0xbb, 0xbf}, text...),
		},
		{
			name:     "正常系: BOM付きUTF-16LEの.txtもUTF-8 BOM付きに変換される",
			fileName: "readme.txt",
			want:     append([]byte{0xef, 0xbb, 0xbf}, text...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			source := filepath.Join(dir, tt.fileName)
			dest := filepath.Join(dir, "out", tt.fileName)
			writeFile(t, source, encodeUTF16(text, tt.bigEndian, true))

			c := converter.NewEncodingConverter("", "")
			require.True(t, c.CanConvert(source))

			result, err := c.Convert(source, dest)

			require.NoError(t, err)
			assert.Equal(t, converter.StatusSuccess, result.Status)
			assert.Equal(t, tt.want, readFile(t, dest))
		})
	}

	t.Run("異常系: UTF-16は変換先としては受け付けない", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.txt")
		dest := filepath.Join(dir, "dest.txt")
		writeFile(t, source, []byte("plain ascii text"))

		c := converter.NewEncodingConverter("utf-16le", "utf-8")
		_, err := c.Convert(source, dest)

		require.ErrorIs(t, err, converter.ErrUnsupportedEncoding)
		require.ErrorIs(t, err, converter.ErrPermanentFailure)
		assert.NoFileExists(t, dest)
	})
}

func TestEncodingConverter_Convert_SimpleCrypt(t *testing.T) {
	t.Parallel()

	const text = "[playbgm storage=\"bgm.mid\"]\r\n吉里吉里の暗号化スクリプトです。"
	utf8BOM := []byte{0xef, 0xbb, 0xbf}

	tests := []struct {
		name     string
		fileName string
		data     []byte
		want     []byte
	}{
		{
			name:     "正常系: simple crypt mode 0の.ksはUTF-8 BOM付きの平文に変換される",
			fileName: "first.ks",
			data:     encodeSimpleCrypt(t, text, 0),
			want:     append(bytes.Clone(utf8BOM), text...),
		},
		{
			name:     "正常系: simple crypt mode 1の.ksはUTF-8 BOM付きの平文に変換される",
			fileName: "first.ks",
			data:     encodeSimpleCrypt(t, text, 1),
			want:     append(bytes.Clone(utf8BOM), text...),
		},
		{
			name:     "正常系: simple crypt mode 2（zlib圧縮）の.ksはUTF-8 BOM付きの平文に変換される",
			fileName: "first.ks",
			data:     encodeSimpleCryptCompressed(t, text),
			want:     append(bytes.Clone(utf8BOM), text...),
		},
		{
			name:     "正常系: simple crypt mode 1の.txtもUTF-16由来としてUTF-8 BOM付きに変換される",
			fileName: "readme.txt",
			data:     encodeSimpleCrypt(t, text, 1),
			want:     append(bytes.Clone(utf8BOM), text...),
		},
		{
			name:     "正常系: simple crypt mode 1の符号単位に満たない末尾の1バイトは捨てる",
			fileName: "first.ks",
			data:     append(encodeSimpleCrypt(t, text, 1), 0x41),
			want:     append(bytes.Clone(utf8BOM), text...),
		},
		{
			name:     "正常系: mode 0の既知の暗号文（A→40 40・改行はそのまま・あ→43 72）を復号する",
			fileName: "vector.ks",
			data:     []byte{0xfe, 0xfe, 0x00, 0xff, 0xfe, 0x40, 0x40, 0x0a, 0x00, 0x43, 0x72},
			want:     append(bytes.Clone(utf8BOM), "A\nあ"...),
		},
		{
			name:     "正常系: mode 0の境界 0x0020 は復号され 0x001F はそのまま",
			fileName: "vector.ks",
			data:     []byte{0xfe, 0xfe, 0x00, 0xff, 0xfe, 0x20, 0x00, 0x1f, 0x00},
			want:     append(bytes.Clone(utf8BOM), "‡\x1f"...),
		},
		{
			name:     "正常系: mode 1の既知の暗号文（A→82 00・改行→05 00・あ→81 30）を復号する",
			fileName: "vector.ks",
			data:     []byte{0xfe, 0xfe, 0x01, 0xff, 0xfe, 0x82, 0x00, 0x05, 0x00, 0x81, 0x30},
			want:     append(bytes.Clone(utf8BOM), "A\nあ"...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			source := filepath.Join(dir, tt.fileName)
			dest := filepath.Join(dir, "out", tt.fileName)
			writeFile(t, source, tt.data)

			c := converter.NewEncodingConverter("", "")
			require.True(t, c.CanConvert(source))

			result, err := c.Convert(source, dest)

			require.NoError(t, err)
			assert.Equal(t, converter.StatusSuccess, result.Status)
			assert.Equal(t, tt.want, readFile(t, dest))
		})
	}

	plain := encodeUTF16(text, false, false)
	compressed := zlibCompress(t, plain)

	errorTests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{
			name:    "異常系: 未対応のモードバイトは恒久的な変換失敗になる",
			data:    []byte{0xfe, 0xfe, 0x03, 0xff, 0xfe, 0x41, 0x00},
			wantErr: converter.ErrEncodingConversionFailed,
		},
		{
			name:    "異常系: モードバイトの後にUTF-16LEのBOMが無ければ恒久的な変換失敗になる",
			data:    []byte{0xfe, 0xfe, 0x00, 0x41, 0x00, 0x42, 0x00},
			wantErr: converter.ErrEncodingConversionFailed,
		},
		{
			name:    "異常系: mode 2でサイズ欄が欠けていれば恒久的な変換失敗になる",
			data:    []byte{0xfe, 0xfe, 0x02, 0xff, 0xfe, 0x00, 0x00},
			wantErr: converter.ErrEncodingConversionFailed,
		},
		{
			name:    "異常系: mode 2で宣言された展開後サイズが上限を超えればErrScriptTooLargeになる",
			data:    simpleCryptCompressed(uint64(len(compressed)), 64<<20+1, compressed),
			wantErr: converter.ErrScriptTooLarge,
		},
		{
			name:    "異常系: mode 2で宣言された圧縮後サイズが実データより大きければ恒久的な変換失敗になる",
			data:    simpleCryptCompressed(uint64(len(compressed))+10, uint64(len(plain)), compressed),
			wantErr: converter.ErrEncodingConversionFailed,
		},
		{
			name:    "異常系: mode 2のzlibストリームが途中で切れていれば恒久的な変換失敗になる",
			data:    simpleCryptCompressed(uint64(len(compressed)/2), uint64(len(plain)), compressed[:len(compressed)/2]),
			wantErr: converter.ErrEncodingConversionFailed,
		},
		{
			name:    "異常系: mode 2の展開結果が宣言サイズより長ければ恒久的な変換失敗になる",
			data:    simpleCryptCompressed(uint64(len(compressed)), uint64(len(plain))-2, compressed),
			wantErr: converter.ErrEncodingConversionFailed,
		},
		{
			name:    "異常系: mode 2の展開結果が宣言サイズより短ければ恒久的な変換失敗になる",
			data:    simpleCryptCompressed(uint64(len(compressed)), uint64(len(plain))+2, compressed),
			wantErr: converter.ErrEncodingConversionFailed,
		},
	}

	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			source := filepath.Join(dir, "broken.ks")
			dest := filepath.Join(dir, "out", "broken.ks")
			writeFile(t, source, tt.data)

			c := converter.NewEncodingConverter("", "")
			_, err := c.Convert(source, dest)

			require.ErrorIs(t, err, tt.wantErr)
			require.ErrorIs(t, err, converter.ErrEncodingConversionFailed)
			require.ErrorIs(t, err, converter.ErrPermanentFailure)
			assert.NoFileExists(t, dest)
		})
	}
}

func TestEncodingConverter_ConvertBytes_SimpleCrypt(t *testing.T) {
	t.Parallel()

	const text = "吉里吉里の暗号化スクリプトです。"

	tests := []struct {
		name           string
		sourceEncoding string
	}{
		{name: "正常系: 自動検出ではsimple cryptを復号してutf-16leとして変換する", sourceEncoding: ""},
		{name: "正常系: 変換元エンコーディングの指定があってもsimple cryptは復号後のutf-16leとして変換する", sourceEncoding: "shift_jis"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := converter.NewEncodingConverter("", tt.sourceEncoding)
			resultBytes, detected, err := c.ConvertBytes(encodeSimpleCrypt(t, text, 1))

			require.NoError(t, err)
			assert.Equal(t, text, string(resultBytes))
			assert.Equal(t, "utf-16le", detected)
		})
	}

	t.Run("異常系: 未対応のモードバイトはエラーになる", func(t *testing.T) {
		t.Parallel()

		c := converter.NewEncodingConverter("", "")
		_, _, err := c.ConvertBytes([]byte{0xfe, 0xfe, 0x03, 0xff, 0xfe, 0x41, 0x00})

		require.Error(t, err)
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

	t.Run("正常系: ASCIIのみのバイトデータはutf-8としてそのまま通過する", func(t *testing.T) {
		t.Parallel()

		asciiBytes := []byte("key=value\nname=example\n")

		c := converter.NewEncodingConverter("", "")
		resultBytes, detected, err := c.ConvertBytes(asciiBytes)

		require.NoError(t, err)
		assert.Equal(t, asciiBytes, resultBytes)
		assert.Equal(t, "utf-8", detected)
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

func TestEncodingConverter_KirikiriScriptBOM(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 吉里吉里スクリプトファイルにはUTF-8 BOMが追加される", func(t *testing.T) {
		t.Parallel()

		for _, ext := range []string{".tjs", ".ks", ".asd"} {
			t.Run(ext, func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				source := filepath.Join(dir, "source"+ext)
				dest := filepath.Join(dir, "dest"+ext)
				text := "テスト文字列です。これは吉里吉里スクリプトファイルのテストです。"
				writeSJIS(t, source, text)

				c := converter.NewEncodingConverter("utf-8", "shift_jis")
				result, err := c.Convert(source, dest)

				require.NoError(t, err)
				assert.Equal(t, converter.StatusSuccess, result.Status)

				raw, readErr := os.ReadFile(dest) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
				require.NoError(t, readErr)
				assert.True(t, bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}), "UTF-8 BOMが付与されているべき")
			})
		}
	})

	t.Run("正常系: 非スクリプトファイルにはUTF-8 BOMが追加されない", func(t *testing.T) {
		t.Parallel()

		for _, ext := range []string{".txt", ".csv", ".ini"} {
			t.Run(ext, func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				source := filepath.Join(dir, "source"+ext)
				dest := filepath.Join(dir, "dest"+ext)
				text := "テスト文字列です。これは一般的なテキストファイルのテストです。"
				writeSJIS(t, source, text)

				c := converter.NewEncodingConverter("utf-8", "shift_jis")
				result, err := c.Convert(source, dest)

				require.NoError(t, err)
				assert.Equal(t, converter.StatusSuccess, result.Status)

				raw, readErr := os.ReadFile(dest) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
				require.NoError(t, readErr)
				assert.False(t, bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}), "UTF-8 BOMが付与されるべきではない")
			})
		}
	})

	t.Run("正常系: ASCIIのみの吉里吉里スクリプトファイルにもBOMが追加される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "source.asd")
		dest := filepath.Join(dir, "dest.asd")
		// why: .asdアニメーションファイルはASCIIのみの内容が一般的だが、吉里吉里の読み手の
		// 既定エンコーディング（ビルドによりShift_JISか厳格なUTF-8）に依存しないようBOMを
		// 付ける（charset.IsASCIIの短絡でsourceEncoding自動検出時に"utf-8"判定される
		// ケースでもBOM付与ルールが正しく効くことを確認する）。
		asciiContent := []byte("*start\r\n@wait time=150\r\n@clip left=445 top=0")
		require.NoError(t, os.WriteFile(source, asciiContent, 0o600))

		c := converter.NewEncodingConverter("utf-8", "")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)

		raw, readErr := os.ReadFile(dest) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, readErr)
		assert.True(t, bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}), "ASCIIスクリプトでもUTF-8 BOMが付与されるべき")
	})
}

// encodeWith はtextをencで符号化するテストヘルパー。
func encodeWith(t *testing.T, enc encoding.Encoding, text string) []byte {
	t.Helper()

	encoded, err := enc.NewEncoder().Bytes([]byte(text))
	require.NoError(t, err)

	return encoded
}

// why: chardetの推定名と信頼度は短い入力で外れやすく、変換元エンコーディングの
// 決定規則はその外れ方を前提に組んでいる。go.modでchardetの版を固定しているため
// 推定結果そのものを前提条件として固定し、chardetを更新して前提が変わった場合は
// 変換結果より先にここで検出できるようにする。
func TestEncodingDetector_DetectBytes_ChardetLabels(t *testing.T) {
	t.Parallel()

	detector := converter.NewEncodingDetector()

	tests := []struct {
		name           string
		data           []byte
		wantEncoding   string
		wantConfidence float64
		wantSupported  bool
	}{
		{
			name:           "前提: 短いShift_JISの[config]\\ntitle=猫はwindows-1252と推定される",
			data:           encodeWith(t, japanese.ShiftJIS, "[config]\ntitle=猫"),
			wantEncoding:   "windows-1252",
			wantConfidence: 0.75,
		},
		{
			name:           "前提: 短いShift_JISのid,name\\n1,ｱｲﾃﾑはiso-8859-1と推定される",
			data:           encodeWith(t, japanese.ShiftJIS, "id,name\n1,ｱｲﾃﾑ"),
			wantEncoding:   "iso-8859-1",
			wantConfidence: 0.27,
		},
		{
			name:           "前提: 短いShift_JISのversion=1.0\\nauthor=山田はwindows-1252と推定される",
			data:           encodeWith(t, japanese.ShiftJIS, "version=1.0\nauthor=山田"),
			wantEncoding:   "windows-1252",
			wantConfidence: 0.63,
		},
		{
			name:           "前提: 短いShift_JISのセーブデータはwindows-1252と推定される",
			data:           encodeWith(t, japanese.ShiftJIS, "セーブデータ"),
			wantEncoding:   "windows-1252",
			wantConfidence: 0.42,
		},
		{
			name:           "前提: 短いShift_JISのfont=ＭＳ ゴシックはwindows-1252と推定される",
			data:           encodeWith(t, japanese.ShiftJIS, "font=ＭＳ ゴシック"),
			wantEncoding:   "windows-1252",
			wantConfidence: 0.35,
		},
		{
			name:           "前提: 短いShift_JISのｾｰﾌﾞは低信頼度のgb18030と推定される",
			data:           encodeWith(t, japanese.ShiftJIS, "ｾｰﾌﾞ"),
			wantEncoding:   "gb18030",
			wantConfidence: 0.1,
			wantSupported:  true,
		},
		{
			name:           "前提: 短いShift_JISのname=ｱｲﾃﾑは低信頼度のgb18030と推定される",
			data:           encodeWith(t, japanese.ShiftJIS, "name=ｱｲﾃﾑ"),
			wantEncoding:   "gb18030",
			wantConfidence: 0.1,
			wantSupported:  true,
		},
		{
			name:           "前提: GB18030でもShift_JISでも復号できない短いバイト列は低信頼度のgb18030と推定される",
			data:           []byte{0x6b, 0x3d, 0xca, 0xea, 0xba},
			wantEncoding:   "gb18030",
			wantConfidence: 0.1,
			wantSupported:  true,
		},
		{
			name:           "前提: Shift_JISの1文字の猫はどの文字コードとも推定されない",
			data:           encodeWith(t, japanese.ShiftJIS, "猫"),
			wantEncoding:   "",
			wantConfidence: 0,
		},
		{
			name:           "前提: 不正なUTF-8であるLatin-1のcaf\\xe9は低信頼度のutf-8と推定される",
			data:           []byte("caf\xe9"),
			wantEncoding:   "utf-8",
			wantConfidence: 0.1,
			wantSupported:  true,
		},
		{
			name:           "前提: UTF-8 BOMの後ろが不正なUTF-8のcaf\\xe9は高信頼度のutf-8と推定される",
			data:           []byte("\xef\xbb\xbfcaf\xe9"),
			wantEncoding:   "utf-8",
			wantConfidence: 1.0,
			wantSupported:  true,
		},
		{
			name:           "前提: UTF-8 BOMに続く短いShift_JISの[config]\\ntitle=猫はwindows-1252と推定される",
			data:           append([]byte("\xef\xbb\xbf"), encodeWith(t, japanese.ShiftJIS, "[config]\ntitle=猫")...),
			wantEncoding:   "windows-1252",
			wantConfidence: 0.7,
		},
		{
			name:           "前提: UTF-8 BOMに続くShift_JISの1文字の猫は低信頼度のshift_jisと推定される",
			data:           append([]byte("\xef\xbb\xbf"), encodeWith(t, japanese.ShiftJIS, "猫")...),
			wantEncoding:   "shift_jis",
			wantConfidence: 0.1,
			wantSupported:  true,
		},
		{
			name:           "前提: 末尾の文字が途切れたUTF-8は途切れた並びが数えられず高信頼度のutf-8と推定される",
			data:           []byte("猫猫猫猫\xe7\x8c"),
			wantEncoding:   "utf-8",
			wantConfidence: 1.0,
			wantSupported:  true,
		},
		{
			name:           "前提: 半角カナだけの短いShift_JISのtitle=ﾀｲはC0 B2が有効な並びと数えられutf-8と推定される",
			data:           encodeWith(t, japanese.ShiftJIS, "title=ﾀｲ"),
			wantEncoding:   "utf-8",
			wantConfidence: 0.8,
			wantSupported:  true,
		},
		{
			name:           "前提: 非ASCIIが少ない正しいUTF-8のtitle=名前はwindows-1252と推定される",
			data:           []byte("title=名前"),
			wantEncoding:   "windows-1252",
			wantConfidence: 0.9,
		},
		{
			name:           "正常系: chardetのGB-18030はgb18030として対応済みになる",
			data:           encodeWith(t, simplifiedchinese.GBK, "这是一个中文测试文本，用于检测编码。"),
			wantEncoding:   "gb18030",
			wantConfidence: 1.0,
			wantSupported:  true,
		},
		{
			name:           "正常系: chardetのEUC-KRはcp949として対応済みになる",
			data:           encodeWith(t, korean.EUCKR, "이것은 한국어 테스트 텍스트입니다."),
			wantEncoding:   "cp949",
			wantConfidence: 1.0,
			wantSupported:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := detector.DetectBytes(tt.data)

			assert.Equal(t, tt.wantEncoding, result.Encoding)
			assert.InDelta(t, tt.wantConfidence, result.Confidence, 1e-9)
			assert.Equal(t, tt.wantSupported, result.IsSupported)

			// why: chardetは同じ信頼度の候補の順序が実行ごとに変わるため、1回の
			// 一致だけでは推定結果が決定的であることを示せない。
			for range 50 {
				require.Equal(t, result, detector.DetectBytes(tt.data))
			}
		})
	}
}

func TestEncodingConverter_Convert_AutoDetectedSource(t *testing.T) {
	t.Parallel()

	utf8BOM := []byte{0xef, 0xbb, 0xbf}

	tests := []struct {
		name     string
		fileName string
		data     []byte
		want     []byte
	}{
		{
			name:     "正常系: windows-1252と推定される短いShift_JISの.iniはShift_JISとして変換される",
			fileName: "config.ini",
			data:     encodeWith(t, japanese.ShiftJIS, "[config]\ntitle=猫"),
			want:     []byte("[config]\ntitle=猫"),
		},
		{
			name:     "正常系: iso-8859-1と推定される短いShift_JISの.csvはShift_JISとして変換される",
			fileName: "items.csv",
			data:     encodeWith(t, japanese.ShiftJIS, "id,name\n1,ｱｲﾃﾑ"),
			want:     []byte("id,name\n1,ｱｲﾃﾑ"),
		},
		{
			name:     "正常系: windows-1252と推定される短いShift_JISの.iniはShift_JISとして変換される（漢字）",
			fileName: "version.ini",
			data:     encodeWith(t, japanese.ShiftJIS, "version=1.0\nauthor=山田"),
			want:     []byte("version=1.0\nauthor=山田"),
		},
		{
			name:     "正常系: windows-1252と推定される短いShift_JISの.txtはShift_JISとして変換される",
			fileName: "save.txt",
			data:     encodeWith(t, japanese.ShiftJIS, "セーブデータ"),
			want:     []byte("セーブデータ"),
		},
		{
			name:     "正常系: windows-1252と推定される短いShift_JISの全角英字を含む.iniはShift_JISとして変換される",
			fileName: "font.ini",
			data:     encodeWith(t, japanese.ShiftJIS, "font=ＭＳ ゴシック"),
			want:     []byte("font=ＭＳ ゴシック"),
		},
		{
			name:     "正常系: 未対応と推定される短いShift_JISの.ksはBOM付きUTF-8に変換される",
			fileName: "first.ks",
			data:     encodeWith(t, japanese.ShiftJIS, "セーブデータ"),
			want:     append(bytes.Clone(utf8BOM), "セーブデータ"...),
		},
		{
			name:     "正常系: 低信頼度のgb18030と推定される短いShift_JISはShift_JISとして変換される",
			fileName: "save.ini",
			data:     encodeWith(t, japanese.ShiftJIS, "ｾｰﾌﾞ"),
			want:     []byte("ｾｰﾌﾞ"),
		},
		{
			name:     "正常系: 低信頼度のgb18030と推定される短いShift_JISの.csvはShift_JISとして変換される",
			fileName: "name.csv",
			data:     encodeWith(t, japanese.ShiftJIS, "name=ｱｲﾃﾑ"),
			want:     []byte("name=ｱｲﾃﾑ"),
		},
		{
			name:     "正常系: 文字コードを推定できないShift_JISの1文字の.csvはShift_JISとして変換される",
			fileName: "name.csv",
			data:     encodeWith(t, japanese.ShiftJIS, "猫"),
			want:     []byte("猫"),
		},
		{
			name:     "正常系: gb18030と推定されるGBKの中国語はGB18030として変換される",
			fileName: "readme.txt",
			data:     encodeWith(t, simplifiedchinese.GBK, "这是一个中文测试文本，用于检测编码。"),
			want:     []byte("这是一个中文测试文本，用于检测编码。"),
		},
		{
			name:     "正常系: GB18030の4バイト符号も復号される",
			fileName: "readme.txt",
			data:     encodeWith(t, simplifiedchinese.GB18030, "这是一个中文测试文本，用于检测编码。😀"),
			want:     []byte("这是一个中文测试文本，用于检测编码。😀"),
		},
		{
			name:     "正常系: cp949と推定されるEUC-KRの韓国語はCP949として変換される",
			fileName: "readme.txt",
			data:     encodeWith(t, korean.EUCKR, "이것은 한국어 테스트 텍스트입니다."),
			want:     []byte("이것은 한국어 테스트 텍스트입니다."),
		},
		{
			name:     "正常系: BOM付きUTF-16LEの本文にあるU+FFFDは復号失敗とみなさずそのまま変換される",
			fileName: "readme.txt",
			data:     encodeUTF16("置換文字\uFFFDを含む本文", false, true),
			want:     append(bytes.Clone(utf8BOM), "置換文字\uFFFDを含む本文"...),
		},
		{
			name:     "正常系: 高信頼度のeuc-jpと推定されるEUC-JPはShift_JISではなくEUC-JPとして変換される",
			fileName: "readme.txt",
			data:     encodeWith(t, japanese.EUCJP, "これは日本語のテストです。文字コードの検出を確認します。"),
			want:     []byte("これは日本語のテストです。文字コードの検出を確認します。"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			source := filepath.Join(dir, tt.fileName)
			dest := filepath.Join(dir, "out", tt.fileName)
			writeFile(t, source, tt.data)

			c := converter.NewEncodingConverter("", "")
			result, err := c.Convert(source, dest)

			require.NoError(t, err)
			assert.Equal(t, converter.StatusSuccess, result.Status)
			assert.Equal(t, tt.want, readFile(t, dest))
		})
	}

	t.Run("正常系: 未対応と推定されても正しいUTF-8はShift_JISとして読まずSKIPPEDになる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "title.ini")
		dest := filepath.Join(dir, "out", "title.ini")
		writeFile(t, source, []byte("title=名前"))

		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSkipped, result.Status)
	})

	t.Run("正常系: U+FFFDを含むBOM無しの正しいUTF-8はSKIPPEDになる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "readme.txt")
		dest := filepath.Join(dir, "out", "readme.txt")
		writeFile(t, source, []byte("置換文字\uFFFDを含む本文です。"))

		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSkipped, result.Status)
	})

	t.Run("正常系: 空の.txtはSKIPPEDになる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "empty.txt")
		dest := filepath.Join(dir, "out", "empty.txt")
		writeFile(t, source, []byte{})

		c := converter.NewEncodingConverter("", "")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSkipped, result.Status)
	})

	t.Run("正常系: 変換先と同じ指定のShift_JISはUTF-8として不正でもSKIPPEDになる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "readme.txt")
		dest := filepath.Join(dir, "out", "readme.txt")
		writeFile(t, source, encodeWith(t, japanese.ShiftJIS, "猫"))

		c := converter.NewEncodingConverter("shift_jis", "shift_jis")
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSkipped, result.Status)
	})

	invalidUTF8Tests := []struct {
		name           string
		fileName       string
		sourceEncoding string
		data           []byte
	}{
		{
			name:           "異常系: 変換元にutf-8を指定されても不正なUTF-8バイト列はSKIPPEDにせず再試行不要なエラーにする",
			fileName:       "readme.txt",
			sourceEncoding: "utf-8",
			data:           []byte("caf\xe9"),
		},
		{
			name:     "異常系: utf-8と推定される不正なUTF-8のLatin-1はShift_JISとして読まず再試行不要なエラーにする",
			fileName: "readme.txt",
			data:     []byte("caf\xe9"),
		},
		{
			name:     "異常系: 末尾の文字が途切れたUTF-8はShift_JISとして読まず再試行不要なエラーにする",
			fileName: "first.ks",
			data:     []byte("猫猫猫猫\xe7\x8c"),
		},
		{
			name:     "既知の制限: 半角カナだけの短いShift_JISはutf-8と推定され変換できない",
			fileName: "config.ini",
			data:     encodeWith(t, japanese.ShiftJIS, "title=ﾀｲ"),
		},
		{
			name:     "異常系: utf-8と推定されるUTF-8 BOM付きの不正なUTF-8はShift_JISとして読まず再試行不要なエラーにする",
			fileName: "readme.txt",
			data:     []byte("\xef\xbb\xbfcaf\xe9"),
		},
		{
			name:     "異常系: windows-1252と推定されるUTF-8 BOM付きのShift_JISもShift_JISとして読まず再試行不要なエラーにする",
			fileName: "readme.txt",
			data:     append([]byte("\xef\xbb\xbf"), encodeWith(t, japanese.ShiftJIS, "[config]\ntitle=猫")...),
		},
		{
			name:     "異常系: shift_jisと推定されるUTF-8 BOM付きのShift_JISもShift_JISとして読まず再試行不要なエラーにする",
			fileName: "readme.txt",
			data:     append([]byte("\xef\xbb\xbf"), encodeWith(t, japanese.ShiftJIS, "猫")...),
		},
	}

	for _, tt := range invalidUTF8Tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			source := filepath.Join(dir, tt.fileName)
			dest := filepath.Join(dir, "out", tt.fileName)
			writeFile(t, source, tt.data)

			c := converter.NewEncodingConverter("", tt.sourceEncoding)
			_, err := c.Convert(source, dest)

			require.ErrorIs(t, err, converter.ErrEncodingConversionFailed)
			require.ErrorIs(t, err, converter.ErrPermanentFailure)
			require.ErrorContains(t, err, "不正なUTF-8")
			assert.NotContains(t, err.Error(), "有効なUTF-8ではなく")
			assert.NotContains(t, err.Error(), "Shift_JIS")
			assert.NoFileExists(t, dest)
		})
	}

	errorTests := []struct {
		name           string
		sourceEncoding string
		data           []byte
		wantInMessage  []string
	}{
		{
			name:          "異常系: 文字コードを推定できずShift_JISとしても復号できないバイト列は検出結果なしを示して失敗する",
			data:          []byte{0x94},
			wantInMessage: []string{"検出結果なし", "Shift_JIS"},
		},
		{
			name:          "異常系: 未対応と推定されShift_JISとしても復号できないバイト列は推定名を示して失敗する",
			data:          []byte("id,name\n1,\xfd\xfe\xff\xfd"),
			wantInMessage: []string{"iso-8859-1", "Shift_JIS"},
		},
		{
			name:          "異常系: Shift_JISとして不正なバイトを含むLatin-1の文章は推定名を示して失敗する",
			data:          []byte("caf\xe9 cr\xe8me br\xfbl\xe9e"),
			wantInMessage: []string{"iso-8859-1", "Shift_JIS"},
		},
		{
			name:          "異常系: 低信頼度のgb18030と推定されGB18030とShift_JISのどちらでも復号できないバイト列は両方を示して失敗する",
			data:          []byte{0x6b, 0x3d, 0xca, 0xea, 0xba},
			wantInMessage: []string{"gb18030", "Shift_JIS"},
		},
		{
			name:           "異常系: 指定されたShift_JISとして復号できないバイト列は失敗する",
			sourceEncoding: "shift_jis",
			data:           encodeWith(t, simplifiedchinese.GBK, "这是一个中文测试文本，用于检测编码。"),
			wantInMessage:  []string{"shift_jis"},
		},
	}

	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			source := filepath.Join(dir, "data.csv")
			dest := filepath.Join(dir, "out", "data.csv")
			writeFile(t, source, tt.data)

			c := converter.NewEncodingConverter("", tt.sourceEncoding)
			_, err := c.Convert(source, dest)

			require.ErrorIs(t, err, converter.ErrUndecodableSource)
			require.ErrorIs(t, err, converter.ErrEncodingConversionFailed)
			require.ErrorIs(t, err, converter.ErrPermanentFailure)
			for _, want := range tt.wantInMessage {
				assert.Contains(t, err.Error(), want)
			}
			assert.NoFileExists(t, dest)
		})
	}
}

func TestEncodingConverter_ConvertBytes_AutoDetectedSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		data         []byte
		want         string
		wantEncoding string
	}{
		{
			name:         "正常系: 未対応と推定されたShift_JISは実際に使ったshift_jisを返す",
			data:         encodeWith(t, japanese.ShiftJIS, "[config]\ntitle=猫"),
			want:         "[config]\ntitle=猫",
			wantEncoding: "shift_jis",
		},
		{
			name:         "正常系: 低信頼度のgb18030からShift_JISを選んだ場合はshift_jisを返す",
			data:         encodeWith(t, japanese.ShiftJIS, "ｾｰﾌﾞ"),
			want:         "ｾｰﾌﾞ",
			wantEncoding: "shift_jis",
		},
		{
			name:         "正常系: 文字コードを推定できないShift_JISの1文字はshift_jisを返す",
			data:         encodeWith(t, japanese.ShiftJIS, "猫"),
			want:         "猫",
			wantEncoding: "shift_jis",
		},
		{
			name:         "正常系: 未対応と推定された正しいUTF-8はutf-8を返す",
			data:         []byte("title=名前"),
			want:         "title=名前",
			wantEncoding: "utf-8",
		},
		{
			name:         "正常系: 高信頼度のgb18030はgb18030を返す",
			data:         encodeWith(t, simplifiedchinese.GBK, "这是一个中文测试文本，用于检测编码。"),
			want:         "这是一个中文测试文本，用于检测编码。",
			wantEncoding: "gb18030",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := converter.NewEncodingConverter("", "")
			got, detected, err := c.ConvertBytes(tt.data)

			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
			assert.Equal(t, tt.wantEncoding, detected)
		})
	}

	t.Run("異常系: 未対応と推定されShift_JISとしても復号できないバイト列はErrUndecodableSourceを返す", func(t *testing.T) {
		t.Parallel()

		c := converter.NewEncodingConverter("", "")
		_, _, err := c.ConvertBytes([]byte("id,name\n1,\xfd\xfe\xff\xfd"))

		require.ErrorIs(t, err, converter.ErrUndecodableSource)
	})
}
