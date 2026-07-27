// Package tlg は吉里吉里2 TLG画像フォーマット（TLG5/TLG6）のデコード機能を提供する。
package tlg

import (
	"errors"
	"fmt"
)

// LZSS解凍で使用する定数群。
//
// TLG5のブロック内圧縮データは吉里吉里方式のLZSSでエンコードされており、
// 4096バイトのスライド辞書・最大マッチ長18バイト・最小マッチ長3バイトという
// 固定パラメータを持つ（フォーマット仕様であり構成不可）。
const (
	WindowSize     = 4096
	MatchMaxLength = 18
	MatchMinLength = 3
)

// ErrLZSSIncompleteData はLZSS圧縮データが指定されたoutputSizeを満たす前に
// 尽きた場合のエラー（フラグバイト・マッチ情報・リテラルバイト・追加長バイトの
// いずれかが不足）。
var ErrLZSSIncompleteData = errors.New("不完全な圧縮データです")

// LZSSDecoder はTLG5で使用される吉里吉里方式LZSS圧縮データを解凍する。
// スライド辞書（4096バイト）とリングポインタ（slidePos）を状態として保持し、
// 同一インスタンスへの複数回のDecode呼び出しをまたいで辞書を持続させる。
//
// why not(辞書をDecodeローカルではなくフィールドで持つ理由): krkrz（吉里吉里
// ネイティブC++実装）のTVPLoadTLG5ローダーは、スライド辞書text[4096]とリング
// ポインタrを画像1枚のデコード（全ブロック×全チャンネル）を通じて1つだけ確保し、
// TVPTLG5DecompressSlideの呼び出しごとにrを引き回して辞書状態を持続させる。実機の
// RGBA .tlgアセットはブロック境界をまたぐバックリファレンス（前のチャンクで展開した
// バイトを後のチャンクが参照する）を含むため、辞書をチャンク（ブロック×チャンネル）
// ごとにゼロ初期化するとバックリファレンスが解決できずデコード結果がノイズになる。
// 移植元のPython参照実装はDecodeローカルで毎回スライド辞書を確保しており、この
// krkrz準拠の持続性を再現できていなかった（実アセットでノイズになることを実証済み）。
// 本Go実装はここで意図的にPython参照実装から乖離し、辞書状態をフィールドとして
// 保持する。1枚の画像に対しては呼び出し側（tlg5.go）が新しいLZSSDecoderを1つだけ
// 生成して全チャンクへ通すことで、画像間の辞書汚染を防ぐ。
//
// why not: LZSS解凍は外部I/Oを持たない純粋なアルゴリズムであり差し替え対象
// でもないため、lang-go.mdの「外部依存はインターフェース化」は適用対象外。
// 具象型として実装する（image.goのTLGImageDecoder等、既存コードベースの
// アルゴリズム実装パターンに合わせる）。
type LZSSDecoder struct {
	slide    [WindowSize]byte
	slidePos int
}

// NewLZSSDecoder はLZSSDecoderを初期化する。スライド辞書はゼロ初期化される
// （krkrz TVPLoadTLG5のmemset(text, 0, 4096)・r=0に相当）。
func NewLZSSDecoder() *LZSSDecoder {
	return &LZSSDecoder{}
}

// Decode はdataをLZSS解凍し、outputSizeバイトの解凍結果を返す。
//
// フラグバイト1バイトにつき8ビット分の制御を読み、ビット1=マッチ
// （2バイトのバックリファレンス。長さが18になる場合は追加の1バイトを読む）、
// ビット0=リテラル1バイトとして展開する。展開結果は同時にレシーバの4096バイトの
// スライド辞書へ書き戻され、以降のマッチ参照元になる。
//
// 辞書とリングポインタはレシーバに保持されるため、同一インスタンスへの連続した
// Decode呼び出しでは前回の呼び出しが残した辞書状態が引き継がれる（krkrzのrの
// スレッディングに相当）。マッチ位置mposは辞書全体（前チャンク由来のバイトを含む）を
// 参照できる。
func (d *LZSSDecoder) Decode(data []byte, outputSize int) ([]byte, error) {
	if outputSize == 0 {
		return []byte{}, nil
	}

	output := make([]byte, outputSize)
	slide := &d.slide
	slidePos := d.slidePos

	outputPos := 0
	inputPos := 0
	dataLen := len(data)

	for outputPos < outputSize {
		if inputPos >= dataLen {
			return nil, fmt.Errorf("%w: フラグバイトが不足しています", ErrLZSSIncompleteData)
		}

		flags := data[inputPos]
		inputPos++

		for bit := range 8 {
			if outputPos >= outputSize {
				break
			}

			if flags&(1<<uint(bit)) != 0 {
				// TLG5: ビット1 = マッチ（バックリファレンス）
				if inputPos+2 > dataLen {
					return nil, fmt.Errorf("%w: マッチ情報が不足しています", ErrLZSSIncompleteData)
				}

				lowByte := data[inputPos]
				highByte := data[inputPos+1]
				inputPos += 2

				mpos := int(lowByte) | (int(highByte&0x0F) << 8)
				mlen := int((highByte>>4)&0x0F) + MatchMinLength

				if mlen == MatchMaxLength {
					if inputPos >= dataLen {
						return nil, fmt.Errorf("%w: 追加長バイトが不足しています", ErrLZSSIncompleteData)
					}

					mlen += int(data[inputPos])
					inputPos++
				}

				for range mlen {
					if outputPos >= outputSize {
						break
					}

					b := slide[mpos]
					output[outputPos] = b
					slide[slidePos] = b
					outputPos++
					mpos = (mpos + 1) & (WindowSize - 1)
					slidePos = (slidePos + 1) & (WindowSize - 1)
				}
			} else {
				// TLG5: ビット0 = リテラルバイト
				if inputPos >= dataLen {
					return nil, fmt.Errorf("%w: リテラルバイトが不足しています", ErrLZSSIncompleteData)
				}

				b := data[inputPos]
				inputPos++
				output[outputPos] = b
				slide[slidePos] = b
				outputPos++
				slidePos = (slidePos + 1) & (WindowSize - 1)
			}
		}
	}

	// why not(エラー時にslidePosを書き戻さない): 解凍が途中でエラー終了した場合、
	// 呼び出し側（tlg5.go）は画像デコード全体を中断しこのLZSSDecoderを破棄する
	// ため、中途半端な辞書状態を書き戻す意味が無い。成功時のみ次チャンクへ引き継ぐ。
	d.slidePos = slidePos

	return output, nil
}
