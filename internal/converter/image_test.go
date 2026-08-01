package converter_test

import (
	"bytes"
	"encoding/binary"
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

// テストで用いる画像サイズ。wazeroベースのWebPエンコーダの初回呼び出しコストは
// 主にWASMモジュールのインスタンス化に起因し画素数への依存は小さいが、
// CI時間短縮のため小さいサイズに揃える。
const testImageSize = 8

var (
	tlg5Magic = []byte("TLG5.0\x00raw\x1a")
	tlg6Magic = []byte("TLG6.0\x00raw\x1a")
	sdsMagic  = []byte("TLG0.0\x00sds\x1a")
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

// forceAlphaImage はOpaque()を常にfalseとして偽装するimage.Imageラッパー。
//
// why: stdlib image/pngのエンコーダは実際に全ピクセルが不透明な画像を
// 渡すとOpaque()==trueを検出しアルファチャンネル無しのカラータイプで
// 書き出してしまう（image/png/writer.goのopaque(m)判定）。そのため
// 「フォーマット上アルファチャンネルを持つが全ピクセルは不透明なPNG」
// （何らかの画像処理ツールがRGBAモードで保存した場合に生じうる状態）を
// stdlibのpng.Encodeだけで再現できない。Opaque()を偽装してこの
// 最適化を回避し、テスト用フィクスチャとして意図的な状態を作る。
type forceAlphaImage struct {
	image.Image
}

func (forceAlphaImage) Opaque() bool { return false }

// writeOpaqueAlphaPNGFixture はフォーマット上アルファチャンネルを持つが
// 全ピクセルが不透明なPNGファイルを書き出す
// （image/png.Decode時に*image.NRGBAとしてデコードされ、hasAlpha判定の
// テスト対象になる）。
func writeOpaqueAlphaPNGFixture(t *testing.T, path string, c color.RGBA) {
	t.Helper()

	c.A = 255
	nc := color.NRGBAModel.Convert(c).(color.NRGBA) //nolint:forcetypeassert // color.NRGBAModel.Convertは常にcolor.NRGBAを返す
	img := image.NewNRGBA(image.Rect(0, 0, testImageSize, testImageSize))
	for y := range testImageSize {
		for x := range testImageSize {
			img.SetNRGBA(x, y, nc)
		}
	}

	f, err := os.Create(path) //nolint:gosec // テスト用の一時ファイル
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	require.NoError(t, png.Encode(f, forceAlphaImage{img}))
}

// isLosslessWebP はWebPファイルがロスレス(VP8Lチャンク)でエンコードされて
// いるかを判定するテストヘルパー。WebPのRIFFコンテナはロスレスなら"VP8L"、
// ロッシーなら"VP8 "のFourCCチャンクを含む。
func isLosslessWebP(t *testing.T, path string) bool {
	t.Helper()

	return bytes.Contains(readFile(t, path), []byte("VP8L"))
}

// tlg5FixtureHeight / tlg5FixtureBlockHeight / tlg5FixtureR / tlg5FixtureG /
// tlg5FixtureB / tlg5FixtureA はbuildTLG5Fixtureが常に単一ブロック(2行)・
// 単一色のみを生成するために使う固定値。
//
// why not: 複数ブロック・可変block_height・画素値のバリエーションは
// アルゴリズムレベルでinternal/converter/tlg/tlg5_test.goが既に網羅している
// ため、image.go統合テスト側では変える必要が無い（golangci-lint unparamの
// 指摘に対する対応でもある）。width/colorDepthのみを可変にし、TLGImageDecoder
// /ImageConverterの結線（ヘッダ解析結果の伝播・BGRA→RGBA変換・decode成功時の
// 画素値伝播）を検証するにはこれで十分。
const (
	tlg5FixtureHeight      = 2
	tlg5FixtureBlockHeight = 2
	tlg5FixtureR           = 255
	tlg5FixtureG           = 128
	tlg5FixtureB           = 64
	tlg5FixtureA           = 255
)

// buildTLG5Fixture はwidth x tlg5FixtureHeightの単色TLG5画像バイト列を生成
// するテストヘルパー（アルゴリズム自体のテストはinternal/converter/tlg側で
// 行うため、ここではImageConverter/TLGImageDecoder統合経路の検証に十分な
// 単純なフィクスチャのみを生成する）。
func buildTLG5Fixture(width int, colorDepth byte) []byte {
	const (
		height      = tlg5FixtureHeight
		blockHeight = tlg5FixtureBlockHeight
		r           = tlg5FixtureR
		g           = tlg5FixtureG
		b           = tlg5FixtureB
		a           = tlg5FixtureA
	)

	header := append([]byte{}, tlg5Magic...)
	header = append(header, colorDepth)

	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(width)) //nolint:gosec // テストフィクスチャの小さい値のみを扱う
	header = append(header, buf...)
	binary.LittleEndian.PutUint32(buf, uint32(height))
	header = append(header, buf...)
	binary.LittleEndian.PutUint32(buf, uint32(blockHeight))
	header = append(header, buf...)

	channelValues := []byte{b, g, r, a} // BGRA順
	if colorDepth != 32 {
		channelValues = []byte{b, g, r} // BGR順
	}

	blockCount := (height + blockHeight - 1) / blockHeight

	literalBlock := func(channelData []byte) []byte {
		compressed := []byte{}
		for pos := 0; pos < len(channelData); pos += 8 {
			end := min(pos+8, len(channelData))
			compressed = append(compressed, 0x00)
			compressed = append(compressed, channelData[pos:end]...)
		}

		block := []byte{0} // mark
		sizeBuf := make([]byte, 4)
		binary.LittleEndian.PutUint32(sizeBuf, uint32(len(compressed))) //nolint:gosec // テストフィクスチャの小さいサイズのみを扱う
		block = append(block, sizeBuf...)

		return append(block, compressed...)
	}

	blocks := make([][]byte, 0, blockCount)
	for blockIdx := range blockCount {
		blockRows := min(blockHeight, height-blockIdx*blockHeight)
		pixelCount := width * blockRows

		var blockBytes []byte
		for _, value := range channelValues {
			channelData := make([]byte, pixelCount)
			if blockIdx == 0 {
				channelData[0] = value
			}
			blockBytes = append(blockBytes, literalBlock(channelData)...)
		}
		blocks = append(blocks, blockBytes)
	}

	blockSizes := make([]byte, 0, blockCount*4)
	sizeBuf := make([]byte, 4)
	for _, b := range blocks {
		binary.LittleEndian.PutUint32(sizeBuf, uint32(len(b))) //nolint:gosec // テストフィクスチャの小さいサイズのみを扱う
		blockSizes = append(blockSizes, sizeBuf...)
	}

	result := make([]byte, 0, len(header)+len(blockSizes))
	result = append(result, header...)
	result = append(result, blockSizes...)
	for _, b := range blocks {
		result = append(result, b...)
	}

	return result
}

// buildTLG6HeaderFixture はTLG6ヘッダーのみを持つバイト列を生成するテスト
// ヘルパー（本体データは無い。TLG6は本PRの時点でヘッダー解析のみ実装のため
// 十分）。
func buildTLG6HeaderFixture(colors, dataFlags byte, width, height, xBlockCount, yBlockCount uint32) []byte {
	header := append([]byte{}, tlg6Magic...)
	header = append(header, colors, dataFlags)

	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, width)
	header = append(header, buf...)
	binary.LittleEndian.PutUint32(buf, height)
	header = append(header, buf...)
	binary.LittleEndian.PutUint32(buf, xBlockCount)
	header = append(header, buf...)
	binary.LittleEndian.PutUint32(buf, yBlockCount)
	header = append(header, buf...)

	return header
}

