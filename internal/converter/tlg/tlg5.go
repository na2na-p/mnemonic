package tlg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
)

// TLG5Magic はTLG5形式のマジックバイト。
var TLG5Magic = []byte("TLG5.0\x00raw\x1a")

// TLG5HeaderSize はTLG5ヘッダーサイズ: マジック(11) + 色深度(1) + width(4) +
// height(4) + block_height(4)。
const TLG5HeaderSize = 24

// TLG5デコード時のセンチネルエラー群。
var (
	ErrTLG5InvalidMagic        = errors.New("TLG5形式ではありません")
	ErrTLG5DataTooShort        = errors.New("データが短すぎます")
	ErrTLG5IncompleteBlockData = errors.New("ブロックデータが不完全です")

	// ErrTLG5InvalidBlockHeight はblock_heightが0以下の場合のエラー。
	//
	// why not: Python参照実装はblock_height==0をガードせずブロック数計算
	// (height + block_height - 1) // block_heightでZeroDivisionErrorを
	// 未捕捉のまま送出する（呼び出し元プロセスがクラッシュする）。mnemonicは
	// 信頼できないゲームアセットを処理するCLIであり、不正なTLG5ファイル1件で
	// ビルド全体がpanicで落ちるのは受容できないため、Go版はここでガードし
	// 通常のerrorとして返す。
	ErrTLG5InvalidBlockHeight = errors.New("ブロック高さが不正です")
)

// TLG5Header はTLG5画像ファイルのヘッダーから読み取った情報を保持する不変値。
type TLG5Header struct {
	Width       int
	Height      int
	Colors      int // 3=RGB、4=RGBA
	BlockHeight int
}

// TLG5Decoder はTLG5形式（LZSS圧縮・カラープレーン分離方式）の画像を
// デコードする。
type TLG5Decoder struct {
	lzss *LZSSDecoder
}

// NewTLG5Decoder はTLG5Decoderを初期化する。
func NewTLG5Decoder() *TLG5Decoder {
	return &TLG5Decoder{lzss: NewLZSSDecoder()}
}

// IsValid はdataがTLG5形式のマジックバイトを持つかどうかを判定する。
func (d *TLG5Decoder) IsValid(data []byte) bool {
	return bytes.HasPrefix(data, TLG5Magic)
}

// ParseHeader はTLG5ヘッダーを解析する。
func (d *TLG5Decoder) ParseHeader(data []byte) (TLG5Header, error) {
	if !d.IsValid(data) {
		return TLG5Header{}, ErrTLG5InvalidMagic
	}

	if len(data) < TLG5HeaderSize {
		return TLG5Header{}, ErrTLG5DataTooShort
	}

	offset := len(TLG5Magic)

	// 色深度: 24=RGB(3色), 32=RGBA(4色)
	colorDepth := data[offset]
	colors := 3
	if colorDepth == 32 {
		colors = 4
	}
	offset++

	width := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4
	height := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4
	blockHeight := int(binary.LittleEndian.Uint32(data[offset : offset+4]))

	return TLG5Header{Width: width, Height: height, Colors: colors, BlockHeight: blockHeight}, nil
}

