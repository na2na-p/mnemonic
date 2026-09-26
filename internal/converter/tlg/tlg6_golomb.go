package tlg

import (
	"fmt"
	"math/bits"
)

const (
	// tlg6GolombColumns はkrkrzのTVP_TLG6_GOLOMB_N_COUNT。直近4値ごとに
	// 誤差の絶対値の和を半減させる周期でもある。
	tlg6GolombColumns = 4

	// tlg6GolombTableRows はビット長表の行数（TVP_TLG6_GOLOMB_N_COUNT*2*128）。
	tlg6GolombTableRows = tlg6GolombColumns * 2 * 128

	// tlg6MaxGammaExponent はガンマ符号の先頭の0の個数として受け付ける上限。
	// 1ブロック行の画素数は最大でTLG6MaxDimension*8=2^19なので、連続数の
	// 符号にこれを超える指数は現れない。上限が無いと1<<指数が桁あふれする。
	tlg6MaxGammaExponent = 24

	// tlg6GolombEscapeOffset は、ゴロム符号の0の並びが読み取り窓の4バイトに
	// 収まらないときに、商を8ビットで格納するバイトの現在バイトからの位置。
	tlg6GolombEscapeOffset = 4
)

// tlg6GolombCompressed はkrkrz TVPTLG6GolombCompressed（W.Deeによる調整値）。
// 列nごとに、ビット長k=0..8をそれぞれ何行並べるかを表す。
var tlg6GolombCompressed = [tlg6GolombColumns][9]int{
	{3, 7, 15, 27, 63, 108, 223, 448, 130},
	{3, 5, 13, 24, 51, 95, 192, 384, 257},
	{2, 5, 12, 21, 39, 86, 155, 320, 384},
	{2, 3, 9, 18, 33, 61, 129, 258, 511},
}

// tlg6GolombBitLengthTable は[誤差の絶対値の和][残り個数]からゴロム・ライス
// 符号の予測ビット長kを引く表（krkrz TVPTLG6GolombBitLengthTable）。
var tlg6GolombBitLengthTable = buildTLG6GolombBitLengthTable()

// buildTLG6GolombBitLengthTable はkrkrz TVPTLG6InitGolombTableと同じ規則で
// 圧縮表を展開する。
func buildTLG6GolombBitLengthTable() [tlg6GolombTableRows][tlg6GolombColumns]uint8 {
	var table [tlg6GolombTableRows][tlg6GolombColumns]uint8

	for n, counts := range tlg6GolombCompressed {
		row := 0
		for k, count := range counts {
			for range count {
				table[row][n] = uint8(k)
				row++
			}
		}
	}

	return table
}

// tlg6BitReader はTLG6のエントロピー符号化データをLSB側から読むビット
// リーダー。
type tlg6BitReader struct {
	data []byte
	pos  int
}

func (r *tlg6BitReader) remainingBits() int {
	return len(r.data)*8 - r.pos
}

// window はkrkrzのTVP_TLG6_FETCH_32BITS(bit_pool) >> bit_posに相当する値を
// 返す。現在のバイトから4バイトを読み、データ末尾より先は0として扱う。
//
// why not(末尾の先を0にする): krkrzは確保済みバッファの残骸を読むが、
// 正しいストリームではその値が結果に影響しない。0で埋めても消費するビットは
// 必ずデータ内にあることを各読み出しで検査するため、壊れた入力は範囲外を
// 読まずにエラーになる。
func (r *tlg6BitReader) window() uint32 {
	start := r.pos >> 3

	var w uint32
	for i := range 4 {
		if start+i < len(r.data) {
			w |= uint32(r.data[start+i]) << (8 * i)
		}
	}

	return w >> (r.pos & 7)
}

// readZerosUntilOne は次の1ビットまでの0の個数を返し、0の並びと終端の1を
// 消費する。
//
// why not(krkrzの12ビット先頭ゼロ表を移植しない): TVPTLG6LeadingZeroTableは
// Cで最下位の1を速く探すための表であり、math/bits.TrailingZeros32で同じ
// 位置が得られる。
func (r *tlg6BitReader) readZerosUntilOne() (int, error) {
	zeros := 0

	for r.remainingBits() > 0 {
		w := r.window()
		if w != 0 {
			n := bits.TrailingZeros32(w)
			r.pos += n + 1

			return zeros + n, nil
		}

		available := 32 - (r.pos & 7)
		zeros += available
		r.pos += available
	}

	return 0, fmt.Errorf("%w: 符号の終端ビットがありません", ErrTLG6CorruptData)
}

// readBits は次のnビットを値として読む（n<=25）。
func (r *tlg6BitReader) readBits(n int) (int, error) {
	if n == 0 {
		return 0, nil
	}

	if n > r.remainingBits() {
		return 0, fmt.Errorf("%w: ビット列が途中で終わっています", ErrTLG6CorruptData)
	}

	value := int(r.window() & (1<<n - 1))
	r.pos += n

	return value, nil
}

