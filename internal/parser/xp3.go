package parser

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"unicode/utf16"

	"github.com/na2na-p/mnemonic/internal/safepath"
	"github.com/na2na-p/mnemonic/internal/saturate"
)

// センチネルエラー群。
var (
	// ErrXP3NotFound はXP3ファイルが存在しない場合のエラー。
	ErrXP3NotFound = errors.New("XP3ファイルが見つかりません")
	// ErrInvalidXP3 は不正なXP3ファイル形式の場合のエラー。
	ErrInvalidXP3 = errors.New("不正なXP3ファイル形式です")
	// ErrDecompressedTooLarge はzlib解凍結果が許容サイズを超えた場合のエラー。
	ErrDecompressedTooLarge = errors.New("zlib解凍結果が許容サイズを超えています")
	// ErrEntryPathConflict は、あるエントリの展開先が別のエントリの展開先の
	// ディレクトリ部分と一致し、両方をファイルとして展開できない場合のエラー。
	//
	// why not: ErrInvalidXP3で包まない。エンジン（krkrz base/XP3Archive.cpp）の
	// 索引の読み込みは各名前を正規化して表に加えるだけで、ほかのエントリの名前との
	// 関係を調べない。この組み合わせを形式の不正とは言えない。
	ErrEntryPathConflict = errors.New("ファイルとディレクトリを兼ねるエントリ名があるため展開できません")
)

// maxFileTableSize はファイルテーブルの解凍後サイズの上限。
//
// why not: 圧縮後サイズはファイル残量を超えられないが、zlibは約1000:1まで
// 膨張しうるため、それだけでは1MBのアーカイブで約1GBを確保させられる。
// original_sizeもアーカイブ自身の値なので上限には使えない。
// 実アーカイブの索引は数MBが上限で、64MiBなら細工されたzlibによる膨張だけを弾ける。
const maxFileTableSize = 64 << 20

// maxSegmentDecompressedSize はセグメントの解凍後サイズの絶対的な上限。ExtractAllは
// エントリ全体の解凍後サイズの合計の上限としても同じ値を使う（entryBudget参照）。
//
// why not: セグメントの宣言サイズ（OriginalSize）はアーカイブ自身の値なので、
// 上限として信用しきれない。1GiBなら実素材（動画もzlib圧縮されることはまずない）を
// 締め出さず、宣言サイズを偽ったTB級の膨張だけを弾ける。
const maxSegmentDecompressedSize = 1 << 30

// XP3Segment はXP3ファイルセグメント情報を表す。
//
// XP3アーカイブ内のファイルは複数のセグメントに分割されている場合がある。
// 各セグメントは異なるオフセットに配置され、個別に圧縮される可能性がある。
type XP3Segment struct {
	// Offset はセグメントデータのオフセット。
	Offset int64
	// Size はアーカイブに格納されたバイト数（圧縮セグメントでは圧縮後サイズ）。
	// 展開時に読み取る長さとして使うのは圧縮セグメントだけで、非圧縮セグメントは
	// OriginalSizeぶんを読む。
	Size int64
	// OriginalSize は元のサイズ（圧縮セグメントでは解凍後サイズ）。
	OriginalSize int64
	// IsCompressed は圧縮されているか。
	IsCompressed bool
}

// XP3FileEntry はXP3アーカイブ内のファイルエントリ情報を表す。
type XP3FileEntry struct {
	// Name はファイル名（パス含む）。
	Name string
	// Segments はファイルを構成するセグメントの一覧（登場順）。
	Segments []XP3Segment
}

// XP3Archive はXP3アーカイブを操作する。
//
// 吉里吉里/KAG形式のXP3アーカイブファイルを開き、
// 内包されているファイルの一覧取得や展開を行う。
type XP3Archive struct {
	archivePath string
	fileSize    int64
	fileEntries []XP3FileEntry
}

// NewXP3Archive はarchivePathのアーカイブファイルを開く。
//
// ファイルが存在しない場合はErrXP3NotFound、
// 不正なXP3ファイル形式の場合はErrInvalidXP3を返す。
func NewXP3Archive(archivePath string) (*XP3Archive, error) {
	if _, err := os.Stat(archivePath); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrXP3NotFound, archivePath)
	}

	archive := &XP3Archive{archivePath: archivePath}
	if err := archive.parseArchive(); err != nil {
		return nil, err
	}

	return archive, nil
}

func validateXP3Magic(data []byte) bool {
	return bytes.HasPrefix(data, XP3Magic)
}

func (a *XP3Archive) parseArchive() error {
	f, err := os.Open(a.archivePath) //nolint:gosec // コンストラクタでexists検証済みのユーザー指定パスを読む用途のため妥当
	if err != nil {
		return fmt.Errorf("XP3ファイルを開けません: %w", err)
	}
	defer func() { _ = f.Close() }()

	header := make([]byte, 32)
	n, err := io.ReadFull(f, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return fmt.Errorf("XP3ヘッダーの読み込みに失敗しました: %w", err)
	}
	header = header[:n]

	if len(header) < len(XP3Magic) || !validateXP3Magic(header) {
		return fmt.Errorf("%w: %s", ErrInvalidXP3, a.archivePath)
	}
	if len(header) < 19 {
		return fmt.Errorf("%w: インデックスオフセットが途切れています: %s", ErrInvalidXP3, a.archivePath)
	}

	return a.parseFileIndex(f, header)
}

// parseFileIndex はファイルインデックスをパースする。
//
// headerはマジックとインデックスオフセットを含む19バイト以上であることを
// 前提とする（parseArchiveが保証する）。
func (a *XP3Archive) parseFileIndex(f io.ReadSeeker, header []byte) error {
	return a.parseStandardIndex(f, header)
}

func (a *XP3Archive) invalidIndexError(what string) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalidXP3, what, a.archivePath)
}

// インデックスのフラグバイトの値。krkrz base/XP3Archive.h の
// TVP_XP3_INDEX_ENCODE_METHOD_MASK / _RAW / _ZLIB / TVP_XP3_INDEX_CONTINUE と同じ。
const (
	indexEncodeMethodMask = 0x07
	indexEncodeRaw        = 0x00
	indexEncodeZlib       = 0x01
	indexContinue         = 0x80
)

