package tlg_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/converter/tlg"
)

// tlg6Header はTLG6形式のヘッダーバイト列（マジック + 色数・データフラグ・
// カラータイプ・外部ゴロムテーブル各1バイト + width・height・max_bit_length
// 各4バイト）を生成するテストヘルパー。
func tlg6Header(colors, dataFlag, colorType, externalGolomb byte, width, height, maxBitLength uint32) []byte {
	header := bytes.Clone(tlg.TLG6Magic)
	header = append(header, colors, dataFlag, colorType, externalGolomb)
	header = binary.LittleEndian.AppendUint32(header, width)
	header = binary.LittleEndian.AppendUint32(header, height)
	header = binary.LittleEndian.AppendUint32(header, maxBitLength)

	return header
}

// tlg6Channel は1チャンネル分のエントロピー符号化データ（ビット長4バイト +
// ビット列）を表すテスト用の値。
type tlg6Channel struct {
	bitLength uint32
	bits      []byte
}

// buildTLG6 はRGB(色数3)のTLG6ストリームを組み立てるテストヘルパー。
// filterTypes はLZSS圧縮済みのフィルタ種別列、channels はブロック行ごと・
// 色ごとの順に並べたエントロピー符号化データ。
func buildTLG6(width, height uint32, filterTypes []byte, channels ...tlg6Channel) []byte {
	var maxBitLength uint32
	for _, ch := range channels {
		maxBitLength = max(maxBitLength, ch.bitLength)
	}

	data := tlg6Header(3, 0, 0, 0, width, height, maxBitLength)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(filterTypes))) //nolint:gosec // テスト用の数バイトのみ
	data = append(data, filterTypes...)

	for _, ch := range channels {
		data = binary.LittleEndian.AppendUint32(data, ch.bitLength)
		data = append(data, ch.bits...)
	}

	return data
}

// 手組みストリームの部品。
var (
	// filterTypeMEDNoChroma はブロック1個分のフィルタ種別0（MED予測・色相関
	// フィルタ無し）をLZSSで符号化したもの: フラグバイト0x00（リテラル）+
	// リテラル0x00。
	filterTypeMEDNoChroma = []byte{0x00, 0x00}

	// zeroRun1 は「最初の値はゼロ」(bit0=0) + ゼロの連続数1のガンマ符号"1"。
	zeroRun1 = tlg6Channel{bitLength: 2, bits: []byte{0x02}}

	// zeroRun4 は bit0=0 + ゼロの連続数4のガンマ符号（0,0,1 のあと下位2ビット
	// 0,0）。
	zeroRun4 = tlg6Channel{bitLength: 6, bits: []byte{0x08}}

	// residual5 は bit0=1（最初の値は非ゼロ）+ 非ゼロの連続数1のガンマ符号"1" +
	// 値+5のゴロム・ライス符号（先頭値なのでk=0: m=2*5-1=9 個の0と終端の1）。
	residual5 = tlg6Channel{bitLength: 12, bits: []byte{0x03, 0x08}}

	// residual5ThenZeroRun3 は residual5 に続けてゼロの連続数3のガンマ符号
	// （0, 1, 下位1ビット1）を置いたもの。
	residual5ThenZeroRun3 = tlg6Channel{bitLength: 15, bits: []byte{0x03, 0x68}}

	// zeroRun3ThenResidual5 は bit0=0 + ゼロの連続数3 + 非ゼロの連続数1 +
	// 値+5（k=0）。
	zeroRun3ThenResidual5 = tlg6Channel{bitLength: 15, bits: []byte{0x1C, 0x40}}
)

func loadTLG6Fixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name+".tlg"))
	require.NoError(t, err)

	return data
}