func TestTLGImageDecoder_IsTLGFile(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		content  []byte
		expected bool
	}{
		"正常系: TLG5形式のファイルはtrueを返す":    {append(append([]byte{}, tlg5Magic...), make([]byte, 100)...), true},
		"正常系: TLG6形式のファイルはtrueを返す":    {append(append([]byte{}, tlg6Magic...), make([]byte, 100)...), true},
		"正常系: SDSコンテナはtrueを返す":        {append(append([]byte{}, sdsMagic...), make([]byte, 100)...), true},
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

func TestTLGImageDecoder_GetInfo(t *testing.T) {
	t.Parallel()

	t.Run("正常系: TLG5ファイルのメタ情報を取得できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.tlg")
		writeFile(t, path, buildTLG5Fixture(4, 24))

		decoder := converter.NewTLGImageDecoder()
		info, err := decoder.GetInfo(path)

		require.NoError(t, err)
		assert.Equal(t, converter.TLGVersionTLG5, info.Version)
		assert.Equal(t, 4, info.Width)
		assert.Equal(t, 2, info.Height)
		assert.False(t, info.HasAlpha)
	})

	t.Run("正常系: TLG6ファイルのメタ情報を取得できる（本体デコード未実装でも成功する）", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.tlg")
		writeFile(t, path, buildTLG6HeaderFixture(32, 0, 8, 4, 1, 1))

		decoder := converter.NewTLGImageDecoder()
		info, err := decoder.GetInfo(path)

		require.NoError(t, err)
		assert.Equal(t, converter.TLGVersionTLG6, info.Version)
		assert.Equal(t, 8, info.Width)
		assert.Equal(t, 4, info.Height)
		assert.True(t, info.HasAlpha)
	})

	t.Run("正常系: SDSコンテナ内のTLG5メタ情報を取得できる", func(t *testing.T) {
		t.Parallel()

		inner := buildTLG5Fixture(2, 32)
		sds := append([]byte{}, sdsMagic...)
		sizeBuf := make([]byte, 4)
		binary.LittleEndian.PutUint32(sizeBuf, uint32(len(inner))) //nolint:gosec // テストフィクスチャの小さいサイズのみを扱う
		sds = append(sds, sizeBuf...)
		sds = append(sds, inner...)

		dir := t.TempDir()
		path := filepath.Join(dir, "test.tlg")
		writeFile(t, path, sds)

		decoder := converter.NewTLGImageDecoder()
		info, err := decoder.GetInfo(path)

		require.NoError(t, err)
		assert.Equal(t, converter.TLGVersionTLG5, info.Version)
		assert.Equal(t, 2, info.Width)
		assert.Equal(t, 2, info.Height)
		assert.True(t, info.HasAlpha)
	})

	t.Run("異常系: 存在しないファイルはErrSourceNotFoundを返す", func(t *testing.T) {
		t.Parallel()

		decoder := converter.NewTLGImageDecoder()
		_, err := decoder.GetInfo("/nonexistent/path/to/file.tlg")

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrSourceNotFound)
	})

	t.Run("異常系: TLG形式でないファイルはErrTLGInvalidFormatを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.tlg")
		writeFile(t, path, append([]byte("NOT_TLG_FORMAT"), make([]byte, 100)...))

		decoder := converter.NewTLGImageDecoder()
		_, err := decoder.GetInfo(path)

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrTLGInvalidFormat)
	})
}

