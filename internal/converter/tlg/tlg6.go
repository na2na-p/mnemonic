package tlg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
)

// TLG6Magic はTLG6形式のマジックバイト。
var TLG6Magic = []byte("TLG6.0\x00raw\x1a")

// tlg6MinHeaderSize はヘッダーの最小サイズ: マジック(11) + 色深度(1) +
// フラグ(1) + width(4) + height(4) + x_block(4) + y_block(4)。
const tlg6MinHeaderSize = 11 + 1 + 1 + 4 + 4 + 4 + 4

// TLG6デコード時のセンチネルエラー群。
var (
	ErrTLG6InvalidMagic = errors.New("TLG6形式ではありません")
	ErrTLG6DataTooShort = errors.New("データが短すぎます")

	// ErrTLG6NotImplemented はTLG6本体デコードが未実装であることを示す
	// センチネルエラー。
	//
	// why not: TLG6はゴロム・ライス符号によるエントロピー符号化と複数の
	// フィルタリング手法を組み合わせた可逆圧縮形式であり実装コストが高い。
	// Python参照実装（feat/exe-icon-extraction）もヘッダ解析のみ実装し
	// decode()はNotImplementedErrorを送出する（本タスクのNonGoals）。
	ErrTLG6NotImplemented = errors.New("TLG6デコードは未実装です")
)

// TLG6Header はTLG6画像ファイルのヘッダーから読み取った情報を保持する不変値。
type TLG6Header struct {
	Width       int
	Height      int
	Colors      int // 3=RGB、4=RGBA
	DataFlags   int
	FilterTypes int
	XBlockCount int
	YBlockCount int
}

// TLG6Decoder はTLG6形式のヘッダー解析を行う。本体デコードは未実装
// （ErrTLG6NotImplementedを返す）。
type TLG6Decoder struct{}

// NewTLG6Decoder はTLG6Decoderを初期化する。
func NewTLG6Decoder() *TLG6Decoder {
	return &TLG6Decoder{}
}

// IsValid はdataがTLG6形式のマジックバイトを持つかどうかを判定する。
func (d *TLG6Decoder) IsValid(data []byte) bool {
	return bytes.HasPrefix(data, TLG6Magic)
}

// ParseHeader はTLG6ヘッダーを解析する。
func (d *TLG6Decoder) ParseHeader(data []byte) (TLG6Header, error) {
	if !d.IsValid(data) {
		return TLG6Header{}, ErrTLG6InvalidMagic
	}

	if len(data) < tlg6MinHeaderSize {
		return TLG6Header{}, ErrTLG6DataTooShort
	}

	offset := len(TLG6Magic)

	// 色深度: 24=RGB(3色), 32=RGBA(4色)
	colorDepth := data[offset]
	colors := 3
	if colorDepth == 32 {
		colors = 4
	}
	offset++

	dataFlags := int(data[offset])
	offset++

	width := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4
	height := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4
	xBlockCount := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4
	yBlockCount := int(binary.LittleEndian.Uint32(data[offset : offset+4]))

	return TLG6Header{
		Width:       width,
		Height:      height,
		Colors:      colors,
		DataFlags:   dataFlags,
		FilterTypes: 0,
		XBlockCount: xBlockCount,
		YBlockCount: yBlockCount,
	}, nil
}

// Decode はTLG6形式のバイト列をデコードする。
// 本体デコードは未実装のため、有効なTLG6データに対してもErrTLG6NotImplemented
// を返す（マジックバイトが無効な場合のみErrTLG6InvalidMagicを返す）。
func (d *TLG6Decoder) Decode(data []byte) (image.Image, error) {
	if !d.IsValid(data) {
		return nil, ErrTLG6InvalidMagic
	}

	return nil, ErrTLG6NotImplemented
}