// maxIndexBlocks は継続フラグで連ねられるインデックスの最大数。
//
// why not: krkrz の tTVPXP3Archive は継続の連鎖に上限を持たないため、次インデックス
// オフセットが自身を指すだけで無限ループする。krkrrel（krdevui RelSettingsUnit.cpp）
// が書くのはクッションヘッダーと実インデックスの2つだけなので、16で打ち切っても
// 実アーカイブは締め出さない。
const maxIndexBlocks = 16

// parseStandardIndex はインデックスオフセットが指す標準インデックスをパースする。
//
// why not: インデックスオフセットがファイル末尾ちょうどを指すアーカイブを、
// インデックスを持たない空のアーカイブとして受け入れない。krkrz
// （base/XP3Archive.cpp の tTVPXP3Archive）はそこからフラグ1バイトをReadBufferで
// 読み、読み取り不足としてエラーにする（tjs2/tjs.cpp）。空のアーカイブは
// index_sizeが0の非圧縮インデックスで表す。
//
// why not: フラグ0x80を「テーブルサイズとテーブル位置が続く形式」とは読まない。
// krkrz の tTVPXP3Archive（base/XP3Archive.cpp）は0x80を継続フラグとして扱い、
// 下位3ビットのエンコード方式でテーブルを読んだ直後の8バイトを次のインデックス
// オフセットとして読む。krkrrel はインデックスオフセットの後ろに4バイトの
// マイナーバージョンと継続フラグ付きの空の非圧縮インデックス（クッションヘッダー）
// を置き、そこから実インデックスを指すため、この連鎖を辿らないと krkrrel 製の
// アーカイブを読めない。マイナーバージョンはインデックスオフセットが飛び越える
// 位置にあり、krkrz も読まないため検証しない。
func (a *XP3Archive) parseStandardIndex(f io.ReadSeeker, header []byte) error {
	indexOffset, ok := safeInt64(binary.LittleEndian.Uint64(header[11:19]))
	if !ok {
		return a.invalidIndexError("インデックスオフセットが範囲外です")
	}

	fileSize, err := streamSize(f)
	if err != nil {
		return fmt.Errorf("%w: %w: %s", ErrInvalidXP3, err, a.archivePath)
	}
	a.fileSize = fileSize
	if indexOffset >= fileSize {
		return a.invalidIndexError("インデックスオフセットがファイルの範囲外です")
	}

	// why not: ファイルテーブルの上限はインデックスごとではなく全体に適用する。
	// インデックスごとでは継続の連鎖でmaxIndexBlocks倍まで確保させられる。
	budget := int64(maxFileTableSize)
	for range maxIndexBlocks {
		if _, err := f.Seek(indexOffset, io.SeekStart); err != nil {
			return a.invalidIndexError("インデックスオフセットへシークできません")
		}

		continued, used, err := a.readIndexBlock(f, fileSize, budget)
		if err != nil {
			return err
		}
		budget -= used
		if !continued {
			return nil
		}

		next, ok := readUint64(f)
		if !ok {
			return a.invalidIndexError("次のインデックスオフセットを読み取れません")
		}
		indexOffset, ok = safeInt64(next)
		if !ok || indexOffset >= fileSize {
			return a.invalidIndexError("次のインデックスオフセットがファイルの範囲外です")
		}
	}

	return a.invalidIndexError("継続するインデックスが多すぎます")
}

// readIndexBlock は現在位置のインデックス1つ（フラグとファイルテーブル）を読み取り、
// エントリをパースする。戻り値のcontinuedは継続フラグの有無、usedはファイル
// テーブルのバイト数。呼び出し後の位置はファイルテーブルの直後になる。
func (a *XP3Archive) readIndexBlock(f io.ReadSeeker, fileSize, budget int64) (continued bool, used int64, err error) {
	flagByte := make([]byte, 1)
	if _, err := io.ReadFull(f, flagByte); err != nil {
		return false, 0, a.invalidIndexError("インデックスフラグを読み取れません")
	}
	flag := flagByte[0]

	var tableData []byte
	switch flag & indexEncodeMethodMask {
	case indexEncodeZlib:
		tableData, err = a.readZlibFileTable(f, fileSize, budget)
	case indexEncodeRaw:
		tableData, err = a.readRawFileTable(f, fileSize, budget)
	default:
		return false, 0, a.invalidIndexError("インデックスのエンコード方式が不明です")
	}
	if err != nil {
		return false, 0, err
	}

	if err := a.parseFileEntries(tableData); err != nil {
		return false, 0, err
	}

	return flag&indexContinue != 0, int64(len(tableData)), nil
}

// readRawFileTable はフラグ直後のindex_sizeと、それに続く非圧縮のファイル
// テーブルを読み取る。
//
// why not: 非圧縮インデックスはzlib形式と違いoriginal_sizeを持たず、index_sizeの
// 直後がテーブルになる（krkrz base/XP3Archive.cpp の TVP_XP3_INDEX_ENCODE_RAW 分岐、
// krkrrel の書き出しも同じ）。そのためzlib形式と同じく2つのuint64を読むと
// テーブルの先頭8バイトを読み飛ばしてしまう。
//
// why not: 宣言サイズを残量へ縮めて読まない。index_sizeは圧縮前のテーブル長
// そのものなので、残量が足りないのは途切れたテーブルであり、krkrz も
// ReadBuffer の読み取り不足（tjs2/tjs.cpp）としてエラーにする。
func (a *XP3Archive) readRawFileTable(f io.ReadSeeker, fileSize, budget int64) ([]byte, error) {
	indexSize, ok := readUint64(f)
	if !ok {
		return nil, a.invalidIndexError("非圧縮ファイルテーブルのサイズを読み取れません")
	}

	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, a.invalidIndexError("ファイルテーブルの位置を取得できません")
	}
	if indexSize > uint64(max(fileSize-offset, 0)) {
		return nil, a.invalidIndexError("非圧縮ファイルテーブルのサイズがファイル残量を超えています")
	}
	if indexSize > uint64(budget) { //nolint:gosec // budgetはmaxFileTableSizeから使用量を引いた非負値
		return nil, a.invalidIndexError("ファイルテーブルの合計が上限を超えています")
	}

	tableData := make([]byte, indexSize)
	if _, err := io.ReadFull(f, tableData); err != nil {
		return nil, a.invalidIndexError("非圧縮ファイルテーブルを読み取れません")
	}

	return tableData, nil
}

