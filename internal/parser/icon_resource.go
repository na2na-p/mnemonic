package parser

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
)

// PEリソースディレクトリの既知のリソースタイプID(Windows SDK winuser.h準拠)。
const (
	resourceTypeIcon      uint32 = 3
	resourceTypeGroupIcon uint32 = 14
)

// IMAGE_RESOURCE_DIRECTORY / IMAGE_RESOURCE_DIRECTORY_ENTRY /
// IMAGE_RESOURCE_DATA_ENTRYの構造サイズ(PE/COFF仕様準拠)。
const (
	resourceDirHeaderSize = 16
	resourceDirEntrySize  = 8
	resourceDataEntrySize = 16

	// resourceHighBit はIMAGE_RESOURCE_DIRECTORY_ENTRYのName/OffsetToData
	// フィールド最上位ビット(0x80000000)。Nameでは「名前付きエントリ」、
	// OffsetToDataでは「サブディレクトリへのオフセット(立っていなければ
	// IMAGE_RESOURCE_DATA_ENTRYへのオフセット)」を示す。
	resourceHighBit = uint32(1) << 31
)

// ErrIconInvalidResourceDir はリソースディレクトリ/データエントリの
// バイト列がPE/COFF仕様の構造として不正(範囲外参照等)な場合のエラー。
var ErrIconInvalidResourceDir = errors.New("リソースディレクトリの形式が不正です")

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// resourceDirEntry はIMAGE_RESOURCE_DIRECTORY_ENTRYの読み取り結果。
type resourceDirEntry struct {
	// id はName(高位ビット無し)の値。名前付きエントリ(高位ビット有り)は
	// GRPICONDIR/RT_ICON探索では使わないため、idとしては扱わない。
	id       uint32
	isName   bool
	isSubdir bool
	// offset はisSubdirならサブディレクトリ、そうでなければ
	// IMAGE_RESOURCE_DATA_ENTRYへの、.rsrcセクション先頭からの相対オフセット。
	offset uint32
}

// readResourceDirectory はrsrc内のoffset位置にあるIMAGE_RESOURCE_DIRECTORYの
// エントリ一覧を返す。
//
// エントリの並び順はPE/COFF仕様により名前付き(文字列比較昇順)→ID(数値昇順)の
// 順で格納されている。この並び順は先頭グループの選択(extractBestIcon参照)で
// そのまま利用する。
func readResourceDirectory(rsrc []byte, offset uint32) ([]resourceDirEntry, error) {
	if uint64(offset)+resourceDirHeaderSize > uint64(len(rsrc)) {
		return nil, ErrIconInvalidResourceDir
	}

	header := rsrc[offset : offset+resourceDirHeaderSize]
	numNamed := binary.LittleEndian.Uint16(header[12:14])
	numID := binary.LittleEndian.Uint16(header[14:16])
	total := int(numNamed) + int(numID)

	entriesOffset := uint64(offset) + resourceDirHeaderSize
	entries := make([]resourceDirEntry, 0, total)

	for i := range total {
		entryOffset := entriesOffset + uint64(i)*resourceDirEntrySize
		if entryOffset+resourceDirEntrySize > uint64(len(rsrc)) {
			return nil, ErrIconInvalidResourceDir
		}

		raw := rsrc[entryOffset : entryOffset+resourceDirEntrySize]
		nameField := binary.LittleEndian.Uint32(raw[0:4])
		dataField := binary.LittleEndian.Uint32(raw[4:8])

		entries = append(entries, resourceDirEntry{
			id:       nameField &^ resourceHighBit,
			isName:   nameField&resourceHighBit != 0,
			isSubdir: dataField&resourceHighBit != 0,
			offset:   dataField &^ resourceHighBit,
		})
	}

	return entries, nil
}

// findByID はentries内でid付き(名前付きでない)かつidに一致する最初の
// エントリを返す。
func findByID(entries []resourceDirEntry, id uint32) (resourceDirEntry, bool) {
	for _, e := range entries {
		if !e.isName && e.id == id {
			return e, true
		}
	}

	return resourceDirEntry{}, false
}

// readResourceDataEntry はrsrc内のoffset位置にあるIMAGE_RESOURCE_DATA_ENTRYを
// 読み取り、rsrcVAを使ってOffsetToData(RVA)を.rsrcセクション内オフセットへ
// 変換した実データのスライスを返す。
func readResourceDataEntry(rsrc []byte, offset, rsrcVA uint32) ([]byte, error) {
	if uint64(offset)+resourceDataEntrySize > uint64(len(rsrc)) {
		return nil, ErrIconInvalidResourceDir
	}

	raw := rsrc[offset : offset+resourceDataEntrySize]
	dataRVA := binary.LittleEndian.Uint32(raw[0:4])
	size := binary.LittleEndian.Uint32(raw[4:8])

	if dataRVA < rsrcVA {
		return nil, ErrIconInvalidResourceDir
	}

	dataOffset := uint64(dataRVA - rsrcVA)
	if dataOffset+uint64(size) > uint64(len(rsrc)) {
		return nil, ErrIconInvalidResourceDir
	}

	return rsrc[dataOffset : dataOffset+uint64(size)], nil
}

