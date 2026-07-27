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
//
// why not: LZSS解凍は外部I/Oを持たない純粋なアルゴリズムであり差し替え対象
// でもないため、lang-go.mdの「外部依存はインターフェース化」は適用対象外。
// 具象型として実装する（image.goのTLGImageDecoder等、既存コードベースの
// アルゴリズム実装パターンに合わせる）。
type LZSSDecoder struct{}

// NewLZSSDecoder はLZSSDecoderを初期化する。
func NewLZSSDecoder() *LZSSDecoder {
	return &LZSSDecoder{}
}

// Decode はdataをLZSS解凍し、outputSizeバイトの解凍結果を返す。
//
// フラグバイト1バイトにつき8ビット分の制御を読み、ビット1=マッチ
// （2バイトのバックリファレンス。長さが18になる場合は追加の1バイトを読む）、
// ビット0=リテラル1バイトとして展開する。展開結果は同時に4096バイトの
// スライド辞書（0初期化）へ書き戻され、以降のマッチ参照元になる。
func (d *LZSSDecoder) Decode(data []byte, outputSize int) ([]byte, error) {
	if outputSize == 0 {
		return []byte{}, nil
	}

	output := make([]byte, outputSize)
	slide := make([]byte, WindowSize)
	slidePos := 0

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

	return output, nil
}