// readZlibFileTable はフラグ直後のcompressed_size、original_sizeと、それに続く
// zlib圧縮のファイルテーブルを読み取り、budgetバイトを上限に解凍して返す。
//
// why not: 解凍できないテーブルを非圧縮のテーブルとして読み替えない。krkrz
// （base/XP3Archive.cpp の TVP_XP3_INDEX_ENCODE_ZLIB 分岐）は解凍失敗を
// TVPUncompressionFailed として投げ、krkrrel（krdevui RelSettingsUnit.cpp）は
// 圧縮に失敗した索引をフラグ0x00の非圧縮インデックスとして書く。読み替えても
// 正しいアーカイブは救えず、途切れたり壊れたりしたテーブルは原因と無関係な
// チャンクのエラーになり、細工されたバイト列はそのままファイルテーブルとして
// 通ってしまう。
//
// why not: 解凍後の長さがoriginal_sizeと違うテーブルを受け入れない。krkrz は
// original_sizeぶんの領域へ解凍し、長さが一致しなければ同じく投げる。krkrrel は
// 圧縮前のテーブル長をそのままoriginal_sizeに書くため、正しいアーカイブは
// 締め出さない。
//
// why not: compressed_sizeを残量へ縮めて読まない。krkrz はcompressed_sizeぶんを
// ReadBufferで読み、足りなければエラーにする（tjs2/tjs.cpp）。縮めると、宣言が
// 残量を超えていても残りに完結したzlibストリームがあるテーブルを、krkrz が
// 読めないのに受け入れてしまう。
func (a *XP3Archive) readZlibFileTable(f io.ReadSeeker, fileSize, budget int64) ([]byte, error) {
	compressedSize, ok := readUint64(f)
	if !ok {
		return nil, a.invalidIndexError("ファイルテーブルの圧縮後サイズを読み取れません")
	}
	originalSize, ok := readUint64(f)
	if !ok {
		return nil, a.invalidIndexError("ファイルテーブルの元サイズを読み取れません")
	}

	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, a.invalidIndexError("ファイルテーブルの位置を取得できません")
	}
	if compressedSize > uint64(max(fileSize-offset, 0)) {
		return nil, a.invalidIndexError("ファイルテーブルの圧縮後サイズがファイル残量を超えています")
	}

	compressed := make([]byte, compressedSize)
	if _, err := io.ReadFull(f, compressed); err != nil {
		return nil, a.invalidIndexError("ファイルテーブルを読み取れません")
	}

	tableData, err := decompressZlib(compressed, budget)
	if err != nil {
		return nil, fmt.Errorf("%w: ファイルテーブル: %w: %s", ErrInvalidXP3, err, a.archivePath)
	}
	if uint64(len(tableData)) != originalSize {
		return nil, a.invalidIndexError("ファイルテーブルの展開後サイズが宣言と一致しません")
	}

	return tableData, nil
}

func readUint64(f io.Reader) (uint64, bool) {
	buf := make([]byte, 8)
	if _, err := io.ReadFull(f, buf); err != nil {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buf), true
}

// safeInt64 はuint64値をint64へ変換する。
//
// why not: XP3インデックス内のオフセット・サイズは信頼できないアーカイブ
// バイナリ由来の値であり、math.MaxInt64を超える値をint64へ単純キャストすると
// 符号が反転し負値になる（seekの意図しない巻き戻り等につながる）。そのため
// 変換前に範囲チェックし、超過時はパース失敗として扱う。
func safeInt64(v uint64) (int64, bool) {
	if v > math.MaxInt64 {
		return 0, false
	}

	return int64(v), true //nolint:gosec // 直前のv > math.MaxInt64チェックによりオーバーフローしないことを保証済み
}

// decompressZlib はdataをzlib解凍する。解凍結果がlimitバイトを超える場合は
// ErrDecompressedTooLargeを返す。
func decompressZlib(data []byte, limit int64) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("zlib解凍に失敗しました: %w", err)
	}
	defer func() { _ = r.Close() }()

	// why not: limitそのものを読み取り上限にすると、ちょうどlimitバイトで終わる
	// ストリームと超過するストリームを区別できない。1バイト余分に読んで超過を検出する
	// （limitがmath.MaxInt64でも加算が溢れないよう先に1減らす）。
	decompressed, err := io.ReadAll(io.LimitReader(r, min(limit, math.MaxInt64-1)+1))
	if err != nil {
		return nil, fmt.Errorf("zlib解凍に失敗しました: %w", err)
	}
	if int64(len(decompressed)) > limit {
		return nil, fmt.Errorf("%w: 上限%dバイト", ErrDecompressedTooLarge, limit)
	}

	return decompressed, nil
}

func (a *XP3Archive) parseFileEntries(tableData []byte) error {
	stream := bytes.NewReader(tableData)

	for {
		chunkName := make([]byte, 4)
		if _, err := io.ReadFull(stream, chunkName); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return a.invalidIndexError("ファイルテーブルのチャンク名が途切れています")
		}

		chunkSize, ok := readUint64(stream)
		if !ok {
			return a.invalidIndexError("ファイルテーブルのチャンクサイズを読み取れません")
		}
		if chunkSize > uint64(len(tableData)) { //nolint:gosec // len()は非負でuint64との比較として安全
			return a.invalidIndexError("ファイルテーブルのチャンクサイズがテーブル長を超えています")
		}

		if !bytes.Equal(chunkName, []byte("File")) {
			skipChunk(stream, chunkSize)

			continue
		}

		// why not: テーブル残量を超えるFileチャンクを残りだけで読まない。krkrz の
		// tTVPXP3Archive（base/XP3Archive.cpp）はFindChunkでチャンク長をテーブル長と
		// 比べず、次のFileチャンクの探索範囲を index_size - 開始位置 として符号なしで
		// 求めるため、開始位置がテーブルを越えると範囲が桁あふれしてテーブルの外を読む。
		// 読み込みが定まらないアーカイブであり、受け入れる根拠がない。
		if chunkSize > uint64(stream.Len()) { //nolint:gosec // bytes.Reader.Len()は常に非負
			return a.invalidIndexError("Fileチャンクの長さがファイルテーブルの残量を超えています")
		}

		entry, ok, err := parseSingleEntry(readChunk(stream, chunkSize))
		if err != nil {
			return fmt.Errorf("%w: %w: %s", ErrInvalidXP3, err, a.archivePath)
		}
		if ok {
			a.fileEntries = append(a.fileEntries, entry)
		}
	}
}

