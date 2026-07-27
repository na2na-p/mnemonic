package parser_test

import (
	"encoding/binary"
	"testing"
)

// このファイルはicon_test.go専用の最小PEフィクスチャビルダー。
// Go版はPEリソース走査(RT_GROUP_ICON/RT_ICON)を stdlib debug/pe + 手動実装で
// 行うため(SSOTにPEリソースパーサーが無いため。T-210参照)、Python版のように
// icoextractをモックしてテストすることができない。代わりに最小限の妥当な
// PEバイナリ + リソースディレクトリツリーをバイト列として組み立て、
// 実際のパース経路を通してテストする。

// rsrcVA はフィクスチャで使う.rsrcセクションの仮想アドレス(RVA)。値自体に
// 意味はなく、DataEntry.OffsetToDataの計算(セクション内オフセット+この値)と
// 本パッケージ側のRVA→セクション内オフセット変換が整合すればよい。
const fixtureRsrcVA = 0x2000

// fixtureIconImage はRT_ICON1件分のフィクスチャ定義。
type fixtureIconImage struct {
	id       uint16
	width    byte // GRPICONDIRENTRYのbWidth(0は256を表す)
	height   byte // GRPICONDIRENTRYのbHeight(0は256を表す)
	bitCount uint16
	data     []byte
}

