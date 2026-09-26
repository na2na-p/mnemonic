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
)

// センチネルエラー群。
var (
	// ErrXP3NotFound はXP3ファイルが存在しない場合のエラー。
	ErrXP3NotFound = errors.New("XP3ファイルが見つかりません")
	// ErrInvalidXP3 は不正なXP3ファイル形式の場合のエラー。
	ErrInvalidXP3 = errors.New("不正なXP3ファイル形式です")
	// ErrDecompressedTooLarge はzlib解凍結果が許容サイズを超えた場合のエラー。
	ErrDecompressedTooLarge = errors.New("zlib解凍結果が許容サイズを超えています")
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

// EncryptionType は検出可能な暗号化タイプを表す。
//
// XP3アーカイブで使用される暗号化方式を表す。
type EncryptionType string

// EncryptionTypeの各値。
const (
	// EncryptionNone は暗号化なしを表す。
	EncryptionNone EncryptionType = "none"
	// EncryptionSimpleXOR は単純なXOR暗号化を表す。
	EncryptionSimpleXOR EncryptionType = "simple_xor"
	// EncryptionCustom はカスタム暗号化（ゲーム固有の実装）を表す。
	EncryptionCustom EncryptionType = "custom"
	// EncryptionUnknown は未知の暗号化方式を表す。
	EncryptionUnknown EncryptionType = "unknown"
)

// EncryptionInfo は暗号化情報を保持する。
//
// XP3アーカイブの暗号化状態に関する情報を格納する不変値。
type EncryptionInfo struct {
	// IsEncrypted は暗号化されているかどうか。
	IsEncrypted bool
	// EncryptionType は検出された暗号化タイプ。
	EncryptionType EncryptionType
	// Details は暗号化に関する追加情報（未設定の場合は空文字列）。
	Details string
}

// XP3EncryptionError はXP3が暗号化されている場合に返されるエラー。
//
// 暗号化情報を保持し、エラーメッセージとして詳細を提供する。
type XP3EncryptionError struct {
	// Info は検出された暗号化情報。
	Info EncryptionInfo
}

// Error はエラーメッセージを返す。
func (e *XP3EncryptionError) Error() string {
	message := fmt.Sprintf("XP3アーカイブは暗号化されています (タイプ: %s)", e.Info.EncryptionType)
	if e.Info.Details != "" {
		return fmt.Sprintf("%s: %s", message, e.Info.Details)
	}

	return message
}

// XP3Segment はXP3ファイルセグメント情報を表す。
//
// XP3アーカイブ内のファイルは複数のセグメントに分割されている場合がある。
// 各セグメントは異なるオフセットに配置され、個別に圧縮される可能性がある。
type XP3Segment struct {
	// Offset はセグメントデータのオフセット。
	Offset int64
	// Size は圧縮後サイズ。
	Size int64
	// OriginalSize は元のサイズ。
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
	// IsEncrypted は暗号化されているか。
	IsEncrypted bool
}

// XP3Archive はXP3アーカイブを操作する。
//
// 吉里吉里/KAG形式のXP3アーカイブファイルを開き、
// 内包されているファイルの一覧取得や展開を行う。
type XP3Archive struct {
	archivePath string
	fileEntries []XP3FileEntry
	isEncrypted bool
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
// インデックスオフセットがファイル末尾ちょうどを指す場合はインデックスを持たない
// 空のアーカイブとして扱い、末尾より先を指す場合は壊れたアーカイブとして扱う。
// why not: 末尾より先へのSeekは成功し、フラグの読み取りはどちらでもio.EOFになる
// ため、読み取りエラーからは両者を区別できない。そのためファイルサイズと比較する。
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
	if indexOffset > fileSize {
		return a.invalidIndexError("インデックスオフセットがファイル末尾を超えています")
	}
	if indexOffset == fileSize {
		return nil
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

		// why not: ヘッダーのインデックスオフセットと違い、ファイル末尾ちょうどを
		// 空のアーカイブとして扱わない。継続フラグは次のインデックスがあることを
		// 示すため、末尾を指すのは途切れたアーカイブである。
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

		entryData := make([]byte, chunkSize)
		n, err := io.ReadFull(stream, entryData)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return a.invalidIndexError("Fileチャンクを読み取れません")
		}
		entryData = entryData[:n]

		if entry, ok := parseSingleEntry(entryData); ok {
			a.fileEntries = append(a.fileEntries, entry)
			if entry.IsEncrypted {
				a.isEncrypted = true
			}
		}
	}
}