func TestTLGImageDecoder_Decode(t *testing.T) {
	t.Parallel()

	t.Run("正常系: TLG5ファイルをデコードできる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.tlg")
		writeFile(t, path, buildTLG5Fixture(2, 32))

		decoder := converter.NewTLGImageDecoder()
		img, err := decoder.Decode(path)

		require.NoError(t, err)
		bounds := img.Bounds()
		assert.Equal(t, 2, bounds.Dx())
		assert.Equal(t, 2, bounds.Dy())

		c := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA) //nolint:forcetypeassert // color.NRGBAModel.Convertは常にcolor.NRGBAを返す
		assert.Equal(t, color.NRGBA{R: tlg5FixtureR, G: tlg5FixtureG, B: tlg5FixtureB, A: tlg5FixtureA}, c)
	})

	t.Run("正常系: SDSコンテナ内のTLG5はアンラップ後の生TLG5と同一にデコードされる", func(t *testing.T) {
		t.Parallel()

		// DEFECT 1の回帰防止: 実TLGは TLG0.0\x00sds\x1a のSDSコンテナで包まれ、
		// 内部に生のTLG5データを持つ。SDSラッパー付きファイルとアンラップ済みの
		// 生TLG5ファイルが同一の画像にデコードされることを検証する。
		inner := buildTLG5Fixture(2, 32)

		sds := append([]byte{}, sdsMagic...)
		sizeBuf := make([]byte, 4)
		binary.LittleEndian.PutUint32(sizeBuf, uint32(len(inner))) //nolint:gosec // テストフィクスチャの小さいサイズのみを扱う
		sds = append(sds, sizeBuf...)
		sds = append(sds, inner...)

		dir := t.TempDir()
		rawPath := filepath.Join(dir, "raw.tlg")
		sdsPath := filepath.Join(dir, "sds.tlg")
		writeFile(t, rawPath, inner)
		writeFile(t, sdsPath, sds)

		decoder := converter.NewTLGImageDecoder()
		rawImg, rawErr := decoder.Decode(rawPath)
		require.NoError(t, rawErr)
		sdsImg, sdsErr := decoder.Decode(sdsPath)
		require.NoError(t, sdsErr)

		require.Equal(t, rawImg.Bounds(), sdsImg.Bounds())
		for y := range sdsImg.Bounds().Dy() {
			for x := range sdsImg.Bounds().Dx() {
				rawC := color.NRGBAModel.Convert(rawImg.At(x, y))
				sdsC := color.NRGBAModel.Convert(sdsImg.At(x, y))
				assert.Equal(t, rawC, sdsC, "pixel (%d,%d)", x, y)
			}
		}
	})

	t.Run("異常系: TLG6ファイルはErrTLGDecodeNotImplementedを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.tlg")
		writeFile(t, path, buildTLG6HeaderFixture(32, 0, 8, 4, 1, 1))

		decoder := converter.NewTLGImageDecoder()
		_, err := decoder.Decode(path)

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrTLGDecodeNotImplemented)
	})

	t.Run("異常系: 存在しないファイルはErrSourceNotFoundを返す", func(t *testing.T) {
		t.Parallel()

		decoder := converter.NewTLGImageDecoder()
		_, err := decoder.Decode("/nonexistent/path/to/file.tlg")

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrSourceNotFound)
	})

	t.Run("異常系: TLG形式でないファイルはErrTLGInvalidFormatを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.tlg")
		writeFile(t, path, append([]byte("NOT_TLG_FORMAT"), make([]byte, 100)...))

		decoder := converter.NewTLGImageDecoder()
		_, err := decoder.Decode(path)

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrTLGInvalidFormat)
	})
}

