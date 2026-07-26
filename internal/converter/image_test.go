package converter_test

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/bmp"

	"github.com/na2na-p/mnemonic/internal/converter"
)

// テストで用いる画像サイズ。wazoreベースのWebPエンコーダの初回呼び出しコストは
// 主にWASMモジュールのインスタンス化に起因し画素数への依存は小さいが、
// CI時間短縮のためPython版(100x100)より小さいサイズに揃える。
const testImageSize = 8

var (
	tlg5Magic = []byte("TLG5.0\x00raw\x1a")
	tlg6Magic = []byte("TLG6.0\x00raw\x1a")
)

func newSolidRGBA(c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, testImageSize, testImageSize))
	for y := range testImageSize {
		for x := range testImageSize {
			img.SetRGBA(x, y, c)
		}
	}

	return img
}

func writeBMPFixture(t *testing.T, path string, c color.RGBA) {
	t.Helper()

	f, err := os.Create(path) //nolint:gosec // テスト用の一時ファイル
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	require.NoError(t, bmp.Encode(f, newSolidRGBA(c)))
}

func writeJPEGFixture(t *testing.T, path string, c color.RGBA) {
	t.Helper()

	f, err := os.Create(path) //nolint:gosec // テスト用の一時ファイル
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	require.NoError(t, jpeg.Encode(f, newSolidRGBA(c), nil))
}

func writePNGFixture(t *testing.T, path string, c color.RGBA) {
	t.Helper()

	f, err := os.Create(path) //nolint:gosec // テスト用の一時ファイル
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	require.NoError(t, png.Encode(f, newSolidRGBA(c)))
}

func TestTLGImageDecoder_IsTLGFile(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		content  []byte
		expected bool
	}{
		"正常系: TLG5形式のファイルはtrueを返す":    {append(append([]byte{}, tlg5Magic...), make([]byte, 100)...), true},
		"正常系: TLG6形式のファイルはtrueを返す":    {append(append([]byte{}, tlg6Magic...), make([]byte, 100)...), true},
		"異常系: PNG形式のファイルはfalseを返す":    {append([]byte("PNG\x00\x00\x00"), make([]byte, 100)...), false},
		"異常系: JPEG形式のファイルはfalseを返す":   {append([]byte("JPEG\x00\x00\x00"), make([]byte, 100)...), false},
		"異常系: マジックバイトが不完全な場合falseを返す": {[]byte("TLG5"), false},
		"異常系: 空ファイルはfalseを返す":         {[]byte{}, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "test.tlg")
			writeFile(t, path, tc.content)

			decoder := converter.NewTLGImageDecoder()
			assert.Equal(t, tc.expected, decoder.IsTLGFile(path))
		})
	}

	t.Run("異常系: 存在しないファイルはfalseを返す", func(t *testing.T) {
		t.Parallel()

		decoder := converter.NewTLGImageDecoder()
		assert.False(t, decoder.IsTLGFile("/nonexistent/path/to/file.tlg"))
	})
}

func TestTLGImageDecoder_NotImplemented(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.tlg")
	writeFile(t, path, append(append([]byte{}, tlg5Magic...), make([]byte, 100)...))

	decoder := converter.NewTLGImageDecoder()

	t.Run("異常系: GetInfoはErrTLGDecodeNotImplementedを返す", func(t *testing.T) {
		t.Parallel()

		_, err := decoder.GetInfo(path)
		assert.ErrorIs(t, err, converter.ErrTLGDecodeNotImplemented)
	})

	t.Run("異常系: DecodeはErrTLGDecodeNotImplementedを返す", func(t *testing.T) {
		t.Parallel()

		_, err := decoder.Decode(path)
		assert.ErrorIs(t, err, converter.ErrTLGDecodeNotImplemented)
	})

	t.Run("異常系: DecodeToFileはErrTLGDecodeNotImplementedを返す", func(t *testing.T) {
		t.Parallel()

		err := decoder.DecodeToFile(path, filepath.Join(dir, "output.png"))
		assert.ErrorIs(t, err, converter.ErrTLGDecodeNotImplemented)
	})
}

func TestTLGVersion_Values(t *testing.T) {
	t.Parallel()

	assert.Equal(t, converter.TLGVersionTLG5, converter.TLGVersion("TLG5"))
	assert.Equal(t, converter.TLGVersionTLG6, converter.TLGVersion("TLG6"))
	assert.Equal(t, converter.TLGVersionUnknown, converter.TLGVersion("UNKNOWN"))
}

func TestQualityPreset_Values(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		preset   converter.QualityPreset
		expected int
	}{
		"正常系: HIGHプリセットは95":   {converter.QualityHigh, 95},
		"正常系: MEDIUMプリセットは85": {converter.QualityMedium, 85},
		"正常系: LOWプリセットは70":    {converter.QualityLow, 70},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, int(tc.preset))
		})
	}
}

