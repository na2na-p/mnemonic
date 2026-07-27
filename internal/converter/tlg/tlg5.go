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

	// ErrTLG5InvalidDimensions はwidth/heightが0以下、または許容上限
	// （TLG5MaxDimension・TLG5MaxPixelCount）を超える場合のエラー。
	//
	// why not: レビュー指摘の回帰防止。width=height=0xFFFFFFFFのような
	// 24バイトの不正な最小ヘッダーに対し、旧実装はmake([]byte, width*height)
	// をチャンネル数分呼び出す前に何の検証も行っていなかった。
	// ConversionManagerのワーカーgoroutineはrecoverを持たないため、この
	// makeslice panicはビルド処理全体を巻き込んでクラッシュさせる
	// （実際にpipeline経由で再現された）。信頼できないゲームアセットを
	// 処理するCLIとして、確保前にサイズを検証し通常のerrorとして拒否する。
	ErrTLG5InvalidDimensions = errors.New("画像サイズが不正です")

	// ErrTLG5InvalidColorDepth はTLG5ヘッダーの色深度バイトが既知のいずれの
	// 表記（3/24=RGB、4/32=RGBA）にも一致しない場合のエラー。
	ErrTLG5InvalidColorDepth = errors.New("色深度が不正です")

	// ErrTLG5InvalidBlockMark はブロックのmarkバイトが既知の値
	// （0=圧縮、1=格納）以外の場合のエラー。
	ErrTLG5InvalidBlockMark = errors.New("不正なブロックマークです")
)

const (
	// TLG5MaxDimension は幅・高さそれぞれの許容上限。実在するゲームアセットの
	// TLG5画像がこれを超える例は無い。上限が無いとwidth*heightの計算や
	// 後続のバイト列確保がオーバーフロー・OOM相当の巨大確保を引き起こす
	// （例: width=height=0xFFFFFFFFの最小ヘッダーでのmakeslice panic）。
	TLG5MaxDimension = 1 << 16 // 65536

	// TLG5MaxPixelCount はwidth*heightの許容上限。RGBA(4ch)ではcolors数分の
	// バイト列（各width*heightバイト）を確保するため、このキャップは概ね
	// 256MB (67,108,864 pixels * 4ch * 1byte) のメモリ確保上限に相当する。
	// 65536×65536のような上限ぎりぎりの正方形1辺だけでは約17GBの確保に
	// 到達しうるため、辺の上限とは別に面積（pixel数）でも制限する。
	TLG5MaxPixelCount = 64 * 1024 * 1024 // 64Mピクセル

	// TLG5ブロックのmarkバイトの意味（krkrz TVPLoadTLG5ローダー準拠）。
	tlg5BlockMarkCompressed = 0 // LZSS圧縮ブロック
	tlg5BlockMarkStored     = 1 // 非圧縮(格納)ブロック
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

	colorDepth := data[offset]
	colors, colorErr := channelCountFromColorDepth(colorDepth)
	if colorErr != nil {
		return TLG5Header{}, colorErr
	}
	offset++

	width := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4
	height := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4
	blockHeight := int(binary.LittleEndian.Uint32(data[offset : offset+4]))

	return TLG5Header{Width: width, Height: height, Colors: colors, BlockHeight: blockHeight}, nil
}

// channelCountFromColorDepth はTLG5ヘッダーの色深度バイトをチャンネル数へ
// 変換する。
//
// why not: krkrz（吉里吉里ネイティブC++実装）のTVPLoadTLG5ローダーは、この
// バイトを「チャンネル数」（3=RGB、4=RGBA）として解釈する。一方、Python
// 参照実装（mnemonic feat/exe-icon-extraction）は「24=RGB、32=RGBA」という
// ビット深度風の値としてのみ解釈しており、colors=4のように実機で生成された
// RGBA TLG5ファイルを誤って3チャンネルとしてデコードしチャンネル境界が
// ズレる(desync)欠陥を引き継いでいた。どちらの表記のTLG5ファイルも実在し
// うる（レビューア差分フィクスチャ生成器はPython流の24/32を書き出す）ため、
// 本実装は両方の表記を受け付けるハイブリッド解釈とし、それ以外の値は
// ErrTLG5InvalidColorDepthとして拒否する。
func channelCountFromColorDepth(colorDepth byte) (int, error) {
	switch colorDepth {
	case 3, 24:
		return 3, nil
	case 4, 32:
		return 4, nil
	default:
		return 0, fmt.Errorf("%w: %d", ErrTLG5InvalidColorDepth, colorDepth)
	}
}