func TestTLGImageDecoder_DecodeToFile(t *testing.T) {
	t.Parallel()

	t.Run("正常系: PNGファイルに保存できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.tlg")
		writeFile(t, source, buildTLG5Fixture(2, 32))
		dest := filepath.Join(dir, "output.png")

		decoder := converter.NewTLGImageDecoder()
		require.NoError(t, decoder.DecodeToFile(source, dest))

		assert.FileExists(t, dest)

		f, err := os.Open(dest) //nolint:gosec // テスト用の一時ファイル
		require.NoError(t, err)
		defer func() { _ = f.Close() }()

		img, err := png.Decode(f)
		require.NoError(t, err)
		assert.Equal(t, 2, img.Bounds().Dx())
		assert.Equal(t, 2, img.Bounds().Dy())
	})

	t.Run("正常系: 親ディレクトリが存在しない場合に作成される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.tlg")
		writeFile(t, source, buildTLG5Fixture(2, 32))
		dest := filepath.Join(dir, "subdir", "nested", "output.png")

		decoder := converter.NewTLGImageDecoder()
		require.NoError(t, decoder.DecodeToFile(source, dest))

		assert.FileExists(t, dest)
	})

	t.Run("正常系: WebPファイルに保存できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.tlg")
		writeFile(t, source, buildTLG5Fixture(2, 32))
		dest := filepath.Join(dir, "output.webp")

		decoder := converter.NewTLGImageDecoder()
		require.NoError(t, decoder.DecodeToFile(source, dest))

		assert.FileExists(t, dest)
	})

	t.Run("異常系: 存在しないソースファイルはErrSourceNotFoundを返す", func(t *testing.T) {
		t.Parallel()

		decoder := converter.NewTLGImageDecoder()
		err := decoder.DecodeToFile("/nonexistent/source.tlg", filepath.Join(t.TempDir(), "output.png"))

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrSourceNotFound)
	})

	t.Run("異常系: サポートされていない拡張子はErrUnsupportedImageFormatを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.tlg")
		writeFile(t, source, buildTLG5Fixture(2, 32))
		dest := filepath.Join(dir, "output.gif")

		decoder := converter.NewTLGImageDecoder()
		err := decoder.DecodeToFile(source, dest)

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrUnsupportedImageFormat)
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