// descendToData はparentDir内の最初のエントリを辿り、それがサブディレクトリで
// あればさらに1階層降りて最初のエントリのIMAGE_RESOURCE_DATA_ENTRYが指す実データ
// を返す。PEリソースディレクトリの「言語」階層(Name/ID→Languageの1階層のみ)を
// 読み飛ばすための共通処理で、GRPICONDIRグループ・RT_ICONイメージのどちらの
// 探索にも同じ形で使う。
func descendToData(rsrc []byte, rsrcVA uint32, parentDir []resourceDirEntry) ([]byte, error) {
	if len(parentDir) == 0 {
		return nil, ErrNoIconsAvailable
	}

	first := parentDir[0]
	if !first.isSubdir {
		return nil, ErrNoIconsAvailable
	}

	langEntries, err := readResourceDirectory(rsrc, first.offset)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
	}
	if len(langEntries) == 0 || langEntries[0].isSubdir {
		return nil, ErrNoIconsAvailable
	}

	data, err := readResourceDataEntry(rsrc, langEntries[0].offset, rsrcVA)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
	}

	return data, nil
}

// extractBestIcon はpeFileのRT_GROUP_ICON/RT_ICONリソースから先頭の
// アイコングループを取得し、その中で最大サイズのフレームをデコードして返す。
//
// why not(先頭グループを選ぶ理由): Python版が委譲するicoextractは、グループ
// 選択オプション未指定時にリソースディレクトリの並び順で最初に現れる
// RT_GROUP_ICONグループ(通常はリンカが最初に埋め込んだ既定アイコン=アプリの
// メインアイコン)を既定として抽出する。readResourceDirectoryのコメントの
// とおりPEリソースディレクトリのエントリは名前付き→ID昇順で格納されるため、
// 「先頭エントリ」を選べばicoextractの既定選択と一致する。
func extractBestIcon(peFile *pe.File) (image.Image, error) {
	section := peFile.Section(".rsrc")
	if section == nil {
		return nil, ErrNoIconsAvailable
	}

	rsrc, err := section.Data()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
	}

	rsrcVA := section.VirtualAddress

	root, err := readResourceDirectory(rsrc, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
	}

	groupTypeEntry, ok := findByID(root, resourceTypeGroupIcon)
	if !ok || !groupTypeEntry.isSubdir {
		return nil, ErrNoIconsAvailable
	}

	iconTypeEntry, ok := findByID(root, resourceTypeIcon)
	if !ok || !iconTypeEntry.isSubdir {
		return nil, ErrNoIconsAvailable
	}

	groupEntries, err := readResourceDirectory(rsrc, groupTypeEntry.offset)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
	}

	grpIconDirData, err := descendToData(rsrc, rsrcVA, groupEntries)
	if err != nil {
		return nil, err
	}

	grpEntries, err := parseGrpIconDir(grpIconDirData)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
	}
	if len(grpEntries) == 0 {
		return nil, ErrNoIconsAvailable
	}

	best := selectLargestGrpIconEntry(grpEntries)

	iconEntries, err := readResourceDirectory(rsrc, iconTypeEntry.offset)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
	}

	iconIDEntry, ok := findByID(iconEntries, uint32(best.id))
	if !ok {
		return nil, ErrNoIconsAvailable
	}

	iconData, err := descendToData(rsrc, rsrcVA, []resourceDirEntry{iconIDEntry})
	if err != nil {
		return nil, err
	}

	return decodeIconImage(iconData)
}

// decodeIconImage はRT_ICONの生データをimage.Imageへデコードする。
//
// why not: Vista以降のICOは256x256等の大サイズフレームをPNG形式で埋め込める
// (icoextract/Pillowも同様にフォールバックする)ため、PNGマジックバイトで
// 判定しPNGならstdlib image/pngへ、そうでなければBITMAPINFOHEADER系DIBとして
// decodeDIB(icon_dib.go)へ委譲する。
func decodeIconImage(data []byte) (image.Image, error) {
	if bytes.HasPrefix(data, pngSignature) {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
		}

		return img, nil
	}

	return decodeDIB(data)
}