func TestImageConverter_Convert(t *testing.T) {
	t.Parallel()

	t.Run("正常系: BMPファイルをWebPに変換できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.bmp")
		writeBMPFixture(t, source, color.RGBA{R: 255, A: 255})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("正常系: JPGファイルをWebPに変換できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.jpg")
		writeJPEGFixture(t, source, color.RGBA{G: 255, A: 255})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("正常系: PNGファイルをWebPに変換できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.png")
		writePNGFixture(t, source, color.RGBA{B: 255, A: 255})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("正常系: 品質プリセットが適用される", func(t *testing.T) {
		t.Parallel()

		cases := map[string]struct {
			preset  converter.QualityPreset
			quality int
		}{
			"正常系: HIGHプリセットで品質95":   {converter.QualityHigh, 95},
			"正常系: MEDIUMプリセットで品質85": {converter.QualityMedium, 85},
			"正常系: LOWプリセットで品質70":    {converter.QualityLow, 70},
		}

		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				source := filepath.Join(dir, "test.bmp")
				writeBMPFixture(t, source, color.RGBA{R: 255, A: 255})
				dest := filepath.Join(dir, "output.webp")

				c := converter.NewImageConverter(int(tc.preset), true)
				assert.Equal(t, tc.quality, c.Quality())

				result, err := c.Convert(source, dest)
				require.NoError(t, err)
				assert.Equal(t, converter.StatusSuccess, result.Status)
			})
		}
	})

	t.Run("正常系: カスタム品質値が適用される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.bmp")
		writeBMPFixture(t, source, color.RGBA{R: 255, A: 255})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverter(50, true)
		assert.Equal(t, 50, c.Quality())

		result, err := c.Convert(source, dest)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
	})

	t.Run("正常系: アルファチャンネルが保持される(lossless_alpha=true)", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test_alpha.png")
		writePNGFixture(t, source, color.RGBA{R: 255, A: 128})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		assert.True(t, c.LosslessAlpha())

		result, err := c.Convert(source, dest)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("正常系: lossless_alpha=falseで非ロスレスアルファが適用される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test_alpha.png")
		writePNGFixture(t, source, color.RGBA{R: 255, A: 128})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverter(int(converter.QualityHigh), false)
		assert.False(t, c.LosslessAlpha())

		result, err := c.Convert(source, dest)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("正常系: 変換結果に成功状態が正しく記録される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.bmp")
		writeBMPFixture(t, source, color.RGBA{R: 255, A: 255})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.True(t, result.IsSuccess())
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.Equal(t, source, result.SourcePath)
		assert.Equal(t, dest, result.DestPath)
	})

	t.Run("正常系: 変換結果にファイルサイズが記録される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.bmp")
		writeBMPFixture(t, source, color.RGBA{R: 255, A: 255})
		dest := filepath.Join(dir, "output.webp")

		sourceInfo, err := os.Stat(source)
		require.NoError(t, err)

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		result, convErr := c.Convert(source, dest)

		require.NoError(t, convErr)
		assert.Equal(t, sourceInfo.Size(), result.BytesBefore)
		assert.Positive(t, result.BytesAfter)

		destInfo, err := os.Stat(dest)
		require.NoError(t, err)
		assert.Equal(t, destInfo.Size(), result.BytesAfter)
	})

	t.Run("正常系: 変換先の親ディレクトリが存在しない場合に作成される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.bmp")
		writeBMPFixture(t, source, color.RGBA{R: 255, A: 255})
		dest := filepath.Join(dir, "subdir", "nested", "output.webp")

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("異常系: TLGファイルは未実装エラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.tlg")
		writeFile(t, source, append(append([]byte{}, tlg5Magic...), make([]byte, 100)...))
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		_, err := c.Convert(source, dest)

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrTLGDecodeNotImplemented)
	})

	t.Run("異常系: 存在しない変換元ファイルはエラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		_, err := c.Convert(filepath.Join(dir, "nonexistent.bmp"), filepath.Join(dir, "output.webp"))

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrSourceNotFound)
	})

	t.Run("異常系: 変換元がディレクトリの場合エラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		_, err := c.Convert(dir, filepath.Join(dir, "output.webp"))

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrSourceIsDirectory)
	})
}

func TestImageConverter_ConvertFromImage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dest := filepath.Join(dir, "from_image.webp")

	img := newSolidRGBA(color.RGBA{R: 255, G: 128, A: 255})

	c := converter.NewImageConverter(int(converter.QualityHigh), true)
	result, err := c.ConvertFromImage(img, dest)

	require.NoError(t, err)
	assert.Equal(t, converter.StatusSuccess, result.Status)
	assert.FileExists(t, dest)
}

func TestImageConverter_SupportedExtensions(t *testing.T) {
	t.Parallel()

	c := converter.NewImageConverter(int(converter.QualityHigh), true)
	assert.Equal(t, []string{".tlg", ".bmp", ".jpg", ".jpeg", ".png"}, c.SupportedExtensions())
}

func TestImageConverter_CanConvert(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		filename string
		expected bool
	}{
		"正常系: BMPファイルは変換可能":   {"test.bmp", true},
		"正常系: JPGファイルは変換可能":   {"test.jpg", true},
		"正常系: JPEGファイルは変換可能":  {"test.jpeg", true},
		"正常系: PNGファイルは変換可能":   {"test.png", true},
		"正常系: TLGファイルは変換可能":   {"test.tlg", true},
		"異常系: GIFファイルは変換不可":   {"test.gif", false},
		"異常系: WebPファイルは変換不可":  {"test.webp", false},
		"異常系: TXTファイルは変換不可":   {"test.txt", false},
		"正常系: 大文字拡張子BMPも変換可能": {"TEST.BMP", true},
		"正常系: 大文字拡張子PNGも変換可能": {"TEST.PNG", true},
	}

	c := converter.NewImageConverter(int(converter.QualityHigh), true)
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, c.CanConvert(tc.filename))
		})
	}
}
