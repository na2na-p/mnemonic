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

// locateResourceSection はリソースディレクトリ(RT_GROUP_ICON/RT_ICONを含む
// ツリー全体)の生データと、そのRVA(readResourceDataEntryのRVA→オフセット
// 変換に使う基準値)を返す。
//
// why not(セクション名".rsrc"だけで探さない理由): Windowsローダーは
// OptionalHeaderのデータディレクトリ(IMAGE_DIRECTORY_ENTRY_RESOURCE、
// インデックス2)が指すRVAを正とし、セクション名では探さない——セクション名は
// 単なるラベルであり、パッカーや難読化ツールが自由にリネームできる。名前
// だけで探すと、リンカ後にセクションをリネーム/パックしたEXEに対してだけ
// 本パッケージがアイコンを取りこぼす(同一PEの.rsrcを.dataへリネームした
// だけでデータディレクトリ経由の抽出は成功するがセクション名探索は失敗する
// ことを確認済み)。データディレクトリが無い/空(サイズ0)の場合のみ、
// フォールバックとしてセクション名".rsrc"で探す。
func locateResourceSection(peFile *pe.File) (rsrc []byte, rsrcVA uint32, err error) {
	if dir, ok := resourceDataDirectory(peFile); ok && dir.Size > 0 {
		data, va, findErr := sectionDataContainingRVA(peFile, dir.VirtualAddress)
		if findErr != nil {
			return nil, 0, findErr
		}
		if data != nil {
			return data, va, nil
		}
		// データディレクトリはあるがどのセクションにも収まらない(壊れた
		// PE)。実在するEXEでは起こらない想定だが、フォールバックへ進む。
	}

	section := peFile.Section(".rsrc")
	if section == nil {
		return nil, 0, ErrNoIconsAvailable
	}

	data, err := section.Data()
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
	}

	return data, section.VirtualAddress, nil
}

// resourceDataDirectory はOptionalHeader32/64いずれの場合もIMAGE_DIRECTORY_
// ENTRY_RESOURCEエントリを取り出す。NumberOfRvaAndSizesがそのインデックス
// に満たない(古い/縮小された最適化ヘッダー)場合はfalseを返す。
func resourceDataDirectory(peFile *pe.File) (pe.DataDirectory, bool) {
	const idx = pe.IMAGE_DIRECTORY_ENTRY_RESOURCE

	switch oh := peFile.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		if int(oh.NumberOfRvaAndSizes) <= idx || idx >= len(oh.DataDirectory) {
			return pe.DataDirectory{}, false
		}

		return oh.DataDirectory[idx], true
	case *pe.OptionalHeader64:
		if int(oh.NumberOfRvaAndSizes) <= idx || idx >= len(oh.DataDirectory) {
			return pe.DataDirectory{}, false
		}

		return oh.DataDirectory[idx], true
	default:
		return pe.DataDirectory{}, false
	}
}

// sectionDataContainingRVA はrvaを含むセクションのデータとそのセクションの
// VirtualAddress(rva変換の基準値)を返す。該当セクションが無い場合は
// (nil, 0, nil)を返す(呼び出し元がフォールバックへ進めるようにするため、
// エラーではなくnilで「見つからなかった」ことを表す)。
func sectionDataContainingRVA(peFile *pe.File, rva uint32) ([]byte, uint32, error) {
	for _, sec := range peFile.Sections {
		if rva < sec.VirtualAddress || rva >= sec.VirtualAddress+sec.VirtualSize {
			continue
		}

		data, err := sec.Data()
		if err != nil {
			return nil, 0, fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
		}

		offset := rva - sec.VirtualAddress
		if uint64(offset) > uint64(len(data)) {
			return nil, 0, fmt.Errorf("%w: リソースディレクトリRVAがセクション範囲外です", ErrIconInvalidPEFile)
		}

		return data[offset:], sec.VirtualAddress, nil
	}

	return nil, 0, nil
}

// extractBestIcon はpeFileのRT_GROUP_ICON/RT_ICONリソースから先頭の
// アイコングループを取得し、その中で最大サイズのフレームをデコードして返す。
//
// why not(先頭グループを選ぶ理由): リソースディレクトリの並び順で最初に
// 現れるRT_GROUP_ICONグループ(通常はリンカが最初に埋め込んだ既定アイコン=
// アプリのメインアイコン)を既定として抽出する。readResourceDirectoryの
// コメントのとおりPEリソースディレクトリのエントリは名前付き→ID昇順で
// 格納されるため、「先頭エントリ」を選べばアプリの既定アイコンと一致する。
func extractBestIcon(peFile *pe.File) (image.Image, error) {
	rsrc, rsrcVA, err := locateResourceSection(peFile)
	if err != nil {
		return nil, err
	}
	if rsrc == nil {
		return nil, ErrNoIconsAvailable
	}

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
// why not(PNG判定): Vista以降のICOは256x256等の大サイズフレームをPNG形式で
// 埋め込める(icoextract/Pillowも同様にフォールバックする)ため、PNGマジック
// バイトで判定しPNGならstdlib image/pngへ、そうでなければBITMAPINFOHEADER系
// DIBとしてdecodeDIB(icon_dib.go)へ委譲する。
//
// why not(decodeDIBのエラーをErrIconInvalidPEFileでラップする理由):
// Extract()が公開する契約はErrEXENotFound/ErrIconInvalidPEFile/
// ErrNoIconsAvailableの3種類のみ(icon.goのExtractドキュメント参照)。
// decodeDIBが返すErrIconInvalidDIB/ErrIconUnsupportedDIBをそのまま
// 伝播させるとこの契約から漏れるため、ここでErrIconInvalidPEFileへ
// %wで包む(errors.Isは両方に対しtrueを返せる)。
func decodeIconImage(data []byte) (image.Image, error) {
	if bytes.HasPrefix(data, pngSignature) {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
		}

		return img, nil
	}

	img, err := decodeDIB(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIconInvalidPEFile, err)
	}

	return img, nil
}