func loadExpectedPNG(t *testing.T, name string) *image.NRGBA {
	t.Helper()

	f, err := os.Open(filepath.Join("testdata", name+".png"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	img, err := png.Decode(f)
	require.NoError(t, err)

	if nrgba, ok := img.(*image.NRGBA); ok {
		return nrgba
	}

	// 不透明な画像はimage/pngがRGB・グレースケールとして保存するため、読み戻すと
	// NRGBA以外になる。不透明なら色モデルの変換は可逆。
	nrgba := image.NewNRGBA(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			nrgba.Set(x, y, color.NRGBAModel.Convert(img.At(x, y)))
		}
	}

	return nrgba
}

func TestTLG6Decoder_IsValid(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		data     []byte
		expected bool
	}{
		"正常系: 有効なTLG6マジックバイト": {append(append([]byte{}, tlg.TLG6Magic...), make([]byte, 20)...), true},
		"異常系: TLG5形式のマジックバイト": {append(append([]byte{}, tlg.TLG5Magic...), make([]byte, 20)...), false},
		"異常系: PNG形式のマジックバイト":  {[]byte("PNG\x89\x50\x4e\x47"), false},
		"異常系: 不完全なマジックバイト":    {[]byte("TLG6.0"), false},
		"異常系: 空のデータ":          {[]byte{}, false},
	}

	d := tlg.NewTLG6Decoder()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, d.IsValid(tc.data))
		})
	}
}

func TestTLG6Decoder_ParseHeader(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		colors                     byte
		width, height, maxBitLen   uint32
		expectedXBlock, expectedYB int
	}{
		"正常系: RGBA画像ヘッダー解析":            {4, 640, 480, 1234, 80, 60},
		"正常系: RGB画像ヘッダー解析":             {3, 800, 600, 99, 100, 75},
		"正常系: グレースケール画像ヘッダー解析":         {1, 21, 13, 7, 3, 2},
		"正常系: ブロック数は8ピクセル単位の切り上げになる":   {3, 61, 29, 0, 8, 4},
		"正常系: 最小サイズ画像ヘッダー解析":           {3, 1, 1, 2, 1, 1},
		"正常系: ブロック数は幅・高さが0なら0になる":      {4, 0, 0, 0, 0, 0},
		"正常系: 巨大な寸法もヘッダー解析自体は成功する":     {4, 0xFFFFFFFF, 0xFFFFFFFF, 0, 536870912, 536870912},
		"正常系: max_bit_lengthをそのまま保持する": {4, 8, 8, 0x3FFFFFFF, 1, 1},
	}

	d := tlg.NewTLG6Decoder()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			header, err := d.ParseHeader(tlg6Header(tc.colors, 0, 0, 0, tc.width, tc.height, tc.maxBitLen))

			require.NoError(t, err)
			assert.Equal(t, int(tc.width), header.Width)
			assert.Equal(t, int(tc.height), header.Height)
			assert.Equal(t, int(tc.colors), header.Colors)
			assert.Equal(t, int(tc.maxBitLen), header.MaxBitLength)
			assert.Equal(t, tc.expectedXBlock, header.XBlockCount)
			assert.Equal(t, tc.expectedYB, header.YBlockCount)
		})
	}

	errCases := map[string]struct {
		data     []byte
		expected error
	}{
		"異常系: 無効なマジックバイトでErrTLG6InvalidMagicを返す":            {append([]byte("INVALID_MAGIC"), make([]byte, 20)...), tlg.ErrTLG6InvalidMagic},
		"異常系: データが短すぎる場合ErrTLG6DataTooShortを返す":             {append(append([]byte{}, tlg.TLG6Magic...), make([]byte, 5)...), tlg.ErrTLG6DataTooShort},
		"異常系: ヘッダーが1バイト足りない場合ErrTLG6DataTooShortを返す":        {tlg6Header(3, 0, 0, 0, 8, 8, 0)[:26], tlg.ErrTLG6DataTooShort},
		"異常系: 色数0はErrTLG6UnsupportedColorCountを返す":          {tlg6Header(0, 0, 0, 0, 8, 8, 0), tlg.ErrTLG6UnsupportedColorCount},
		"異常系: 色数2はErrTLG6UnsupportedColorCountを返す":          {tlg6Header(2, 0, 0, 0, 8, 8, 0), tlg.ErrTLG6UnsupportedColorCount},
		"異常系: ビット深度表記24はErrTLG6UnsupportedColorCountを返す":    {tlg6Header(24, 0, 0, 0, 8, 8, 0), tlg.ErrTLG6UnsupportedColorCount},
		"異常系: ビット深度表記32はErrTLG6UnsupportedColorCountを返す":    {tlg6Header(32, 0, 0, 0, 8, 8, 0), tlg.ErrTLG6UnsupportedColorCount},
		"異常系: データフラグが0以外ならErrTLG6UnsupportedHeaderFieldを返す": {tlg6Header(3, 1, 0, 0, 8, 8, 0), tlg.ErrTLG6UnsupportedHeaderField},
		"異常系: カラータイプが0以外ならErrTLG6UnsupportedHeaderFieldを返す": {tlg6Header(3, 0, 1, 0, 8, 8, 0), tlg.ErrTLG6UnsupportedHeaderField},
		"異常系: 外部ゴロムテーブル指定はErrTLG6UnsupportedHeaderFieldを返す": {tlg6Header(3, 0, 0, 1, 8, 8, 0), tlg.ErrTLG6UnsupportedHeaderField},
	}

	for name, tc := range errCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := d.ParseHeader(tc.data)

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.expected)
		})
	}
}