// parseSingleEntry は単一のファイルエントリをパースする。
//
// info、segm、adlrのいずれかのサブチャンクを欠く場合はエラーを返す（infoから名前を
// 得られていればエントリ名を添える）。krkrz（base/XP3Archive.cpp の tTVPXP3Archive）は
// どれもFindChunkで見つからなければTVPReadErrorを投げる。adlrの本体長は確かめない。
// krkrzは本体長を見ずに先頭4バイトをハッシュとして読み、mnemonicはハッシュを使わない。
//
// 3つのサブチャンクがそろう前に本体長が32ビットに収まらないサブチャンクがあれば
// エラーを返す。krkrzのFindChunkは辿ったチャンクの本体長をtjs_uint（32ビット）へ
// 切り詰め、値が変われば投げる。探しているチャンクを見つけた時点で戻るため、
// 3つがそろった後ろのチャンクは読まない。
//
// why not: 同じ名前のサブチャンクが複数あっても、後ろのinfoで名前を上書きしたり、
// 後ろのsegmのセグメントを継ぎ足したりしない。krkrzはinfo、segm、adlrをそれぞれ
// Fileチャンクの先頭からFindChunkで探し、最初に一致したものだけを読む。
//
// 最初のinfoから名前を得られなかった場合、または最初のsegmが28バイト未満などで
// セグメントを1つも持てなかった場合はokにfalseを返す。krkrzが読めないセグメントを
// 持つ場合（parseSegments参照）は、名前があればそれを添えたエラーを返す。名前が
// 空のエントリで確かめるのはエンコード方式だけである。
//
// why not: 名前が空であること自体や、セグメントを持たないこと自体はエラーにしない。
// krkrz は長さ0の名前も、28バイト未満のsegm（セグメント0件）も投げずに索引へ
// 加える。
func parseSingleEntry(entryData []byte) (XP3FileEntry, bool, error) {
	stream := bytes.NewReader(entryData)

	var (
		name      string
		foundInfo bool
		foundSegm bool
		foundAdlr bool
		segmData  []byte
	)

	for {
		subChunkName := make([]byte, 4)
		if _, err := io.ReadFull(stream, subChunkName); err != nil {
			break
		}

		subChunkSize, ok := readUint64(stream)
		if !ok {
			break
		}
		requiredFound := foundInfo && foundSegm && foundAdlr
		if subChunkSize > math.MaxUint32 && !requiredFound {
			return XP3FileEntry{}, false, entryError(name, "サブチャンクの長さが32ビットに収まりません")
		}

		switch {
		case !foundInfo && bytes.Equal(subChunkName, []byte("info")):
			foundInfo = true
			infoData := readChunk(stream, subChunkSize)
			// why not: flagsのビット31を暗号化の印として扱わない。krkrz（base/XP3Archive.h）
			// ではこのビットはTVP_XP3_FILE_PROTECTED（展開ツールからの保護の印）であり、
			// base/XP3Archive.cppは既定でTVPAllowExtractProtectedStorageをtrueにして
			// ビット31を見ずに読み、読み出しでも参照しない。ゲーム固有の暗号化は
			// 読み出し後にTVPXP3ArchiveExtractionFilterが施すもので、索引には現れない。
			// flags、original_size、sizeはここでは使わない（サイズはsegmの各セグメントが持つ）。
			if len(infoData) >= 22 {
				nameLen := int(binary.LittleEndian.Uint16(infoData[20:22]))

				if len(infoData) >= 22+nameLen*2 {
					name = decodeUTF16LE(infoData[22 : 22+nameLen*2])
				}
			}
		case !foundSegm && bytes.Equal(subChunkName, []byte("segm")):
			foundSegm = true
			segmData = readChunk(stream, subChunkSize)
		case !foundAdlr && bytes.Equal(subChunkName, []byte("adlr")):
			foundAdlr = true
			skipChunk(stream, subChunkSize)
		default:
			skipChunk(stream, subChunkSize)
		}
	}

	if !foundInfo {
		return XP3FileEntry{}, false, errors.New("Fileチャンクにinfoサブチャンクがありません")
	}
	if !foundSegm {
		return XP3FileEntry{}, false, entryError(name, "Fileチャンクにsegmサブチャンクがありません")
	}
	// why not: 名前が空のエントリやセグメントを持たないエントリを捨てる判定より後に
	// 確かめない。krkrzはadlrを欠けば名前やセグメント数によらず投げる。
	if !foundAdlr {
		return XP3FileEntry{}, false, entryError(name, "Fileチャンクにadlrサブチャンクがありません")
	}
	if name == "" {
		// why not: 名前が空のエントリでもエンコード方式を確かめずには捨てない。krkrzは
		// 名前によらず最初のsegmの全レコードを読み、方式が不明なら投げる。元サイズや
		// 格納サイズの範囲はkrkrzが索引の読み取りで確かめないため、ここでは問わない。
		if err := checkSegmEncodeMethods(segmData); err != nil {
			return XP3FileEntry{}, false, err
		}

		return XP3FileEntry{}, false, nil
	}

	// why not: segmをinfoより先に見つけた時点ではパースしない。サブチャンクの順序は
	// 決まっておらず、エラーにエントリ名を添えるにはinfoを読み終えている必要がある。
	segments, err := parseSegments(segmData)
	if err != nil {
		return XP3FileEntry{}, false, fmt.Errorf("%s: %w", name, err)
	}
	if len(segments) == 0 {
		return XP3FileEntry{}, false, nil
	}

	return XP3FileEntry{
		Name:     name,
		Segments: segments,
	}, true, nil
}

// entryError はnameが空でなければエントリ名を添えたエラーを返す。
func entryError(name, message string) error {
	if name == "" {
		return errors.New(message)
	}

	return fmt.Errorf("%s: %s", name, message)
}

