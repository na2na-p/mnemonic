package parser

import (
	"encoding/binary"
	"errors"
)

// GRPICONDIR/GRPICONDIRENTRYの構造サイズ・定数(ICO/CURフォーマット仕様準拠)。
const (
	grpIconDirHeaderSize = 6  // idReserved(2) + idType(2) + idCount(2)
	grpIconDirEntrySize  = 14 // bWidth(1)+bHeight(1)+bColorCount(1)+bReserved(1)+wPlanes(2)+wBitCount(2)+dwBytesInRes(4)+nID(2)
	grpIconDirTypeIcon   = 1  // idType: 1=アイコン、2=カーソル(本パッケージはアイコンのみ対応)
	grpIconDimensionZero = 256
)

// ErrIconInvalidGroupIconDir はGRPICONDIRのバイト列が仕様上不正
// (マジック値不一致・範囲外参照等)な場合のエラー。
var ErrIconInvalidGroupIconDir = errors.New("GRPICONDIRの形式が不正です")

// grpIconEntry はGRPICONDIRENTRY 1件の読み取り結果。
type grpIconEntry struct {
	width  int
	height int
	id     uint16
}

// parseGrpIconDir はRT_GROUP_ICONリソースの生データ(GRPICONDIR構造)を解析し、
// 含まれる各フレームの情報を返す。
func parseGrpIconDir(data []byte) ([]grpIconEntry, error) {
	if len(data) < grpIconDirHeaderSize {
		return nil, ErrIconInvalidGroupIconDir
	}

	idType := binary.LittleEndian.Uint16(data[2:4])
	if idType != grpIconDirTypeIcon {
		return nil, ErrIconInvalidGroupIconDir
	}

	count := int(binary.LittleEndian.Uint16(data[4:6]))
	entries := make([]grpIconEntry, 0, count)

	for i := range count {
		offset := grpIconDirHeaderSize + i*grpIconDirEntrySize
		if offset+grpIconDirEntrySize > len(data) {
			return nil, ErrIconInvalidGroupIconDir
		}

		e := data[offset : offset+grpIconDirEntrySize]

		// bWidth/bHeightは0の場合256を表す(ICO/CURフォーマット仕様の
		// 慣例。1バイトで256を表現できないための特別扱い)。
		width := int(e[0])
		if width == 0 {
			width = grpIconDimensionZero
		}
		height := int(e[1])
		if height == 0 {
			height = grpIconDimensionZero
		}

		id := binary.LittleEndian.Uint16(e[12:14])

		entries = append(entries, grpIconEntry{width: width, height: height, id: id})
	}

	return entries, nil
}

// selectLargestGrpIconEntry はentries内で面積(width*height)最大のフレームを
// 返す。同面積の場合は先に現れたものを優先する。
//
// why not(strict ">"で比較する理由): Python版(参照実装)はPillowのICOデコード
// 結果を`if size > max_size`で走査して最大フレームを選ぶ。`>=`ではなく`>`の
// ため、同面積が複数あれば最初に見つかったフレームが選ばれる。Go版も挙動を
// 一致させるため同じ比較演算子を使う。
func selectLargestGrpIconEntry(entries []grpIconEntry) grpIconEntry {
	best := entries[0]
	bestArea := best.width * best.height

	for _, e := range entries[1:] {
		area := e.width * e.height
		if area > bestArea {
			best = e
			bestArea = area
		}
	}

	return best
}