func TestOutputFormat_Values(t *testing.T) {
	t.Parallel()

	assert.Equal(t, converter.OutputFormatWebP, converter.OutputFormat("webp"))
	assert.Equal(t, converter.OutputFormatPNG, converter.OutputFormat("png"))
}

func TestImageConverter_OutputFormat(t *testing.T) {
	t.Parallel()

	t.Run("正常系: デフォルトの出力形式はPNG", func(t *testing.T) {
		t.Parallel()

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		assert.Equal(t, converter.OutputFormatPNG, c.OutputFormat())
	})

	t.Run("正常系: WebP出力形式を指定できる", func(t *testing.T) {
		t.Parallel()

		c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, int(converter.QualityHigh), true)
		assert.Equal(t, converter.OutputFormatWebP, c.OutputFormat())
	})
}

func TestImageConverter_GetOutputExtension(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		outputFormat converter.OutputFormat
		expectedExt  string
	}{
		"正常系: PNG出力形式は.pngを返す":   {converter.OutputFormatPNG, ".png"},
		"正常系: WebP出力形式は.webpを返す": {converter.OutputFormatWebP, ".webp"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := converter.NewImageConverterWithFormat(tc.outputFormat, int(converter.QualityHigh), true)
			assert.Equal(t, tc.expectedExt, c.GetOutputExtension("test.tlg"))
		})
	}
}