func TestTLG6Decoder_Decode_ReferenceFixtures(t *testing.T) {
	t.Parallel()

	fixtures := []string{
		"rgba_allfilters_61x29",
		"rgb_allfilters_noise_61x29",
		"rgba_noise_21x13",
		"rgba_grad_16x16",
		"rgb_grad_5x3",
		"rgba_noise_1x1",
		"gray_grad_21x13",
	}

	for _, name := range fixtures {
		t.Run("正常系: krkrz参照デコーダと全画素一致する: "+name, func(t *testing.T) {
			t.Parallel()

			expected := loadExpectedPNG(t, name)

			img, err := tlg.NewTLG6Decoder().Decode(loadTLG6Fixture(t, name))
			require.NoError(t, err)

			nrgba, ok := img.(*image.NRGBA)
			require.True(t, ok, "TLG6のデコード結果はimage.NRGBA")
			require.Equal(t, expected.Bounds(), nrgba.Bounds())
			assert.Equal(t, expected.Pix, nrgba.Pix)

			var buf bytes.Buffer
			require.NoError(t, png.Encode(&buf, img))
		})
	}
}

func TestTLG6Decoder_Decode_HandBuilt(t *testing.T) {
	t.Parallel()

	// 期待値は左上から行優先のNRGBA画素（R, G, B, A）。
	cases := map[string]struct {
		data          []byte
		width, height int
		expected      []byte
	}{
		"正常系: 残差がすべて0の1x1 RGBは黒・不透明になる": {
			data:   buildTLG6(1, 1, filterTypeMEDNoChroma, zeroRun1, zeroRun1, zeroRun1),
			width:  1,
			height: 1,
			expected: []byte{
				0, 0, 0, 255,
			},
		},
		"正常系: Bチャンネルの残差+5が青5として復元される": {
			data:   buildTLG6(1, 1, filterTypeMEDNoChroma, residual5, zeroRun1, zeroRun1),
			width:  1,
			height: 1,
			expected: []byte{
				0, 0, 5, 255,
			},
		},
		"正常系: 4x1で先頭画素の残差だけがMED予測で行全体へ伝わる": {
			data:   buildTLG6(4, 1, filterTypeMEDNoChroma, residual5ThenZeroRun3, zeroRun4, zeroRun4),
			width:  4,
			height: 1,
			expected: []byte{
				0, 0, 5, 255, 0, 0, 5, 255, 0, 0, 5, 255, 0, 0, 5, 255,
			},
		},
		"正常系: 2x2で奇数行は右から左へ並ぶ（4番目の残差は左下画素）": {
			data:   buildTLG6(2, 2, filterTypeMEDNoChroma, zeroRun3ThenResidual5, zeroRun4, zeroRun4),
			width:  2,
			height: 2,
			expected: []byte{
				0, 0, 0, 255, 0, 0, 0, 255,
				0, 0, 5, 255, 0, 0, 5, 255,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			img, err := tlg.NewTLG6Decoder().Decode(tc.data)
			require.NoError(t, err)

			nrgba, ok := img.(*image.NRGBA)
			require.True(t, ok, "TLG6のデコード結果はimage.NRGBA")
			assert.Equal(t, image.Rect(0, 0, tc.width, tc.height), nrgba.Bounds())
			assert.Equal(t, tc.expected, nrgba.Pix)
		})
	}
}