// validateDimensions はheaderのWidth/HeightがTLG5MaxDimension・
// TLG5MaxPixelCountの範囲内にあることを検証する。呼び出し元は、この検証を
// width/height由来のバイト列確保（channels/image.Image作成）より前に必ず
// 行うこと。
func validateDimensions(header TLG5Header) error {
	if header.Width <= 0 || header.Height <= 0 {
		return fmt.Errorf("%w: %dx%d", ErrTLG5InvalidDimensions, header.Width, header.Height)
	}

	if header.Width > TLG5MaxDimension || header.Height > TLG5MaxDimension {
		return fmt.Errorf("%w: %dx%d", ErrTLG5InvalidDimensions, header.Width, header.Height)
	}

	// width・heightは共にTLG5MaxDimension(65536)以下であることが上の
	// チェックで保証されるため、この乗算は最大でも65536*65536=4,294,967,296
	// となり64bit intの範囲でオーバーフローしない。
	if header.Width*header.Height > TLG5MaxPixelCount {
		return fmt.Errorf("%w: %dx%d", ErrTLG5InvalidDimensions, header.Width, header.Height)
	}

	return nil
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

	// why: width/height由来のバイト列確保(下のchannels := make(...))より
	// 前に必ず検証する。この順序を破ると不正な巨大寸法でmakesliceが
	// panicする（レビュー指摘の回帰防止）。
	if dimErr := validateDimensions(header); dimErr != nil {
		return nil, dimErr
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

			mark := data[offset]
			offset++

			channelSize := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
			offset += 4

			if offset+channelSize > len(data) {
				return nil, ErrTLG5IncompleteBlockData
			}

			blockData := data[offset : offset+channelSize]
			offset += channelSize

			decompressed, blockErr := decodeBlock(d.lzss, mark, blockData, blockPixelCount)
			if blockErr != nil {
				return nil, blockErr
			}

			applyDeltaDecoding(channels[channelIdx], decompressed, width, blockYStart, blockRows)
		}
	}

	return createImageFromChannels(channels, width, height, colors), nil
}

// decodeBlock はTLG5ブロック内の1チャンネル分のデータをmarkバイトに従って
// 復元する。
//
// markの意味はkrkrz（吉里吉里ネイティブC++実装）のTVPLoadTLG5ローダーに
// 準拠する: 0=LZSS圧縮ブロック、1=非圧縮の格納ブロック（エンコーダはLZSS
// 圧縮が展開後より大きくなる場合に格納ブロックを使う）。
//
// why not(格納ブロックとスライド辞書の関係、未検証の前提): krkrzの実装では
// 格納ブロックのデータもスライド辞書へ書き戻され、後続の圧縮ブロックの
// バックリファレンスが辞書状態を前提にしている可能性がある。しかし本実装
// （移植元のPython参照実装feat/exe-icon-extractionを含む）はLZSSDecoder.
// Decodeをチャンネル×ブロックの圧縮データチャンクごとに独立して呼び出し、
// スライド辞書を毎回ゼロ初期化する設計になっている（辞書状態はチャンク
// 境界をまたいで一切引き継がれない。圧縮ブロック同士の間でも共有しない）。
// この設計のもとでは、格納ブロックについてのみ辞書へ書き戻す処理を追加
// しても後続チャンクの解凍結果には数学的に影響しない（次のLZSSDecoder.
// Decode呼び出しは常にゼロ初期化された辞書から始まるため）。したがって
// 格納ブロックは単純にバイト列をそのままピクセル値として採用するのみで
// よい、という保守的な解釈を採用する。
//
// 実機のkrkrz実装が本当にチャンク単位で辞書をリセットしているのか、
// この一致がTLG5フォーマットの「正しい」仕様なのか、それとも参照実装の
// 簡略化がたまたま無害だっただけなのかは、参照実装の入手範囲では検証
// できていない。この不確実性を明記した上で、現状の挙動をテストで
// ピン留めする。
func decodeBlock(lzss *LZSSDecoder, mark byte, blockData []byte, pixelCount int) ([]byte, error) {
	switch mark {
	case tlg5BlockMarkCompressed:
		decompressed, decErr := lzss.Decode(blockData, pixelCount)
		if decErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrTLG5IncompleteBlockData, decErr)
		}

		return decompressed, nil
	case tlg5BlockMarkStored:
		if len(blockData) != pixelCount {
			return nil, fmt.Errorf("%w: 格納ブロックのサイズ不一致 got=%d want=%d", ErrTLG5IncompleteBlockData, len(blockData), pixelCount)
		}

		return blockData, nil
	default:
		return nil, fmt.Errorf("%w: %d", ErrTLG5InvalidBlockMark, mark)
	}
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