// segmレコードのflagsの値。krkrz base/XP3Archive.h の
// TVP_XP3_SEGM_ENCODE_METHOD_MASK / _RAW / _ZLIB と同じ。
const (
	segmEncodeMethodMask = 0x07
	segmEncodeRaw        = 0x00
	segmEncodeZlib       = 0x01
)

const segmentRecordSize = 28

// parseSegments はsegmサブチャンクのデータを28バイト単位のセグメント列としてパースする。
//
// why not: 宣言されたsubChunkSizeではなく、実際に読み取れたsegmData
// （readChunkでstream残量にクランプ済み）の長さを28で割った件数だけを対象にする。
// これにより末尾の28バイト未満の断片は自然に無視され、宣言セグメント数が
// どれほど巨大でも実データ長を超えて処理することはない。ただしこれは
// 「セグメントレコードのパース時」に確保する[]XP3Segmentのメモリ量に関する
// 主張に過ぎず、展開時に同一オフセットを指す大量のセグメントを積み重ねる
// 攻撃までは防げない（そちらの対策はextractEntryのwhy not参照）。
//
// why not: エンコード方式が0（非圧縮）と1（zlib）以外のセグメントを圧縮として
// 読まない。krkrz（base/XP3Archive.cpp のsegm読み取り）はflags & 0x07がそれ以外なら
// TVPReadErrorを投げ、その値の格納形式を定めていない。
//
// why not: 読み取る長さを決めるサイズ（元サイズと、圧縮セグメントの格納サイズ）が
// int64の範囲を超えるセグメントを、ゼロ値に読み替えて受け入れない。krkrzは非圧縮
// セグメントを元サイズぶんReadBufferで読み（読み取り不足はtjs2/tjs.cppでエラー）、
// 圧縮セグメントは元サイズと解凍後の長さが一致しなければ投げる（tTVPSegmentData::
// SetData）ため、どちらもアーカイブから満たせない。0に読み替えると空のファイルとして
// 展開してしまう。非圧縮セグメントの格納サイズはkrkrzが読まないため範囲を問わない。
func parseSegments(segmData []byte) ([]XP3Segment, error) {
	if err := checkSegmEncodeMethods(segmData); err != nil {
		return nil, err
	}

	numSegments := len(segmData) / segmentRecordSize
	if numSegments == 0 {
		return nil, nil
	}

	segments := make([]XP3Segment, 0, numSegments)
	for i := range numSegments {
		record := segmData[i*segmentRecordSize : (i+1)*segmentRecordSize]

		segment := XP3Segment{
			IsCompressed: segmEncodeMethod(record) == segmEncodeZlib,
		}

		// why not: OffsetがsafeInt64で範囲外と判定された場合にゼロ値へ
		// フォールバックすると「オフセット0（=アーカイブヘッダー付近）から
		// セグメントの長さぶんを読む」動作になり、本来無関係なアーカイブヘッダーの
		// バイト列を展開結果に混入させてしまう。そのためOffsetが範囲外のセグメントは
		// 丸ごと破棄する（このエントリの他のセグメントには影響しない。全セグメントが
		// 破棄された場合はparseSingleEntry側のsegments判定によりエントリ自体が
		// 破棄される）。
		offset, ok := safeInt64(binary.LittleEndian.Uint64(record[4:12]))
		if !ok {
			continue
		}
		segment.Offset = offset

		// why not: 格納サイズを先に読まない。krkrz（base/XP3Archive.cpp）は+12を
		// 元サイズ、+20を格納サイズとして読み、krkrrel-ng（src/krkrrel.cpp）も
		// この順に書く。逆に読むと解凍後サイズの上限が格納サイズになり、格納サイズ
		// より大きく膨らむ圧縮セグメントが展開に失敗する。
		segment.OriginalSize, ok = safeInt64(binary.LittleEndian.Uint64(record[12:20]))
		if !ok {
			return nil, fmt.Errorf("セグメント%dの元サイズが範囲外です", i)
		}
		size, ok := safeInt64(binary.LittleEndian.Uint64(record[20:28]))
		if !ok && segment.IsCompressed {
			return nil, fmt.Errorf("セグメント%dの格納サイズが範囲外です", i)
		}
		segment.Size = size

		segments = append(segments, segment)
	}

	return segments, nil
}

// checkSegmEncodeMethods はsegmDataの28バイト単位の全レコードについて、エンコード
// 方式が0（非圧縮）か1（zlib）であることを確かめる。末尾の28バイト未満の断片は
// parseSegmentsと同じくレコードとみなさない。
func checkSegmEncodeMethods(segmData []byte) error {
	for i := range len(segmData) / segmentRecordSize {
		method := segmEncodeMethod(segmData[i*segmentRecordSize:])
		if method != segmEncodeRaw && method != segmEncodeZlib {
			return fmt.Errorf("セグメント%dのエンコード方式が不明です", i)
		}
	}

	return nil
}

// segmEncodeMethod はrecordの先頭4バイトだけを読むため、後続のレコードを含む
// スライスをそのまま渡せる。
func segmEncodeMethod(record []byte) uint32 {
	return binary.LittleEndian.Uint32(record[0:4]) & segmEncodeMethodMask
}

// readChunk はstreamからsizeバイトを読み取る。
//
// why not: sizeはXP3インデックス内の宣言値であり信頼できない。
// make([]byte, size)を素朴に呼ぶと巨大なsizeでOOM（回復不能なfatal error）を
// 起こしうる（実際に数十バイトの細工ファイルで再現する）。そのため
// stream.Len()（残りバイト数）でsizeをクランプしてから確保し、「残っている
// 分だけ読む」挙動にする。
func readChunk(stream *bytes.Reader, size uint64) []byte {
	buf := make([]byte, min(size, uint64(stream.Len()))) //nolint:gosec // bytes.Reader.Len()は常に非負
	// why not: bufの長さは残量以下へクランプ済みのため、bytes.Readerからの読み取りは
	// 失敗も不足もせず、エラーを確認する必要がない。
	_, _ = io.ReadFull(stream, buf)

	return buf
}