func TestTLG6Decoder_Decode_Errors(t *testing.T) {
	t.Parallel()

	withBitLength := func(bitLength uint32, bits ...byte) tlg6Channel {
		return tlg6Channel{bitLength: bitLength, bits: bits}
	}

	cases := map[string]struct {
		data     []byte
		expected error
	}{
		"異常系: 無効なマジックバイトでErrTLG6InvalidMagicを返す": {
			append([]byte("INVALID_MAGIC"), make([]byte, 20)...), tlg.ErrTLG6InvalidMagic,
		},
		"異常系: 未対応の色数はErrTLG6UnsupportedColorCountを返す": {
			tlg6Header(2, 0, 0, 0, 1, 1, 0), tlg.ErrTLG6UnsupportedColorCount,
		},
		"異常系: 幅0はErrTLG6InvalidDimensionsを返す": {
			tlg6Header(3, 0, 0, 0, 0, 1, 0), tlg.ErrTLG6InvalidDimensions,
		},
		"異常系: 高さ0はErrTLG6InvalidDimensionsを返す": {
			tlg6Header(3, 0, 0, 0, 1, 0, 0), tlg.ErrTLG6InvalidDimensions,
		},
		"異常系: 幅・高さが0xFFFFFFFFでも確保前にErrTLG6InvalidDimensionsを返す": {
			tlg6Header(4, 0, 0, 0, 0xFFFFFFFF, 0xFFFFFFFF, 0), tlg.ErrTLG6InvalidDimensions,
		},
		"異常系: 幅が一辺の上限を超えるとErrTLG6InvalidDimensionsを返す": {
			tlg6Header(4, 0, 0, 0, tlg.TLG6MaxDimension+1, 1, 0), tlg.ErrTLG6InvalidDimensions,
		},
		"異常系: 画素数が上限を超えると確保前にErrTLG6InvalidDimensionsを返す": {
			tlg6Header(4, 0, 0, 0, tlg.TLG6MaxDimension, tlg.TLG6MaxPixelCount/tlg.TLG6MaxDimension+1, 0), tlg.ErrTLG6InvalidDimensions,
		},
		"異常系: フィルタ種別の圧縮データ長が無いとErrTLG6DataTooShortを返す": {
			tlg6Header(3, 0, 0, 0, 1, 1, 0), tlg.ErrTLG6DataTooShort,
		},
		"異常系: フィルタ種別の圧縮データ長がデータ末尾を超えるとErrTLG6DataTooShortを返す": {
			binary.LittleEndian.AppendUint32(tlg6Header(3, 0, 0, 0, 1, 1, 0), 100), tlg.ErrTLG6DataTooShort,
		},
		"異常系: フィルタ種別が32以上ならErrTLG6CorruptDataを返す": {
			buildTLG6(1, 1, []byte{0x00, 0x40}, zeroRun1, zeroRun1, zeroRun1), tlg.ErrTLG6CorruptData,
		},
		"異常系: フィルタ種別がブロック数に満たないとErrTLG6CorruptDataを返す": {
			buildTLG6(9, 1, filterTypeMEDNoChroma, zeroRun1, zeroRun1, zeroRun1), tlg.ErrTLG6CorruptData,
		},
		"異常系: ゴロム方式以外のエントロピー符号化はErrTLG6UnsupportedEntropyMethodを返す": {
			buildTLG6(1, 1, filterTypeMEDNoChroma, withBitLength(2|1<<30, 0x02), zeroRun1, zeroRun1), tlg.ErrTLG6UnsupportedEntropyMethod,
		},
		"異常系: チャンネルデータがビット長に満たないとErrTLG6DataTooShortを返す": {
			buildTLG6(1, 1, filterTypeMEDNoChroma, withBitLength(1000, 0x02), zeroRun1, zeroRun1), tlg.ErrTLG6DataTooShort,
		},
		"異常系: ゼロの連続数が画素数を超えるとErrTLG6CorruptDataを返す": {
			buildTLG6(1, 1, filterTypeMEDNoChroma, zeroRun4, zeroRun1, zeroRun1), tlg.ErrTLG6CorruptData,
		},
		"異常系: ガンマ符号の終端が無いビット列はErrTLG6CorruptDataを返す": {
			buildTLG6(1, 1, filterTypeMEDNoChroma, withBitLength(16, 0x00, 0x00), zeroRun1, zeroRun1), tlg.ErrTLG6CorruptData,
		},
		"異常系: ゴロム符号のエスケープ値がデータ末尾を超えるとErrTLG6CorruptDataを返す": {
			buildTLG6(1, 1, filterTypeMEDNoChroma, withBitLength(32, 0x03, 0x00, 0x00, 0x00), zeroRun1, zeroRun1), tlg.ErrTLG6CorruptData,
		},
		"異常系: ブロック行のチャンネルデータが欠けるとErrTLG6DataTooShortを返す": {
			buildTLG6(1, 1, filterTypeMEDNoChroma, zeroRun1, zeroRun1), tlg.ErrTLG6DataTooShort,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var err error
			require.NotPanics(t, func() {
				_, err = tlg.NewTLG6Decoder().Decode(tc.data)
			})

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.expected)
		})
	}
}

