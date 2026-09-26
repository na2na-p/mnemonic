package tlg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
)

// TLG6Magic はTLG6形式のマジックバイト。
var TLG6Magic = []byte("TLG6.0\x00raw\x1a")

// tlg6HeaderSize はヘッダーのサイズ: マジック(11) + 色数・データフラグ・
// カラータイプ・外部ゴロムテーブル(各1) + width・height・max_bit_length(各4)。
const tlg6HeaderSize = 11 + 4 + 4*3

const (
	// tlg6BlockSize はフィルタ種別と画素の並び替えの単位となるブロックの
	// 幅と高さ（krkrz TVP_TLG6_W_BLOCK_SIZE / TVP_TLG6_H_BLOCK_SIZE）。
	tlg6BlockSize = 8

	// チャンネルごとのビット長の上位2ビットはエントロピー符号化方式を表す。
	// 0（ゴロム・ライス符号）以外はkrkrzも未実装。
	tlg6EntropyMethodShift  = 30
	tlg6BitLengthMask       = 1<<tlg6EntropyMethodShift - 1
	tlg6EntropyMethodGolomb = 0
)

const (
	// TLG6MaxDimension は幅・高さそれぞれの許容上限。
	TLG6MaxDimension = 1 << 16

	// TLG6MaxPixelCount はwidth*heightの許容上限。出力のimage.NRGBAは1画素
	// 4バイトなので、復号先の確保は最大256MiBに収まる。
	TLG6MaxPixelCount = 64 * 1024 * 1024
)

// TLG6デコード時のセンチネルエラー群。
var (
	ErrTLG6InvalidMagic = errors.New("TLG6形式ではありません")

	// ErrTLG6DataTooShort はヘッダー・フィルタ種別・チャンネルデータのいずれかを
	// 読む途中でデータが尽きた場合のエラー。
	ErrTLG6DataTooShort = errors.New("データが短すぎます")

	// ErrTLG6UnsupportedColorCount は色数が1・3・4以外の場合のエラー。
	ErrTLG6UnsupportedColorCount = errors.New("未対応の色数です")

	// ErrTLG6UnsupportedHeaderField はデータフラグ・カラータイプ・外部ゴロム
	// テーブルのいずれかが0以外の場合のエラー。krkrzもこれらを拒否する。
	ErrTLG6UnsupportedHeaderField = errors.New("未対応のヘッダー値です")

	// ErrTLG6InvalidDimensions はwidth/heightが0以下、またはTLG6MaxDimension・
	// TLG6MaxPixelCountを超える場合のエラー。
	ErrTLG6InvalidDimensions = errors.New("画像サイズが不正です")

	// ErrTLG6UnsupportedEntropyMethod はゴロム・ライス符号以外のエントロピー
	// 符号化方式が指定された場合のエラー。
	ErrTLG6UnsupportedEntropyMethod = errors.New("未対応のエントロピー符号化方式です")

	// ErrTLG6CorruptData はフィルタ種別やエントロピー符号化データの内容が
	// 正しいTLG6ストリームとして解釈できない場合のエラー。
	ErrTLG6CorruptData = errors.New("TLG6データが壊れています")
)

// TLG6Header はTLG6画像ファイルのヘッダーから読み取った情報を保持する不変値。
type TLG6Header struct {
	Width  int
	Height int
	Colors int // 1=グレースケール、3=RGB、4=RGBA

	// MaxBitLength はチャンネルごとのエントロピー符号化データの最大ビット長。
	MaxBitLength int

	// XBlockCount・YBlockCount は幅・高さを8ピクセル単位で切り上げた
	// ブロック数（ヘッダーには格納されず寸法から求まる）。
	XBlockCount int
	YBlockCount int
}

// TLG6Decoder はTLG6形式（MED/平均予測・色相関フィルタ・適応型ゴロム・
// ライス符号による可逆圧縮）の画像をデコードする。
//
// 復号処理はKiriKiri Z（https://github.com/krkrz/krkrz、コミット
// fd5c4baa6a2ef5978db1bd043634351f48667daf、Copyright (c) W.Dee and
// contributors、BSD系ライセンス）のvisual/LoadTLG.cpp TVPLoadTLG6と、
// visual/tvpgl.cのTVPTLG6DecodeGolombValues・TVPTLG6DecodeGolombValuesForFirst・
// TVPTLG6DecodeLineGeneric・TVPTLG6InitGolombTableを移植したもの。
// 著作権表示と許諾条件はリポジトリ直下のTHIRD_PARTY_NOTICES.mdを参照。
type TLG6Decoder struct{}

