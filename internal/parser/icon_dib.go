package parser

import (
	"encoding/binary"
	"errors"
	"image"
	"image/color"
)

// BITMAPINFOHEADERの構造サイズ・オフセット(Windows GDI仕様準拠)。
const (
	dibHeaderMinSize  = 40 // BITMAPINFOHEADERの固定長部分
	dibCompressionRGB = 0  // biCompression: 0=BI_RGB(無圧縮)

	dibPaletteEntrySize = 4 // RGBQUAD: B, G, R, Reserved

	// maxICODimension はDIBのwidth/heightに許容する上限。ICO/CURフォーマット
	// 仕様上、1フレームの実サイズはbWidth/bHeight(1バイト、0は256を表す)で
	// 表現できる256x256が事実上の上限だが、GRPICONDIRENTRYのbWidth/bHeightと
	// 実際のDIB(BITMAPINFOHEADER.biWidth/biHeight)が食い違う壊れた/悪意ある
	// 入力を弾くため、仕様上限より余裕を持たせた1024を採用する。この上限が
	// 無いと、biWidth/biHeightに攻撃的な値(例:
	// 2147483646、int32の最大値付近)を書き込んだ壊れたEXEに対し
	// image.NewNRGBAがmakeslice panicを起こすか、実データサイズに見合わない
	// 数GB単位のメモリ確保を試みる(レビューで実証済み)。1024×1024×4byte
	// (32bpp)は4MiB強で、通常のビルド処理において無視できるサイズ。
	maxICODimension = 1024
)

// ErrIconUnsupportedDIB はDIBが本パッケージの対応範囲外(圧縮あり・
// 未対応ビット深度等)の場合のエラー。
var ErrIconUnsupportedDIB = errors.New("対応していないDIB形式です")

// ErrIconInvalidDIB はDIBのバイト列がヘッダーの示すサイズに対して
// 不足している等、構造として不正な場合のエラー。
var ErrIconInvalidDIB = errors.New("DIBの形式が不正です")

// decodeDIB はRT_ICON内のBITMAPINFOHEADERベースDIB(ICO/CURフォーマットの
// XORビットマップ+ANDマスク結合形式)をimage.Imageへデコードする。
//
// why not(golang.org/x/image/bmpを使わない理由): x/image/bmpはBITMAPFILE
// HEADER付きの独立したBMPファイルを前提とし、標準40バイトBITMAPINFOHEADER
// では32bppでも常にalpha=0xFF固定にする(ICOのXOR+ANDマスク構成を認識しない)。
// アイコンのDIBはBITMAPFILEHEADERを持たずbiHeightがXOR+ANDの合計(実際の
// 高さの2倍)になっているため、そのままではx/image/bmp.Decodeに渡せない。
// ヘッダーを補正して渡す変換コストがこの専用パーサーとほぼ同等になるため、
// 本パッケージで直接パースする。
func decodeDIB(data []byte) (image.Image, error) {
	if len(data) < dibHeaderMinSize {
		return nil, ErrIconInvalidDIB
	}

	headerSize := binary.LittleEndian.Uint32(data[0:4])
	if headerSize < dibHeaderMinSize {
		return nil, ErrIconInvalidDIB
	}

	width := int(int32(binary.LittleEndian.Uint32(data[4:8]))) //nolint:gosec // biWidthはBITMAPINFOHEADER仕様上LONG(符号付き32bit)のため、ビットパターンをそのまま符号付きへ再解釈する意図的な変換
	// biHeightはXORビットマップとANDマスクを合計した高さ(実際の高さの2倍)。
	// ICO/CURフォーマット仕様の慣例で、後方互換のため全ビット深度で
	// ANDマスクが付与される。
	rawHeight := int(int32(binary.LittleEndian.Uint32(data[8:12]))) //nolint:gosec // biHeightも同様にLONG(符号付き32bit)の意図的な再解釈
	bitCount := binary.LittleEndian.Uint16(data[14:16])
	compression := binary.LittleEndian.Uint32(data[16:20])
	clrUsed := binary.LittleEndian.Uint32(data[32:36])

	if compression != dibCompressionRGB {
		return nil, ErrIconUnsupportedDIB
	}
	if width <= 0 || rawHeight <= 0 || rawHeight%2 != 0 {
		return nil, ErrIconInvalidDIB
	}

	height := rawHeight / 2

	// image.NewNRGBAへwidth/heightを渡す前に上限を検査する。ここで弾かない
	// と、後続のパレット/ストライド計算がlen(data)に対する境界検査を通る
	// 前の時点でimage.NewNRGBAのallocateがバイト長に依存せず実行されるため、
	// makeslice panicやOOMを引き起こしうる(maxICODimensionのコメント参照)。
	if width > maxICODimension || height > maxICODimension {
		return nil, ErrIconInvalidDIB
	}

	palette, offset, err := readDIBPalette(data, int(headerSize), bitCount, clrUsed)
	if err != nil {
		return nil, err
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, height))

	offset, err = decodeDIBColorData(img, data, offset, width, height, bitCount, palette)
	if err != nil {
		return nil, err
	}

	if bitCount != 32 {
		// 32bpp以外はXORビットマップにアルファチャンネルを持たないため、
		// 後続のANDマスク(1bpp、ビットが立っている画素を透明とする)で
		// アルファを決定する。32bppはXORのアルファチャンネルをそのまま使う
		// (Windows Icon仕様上32bppはXOR側に既にアルファを持つため、AND
		// マスクは後方互換用の冗長データに過ぎない。Pillow/icoextractが
		// 依存するWindows ICOデコーダも32bppではANDマスクを無視して
		// XORのアルファを採用しており、本実装はそれと同じ挙動にする)。
		applyDIBAndMask(img, data, offset, width, height)
	}

	return img, nil
}