func TestTLG6Decoder_Decode_Truncated(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"rgba_allfilters_61x29", "rgb_allfilters_noise_61x29", "gray_grad_21x13"} {
		t.Run("異常系: 16バイト境界で切り詰めたデータはpanicせずエラーを返す: "+name, func(t *testing.T) {
			t.Parallel()

			data := loadTLG6Fixture(t, name)
			for size := 0; size < len(data); size += 16 {
				var err error
				require.NotPanics(t, func() {
					_, err = tlg.NewTLG6Decoder().Decode(data[:size])
				}, "size=%d", size)
				require.Error(t, err, "size=%d", size)
			}
		})
	}

	t.Run("異常系: 末尾1バイトを欠いたデータはErrTLG6DataTooShortを返す", func(t *testing.T) {
		t.Parallel()

		data := loadTLG6Fixture(t, "rgba_allfilters_61x29")
		_, err := tlg.NewTLG6Decoder().Decode(data[:len(data)-1])

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrTLG6DataTooShort)
	})
}

func TestTLG6Decoder_Decode_CorruptedBytes(t *testing.T) {
	t.Parallel()

	// ヘッダー（寸法を含む先頭27バイト）は書き換えない。寸法を壊すと検証の
	// 上限まで大きな画像を確保しうるため、本文だけを壊して頑健性を確かめる。
	const headerSize = 27

	for _, name := range []string{"rgba_grad_16x16", "rgb_grad_5x3"} {
		t.Run("異常系: 本文の1バイトを書き換えてもpanicしない: "+name, func(t *testing.T) {
			t.Parallel()

			original := loadTLG6Fixture(t, name)
			for offset := headerSize; offset < len(original); offset++ {
				for _, value := range []byte{0x00, 0xFF, original[offset] ^ 0x55} {
					data := bytes.Clone(original)
					data[offset] = value
					require.NotPanics(t, func() {
						_, _ = tlg.NewTLG6Decoder().Decode(data)
					}, "offset=%d value=%#x", offset, value)
				}
			}
		})
	}
}