// parseSingleEntry は単一のファイルエントリをパースする。
//
// nameが取得できなかった場合（infoチャンクを欠くなど）、またはセグメントを
// 1つも持てなかった場合（segmチャンクを欠く、あるいは28バイト未満で
// 有効なセグメントを構成できない場合）はokにfalseを返す。
func parseSingleEntry(entryData []byte) (XP3FileEntry, bool) {
	stream := bytes.NewReader(entryData)

	var (
		name        string
		segments    []XP3Segment
		isEncrypted bool
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

		switch {
		case bytes.Equal(subChunkName, []byte("info")):
			infoData := readChunk(stream, subChunkSize)
			if len(infoData) >= 22 {
				flags := binary.LittleEndian.Uint32(infoData[0:4])
				// original_size, sizeはsegmチャンク側の各セグメントが持つため、
				// ここでは読み飛ばす。
				nameLen := int(binary.LittleEndian.Uint16(infoData[20:22]))

				if len(infoData) >= 22+nameLen*2 {
					name = decodeUTF16LE(infoData[22 : 22+nameLen*2])
				}

				isEncrypted = flags&0x80000000 != 0
			}
		case bytes.Equal(subChunkName, []byte("segm")):
			segmData := readChunk(stream, subChunkSize)
			segments = append(segments, parseSegments(segmData)...)
		default:
			// adlr（Adler32チェックサム）を含む未知のサブチャンクは、既知チャンクと
			// 同じくskipChunkでスキップする（詳細はskipChunkのwhy not参照）。
			skipChunk(stream, subChunkSize)
		}
	}

	if name == "" || len(segments) == 0 {
		return XP3FileEntry{}, false
	}

	return XP3FileEntry{
		Name:        name,
		Segments:    segments,
		IsEncrypted: isEncrypted,
	}, true
}