// readDIBPalette はBITMAPINFOHEADER直後のカラーテーブル(1/4/8bppのみ)を
// 読み取り、パレットと読み取り後のオフセットを返す。24/32bppはパレットを
// 持たない。
//
// why not(numColorsをbiClrUsedの値そのまま使わない理由): biClrUsedは
// attacker-controlledなuint32(最大約42億)であり、そのままnumColors*
// dibPaletteEntrySizeを計算するとint幅(32bit環境)を超えうるうえ、
// make([]color.NRGBA, numColors)に巨大な値を渡すOOM/panicの入口になる。
// BITMAPINFOHEADER仕様上biClrUsedが2^biBitCountを超える値を取ることは
// 本来無い(golang.org/x/image/bmpのdecodeConfigも同じ理由で同じ上限
// 検査をしている)ため、超過を「不正なDIB」として上限側で拒否すれば
// 以降の乗算がオーバーフローする余地自体が無くなる。
func readDIBPalette(data []byte, headerSize int, bitCount uint16, clrUsed uint32) ([]color.NRGBA, int, error) {
	switch bitCount {
	case 1, 4, 8:
	case 24, 32:
		return nil, headerSize, nil
	default:
		return nil, 0, ErrIconUnsupportedDIB
	}

	maxColors := 1 << bitCount

	numColors := int(clrUsed)
	switch {
	case numColors == 0:
		numColors = maxColors
	case numColors > maxColors:
		return nil, 0, ErrIconInvalidDIB
	}

	paletteBytes, ok := safeMulInt(numColors, dibPaletteEntrySize)
	if !ok || headerSize+paletteBytes > len(data) {
		return nil, 0, ErrIconInvalidDIB
	}

	palette := make([]color.NRGBA, numColors)
	for i := range numColors {
		e := data[headerSize+i*dibPaletteEntrySize : headerSize+(i+1)*dibPaletteEntrySize]
		palette[i] = color.NRGBA{R: e[2], G: e[1], B: e[0], A: 255}
	}

	return palette, headerSize + paletteBytes, nil
}

// safeMulInt はa*bをオーバーフロー検査付きで計算する。DIBの行ストライド/
// パレットサイズ計算はwidth・height・biClrUsedといった攻撃者制御可能な値に
// 由来するため、Goのint幅(32bit環境では特に)を超える積になっていないかを
// 都度確認する。a・bは常に非負の値としてのみ呼ばれる想定。
func safeMulInt(a, b int) (int, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}

	p := a * b
	if p < 0 || p/a != b {
		return 0, false
	}

	return p, true
}

// decodeDIBColorData はXORビットマップ(色情報)をimgへ書き込み、読み取り後の
// オフセットを返す。アルファは全画素255(不透明)で初期化し、32bpp以外は
// 呼び出し元がapplyDIBAndMaskで上書きする。
func decodeDIBColorData(img *image.NRGBA, data []byte, offset, width, height int, bitCount uint16, palette []color.NRGBA) (int, error) {
	switch bitCount {
	case 1, 4, 8:
		return decodeDIBPaletted(img, data, offset, width, height, int(bitCount), palette)
	case 24:
		return decodeDIB24(img, data, offset, width, height)
	case 32:
		return decodeDIB32(img, data, offset, width, height)
	default:
		return 0, ErrIconUnsupportedDIB
	}
}