// NewTLG6Decoder はTLG6Decoderを初期化する。
func NewTLG6Decoder() *TLG6Decoder {
	return &TLG6Decoder{}
}

// IsValid はdataがTLG6形式のマジックバイトを持つかどうかを判定する。
func (d *TLG6Decoder) IsValid(data []byte) bool {
	return bytes.HasPrefix(data, TLG6Magic)
}

// ParseHeader はTLG6ヘッダーを解析する。寸法の上限は検証しない（Decodeが
// 確保前に検証する）。
//
// why not(色数に24/32を受け付けない): TLG5は24/32というビット深度風の表記を
// 書き出すツールがあるため両方を受け付けるが、TLG6はkrkrzのTVPLoadTLG6が
// 1/3/4以外を拒否し、SaveTLG6も色数そのものを書く。ビット深度表記のTLG6の
// 出力元は確認できていない。
func (d *TLG6Decoder) ParseHeader(data []byte) (TLG6Header, error) {
	if !d.IsValid(data) {
		return TLG6Header{}, ErrTLG6InvalidMagic
	}

	if len(data) < tlg6HeaderSize {
		return TLG6Header{}, ErrTLG6DataTooShort
	}

	fields := data[len(TLG6Magic):tlg6HeaderSize]

	colors := int(fields[0])
	if colors != 1 && colors != 3 && colors != 4 {
		return TLG6Header{}, fmt.Errorf("%w: %d", ErrTLG6UnsupportedColorCount, colors)
	}

	if fields[1] != 0 || fields[2] != 0 || fields[3] != 0 {
		return TLG6Header{}, fmt.Errorf("%w: data_flag=%d color_type=%d external_golomb_table=%d",
			ErrTLG6UnsupportedHeaderField, fields[1], fields[2], fields[3])
	}

	width := int(binary.LittleEndian.Uint32(fields[4:8]))
	height := int(binary.LittleEndian.Uint32(fields[8:12]))

	return TLG6Header{
		Width:        width,
		Height:       height,
		Colors:       colors,
		MaxBitLength: int(binary.LittleEndian.Uint32(fields[12:16])),
		XBlockCount:  (width + tlg6BlockSize - 1) / tlg6BlockSize,
		YBlockCount:  (height + tlg6BlockSize - 1) / tlg6BlockSize,
	}, nil
}

// validateTLG6Dimensions はheaderの寸法が上限内にあることを検証する。
// 寸法由来の確保より前に必ず呼ぶこと。
func validateTLG6Dimensions(header TLG6Header) error {
	if header.Width <= 0 || header.Height <= 0 ||
		header.Width > TLG6MaxDimension || header.Height > TLG6MaxDimension ||
		header.Width*header.Height > TLG6MaxPixelCount {
		return fmt.Errorf("%w: %dx%d", ErrTLG6InvalidDimensions, header.Width, header.Height)
	}

	return nil
}

// Decode はTLG6形式のバイト列をデコードし、*image.NRGBAを返す。
//
// why not(常にimage.NRGBAを返す): 色数4のアルファは非プリマルチプライの値で
// 復元されるため、プリマルチプライ前提のimage.RGBAには入れられない。色数3と1も
// 同じ型にそろえ、アルファを255とする。色数1はkrkrzがB成分に置く輝度を
// R・G・Bへ複製する。
func (d *TLG6Decoder) Decode(data []byte) (image.Image, error) {
	header, err := d.ParseHeader(data)
	if err != nil {
		return nil, err
	}

	if dimErr := validateTLG6Dimensions(header); dimErr != nil {
		return nil, dimErr
	}

	src := &tlg6ByteReader{data: data, pos: tlg6HeaderSize}

	filterTypes, err := readTLG6FilterTypes(src, header.XBlockCount*header.YBlockCount)
	if err != nil {
		return nil, err
	}

	img := image.NewNRGBA(image.Rect(0, 0, header.Width, header.Height))
	lines := newTLG6LineReconstructor(header)
	residuals := make([]byte, header.Width*tlg6BlockSize*4)

	for blockRow := range header.YBlockCount {
		y := blockRow * tlg6BlockSize
		rows := min(tlg6BlockSize, header.Height-y)
		blockResiduals := residuals[:header.Width*rows*4]

		if err := readTLG6BlockRowResiduals(src, header.Colors, blockResiduals); err != nil {
			return nil, fmt.Errorf("ブロック行%d: %w", blockRow, err)
		}

		rowFilterTypes := filterTypes[blockRow*header.XBlockCount : (blockRow+1)*header.XBlockCount]
		lines.reconstructBlockRow(img, y, rows, rowFilterTypes, blockResiduals)
	}

	return img, nil
}

