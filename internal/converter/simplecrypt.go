package converter

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ErrScriptTooLarge は吉里吉里の圧縮テキスト（simple crypt mode 2）が宣言する
// 展開後サイズが上限を超える場合のエラー。
var ErrScriptTooLarge = errors.New("展開後のテキストが大きすぎます")

// simpleCryptSignature は吉里吉里のsimple crypt形式の先頭2バイト。続く1バイトが
// モード、その後にUTF-16LEのBOMが続く。
var simpleCryptSignature = []byte{0xfe, 0xfe}

const (
	simpleCryptModeXOR        byte = 0
	simpleCryptModeBitSwap    byte = 1
	simpleCryptModeCompressed byte = 2
)

// simpleCryptMaxUncompressedSize はmode 2が宣言する展開後サイズの上限。
//
// why not: 宣言値を上限なしに信じて展開しない。宣言値はファイル側が自由に書ける
// 値で、展開時の読み取り上限にもそのまま使う。deflateは最大で約1000倍に展開できる
// ため、数MBの細工したファイルでもGB単位のメモリを確保させられる。展開を始める前に、
// テキスト1ファイルとして十分に大きい64MiBで打ち切る。
const simpleCryptMaxUncompressedSize = 64 << 20

// isSimpleCrypt はdataが吉里吉里のsimple crypt形式（FE FE + モードバイト）で
// 始まり、モードが対応済み（0/1/2）であればそのモードとtrueを返す。
func isSimpleCrypt(data []byte) (byte, bool) {
	if len(data) <= len(simpleCryptSignature) || !bytes.HasPrefix(data, simpleCryptSignature) {
		return 0, false
	}

	mode := data[len(simpleCryptSignature)]
	switch mode {
	case simpleCryptModeXOR, simpleCryptModeBitSwap, simpleCryptModeCompressed:
		return mode, true
	default:
		return 0, false
	}
}

// decodeSimpleCrypt はsimple crypt形式のdataを復号し、先頭にUTF-16LEのBOMを
// 付けたUTF-16LEバイト列を返す。形式はKirikiriZのtTVPTextReadStreamに従う。
//
// why not: 復号結果を直接UTF-8へ変換しない。BOMの除去やUTF-16由来ファイルへの
// UTF-8 BOM付与といったUTF-16の扱いを既存のUTF-16経路1か所に保つため、BOM付き
// UTF-16LEに戻してそちらへ委ねる。
func decodeSimpleCrypt(data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, simpleCryptSignature) || len(data) <= len(simpleCryptSignature) {
		return nil, errors.New("simple cryptのヘッダが不完全です")
	}

	mode, ok := isSimpleCrypt(data)
	if !ok {
		return nil, fmt.Errorf("未対応のsimple cryptモードです: %d", data[len(simpleCryptSignature)])
	}

	afterMode := data[len(simpleCryptSignature)+1:]
	if !bytes.HasPrefix(afterMode, utf16LEBOM) {
		return nil, errors.New("simple cryptのモードバイトの後にUTF-16LEのBOMがありません")
	}

	body := afterMode[len(utf16LEBOM):]

	switch mode {
	case simpleCryptModeXOR:
		return transformUTF16LEUnits(body, func(u uint16) uint16 {
			if u >= 0x20 {
				return u ^ ((u&0xfe)<<8 ^ 1)
			}

			return u
		}), nil
	case simpleCryptModeBitSwap:
		return transformUTF16LEUnits(body, func(u uint16) uint16 {
			return (u&0xaaaa)>>1 | (u&0x5555)<<1
		}), nil
	default:
		return inflateSimpleCrypt(body)
	}
}

// transformUTF16LEUnits はbodyをUTF-16LEの符号単位ごとにfで変換し、先頭に
// UTF-16LEのBOMを付けて返す。符号単位に満たない末尾の1バイトは捨てる。
func transformUTF16LEUnits(body []byte, f func(uint16) uint16) []byte {
	out := make([]byte, 0, len(utf16LEBOM)+len(body))
	out = append(out, utf16LEBOM...)

	for i := 0; i+1 < len(body); i += 2 {
		out = binary.LittleEndian.AppendUint16(out, f(binary.LittleEndian.Uint16(body[i:])))
	}

	return out
}

// inflateSimpleCrypt はmode 2の本体（圧縮後サイズ・展開後サイズの64bitリトル
// エンディアン値とzlibストリーム）を展開し、先頭にUTF-16LEのBOMを付けて返す。
func inflateSimpleCrypt(body []byte) ([]byte, error) {
	const sizeFieldsLen = 16
	if len(body) < sizeFieldsLen {
		return nil, errors.New("simple crypt mode 2のサイズ欄が不完全です")
	}

	compressedSize := binary.LittleEndian.Uint64(body[0:8])
	uncompressedSize := binary.LittleEndian.Uint64(body[8:16])
	stream := body[sizeFieldsLen:]

	if uncompressedSize > simpleCryptMaxUncompressedSize {
		return nil, fmt.Errorf("%w: 宣言された展開後サイズ%dバイトが上限%dバイトを超えています",
			ErrScriptTooLarge, uncompressedSize, simpleCryptMaxUncompressedSize)
	}

	if compressedSize > uint64(len(stream)) {
		return nil, fmt.Errorf("simple crypt mode 2の圧縮データが宣言サイズ%dバイトに足りません（%dバイト）",
			compressedSize, len(stream))
	}

	zr, err := zlib.NewReader(bytes.NewReader(stream[:compressedSize]))
	if err != nil {
		return nil, fmt.Errorf("simple crypt mode 2のzlibストリームを開けません: %w", err)
	}

	// why not: 宣言された展開後サイズで事前にバッファを確保しない。宣言値は展開して
	// 確かめる前の値であり、小さなファイルでも上限いっぱいの確保を招くため。
	// 上限+1バイトまで読み、宣言値より長い展開結果も検出する。
	plain, readErr := io.ReadAll(io.LimitReader(zr, int64(uncompressedSize)+1)) //nolint:gosec // uncompressedSizeは上で64MiB以下に制限済みのためint64へ安全に変換できる
	closeErr := zr.Close()

	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, fmt.Errorf("simple crypt mode 2の展開に失敗しました: %w", err)
	}

	if uint64(len(plain)) != uncompressedSize {
		return nil, fmt.Errorf("simple crypt mode 2の展開結果%dバイトが宣言サイズ%dバイトと一致しません",
			len(plain), uncompressedSize)
	}

	return append(bytes.Clone(utf16LEBOM), plain...), nil
}