// skipChunk はstreamの現在位置からsizeバイト分を前方へスキップする。
//
// why not: bytes.Reader.Seek はint64変換後の絶対位置が負になる場合にエラーを
// 返すため、sizeをuint64のまま素朴にint64変換すると（math.MaxInt64超で符号が
// 反転し）Seekが失敗し、呼び出し元でエントリ全体を破棄してしまう
// （情報チャンクを先に読んでいた場合、正常に得られたはずのデータまで
// 失われる）。そのためsizeをstream.Len()でクランプし、Seekが常に成功する
// ようにして「バッファ終端で止まるだけ」という安全な挙動にする。
func skipChunk(stream *bytes.Reader, size uint64) {
	size = min(size, uint64(stream.Len())) //nolint:gosec // bytes.Reader.Len()は常に非負

	_, _ = stream.Seek(int64(size), io.SeekCurrent) //nolint:gosec // 直前にstream.Len()(int)以下へクランプ済みでint64へ安全に変換可能
}

// decodeUTF16LE はUTF-16LEバイト列をデコードする。
//
// utf16.Decodeは不正なサロゲートを置換文字に変換して継続するため、不正な
// サロゲート列を含む入力でも処理を継続できるが、正常なファイル名では
// 問題なく変換できる。
func decodeUTF16LE(data []byte) string {
	if len(data)%2 != 0 {
		return ""
	}

	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[i*2 : i*2+2])
	}

	return string(utf16.Decode(units))
}

// ListFiles はアーカイブ内のファイル一覧を取得する。
func (a *XP3Archive) ListFiles() []string {
	names := make([]string, 0, len(a.fileEntries))
	for _, entry := range a.fileEntries {
		names = append(names, entry.Name)
	}

	return names
}

// ExtractAll はすべてのファイルを指定ディレクトリに展開する。
//
// 書き出しに先立ち、索引の名前だけから次を調べる。safepath.RelPathが外を指すと
// 判定するエントリがあればErrInvalidXP3を、あるエントリの展開先が別のエントリの
// 展開先のディレクトリ部分と一致すればErrEntryPathConflictを、outputDirを作らず
// 何も書き出さずに返す。展開中にsafepath.JoinがErrOutsideBaseを返した場合も
// ErrInvalidXP3を返すが、そのときoutputDirは作成済みで、それより前のエントリは
// 書き出し済みである。
func (a *XP3Archive) ExtractAll(outputDir string) error {
	if err := a.checkEntryPaths(); err != nil {
		return err
	}

	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return fmt.Errorf("出力ディレクトリの作成に失敗しました: %w", err)
	}

	f, err := os.Open(a.archivePath) //nolint:gosec // コンストラクタでexists検証済みのユーザー指定パスを読む用途のため妥当
	if err != nil {
		return fmt.Errorf("XP3ファイルを開けません: %w", err)
	}
	defer func() { _ = f.Close() }()

	for _, entry := range a.fileEntries {
		outputPath, err := safepath.Join(outputDir, entry.Name)
		if errors.Is(err, safepath.ErrOutsideBase) {
			return fmt.Errorf("%w: %w", ErrInvalidXP3, err)
		}
		if err != nil {
			return err
		}
		if err := extractEntry(f, entry, outputPath, maxSegmentDecompressedSize); err != nil {
			return fmt.Errorf("%s: %s: %w", a.archivePath, entry.Name, err)
		}
	}

	return nil
}

// checkEntryPaths は、safepath.RelPathが外を指すと判定するエントリがあれば
// ErrInvalidXP3を、あるエントリの展開先が別のエントリの展開先のディレクトリ部分と
// 一致すればErrEntryPathConflictを返す。外を指すエントリの検査を先に済ませる。
//
// why not: エントリ名をそのまま比べない。展開先はsafepath.Joinが"\"の区切り化・
// 先頭の"/"の除去・"."や".."の解決をした位置で決まり、"/a"と"a/b"は衝突する一方、
// "a"と"a/"は同じファイルへの上書き、"a"と"a/../b"は別のファイルになる。
// 展開と食い違わないよう、Joinと同じsafepath.RelPathの結果で比べる。
//
// why not: 名前を整列して隣り合う組だけを比べない。"a"・"a-b"・"a/b"のように
// '-'（0x2D）が'/'（0x2F）より前に並ぶため、衝突する組の間に別の名前が入る。
// 全展開先を集合に入れ、各展開先についてディレクトリ部分（'/'の手前）ごとに
// 集合を引く。
func (a *XP3Archive) checkEntryPaths() error {
	keys := make([]string, len(a.fileEntries))
	for i, entry := range a.fileEntries {
		rel, err := safepath.RelPath(entry.Name)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidXP3, err)
		}
		keys[i] = foldASCIICase(rel)
	}

	files := make(map[string]string, len(keys))
	for i, key := range keys {
		if _, ok := files[key]; !ok {
			files[key] = a.fileEntries[i].Name
		}
	}

	for i, key := range keys {
		for j := range len(key) {
			if key[j] != '/' {
				continue
			}
			if file, ok := files[key[:j]]; ok {
				return fmt.Errorf("%w: %s: %q と %q", ErrEntryPathConflict, a.archivePath, file, a.fileEntries[i].Name)
			}
		}
	}

	return nil
}

// foldASCIICase はsのA-Zだけを小文字に置き換える。
//
// why not: 大文字小文字を区別して比べない。macOS既定の大文字小文字を区別しない
// ファイルシステムでは、ファイル"A"を書いた後に"a/b"のためのディレクトリ"a"を
// 作れない。区別して比べると結果が展開先のファイルシステムによって変わる。
func foldASCIICase(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}

	return string(b)
}

// PlannedOutputSize はExtractAllが書き出しうるバイト数の上限を、索引だけから
// 求めて返す。合計がint64を超える場合はmath.MaxInt64を返す。
//
// why not: アーカイブサイズに対する比率では上限を決めない。krkrrel
// （krdevui RelSettingsUnit.cpp）は内容が同一のファイルを格納し直さず、先に
// 格納したファイルのセグメント情報を複写した索引エントリを追加する。krkrzの
// tTVPXP3Archive（base/XP3Archive.cpp）もOggVorbisのコードブック共有のために
// セグメントがアーカイブ内の範囲を共有しうる前提で読む。そのため正当な
// アーカイブでもエントリサイズの合計はアーカイブサイズの何倍にもなりうる。
func (a *XP3Archive) PlannedOutputSize() int64 {
	return plannedOutputSize(a.fileEntries, a.fileSize)
}