// tlg6ByteReader はTLG6ストリームのバイト単位の読み出し位置を管理する。
type tlg6ByteReader struct {
	data []byte
	pos  int
}

func (r *tlg6ByteReader) next(n int) ([]byte, error) {
	if n < 0 || n > len(r.data)-r.pos {
		return nil, fmt.Errorf("%w: offset=%d 必要=%d 残り=%d", ErrTLG6DataTooShort, r.pos, n, len(r.data)-r.pos)
	}

	chunk := r.data[r.pos : r.pos+n]
	r.pos += n

	return chunk, nil
}

func (r *tlg6ByteReader) uint32() (uint32, error) {
	chunk, err := r.next(4)
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint32(chunk), nil
}

// readTLG6FilterTypes はブロックごとのフィルタ種別（下位1ビット=予測方式、
// 上位=色相関フィルタ番号）をblockCount個読む。
//
// why not(TLG5と同じ辞書の初期状態を使わない): フィルタ種別はTLG5と同じ
// LZSSで圧縮されるが、krkrz TVPLoadTLG6は辞書をゼロではなく
// tlg6FilterTypeDictionaryの並びで初期化し、エンコーダも同じ並びを事前に
// 圧縮してから符号化する。ゼロ初期化の辞書では先頭付近の後方参照を読み違える。
//
// why not(入力を使い切るまで展開しない): krkrzは入力の終わりまで展開し
// 出力長を確かめない。本実装はブロック数だけ展開し、足りなければエラーに
// する。正しいストリームでは両者の結果は同じになる。
func readTLG6FilterTypes(src *tlg6ByteReader, blockCount int) ([]byte, error) {
	size, err := src.uint32()
	if err != nil {
		return nil, fmt.Errorf("フィルタ種別のサイズ: %w", err)
	}

	compressed, err := src.next(int(size))
	if err != nil {
		return nil, fmt.Errorf("フィルタ種別: %w", err)
	}

	lzss := &LZSSDecoder{slide: tlg6FilterTypeDictionary()}

	filterTypes, err := lzss.Decode(compressed, blockCount)
	if err != nil {
		return nil, fmt.Errorf("%w: フィルタ種別: %w", ErrTLG6CorruptData, err)
	}

	// why not(krkrzのように行の途中で打ち切らない): krkrzは未知の色相関
	// フィルタ番号に出会うとその行の残りを書かずに戻り、画像の一部が未初期化の
	// まま残る。本実装は復号前にすべて検査し、壊れたファイルとして拒否する。
	for i, filterType := range filterTypes {
		if int(filterType>>1) >= len(tlg6ChromaFilters) {
			return nil, fmt.Errorf("%w: ブロック%dのフィルタ種別%dが範囲外です", ErrTLG6CorruptData, i, filterType)
		}
	}

	return filterTypes, nil
}

// tlg6FilterTypeDictionary はフィルタ種別用LZSS辞書の初期状態（krkrz
// TVPLoadTLG6のLZSS_text初期化）を返す: i=0..31、j=0..15について、iを4バイト、
// jを4バイト並べることを繰り返した4096バイト。
func tlg6FilterTypeDictionary() [WindowSize]byte {
	var dict [WindowSize]byte

	p := 0
	for i := range byte(32) {
		for j := range byte(16) {
			copy(dict[p:p+4], []byte{i, i, i, i})
			copy(dict[p+4:p+8], []byte{j, j, j, j})
			p += 8
		}
	}

	return dict
}

