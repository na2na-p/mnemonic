package tlg_test

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/converter/tlg"
)

// tlg5Header はTLG5形式のヘッダーバイト列を生成するテストヘルパー。
func tlg5Header(colorDepth byte, width, height, blockHeight uint32) []byte {
	header := append([]byte{}, tlg.TLG5Magic...)
	header = append(header, colorDepth)

	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, width)
	header = append(header, buf...)
	binary.LittleEndian.PutUint32(buf, height)
	header = append(header, buf...)
	binary.LittleEndian.PutUint32(buf, blockHeight)
	header = append(header, buf...)

	return header
}

// tlg5LZSSLiteralBlock はchannelDataを吉里吉里LZSSの全リテラル形式
// （8バイトごとにフラグ0x00を挿入）でエンコードし、mark(1)+size(4)付きの
// チャンネルブロックへ変換するテストヘルパー。
func tlg5LZSSLiteralBlock(channelData []byte) []byte {
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
	block = append(block, compressed...)

	return block
}

// buildTLG5 はwidth x heightの単色画像を表すTLG5バイト列を組み立てるテスト
// ヘルパー。channelValuesはBGRA順（RGBの場合はBGR順）で、各チャンネルの
// 先頭ピクセルにのみ値を置き残りは0（デルタ差分0 = 同色）とする。
func buildTLG5(width, height, blockHeight int, colorDepth byte, channelValues []byte) []byte {
	header := tlg5Header(colorDepth, uint32(width), uint32(height), uint32(blockHeight)) //nolint:gosec // テストフィクスチャの小さい値のみを扱う

	blockCount := (height + blockHeight - 1) / blockHeight

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
			blockBytes = append(blockBytes, tlg5LZSSLiteralBlock(channelData)...)
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

func TestTLG5Decoder_IsValid(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		data     []byte
		expected bool
	}{
		"正常系: 有効なTLG5マジックバイト": {append(append([]byte{}, tlg.TLG5Magic...), make([]byte, 20)...), true},
		"異常系: TLG6形式のマジックバイト": {append(append([]byte{}, tlg.TLG6Magic...), make([]byte, 20)...), false},
		"異常系: PNG形式のマジックバイト":  {[]byte("PNG\x89\x50\x4e\x47"), false},
		"異常系: 不完全なマジックバイト":    {[]byte("TLG5.0"), false},
		"異常系: 空のデータ":          {[]byte{}, false},
	}

	d := tlg.NewTLG5Decoder()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, d.IsValid(tc.data))
		})
	}
}

func TestTLG5Decoder_ParseHeader(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		colorDepth     byte
		width, height  uint32
		blockHeight    uint32
		expectedColors int
	}{
		"正常系: RGBA画像ヘッダー解析":  {32, 640, 480, 4, 4},
		"正常系: RGB画像ヘッダー解析":   {24, 800, 600, 4, 3},
		"正常系: 大きな画像ヘッダー解析":   {32, 1920, 1080, 8, 4},
		"正常系: 最小サイズ画像ヘッダー解析": {24, 1, 1, 1, 3},
	}

	d := tlg.NewTLG5Decoder()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data := tlg5Header(tc.colorDepth, tc.width, tc.height, tc.blockHeight)
			header, err := d.ParseHeader(data)

			require.NoError(t, err)
			assert.Equal(t, int(tc.width), header.Width)
			assert.Equal(t, int(tc.height), header.Height)
			assert.Equal(t, tc.expectedColors, header.Colors)
			assert.Equal(t, int(tc.blockHeight), header.BlockHeight)
		})
	}

	t.Run("異常系: 無効なマジックバイトでErrTLG5InvalidMagicを返す", func(t *testing.T) {
		t.Parallel()

		data := append([]byte("INVALID_MAGIC"), make([]byte, 20)...)
		_, err := d.ParseHeader(data)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrTLG5InvalidMagic)
	})

	t.Run("異常系: データが短すぎる場合ErrTLG5DataTooShortを返す", func(t *testing.T) {
		t.Parallel()

		data := append(append([]byte{}, tlg.TLG5Magic...), make([]byte, 5)...)
		_, err := d.ParseHeader(data)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrTLG5DataTooShort)
	})
}