// buildMinimalPE はrsrcDataを唯一のセクション".rsrc"として持つ最小PE32
// バイナリを組み立てる。debug/pe.NewFileが要求するDOSヘッダー・PE署名・
// FileHeader・OptionalHeader32(データディレクトリ0件)・SectionHeader32の
// みを書き込み、コード/インポート等ビルド成果物として不要な部分は省略する。
func buildMinimalPE(t *testing.T, rsrcData []byte) []byte {
	t.Helper()

	const (
		dosHeaderSize     = 96
		peSigSize         = 4
		fileHeaderSize    = 20
		optHeaderSize     = 96 // NumberOfRvaAndSizes=0のため固定部のみ
		sectionHeaderSize = 40
	)

	peOffset := uint32(dosHeaderSize)
	fileHeaderOffset := peOffset + peSigSize
	optHeaderOffset := fileHeaderOffset + fileHeaderSize
	sectionHeaderOffset := optHeaderOffset + optHeaderSize
	rsrcFileOffset := sectionHeaderOffset + sectionHeaderSize

	buf := make([]byte, rsrcFileOffset+uint32(len(rsrcData)))

	buf[0], buf[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(buf[0x3c:0x40], peOffset)

	copy(buf[peOffset:], []byte("PE\x00\x00"))

	fh := buf[fileHeaderOffset:]
	binary.LittleEndian.PutUint16(fh[0:2], 0x14c) // Machine: IMAGE_FILE_MACHINE_I386
	binary.LittleEndian.PutUint16(fh[2:4], 1)     // NumberOfSections
	binary.LittleEndian.PutUint16(fh[16:18], uint16(optHeaderSize))
	binary.LittleEndian.PutUint16(fh[18:20], 0x0102) // Characteristics(EXECUTABLE_IMAGE|32BIT_MACHINE)

	oh := buf[optHeaderOffset:]
	binary.LittleEndian.PutUint16(oh[0:2], 0x10b) // Magic: PE32
	binary.LittleEndian.PutUint32(oh[92:96], 0)   // NumberOfRvaAndSizes: データディレクトリ無し

	sh := buf[sectionHeaderOffset:]
	copy(sh[0:8], []byte(".rsrc\x00\x00\x00"))
	binary.LittleEndian.PutUint32(sh[8:12], uint32(len(rsrcData)))  // VirtualSize
	binary.LittleEndian.PutUint32(sh[12:16], fixtureRsrcVA)         // VirtualAddress
	binary.LittleEndian.PutUint32(sh[16:20], uint32(len(rsrcData))) // SizeOfRawData
	binary.LittleEndian.PutUint32(sh[20:24], rsrcFileOffset)        // PointerToRawData

	copy(buf[rsrcFileOffset:], rsrcData)

	return buf
}

// fixtureGroupID はフィクスチャで使うRT_GROUP_ICONのName/ID値。全テストで
// 単一グループのみを組み立てるため固定値とする(unparam指摘の回避も兼ねる。
// 値自体に意味は無くグループを一意に指せればよい)。
const fixtureGroupID = 1

// fixtureLangID はフィクスチャで使うLanguage階層のID値(en-US)。全テストで
// 単一言語のみを組み立てるため固定値とする(unparam指摘の回避も兼ねる。
// 値自体に意味は無くテスト全体で一定であればよい)。
const fixtureLangID = 0x0409

// buildRsrcWithIconGroup はRT_GROUP_ICON(1グループ・images由来のGRPICONDIR)と
// RT_ICON(imagesそれぞれの生データ)を持つ.rsrcセクションのバイト列を組み立てる。
//
// リソースディレクトリの階層はPE/COFF仕様どおりType→Name/ID→Languageの3階層。
// 本フィクスチャは常に単一言語(fixtureLangID)・単一グループ(fixtureGroupID)
// のみを持つ最小構成にする(タスク指示: フィクスチャは小さく保つ)。
func buildRsrcWithIconGroup(t *testing.T, images []fixtureIconImage) []byte {
	t.Helper()

	return buildRsrcWithIconGroupType(t, grpIconDirTypeIconFixture, images)
}

// buildRsrcWithIconGroupInvalidType はGRPICONDIRのidTypeフィールドを
// カーソル(2)に書き換えた不正なリソースツリーを組み立てる
// (parser.ErrIconInvalidPEFileへ変換されることを確認するテスト専用)。
func buildRsrcWithIconGroupInvalidType(t *testing.T, images []fixtureIconImage) []byte {
	t.Helper()

	const grpIconDirTypeCursor = 2

	return buildRsrcWithIconGroupType(t, grpIconDirTypeCursor, images)
}

// grpIconDirTypeIconFixture はGRPICONDIR.idTypeの正常値(1=アイコン)。
const grpIconDirTypeIconFixture = 1

func buildRsrcWithIconGroupType(t *testing.T, grpIconDirType uint16, images []fixtureIconImage) []byte {
	t.Helper()

	n := len(images)
	const (
		dirHeaderSize = 16
		dirEntrySize  = 8
		dataEntrySize = 16
		grpDirHeader  = 6
		grpDirEntry   = 14
	)

	rootSize := dirHeaderSize + 2*dirEntrySize
	type3DirSize := dirHeaderSize + n*dirEntrySize
	type14DirSize := dirHeaderSize + 1*dirEntrySize
	lvl3EachSize := dirHeaderSize + 1*dirEntrySize

	offRoot := 0
	offType3Dir := offRoot + rootSize
	offType14Dir := offType3Dir + type3DirSize

	offsLvl3Type3 := make([]int, n)
	cur := offType14Dir + type14DirSize
	for i := range n {
		offsLvl3Type3[i] = cur
		cur += lvl3EachSize
	}
	offLvl3Type14 := cur
	cur += lvl3EachSize

	offsDataType3 := make([]int, n)
	for i := range n {
		offsDataType3[i] = cur
		cur += dataEntrySize
	}
	offDataType14 := cur
	cur += dataEntrySize

	offGrpIconDir := cur
	grpIconDirSize := grpDirHeader + n*grpDirEntry
	cur += grpIconDirSize

	offsImages := make([]int, n)
	for i, img := range images {
		offsImages[i] = cur
		cur += len(img.data)
	}

	buf := make([]byte, cur)

	writeDirHeader := func(off, numID int) {
		binary.LittleEndian.PutUint16(buf[off+12:off+14], 0)             // NumberOfNamedEntries
		binary.LittleEndian.PutUint16(buf[off+14:off+16], uint16(numID)) // NumberOfIdEntries
	}
	writeDirEntry := func(off int, id uint32, isSubdir bool, target uint32) {
		binary.LittleEndian.PutUint32(buf[off:off+4], id)
		v := target
		if isSubdir {
			v |= 1 << 31
		}
		binary.LittleEndian.PutUint32(buf[off+4:off+8], v)
	}
	writeDataEntry := func(off int, dataOffsetInSection int, size int) {
		binary.LittleEndian.PutUint32(buf[off:off+4], fixtureRsrcVA+uint32(dataOffsetInSection))
		binary.LittleEndian.PutUint32(buf[off+4:off+8], uint32(size))
	}

	// Root: Type=3(RT_ICON), Type=14(RT_GROUP_ICON)
	writeDirHeader(offRoot, 2)
	writeDirEntry(offRoot+dirHeaderSize, 3, true, uint32(offType3Dir))
	writeDirEntry(offRoot+dirHeaderSize+dirEntrySize, 14, true, uint32(offType14Dir))

	// Type3(RT_ICON)配下: 画像ごとにID
	writeDirHeader(offType3Dir, n)
	for i, img := range images {
		writeDirEntry(offType3Dir+dirHeaderSize+i*dirEntrySize, uint32(img.id), true, uint32(offsLvl3Type3[i]))
	}

	// Type14(RT_GROUP_ICON)配下: グループID 1件
	writeDirHeader(offType14Dir, 1)
	writeDirEntry(offType14Dir+dirHeaderSize, fixtureGroupID, true, uint32(offLvl3Type14))

	// RT_ICON各画像のLanguage階層(Data Entryへ直接ポイント)
	for i := range images {
		writeDirHeader(offsLvl3Type3[i], 1)
		writeDirEntry(offsLvl3Type3[i]+dirHeaderSize, fixtureLangID, false, uint32(offsDataType3[i]))
	}

	// グループのLanguage階層
	writeDirHeader(offLvl3Type14, 1)
	writeDirEntry(offLvl3Type14+dirHeaderSize, fixtureLangID, false, uint32(offDataType14))

	// Data Entry群
	for i, img := range images {
		writeDataEntry(offsDataType3[i], offsImages[i], len(img.data))
	}
	writeDataEntry(offDataType14, offGrpIconDir, grpIconDirSize)

	// GRPICONDIR本体
	binary.LittleEndian.PutUint16(buf[offGrpIconDir+2:offGrpIconDir+4], grpIconDirType)
	binary.LittleEndian.PutUint16(buf[offGrpIconDir+4:offGrpIconDir+6], uint16(n))
	for i, img := range images {
		e := buf[offGrpIconDir+grpDirHeader+i*grpDirEntry : offGrpIconDir+grpDirHeader+(i+1)*grpDirEntry]
		e[0] = img.width
		e[1] = img.height
		binary.LittleEndian.PutUint16(e[6:8], img.bitCount)
		binary.LittleEndian.PutUint32(e[8:12], uint32(len(img.data)))
		binary.LittleEndian.PutUint16(e[12:14], img.id)
	}

	// 各RT_ICONの生データ
	for i, img := range images {
		copy(buf[offsImages[i]:], img.data)
	}

	return buf
}

// buildEmptyRsrcDir はエントリを1件も持たない(RT_GROUP_ICON/RT_ICONが
// どちらも存在しない)最小のIMAGE_RESOURCE_DIRECTORYのみを返す。
func buildEmptyRsrcDir() []byte {
	return make([]byte, 16)
}