// parseSegments はsegmサブチャンクのデータを28バイト単位のセグメント列としてパースする。
//
// why not: 宣言されたsubChunkSizeではなく、実際に読み取れたsegmData
// （readChunkでstream残量にクランプ済み）の長さを28で割った件数だけを対象にする。
// これにより末尾の28バイト未満の断片は自然に無視され、宣言セグメント数が
// どれほど巨大でも実データ長を超えて処理することはない。ただしこれは
// 「セグメントレコードのパース時」に確保する[]XP3Segmentのメモリ量に関する
// 主張に過ぎず、展開時に同一オフセットを指す大量のセグメントを積み重ねる
// 攻撃までは防げない（そちらの対策はextractEntryのwhy not参照）。
func parseSegments(segmData []byte) []XP3Segment {
	const segmentRecordSize = 28

	numSegments := len(segmData) / segmentRecordSize
	if numSegments == 0 {
		return nil
	}

	segments := make([]XP3Segment, 0, numSegments)
	for i := range numSegments {
		record := segmData[i*segmentRecordSize : (i+1)*segmentRecordSize]

		// why not: OffsetがsafeInt64で範囲外と判定された場合、Size/OriginalSize
		// と同じくゼロ値へフォールバックすると「オフセット0（=アーカイブヘッダー
		// 付近）からSize/OriginalSizeで示されるバイト数を読む」動作になり、
		// 本来無関係なアーカイブヘッダーのバイト列を展開結果に混入させてしまう。
		// Size/OriginalSizeのゼロ値フォールバックは最悪でも
		// 「空読みになるだけ」で実害がないが、Offsetは読み取り位置そのものを
		// 決めるため同列には扱えない。そのためOffsetが範囲外のセグメントは
		// 丸ごと破棄する（このエントリの他のセグメントには影響しない。全セグ
		// メントが破棄された場合はparseSingleEntry側のname/segments判定により
		// エントリ自体が破棄される）。
		offset, ok := safeInt64(binary.LittleEndian.Uint64(record[4:12]))
		if !ok {
			continue
		}

		var segment XP3Segment
		segment.Offset = offset

		flags := binary.LittleEndian.Uint32(record[0:4])
		// safeInt64がfalseの場合、対応フィールドはゼロ値のまま
		// （セグメント自体は破棄せず、パース可能な範囲の情報を活かす）。
		if v, ok := safeInt64(binary.LittleEndian.Uint64(record[12:20])); ok {
			segment.Size = v
		}
		if v, ok := safeInt64(binary.LittleEndian.Uint64(record[20:28])); ok {
			segment.OriginalSize = v
		}
		segment.IsCompressed = flags&0x07 != 0

		segments = append(segments, segment)
	}

	return segments
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
func (a *XP3Archive) ExtractAll(outputDir string) error {
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

// extractEntry はentryの全セグメントを順に読み取り・解凍し、連結して
// outputPathへ書き出す。inflatedLimitはエントリ全体で解凍により生み出せる
// バイト数の上限であり、超えた場合はErrInvalidXP3を返す。失敗した場合は
// 書き出し途中のファイルを残さない。
//
// why not: 個々のセグメントはreadSegment内でfileSizeによりオフセット以降の
// 実際の残量へクランプされるが、それだけでは「同一オフセットを指す大量の
// セグメント」を積み重ねる攻撃を防げない（各セグメントは独立にfileSize近くまで
// 読めてしまうため、セグメント数×fileSizeでアロケーション総量が膨れ上がる。
// 52KBの細工アーカイブ・同一オフセットのセグメント20,000件で5.1GB RSSに
// 達することを確認済み）。正当なアーカイブでは各バイトは高々1つのセグメントに
// しか属さないため、エントリ全体で読み取れる生バイト数の総量をfileSizeを上限に
// entryBudgetで管理し、セグメントをまたいで消費させる。
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
// fileSizeでセグメントのSize宣言値をクランプする理由はstreamSizeのwhy not参照。
// readSizeはsegment.Size（safeInt64通過済みで非負）とremaining（fileSize由来で
// 非負にクランプ済み）の小さい方であり常に非負のため、負値ガードは不要。
func readSegment(f io.ReadSeeker, segment XP3Segment, fileSize int64, budget *entryBudget) ([]byte, error) {
	if _, err := f.Seek(segment.Offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("セグメントオフセットへのシークに失敗しました: %w", err)
	}

	remaining := min(max(fileSize-segment.Offset, 0), budget.raw)

	readSize := min(segment.Size, remaining)

	buf := make([]byte, readSize)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("セグメントデータの読み込みに失敗しました: %w", err)
	}
	buf = buf[:n]
	budget.raw -= int64(n)

	if segment.IsCompressed && segment.Size != segment.OriginalSize {
		segmentLimit := segmentDecompressionLimit(segment)
		decompressed, err := decompressZlib(buf, min(segmentLimit, budget.inflated))
		if errors.Is(err, ErrDecompressedTooLarge) {
			if budget.inflated < segmentLimit {
				return nil, fmt.Errorf("%w: エントリの解凍後サイズの合計が上限%dバイトを超えています: %w", ErrInvalidXP3, budget.inflatedLimit, ErrDecompressedTooLarge)
			}
			return nil, fmt.Errorf("%w: セグメント: %w", ErrInvalidXP3, err)
		}
		if err == nil {
			budget.inflated -= int64(len(decompressed))
			buf = decompressed
		}
	}

	return buf, nil
}

// segmentDecompressionLimit はsegmentの解凍後サイズの上限を返す。
//
// why not: OriginalSizeはsafeInt64で範囲外と判定されるとゼロ値になり、宣言値が0の
// 場合と区別できない。0を上限にすると範囲外を宣言したセグメントが一律に失敗するため、
// 0の場合はファイルテーブルと同じ上限へフォールバックする。
func segmentDecompressionLimit(segment XP3Segment) int64 {
	if segment.OriginalSize > 0 {
		return min(segment.OriginalSize, maxSegmentDecompressedSize)
	}

	return maxFileTableSize
}

// IsEncrypted は暗号化されているかを判定する。
func (a *XP3Archive) IsEncrypted() bool {
	return a.isEncrypted
}

// XP3EncryptionChecker はXP3ファイルの暗号化をチェックする。
//
// XP3アーカイブファイルを解析し、暗号化されているかどうかを判定する。
type XP3EncryptionChecker struct {
	archivePath string
}

// NewXP3EncryptionChecker はarchivePathを対象に初期化する。
func NewXP3EncryptionChecker(archivePath string) *XP3EncryptionChecker {
	return &XP3EncryptionChecker{archivePath: archivePath}
}

// Check は暗号化状態をチェックして返す。
//
// アーカイブファイルが存在しない場合はErrXP3NotFoundを返す。
// 不正なXP3ファイル形式でパースに失敗した場合は、暗号化されていないとみなした
// EncryptionInfoを返す（エラーにしない）。
func (c *XP3EncryptionChecker) Check() (EncryptionInfo, error) {
	if _, err := os.Stat(c.archivePath); err != nil {
		return EncryptionInfo{}, fmt.Errorf("%w: %s", ErrXP3NotFound, c.archivePath)
	}

	archive, err := NewXP3Archive(c.archivePath)
	if err != nil {
		if errors.Is(err, ErrInvalidXP3) {
			return EncryptionInfo{IsEncrypted: false, EncryptionType: EncryptionNone}, nil
		}

		return EncryptionInfo{}, err
	}

	if archive.IsEncrypted() {
		return EncryptionInfo{
			IsEncrypted:    true,
			EncryptionType: EncryptionUnknown,
			Details:        "アーカイブ内のファイルが暗号化されています",
		}, nil
	}

	return EncryptionInfo{IsEncrypted: false, EncryptionType: EncryptionNone}, nil
}

// RaiseIfEncrypted は暗号化されている場合にXP3EncryptionErrorを返す。
func (c *XP3EncryptionChecker) RaiseIfEncrypted() error {
	info, err := c.Check()
	if err != nil {
		return err
	}

	if info.IsEncrypted {
		return &XP3EncryptionError{Info: info}
	}

	return nil
}