func TestTLG5Decoder_Decode(t *testing.T) {
	t.Parallel()

	t.Run("異常系: 無効なマジックバイトでErrTLG5InvalidMagicを返す", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewTLG5Decoder()
		data := append([]byte("INVALID_MAGIC"), make([]byte, 20)...)

		_, err := d.Decode(data)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrTLG5InvalidMagic)
	})

	t.Run("正常系: 2x2 RGBA単色画像をデコードできる", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewTLG5Decoder()
		// BGRA順（B=64, G=128, R=255, A=255）
		data := buildTLG5(2, 2, 2, 32, []byte{64, 128, 255, 255})

		img, err := d.Decode(data)

		require.NoError(t, err)
		require.NotNil(t, img)
		bounds := img.Bounds()
		assert.Equal(t, 2, bounds.Dx())
		assert.Equal(t, 2, bounds.Dy())

		for y := range 2 {
			for x := range 2 {
				r, g, b, a := colorAt(t, img, x, y)
				assert.Equal(t, [4]uint8{255, 128, 64, 255}, [4]uint8{r, g, b, a}, "pixel (%d,%d)", x, y)
			}
		}
	})

	t.Run("正常系: 2x2 RGB画像をデコードできる（アルファは不透明として扱う）", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewTLG5Decoder()
		// BGR順（B=64, G=128, R=255）
		data := buildTLG5(2, 2, 2, 24, []byte{64, 128, 255})

		img, err := d.Decode(data)

		require.NoError(t, err)
		for y := range 2 {
			for x := range 2 {
				r, g, b, a := colorAt(t, img, x, y)
				assert.Equal(t, [4]uint8{255, 128, 64, 255}, [4]uint8{r, g, b, a}, "pixel (%d,%d)", x, y)
			}
		}
	})

	t.Run("正常系: 4x4画像・複数ブロックをデコードできる", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewTLG5Decoder()
		// width=4, height=4, block_height=2 -> 2ブロック
		// BGRA順（B=200, G=150, R=100, A=255）
		data := buildTLG5(4, 4, 2, 32, []byte{200, 150, 100, 255})

		img, err := d.Decode(data)

		require.NoError(t, err)
		for y := range 4 {
			for x := range 4 {
				r, g, b, a := colorAt(t, img, x, y)
				assert.Equal(t, [4]uint8{100, 150, 200, 255}, [4]uint8{r, g, b, a}, "pixel (%d,%d)", x, y)
			}
		}
	})

	t.Run("異常系: block_heightが0の場合ErrTLG5InvalidBlockHeightを返す(ゼロ除算panicの回帰防止)", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewTLG5Decoder()
		data := tlg5Header(32, 4, 4, 0)

		_, err := d.Decode(data)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrTLG5InvalidBlockHeight)
	})

	t.Run("異常系: ブロックデータが不完全な場合ErrTLG5IncompleteBlockDataを返す", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewTLG5Decoder()
		width, height, blockHeight := 2, 2, 2

		header := tlg5Header(32, uint32(width), uint32(height), uint32(blockHeight)) //nolint:gosec // テストフィクスチャの小さい値のみを扱う
		blockCount := (height + blockHeight - 1) / blockHeight
		blockSizes := make([]byte, blockCount*4)

		// block_sizeは10と主張するが実際は5バイトしかない不完全データ
		incomplete := []byte{0, 10, 0, 0, 0, 0, 0, 0, 0, 0}
		data := append(append(header, blockSizes...), incomplete...)

		_, err := d.Decode(data)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrTLG5IncompleteBlockData)
	})
}

// colorAt はimg上の(x, y)ピクセルを8bit RGBA成分として取得するテストヘルパー。
//
// why not: TLG5Decoder.Decodeの戻り値の具象型はcolors(RGB/RGBA)により
// image.RGBA/image.NRGBAで異なる（tlg5.go createImageFromChannelsのwhy not
// コメント参照）。具象型はDecodeの公開契約ではないため、color.NRGBAModel.
// Convertで正規化した値を検証し、具象型に依存しないテストにする。
func colorAt(t *testing.T, img image.Image, x, y int) (r, g, b, a uint8) {
	t.Helper()

	c, ok := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	require.True(t, ok, "color.NRGBAModel.Convertは常にcolor.NRGBAを返す")

	return c.R, c.G, c.B, c.A
}