// readTLG6BlockRowResiduals は1ブロック行分の各色チャンネルのエントロピー
// 符号化データを読み、1画素4バイト（B, G, R, Aの順）のresidualsへ残差を
// 展開する。
//
// why not(ブロック行ごとにresidualsをゼロで埋める): krkrzは色数3・4の先頭
// チャンネルをTVPTLG6DecodeGolombValuesForFirstで展開し、画素の4バイトを
// まとめて書くことで他のバイトを0にする。色数3のA、色数1のG・R・Aは以後
// 書かれないため、あらかじめ0で埋めても同じ結果になる。色数1ではkrkrzは
// G・R・Aを未初期化のまま読むが、エンコーダは色数1に色相関フィルタ0だけを
// 使い、予測はバイトごとに独立なので、復元されるB成分には影響しない。
func readTLG6BlockRowResiduals(src *tlg6ByteReader, colors int, residuals []byte) error {
	clear(residuals)

	for channel := range colors {
		header, err := src.uint32()
		if err != nil {
			return fmt.Errorf("チャンネル%dのビット長: %w", channel, err)
		}

		if method := header >> tlg6EntropyMethodShift; method != tlg6EntropyMethodGolomb {
			return fmt.Errorf("%w: チャンネル%d method=%d", ErrTLG6UnsupportedEntropyMethod, channel, method)
		}

		bitLength := int(header & tlg6BitLengthMask)

		encoded, err := src.next((bitLength + 7) / 8)
		if err != nil {
			return fmt.Errorf("チャンネル%d: %w", channel, err)
		}

		if err := decodeTLG6GolombValues(residuals, channel, encoded); err != nil {
			return fmt.Errorf("チャンネル%d: %w", channel, err)
		}
	}

	return nil
}

// tlg6Pixel は復元中の1画素をkrkrzの32bit画素と同じB, G, R, Aの順で持つ。
type tlg6Pixel [4]byte

// tlg6LineReconstructor は予測と色相関フィルタの逆変換で画素を1行ずつ復元する
// （krkrz TVPTLG6DecodeLineGenericの移植）。直前の行を次のブロック行へ
// 持ち越す。
type tlg6LineReconstructor struct {
	width    int
	colors   int
	initial  tlg6Pixel
	previous []tlg6Pixel
	current  []tlg6Pixel
}

// newTLG6LineReconstructor はy=-1の仮想行と左端の外側を初期画素で埋めた
// 状態で復元器を作る。初期画素は色数3のときだけアルファ0xFF（krkrzの
// initialp・zerolineの0xff000000）。
func newTLG6LineReconstructor(header TLG6Header) *tlg6LineReconstructor {
	var initial tlg6Pixel
	if header.Colors == 3 {
		initial[3] = 0xFF
	}

	previous := make([]tlg6Pixel, header.Width)
	for x := range previous {
		previous[x] = initial
	}

	return &tlg6LineReconstructor{
		width:    header.Width,
		colors:   header.Colors,
		initial:  initial,
		previous: previous,
		current:  make([]tlg6Pixel, header.Width),
	}
}

// reconstructBlockRow はy行目から始まるrows行を復元してimgへ書く。
//
// 残差はブロックごとに連続して並ぶ。ブロック内では、偶数行は左から右へ、
// 奇数行は右から左へ並び、さらに横位置が奇数のブロックは行の順序が上下
// 反転している（SaveTLG6の並び替えの逆）。
//
// why not(krkrzのポインタ操作をそのまま移植しない): krkrzは残差ポインタを
// 行の向きとoddskipで前後に動かし、主ブロックと端数ブロックを別の呼び出しで
// 処理する。ブロック先頭からの位置は「ブロック番号×ブロック内の残差数 +
// 格納行×ブロック幅 + 行内位置」と閉じた式で書け、両者の指す位置は一致する。
func (l *tlg6LineReconstructor) reconstructBlockRow(img *image.NRGBA, y, rows int, filterTypes, residuals []byte) {
	blockResidualCount := rows * tlg6BlockSize

	for row := range rows {
		rightToLeft := (y+row)&1 == 1

		for x := range l.width {
			block := x / tlg6BlockSize
			blockWidth := min(tlg6BlockSize, l.width-block*tlg6BlockSize)

			storedRow := row
			if block&1 == 1 {
				storedRow = rows - 1 - row
			}

			column := x % tlg6BlockSize
			if rightToLeft {
				column = blockWidth - 1 - column
			}

			index := (block*blockResidualCount + storedRow*blockWidth + column) * 4
			filterType := filterTypes[block]
			b, g, r := tlg6ChromaFilters[filterType>>1](residuals[index], residuals[index+1], residuals[index+2])
			residual := tlg6Pixel{b, g, r, residuals[index+3]}

			left, upperLeft := l.initial, l.initial
			if x > 0 {
				left, upperLeft = l.current[x-1], l.previous[x-1]
			}

			predict := tlg6MED
			if filterType&1 == 1 {
				predict = tlg6Average
			}

			var pixel tlg6Pixel
			for i := range pixel {
				pixel[i] = predict(left[i], l.previous[x][i], upperLeft[i]) + residual[i]
			}

			l.current[x] = pixel
		}

		l.writeRow(img, y+row)
		l.previous, l.current = l.current, l.previous
	}
}