func TestImageConverter_Convert(t *testing.T) {
	t.Parallel()

	t.Run("正常系: BMPファイルをPNGに変換できる（デフォルト）", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.bmp")
		writeBMPFixture(t, source, color.RGBA{R: 255, A: 255})
		dest := filepath.Join(dir, "output.png")

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)

		f, err := os.Open(dest) //nolint:gosec // テスト用の一時ファイル
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		_, err = png.Decode(f)
		require.NoError(t, err)
	})

	t.Run("正常系: BMPファイルをWebPに変換できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.bmp")
		writeBMPFixture(t, source, color.RGBA{R: 255, A: 255})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("正常系: JPGファイルをPNGに変換できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.jpg")
		writeJPEGFixture(t, source, color.RGBA{G: 255, A: 255})
		dest := filepath.Join(dir, "output.png")

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

		c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, int(converter.QualityHigh), true)
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

		c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("正常系: TLG(TLG5)ファイルをPNGに変換できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.tlg")
		writeFile(t, source, buildTLG5Fixture(2, 32))
		dest := filepath.Join(dir, "output.png")

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)

		f, err := os.Open(dest) //nolint:gosec // テスト用の一時ファイル
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		img, err := png.Decode(f)
		require.NoError(t, err)

		c2 := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA) //nolint:forcetypeassert // color.NRGBAModel.Convertは常にcolor.NRGBAを返す
		assert.Equal(t, color.NRGBA{R: tlg5FixtureR, G: tlg5FixtureG, B: tlg5FixtureB, A: tlg5FixtureA}, c2)
	})

	t.Run("正常系: TLG(TLG5)ファイルをWebPに変換できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.tlg")
		writeFile(t, source, buildTLG5Fixture(2, 32))
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("正常系: 品質プリセットが適用される（WebP出力時）", func(t *testing.T) {
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

				c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, int(tc.preset), true)
				assert.Equal(t, tc.quality, c.Quality())

				result, err := c.Convert(source, dest)
				require.NoError(t, err)
				assert.Equal(t, converter.StatusSuccess, result.Status)
			})
		}
	})

	t.Run("正常系: カスタム品質値が適用される（WebP出力時）", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.bmp")
		writeBMPFixture(t, source, color.RGBA{R: 255, A: 255})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, 50, true)
		assert.Equal(t, 50, c.Quality())

		result, err := c.Convert(source, dest)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
	})

	t.Run("正常系: quality=0はQualityHighにフォールバックする", func(t *testing.T) {
		t.Parallel()

		// why: NewImageConverterへゼロ値を渡した場合もHIGH(95)相当にフォールバック
		// することをピン留めする。
		c := converter.NewImageConverter(0, true)
		assert.Equal(t, int(converter.QualityHigh), c.Quality())
	})

	t.Run("正常系: PNG出力でアルファチャンネルが保持される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test_alpha.png")
		writePNGFixture(t, source, color.RGBA{R: 255, A: 128})
		dest := filepath.Join(dir, "output.png")

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)

		f, err := os.Open(dest) //nolint:gosec // テスト用の一時ファイル
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		img, err := png.Decode(f)
		require.NoError(t, err)
		_, ok := img.(*image.NRGBA)
		assert.True(t, ok, "アルファ付きPNG入力はimage.NRGBAとしてデコードされることを期待")
	})

	t.Run("正常系: アルファチャンネルが保持される(lossless_alpha=true, WebP出力時)", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test_alpha.png")
		writePNGFixture(t, source, color.RGBA{R: 255, A: 128})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, int(converter.QualityHigh), true)
		assert.True(t, c.LosslessAlpha())

		result, err := c.Convert(source, dest)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("正常系: lossless_alpha=falseで非ロスレスアルファが適用される(WebP出力時)", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test_alpha.png")
		writePNGFixture(t, source, color.RGBA{R: 255, A: 128})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, int(converter.QualityHigh), false)
		assert.False(t, c.LosslessAlpha())

		result, err := c.Convert(source, dest)
		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("正常系: 全ピクセル不透明でもフォーマット上アルファを持つPNGはロスレスになる(WebP出力時)", func(t *testing.T) {
		t.Parallel()

		// why: フォーマット上アルファを持つPNG(*image.NRGBA)は、全ピクセルが
		// 不透明であってもロスレスパスへ通すことをピン留めする
		// （Opaque()のみで判定するとロッシーパスに落ちてしまう回帰を防ぐ）。
		dir := t.TempDir()
		source := filepath.Join(dir, "opaque_alpha.png")
		writeOpaqueAlphaPNGFixture(t, source, color.RGBA{R: 255, A: 255})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.True(t, isLosslessWebP(t, dest))
	})

	t.Run("正常系: 24bppのBMPはロッシーパスになる(WebP出力時)", func(t *testing.T) {
		t.Parallel()

		// why: レビュー指摘の回帰防止。golang.org/x/image/bmpの24bpp BMPデコード
		// 結果はalpha=0xff固定の*image.RGBAであり、アルファチャンネルを持たない
		// フォーマットである。Opaque()フォールバックにより非ロスレスのRGB
		// パスになることをピン留めする。
		dir := t.TempDir()
		source := filepath.Join(dir, "test.bmp")
		writeBMPFixture(t, source, color.RGBA{R: 255, A: 255})
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.False(t, isLosslessWebP(t, dest))
	})

	t.Run("正常系: RGB(3チャンネル)のTLG5画像はWebP出力時にロッシーパスになる", func(t *testing.T) {
		t.Parallel()

		// why: TLG5Decoder.Decodeはcolors==3(RGB)の場合image.RGBAを返しA=255
		// 固定になる。imageHasAlphaがNRGBAModelのみをアルファ有り扱いする
		// ため、RGB由来の画像はhasAlpha=falseとなりロッシーパスになることを
		// ピン留めする。
		dir := t.TempDir()
		source := filepath.Join(dir, "test.tlg")
		writeFile(t, source, buildTLG5Fixture(2, 24))
		dest := filepath.Join(dir, "output.webp")

		c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.False(t, isLosslessWebP(t, dest))
	})

	t.Run("正常系: 変換結果に成功状態が正しく記録される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.bmp")
		writeBMPFixture(t, source, color.RGBA{R: 255, A: 255})
		dest := filepath.Join(dir, "output.png")

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
		dest := filepath.Join(dir, "output.png")

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
		dest := filepath.Join(dir, "subdir", "nested", "output.png")

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("異常系: TLG6ファイルは未実装エラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "test.tlg")
		writeFile(t, source, buildTLG6HeaderFixture(32, 0, 8, 4, 1, 1))
		dest := filepath.Join(dir, "output.png")

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		_, err := c.Convert(source, dest)

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrTLGDecodeNotImplemented)
	})

	t.Run("異常系: 存在しない変換元ファイルはエラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		_, err := c.Convert(filepath.Join(dir, "nonexistent.bmp"), filepath.Join(dir, "output.png"))

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrSourceNotFound)
	})

	t.Run("異常系: 変換元がディレクトリの場合エラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		_, err := c.Convert(dir, filepath.Join(dir, "output.png"))

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrSourceIsDirectory)
	})
}