// Decode はTLG5形式のバイト列をデコードし、image.Imageを返す。
//
// 戻り値の具象型はcolors(3=RGBはimage.RGBA、4=RGBAはimage.NRGBA)に応じて
// 異なる（詳細はcreateImageFromChannelsのwhy notコメントを参照）。
func (d *TLG5Decoder) Decode(data []byte) (image.Image, error) {
	if !d.IsValid(data) {
		return nil, ErrTLG5InvalidMagic
	}

	header, err := d.ParseHeader(data)
	if err != nil {
		return nil, err
	}

	if header.BlockHeight <= 0 {
		return nil, ErrTLG5InvalidBlockHeight
	}

	width, height, colors, blockHeight := header.Width, header.Height, header.Colors, header.BlockHeight
	blockCount := (height + blockHeight - 1) / blockHeight

	// TLG5はBGRA順で格納される
	channels := make([][]byte, colors)
	for i := range channels {
		channels[i] = make([]byte, width*height)
	}

	// ブロックサイズ配列の後からブロックデータが始まる
	offset := TLG5HeaderSize + blockCount*4

	for blockIdx := range blockCount {
		blockYStart := blockIdx * blockHeight
		blockRows := min(blockHeight, height-blockYStart)
		blockPixelCount := width * blockRows

		for channelIdx := range colors {
			// 各チャンネルデータにはmark(1) + size(4)のヘッダーがある
			if offset+5 > len(data) {
				return nil, ErrTLG5IncompleteBlockData
			}

			offset++ // mark は通常0（未使用フラグ）

			channelSize := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
			offset += 4

			if offset+channelSize > len(data) {
				return nil, ErrTLG5IncompleteBlockData
			}

			compressed := data[offset : offset+channelSize]
			offset += channelSize

			decompressed, decErr := d.lzss.Decode(compressed, blockPixelCount)
			if decErr != nil {
				return nil, fmt.Errorf("%w: %w", ErrTLG5IncompleteBlockData, decErr)
			}

			applyDeltaDecoding(channels[channelIdx], decompressed, width, blockYStart, blockRows)
		}
	}

	return createImageFromChannels(channels, width, height, colors), nil
}

// applyDeltaDecoding はデルタエンコーディングされたデータを復元する。
//
// TLG5のデルタエンコーディング: 各ピクセル = (水平累積 + delta + 上のピクセル)
// & 0xFF。水平累積は各行でリセットされ、最初の行（y=0）では上のピクセル=0。
func applyDeltaDecoding(channel, deltaData []byte, width, yStart, rows int) {
	deltaPos := 0

	for row := range rows {
		y := yStart + row
		rowOffset := y * width
		hasPrevRow := y > 0
		prevRowOffset := (y - 1) * width

		// 各行で水平累積をリセット。byte型の加算はGoの仕様上mod 256で
		// wrapするため、Python版の `& 0xFF` に相当する挙動になる。
		var horizontalAccum byte

		for x := range width {
			horizontalAccum += deltaData[deltaPos]
			deltaPos++

			var upperPixel byte
			if hasPrevRow {
				upperPixel = channel[prevRowOffset+x]
			}

			channel[rowOffset+x] = horizontalAccum + upperPixel
		}
	}
}

// createImageFromChannels はチャンネルデータ（BGRA順）からimage.Imageを
// 作成する。
//
// why not: colors==4(RGBA)ではimage.NRGBAを、colors==3(RGB)ではimage.RGBAを
// 使い分ける。両者ともR/G/B/Pix配置は同一（差はAの意味論のみ）だが、
// image.Image.ColorModel()の型はconverter.imageHasAlpha（呼び出し側
// internal/converter/image.go）がPILのRGBA modeに相当する「全画素不透明でも
// アルファ有り扱い」をNRGBAModelのみで判定する根拠にしている。RGB(3チャンネル)
// 由来の画像はA=255固定で意味論上アルファを持たないため、NRGBAModelにすると
// 常にアルファ有りと誤判定されWebP出力時に不要なロスレス強制が起きる。
// image.RGBAはA=255時は非プリマルチプライ相当と数値上一致するため、この
// 使い分けはピクセル値の意味を変えない。
func createImageFromChannels(channels [][]byte, width, height, colors int) image.Image {
	rect := image.Rect(0, 0, width, height)
	pixelCount := width * height

	if colors == 4 {
		img := image.NewNRGBA(rect)
		for i := range pixelCount {
			px := i * 4
			img.Pix[px] = channels[2][i]   // R <- チャンネル2(格納順ではBGRAのR位置)
			img.Pix[px+1] = channels[1][i] // G
			img.Pix[px+2] = channels[0][i] // B <- チャンネル0
			img.Pix[px+3] = channels[3][i] // A
		}

		return img
	}

	img := image.NewRGBA(rect)
	for i := range pixelCount {
		px := i * 4
		img.Pix[px] = channels[2][i]   // R <- チャンネル2(格納順ではBGRのR位置)
		img.Pix[px+1] = channels[1][i] // G
		img.Pix[px+2] = channels[0][i] // B <- チャンネル0
		img.Pix[px+3] = 255
	}

	return img
}