// plannedOutputSize はentriesをExtractAllで展開したときに書き出しうる
// バイト数の上限を返す。エントリごとの上限はentryOutputLimitによる。
//
// why not: 同じ出力パスを持つエントリの上書きや、共有するバイト範囲の重複を
// 差し引かない。上限は展開の実際の書き出し量を下回ってはならず、重複を
// 数えすぎる方向の誤差しか許されないため。
func plannedOutputSize(entries []XP3FileEntry, fileSize int64) int64 {
	var total int64
	for _, entry := range entries {
		total = saturate.Add(total, entryOutputLimit(entry, fileSize, maxSegmentDecompressedSize))
	}

	return total
}

// entryOutputLimit はextractEntryがentryについて書き出しうるバイト数の上限を
// 返す。セグメントごとの上限の合計を、entryBudgetが許す生バイト（fileSize）と
// 解凍後バイト（inflatedLimit）の和で頭打ちにする。
func entryOutputLimit(entry XP3FileEntry, fileSize, inflatedLimit int64) int64 {
	var total int64
	for _, segment := range entry.Segments {
		total = saturate.Add(total, segmentOutputLimit(segment, fileSize))
	}

	return min(total, saturate.Add(fileSize, inflatedLimit))
}

// maxDeflateRatio はdeflateが入力1バイトあたりに生み出せる出力バイト数の上限。
//
// RFC 1951では1組の長さ・距離が出力できるのは最大258バイトで、その符号は
// 少なくとも2ビットを要する。リテラル/長さの符号表には長さ符号のほかに
// ブロック終端（256）が必ず含まれるため長さ符号は1ビット以上になり、距離符号も
// 1つしか使わない場合でも1ビットで符号化される（3.2.7節）。よって入力1バイト
// （8ビット）からの出力は258×4=1032バイト以下になる。zlibのヘッダー・
// adler32・非圧縮ブロックは入力を増やすだけで、この上限を超えさせない。
//
// why not: 展開側（readSegment・entryBudget）の上限と共有しない。これは
// 方針として選んだ値ではなく形式から決まる物理的な上限であり、展開時には
// 実際に解凍した量を数えられるため必要ない。見積もりだけが、解凍せずに
// 解凍後サイズを抑えるためにこの上限を使う。
const maxDeflateRatio = 1032

// segmentOutputLimit はreadSegmentがsegmentについて返しうるバイト数の上限を返す。
// 解凍するセグメントについてreadSegmentが返すのは、宣言した解凍後サイズちょうどの
// 解凍結果だけであり、それ以外はエラーになって何も書き出さない。
//
// why not: 解凍後サイズの上限をsegmentDecompressionLimitだけにしない。宣言サイズは
// アーカイブ自身の値であり、生データのmaxDeflateRatio倍を超えて膨張することはない
// ため、宣言が大きくてもそちらで抑えられる。
func segmentOutputLimit(segment XP3Segment, fileSize int64) int64 {
	raw := segmentReadLimit(segment, fileSize)
	if !segmentDecompresses(segment) {
		return raw
	}

	return min(segmentDecompressionLimit(segment), saturate.Mul(raw, maxDeflateRatio))
}

// segmentReadLimit はsegmentについてファイルから読み取れる生バイト数の上限を返す。
// 読み取る長さの宣言値をオフセット以降の実際の残量へクランプする（理由はstreamSizeの
// why not参照）。
//
// why not: 非圧縮セグメントの長さに格納サイズ（Size）を使わない。krkrzの
// tTVPXP3ArchiveStream（base/XP3Archive.cpp）は非圧縮セグメントを元サイズぶん
// ストリームから直接読み、格納サイズを参照するのは圧縮セグメントの解凍時だけである。
func segmentReadLimit(segment XP3Segment, fileSize int64) int64 {
	length := segment.OriginalSize
	if segmentDecompresses(segment) {
		length = segment.Size
	}

	return min(length, max(fileSize-segment.Offset, 0))
}

// segmentDecompresses はreadSegmentがsegmentをzlib解凍するかどうかを返す。
//
// why not: 格納サイズと元サイズが等しい圧縮セグメントを非圧縮とみなさない。
// krkrrel-ng（src/krkrrel.cpp）は圧縮結果が元より小さくならなくてもzlibの
// フラグで格納し、krkrzのtTVPXP3ArchiveStream::EnsureSegment
// （base/XP3Archive.cpp）はサイズを比べずにzlibのセグメントをすべて解凍する。
// 非圧縮とみなすとzlibストリームそのものを書き出してしまう。
func segmentDecompresses(segment XP3Segment) bool {
	return segment.IsCompressed
}

// extractEntry はentryの全セグメントを順に読み取り・解凍し、連結して
// outputPathへ書き出す。inflatedLimitはエントリ全体で解凍により生み出せる
// バイト数の上限であり、超えた場合はErrInvalidXP3を返す。失敗した場合は
// 書き出し途中のファイルを残さない。
//
// why not: 個々のセグメントはreadSegment内でfileSizeによりオフセット以降の
// 実際の残量へクランプされるが、それだけでは「同一オフセットを指す大量の
// セグメント」を積み重ねる攻撃を防げない（各セグメントは独立にfileSize近くまで
// 読めてしまうため、セグメント数×fileSizeでアロケーション総量が膨れ上がる）。
// 正当なアーカイブでは各バイトは高々1つのセグメントにしか属さないため、
// エントリ全体で読み取れる生バイト数の総量をfileSizeを上限にentryBudgetで
// 管理し、セグメントをまたいで消費させる。
//
// why not: 全セグメントを連結してから一度に書き出すと、エントリ全体を
// メモリに保持することになる。セグメントごとに書き出せば、参照し続けるのは
// 処理中の1セグメント分の生バイトと解凍結果だけで済む。
func extractEntry(f io.ReadSeeker, entry XP3FileEntry, outputPath string, inflatedLimit int64) (err error) {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	fileSize, err := streamSize(f)
	if err != nil {
		return err
	}

	out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // outputPathは呼び出し元がsafepath.Joinで出力ディレクトリ配下に制限済み
	if err != nil {
		return fmt.Errorf("ファイルの書き込みに失敗しました: %w", err)
	}
	defer func() {
		if closeErr := out.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("ファイルの書き込みに失敗しました: %w", closeErr)
		}
		if err != nil {
			_ = os.Remove(outputPath)
		}
	}()

	budget := entryBudget{raw: fileSize, inflated: inflatedLimit, inflatedLimit: inflatedLimit}
	for _, segment := range entry.Segments {
		segmentData, err := readSegment(f, segment, fileSize, &budget)
		if err != nil {
			return err
		}
		if _, err := out.Write(segmentData); err != nil {
			return fmt.Errorf("ファイルの書き込みに失敗しました: %w", err)
		}
	}

	return nil
}