// readGamma はTLG6のガンマ符号（n個の0、1、nビットの下位値）で表された
// 1以上の連続数を読む。
func (r *tlg6BitReader) readGamma() (int, error) {
	exponent, err := r.readZerosUntilOne()
	if err != nil {
		return 0, err
	}

	if exponent > tlg6MaxGammaExponent {
		return 0, fmt.Errorf("%w: 連続数の符号が長すぎます", ErrTLG6CorruptData)
	}

	low, err := r.readBits(exponent)
	if err != nil {
		return 0, err
	}

	return 1<<exponent + low, nil
}

// readGolombCode はビット長kのゴロム・ライス符号を読み、符号化前の非負整数
// （エンコーダのm）を返す。
//
// why not(商を常に0の個数として数えない): 商を表す0の並びと終端の1が現在の
// バイトから4バイトの範囲に収まらない場合、エンコーダ（SaveTLG6の
// CompressValuesGolomb）はその範囲を0で埋めたところで打ち切り、終端の1を
// 書かずに続く5バイト目へ商を8ビットで格納する。この切り替えは読み取り窓
// （現在バイトから4バイト）がすべて0かどうかで判定しなければならず、0を
// 数え続けると格納された商を符号の一部として読み違える。
func (r *tlg6BitReader) readGolombCode(k int) (int, error) {
	var quotient int

	if w := r.window(); w != 0 {
		quotient = bits.TrailingZeros32(w)
		r.pos += quotient + 1
	} else {
		escape := r.pos>>3 + tlg6GolombEscapeOffset
		if escape >= len(r.data) {
			return 0, fmt.Errorf("%w: ゴロム符号の商がデータ末尾を越えています", ErrTLG6CorruptData)
		}

		quotient = int(r.data[escape])
		r.pos = (escape + 1) * 8
	}

	remainder, err := r.readBits(k)
	if err != nil {
		return 0, err
	}

	return quotient<<k + remainder, nil
}

// decodeTLG6GolombValues はkrkrz TVPTLG6DecodeGolombValuesの移植。srcを
// ゼロと非ゼロの連続数（ガンマ符号）と非ゼロ値（適応型ゴロム・ライス符号）の
// 並びとして読み、1画素4バイトのpixelsの各画素のchannelバイト目へ残差を書く。
//
// why not(krkrzに無い検査を足す): 参照実装は連続数が残り画素数を超えるか、
// 誤差の和がビット長表の行数を超えると範囲外へ書き込む・読む。正しい
// ストリームではどちらも起きない（残差は-128..127で和は1024未満に収まる）
// ため、検査はエラーを返すだけで正しい入力の結果を変えない。
func decodeTLG6GolombValues(pixels []byte, channel int, src []byte) error {
	if len(src) == 0 {
		return fmt.Errorf("%w: エントロピー符号化データが空です", ErrTLG6CorruptData)
	}

	r := tlg6BitReader{data: src, pos: 1}
	nonZero := src[0]&1 != 0
	pixelCount := len(pixels) / 4
	n := tlg6GolombColumns - 1
	errorSum := 0

	for i := 0; i < pixelCount; {
		count, err := r.readGamma()
		if err != nil {
			return err
		}

		if count > pixelCount-i {
			return fmt.Errorf("%w: 連続数%dが残り画素数%dを超えています", ErrTLG6CorruptData, count, pixelCount-i)
		}

		for range count {
			var residual byte

			if nonZero {
				if errorSum >= tlg6GolombTableRows {
					return fmt.Errorf("%w: 誤差の和%dがビット長表の範囲外です", ErrTLG6CorruptData, errorSum)
				}

				code, codeErr := r.readGolombCode(int(tlg6GolombBitLengthTable[errorSum][n]))
				if codeErr != nil {
					return codeErr
				}

				magnitude := code >> 1
				errorSum += magnitude
				residual = golombResidual(code, magnitude)

				n--
				if n < 0 {
					errorSum >>= 1
					n = tlg6GolombColumns - 1
				}
			}

			pixels[i*4+channel] = residual
			i++
		}

		nonZero = !nonZero
	}

	return nil
}

// golombResidual はエンコーダの写像m = (e>0 ? 2e : -2e-1) - 1を逆にたどり、
// 残差eを1バイト（2の補数）で返す。mが奇数なら正、偶数なら負。
func golombResidual(code, magnitude int) byte {
	value := magnitude + 1
	if code&1 == 0 {
		value = -value
	}

	return byte(value) //nolint:gosec // 残差は1バイトの2の補数として格納する仕様であり、切り詰めが意図どおり
}