func TestImageConverter_ConvertFromImage(t *testing.T) {
	t.Parallel()

	t.Run("正常系: PNGとして保存できる（デフォルト）", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		dest := filepath.Join(dir, "from_image.png")

		img := newSolidRGBA(color.RGBA{R: 255, G: 128, A: 255})

		c := converter.NewImageConverter(int(converter.QualityHigh), true)
		result, err := c.ConvertFromImage(img, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})

	t.Run("正常系: WebPとして保存できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		dest := filepath.Join(dir, "from_image.webp")

		img := newSolidRGBA(color.RGBA{R: 255, G: 128, A: 255})

		c := converter.NewImageConverterWithFormat(converter.OutputFormatWebP, int(converter.QualityHigh), true)
		result, err := c.ConvertFromImage(img, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
	})
}

func TestImageConverter_SupportedExtensions(t *testing.T) {
	t.Parallel()

	// TLGのみ変換対象（JPEG/PNG/BMPはkrkrsdl2でネイティブサポートのため
	// 変換不要。feat/exe-icon-extraction 680b27fより）。
	c := converter.NewImageConverter(int(converter.QualityHigh), true)
	assert.Equal(t, []string{".tlg"}, c.SupportedExtensions())
}

func TestImageConverter_CanConvert(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		filename string
		expected bool
	}{
		"正常系: TLGファイルは変換可能":   {"test.tlg", true},
		"正常系: 大文字拡張子TLGも変換可能": {"TEST.TLG", true},
		"正常系: BMPファイルは変換不可":   {"test.bmp", false},
		"正常系: JPGファイルは変換不可":   {"test.jpg", false},
		"正常系: JPEGファイルは変換不可":  {"test.jpeg", false},
		"正常系: PNGファイルは変換不可":   {"test.png", false},
		"異常系: GIFファイルは変換不可":   {"test.gif", false},
		"異常系: WebPファイルは変換不可":  {"test.webp", false},
		"異常系: TXTファイルは変換不可":   {"test.txt", false},
	}

	c := converter.NewImageConverter(int(converter.QualityHigh), true)
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, c.CanConvert(tc.filename))
		})
	}
}