// entryBudget はエントリ1件の展開で消費できる残量を表す。rawはファイルから
// 読み取れる生バイト数の残り、inflatedは解凍により生み出せるバイト数の残り、
// inflatedLimitはinflatedの初期値（エラーメッセージ用）。
//
// why not: セグメントごとの解凍上限（segmentDecompressionLimit）は1セグメント
// しか縛らず、各セグメントが自身の上限内に収まっていても合計はセグメント数に
// 応じて上限を超えうる。rawも解凍前のバイト数しか縛らず、zlibは最大圧縮で
// 約1030:1まで膨張しうるため（16MiBのゼロ列をBestCompressionで圧縮して実測）、
// 解凍後の合計はfileSizeのおよそ1000倍まで届く。そこで解凍により生み出した
// バイト数をエントリ全体で数える。上限はセグメント単体と同じ1GiBとし、
// zlib圧縮で格納された1ファイルが解凍後に1GiBを超えることはまず無いという
// 判断に基づく。
// 非圧縮で格納された生バイトはrawにより1:1でfileSize以内に縛られており膨張に
// 当たらないため、inflatedには数えない（非圧縮の大きな動画素材を締め出さない）。
type entryBudget struct {
	raw           int64
	inflated      int64
	inflatedLimit int64
}

// streamSize はfの総バイト数を返す。
//
// why not: 各セグメントのSize宣言値はアーカイブバイナリ由来で信頼できず、
// safeInt64の範囲チェックを通過していても実ファイルサイズを大幅に超える
// 値になりうる（int64範囲内の巨大値の宣言は防げない）。事前にファイル全体の
// サイズを取得しておき、readSegmentでオフセット以降の実際の残量・エントリ
// 全体の生バイトの残量（entryBudget.raw）にSizeをクランプすることで、巨大な
// Size宣言や大量セグメントの積み重ねによるmake([]byte, size)でのOOMを防ぐ
// （詳細はextractEntry・readSegmentのwhy not参照）。
func streamSize(f io.ReadSeeker) (int64, error) {
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("ファイルサイズの取得に失敗しました: %w", err)
	}

	return size, nil
}

// readSegment は1セグメント分のデータを読み取り、圧縮されていれば解凍する。
// 実際にファイルから読み取った生バイト数（解凍前）をbudget.rawから、解凍で
// 生み出したバイト数をbudget.inflatedから差し引き、エントリ全体の累積量を
// 管理する（budgetの必要性はextractEntryとentryBudgetのwhy not参照）。
//
// fileSizeで読み取る長さの宣言値をクランプする理由はstreamSizeのwhy not参照。
// 読み取る長さはsegmentReadLimit（safeInt64通過済みで非負の宣言値と、非負に
// クランプした残量の小さい方）とbudget.raw（読み取った分しか減らないため非負）の
// 小さい方であり常に非負のため、負値ガードは不要。
//
// why not: 元サイズがファイル末尾を超える非圧縮セグメントを、残りだけの短い
// データとして返さない。krkrzのtTVPXP3ArchiveStream::Read（base/XP3Archive.cpp）は
// 非圧縮セグメントをReadBufferで読み、読み取り不足はエラーになる（tjs2/tjs.cpp）。
// 圧縮セグメントは格納サイズが残量を超えても残りだけで解凍を試みる。krkrzの
// tTVPSegmentData::SetDataもReadBufferではなくReadで読み、成否をuncompressに委ねる。
//
// why not: 解凍できない圧縮セグメントを読み取った生データのまま返さない。krkrzの
// tTVPSegmentData::SetData（base/XP3Archive.cpp）は元サイズぶんの領域へ
// uncompressし、結果がZ_OKでないか解凍後の長さが元サイズと違えば
// TVPUncompressionFailedを投げる。生データを返すと壊れたzlibストリームがそのまま
// ファイルとして書き出される。
func readSegment(f io.ReadSeeker, segment XP3Segment, fileSize int64, budget *entryBudget) ([]byte, error) {
	if _, err := f.Seek(segment.Offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("セグメントオフセットへのシークに失敗しました: %w", err)
	}

	readLimit := segmentReadLimit(segment, fileSize)
	if !segmentDecompresses(segment) && readLimit < segment.OriginalSize {
		return nil, fmt.Errorf("%w: 非圧縮セグメントの元サイズ%dバイトがファイル末尾を超えています", ErrInvalidXP3, segment.OriginalSize)
	}

	buf := make([]byte, min(readLimit, budget.raw))
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, fmt.Errorf("セグメントデータの読み込みに失敗しました: %w", err)
	}
	budget.raw -= int64(len(buf))

	if segmentDecompresses(segment) {
		segmentLimit := segmentDecompressionLimit(segment)
		decompressed, err := decompressZlib(buf, min(segmentLimit, budget.inflated))
		if errors.Is(err, ErrDecompressedTooLarge) && budget.inflated < segmentLimit {
			return nil, fmt.Errorf("%w: エントリの解凍後サイズの合計が上限%dバイトを超えています: %w", ErrInvalidXP3, budget.inflatedLimit, ErrDecompressedTooLarge)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: セグメント: %w", ErrInvalidXP3, err)
		}
		if int64(len(decompressed)) != segment.OriginalSize {
			return nil, fmt.Errorf("%w: セグメントの解凍後サイズ%dバイトが宣言%dバイトと一致しません", ErrInvalidXP3, len(decompressed), segment.OriginalSize)
		}
		budget.inflated -= int64(len(decompressed))
		buf = decompressed
	}

	return buf, nil
}

// segmentDecompressionLimit はsegmentの解凍後サイズの上限を返す。
func segmentDecompressionLimit(segment XP3Segment) int64 {
	return min(segment.OriginalSize, maxSegmentDecompressedSize)
}