// writeRow は復元済みの現在行をNRGBAの画素へ並べ替えてimgのy行目へ書く。
func (l *tlg6LineReconstructor) writeRow(img *image.NRGBA, y int) {
	row := img.Pix[y*img.Stride : y*img.Stride+l.width*4]

	for x, pixel := range l.current {
		out := row[x*4 : x*4+4]

		switch l.colors {
		case 1:
			out[0], out[1], out[2], out[3] = pixel[0], pixel[0], pixel[0], 0xFF
		case 3:
			out[0], out[1], out[2], out[3] = pixel[2], pixel[1], pixel[0], 0xFF
		default:
			out[0], out[1], out[2], out[3] = pixel[2], pixel[1], pixel[0], pixel[3]
		}
	}
}

// tlg6MED はMedian Edge Detectorによる予測値を返す（a=左、b=上、c=左上）。
func tlg6MED(a, b, c byte) byte {
	lo, hi := min(a, b), max(a, b)

	switch {
	case c > hi:
		return lo
	case c < lo:
		return hi
	default:
		return a + b - c
	}
}

// tlg6Average は左と上の平均を切り上げた予測値(a+b+1)>>1を返す。cは使わない。
//
// why not(a+b+1をそのまま計算しない): byteの加算は桁あふれするため、同じ値を
// 桁あふれ無しに求める恒等式(a|b) - (a^b)>>1を使う。
func tlg6Average(a, b, _ byte) byte {
	return (a | b) - (a^b)>>1
}

// tlg6ChromaFilters は色相関フィルタ番号ごとの逆変換（krkrz
// TVP_TLG6_DO_CHROMA_DECODEの16通り）。引数と戻り値はB, G, Rの残差。
var tlg6ChromaFilters = [16]func(b, g, r byte) (byte, byte, byte){
	func(b, g, r byte) (byte, byte, byte) { return b, g, r },
	func(b, g, r byte) (byte, byte, byte) { return b + g, g, r + g },
	func(b, g, r byte) (byte, byte, byte) { return b, g + b, r + b + g },
	func(b, g, r byte) (byte, byte, byte) { return b + r + g, g + r, r },
	func(b, g, r byte) (byte, byte, byte) { return b + r, g + b + r, r + b + r + g },
	func(b, g, r byte) (byte, byte, byte) { return b + r, g + b + r, r },
	func(b, g, r byte) (byte, byte, byte) { return b + g, g, r },
	func(b, g, r byte) (byte, byte, byte) { return b, g + b, r },
	func(b, g, r byte) (byte, byte, byte) { return b, g, r + g },
	func(b, g, r byte) (byte, byte, byte) { return b + g + r + b, g + r + b, r + b },
	func(b, g, r byte) (byte, byte, byte) { return b + r, g + r, r },
	func(b, g, r byte) (byte, byte, byte) { return b, g + b, r + b },
	func(b, g, r byte) (byte, byte, byte) { return b, g + r + b, r + b },
	func(b, g, r byte) (byte, byte, byte) { return b + g, g + r + b + g, r + b + g },
	func(b, g, r byte) (byte, byte, byte) { return b + g + r, g + r, r + b + g + r },
	func(b, g, r byte) (byte, byte, byte) { return b, g + b<<1, r + b<<1 },
}