// dibRowStride はbitsPerPixelの画素データ1行を4バイト境界にパディングした
// 際のバイト数を返す(BMP/DIB共通の行アライメント規則)。
func dibRowStride(width, bitsPerPixel int) int {
	return ((width*bitsPerPixel + 31) / 32) * 4
}

func decodeDIBPaletted(img *image.NRGBA, data []byte, offset, width, height, bitCount int, palette []color.NRGBA) (int, error) {
	stride := dibRowStride(width, bitCount)

	size, ok := safeMulInt(stride, height)
	if !ok || offset+size > len(data) {
		return 0, ErrIconInvalidDIB
	}

	pixels := data[offset : offset+size]
	mask := byte(1<<bitCount) - 1

	for y := range height {
		row := pixels[y*stride : (y+1)*stride]
		destY := height - 1 - y // DIBは下から上へ格納される

		byteIndex, bitIndex := 0, 8
		for x := range width {
			bitIndex -= bitCount
			idx := (row[byteIndex] >> bitIndex) & mask
			if int(idx) >= len(palette) {
				return 0, ErrIconInvalidDIB
			}
			img.SetNRGBA(x, destY, palette[idx])

			if bitIndex == 0 {
				byteIndex++
				bitIndex = 8
			}
		}
	}

	return offset + size, nil
}

func decodeDIB24(img *image.NRGBA, data []byte, offset, width, height int) (int, error) {
	stride := dibRowStride(width, 24)

	size, ok := safeMulInt(stride, height)
	if !ok || offset+size > len(data) {
		return 0, ErrIconInvalidDIB
	}

	pixels := data[offset : offset+size]

	for y := range height {
		row := pixels[y*stride : (y+1)*stride]
		destY := height - 1 - y

		for x := range width {
			b, g, r := row[x*3], row[x*3+1], row[x*3+2]
			img.SetNRGBA(x, destY, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}

	return offset + size, nil
}

func decodeDIB32(img *image.NRGBA, data []byte, offset, width, height int) (int, error) {
	stride, ok := safeMulInt(width, 4)
	if !ok {
		return 0, ErrIconInvalidDIB
	}

	size, ok := safeMulInt(stride, height)
	if !ok || offset+size > len(data) {
		return 0, ErrIconInvalidDIB
	}

	pixels := data[offset : offset+size]

	for y := range height {
		row := pixels[y*stride : (y+1)*stride]
		destY := height - 1 - y

		for x := range width {
			b, g, r, a := row[x*4], row[x*4+1], row[x*4+2], row[x*4+3]
			img.SetNRGBA(x, destY, color.NRGBA{R: r, G: g, B: b, A: a})
		}
	}

	return offset + size, nil
}

// applyDIBAndMask はANDマスク(1bpp、ビット1=透明)を読み取りimgのアルファへ
// 反映する。マスクのバイト数が不足している場合は何もしない(不透明のまま
// 扱う)。
//
// why not: ANDマスクはICO/CURフォーマット上は常に存在する前提だが、本パッケージが
// 対象とするのはビルド時にゲーム同梱EXEから機械的に抽出したアイコンであり、
// 全ての生成元ツールがANDマスクを仕様通り書き出すとは限らない。データ不足を
// エラーにすると本来デコード可能なXOR色情報まで捨ててしまうため、マスク欠落は
// 「不透明として扱う」フォールバックにとどめる。
func applyDIBAndMask(img *image.NRGBA, data []byte, offset, width, height int) {
	stride := ((width+7)/8 + 3) &^ 3

	size, ok := safeMulInt(stride, height)
	if !ok || offset+size > len(data) {
		return
	}

	mask := data[offset : offset+size]

	for y := range height {
		row := mask[y*stride : (y+1)*stride]
		destY := height - 1 - y

		for x := range width {
			byteIndex := x / 8
			bit := 7 - (x % 8)
			transparent := (row[byteIndex]>>bit)&1 == 1
			if transparent {
				px := img.NRGBAAt(x, destY)
				px.A = 0
				img.SetNRGBA(x, destY, px)
			}
		}
	}
}
