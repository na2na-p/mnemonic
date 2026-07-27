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
	"strings"
	"unicode/utf16"
)

// XP3MagicTest はテストフィクスチャで使用される簡易マジックナンバー（7バイト）。
var XP3MagicTest = []byte{'X', 'P', '3', 0x0d, 0x0a, 0x1a, 0x0a}

// センチネルエラー群。
var (
	// ErrXP3NotFound はXP3ファイルが存在しない場合のエラー。
	ErrXP3NotFound = errors.New("XP3ファイルが見つかりません")
	// ErrInvalidXP3 は不正なXP3ファイル形式の場合のエラー。
	ErrInvalidXP3 = errors.New("不正なXP3ファイル形式です")
	// ErrFileNotInArchive はアーカイブ内に指定ファイルが存在しない場合のエラー。
	ErrFileNotInArchive = errors.New("アーカイブ内にファイルが見つかりません")
)

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

// TotalSize は全セグメントの元サイズ（OriginalSize）の合計を返す。
func (e XP3FileEntry) TotalSize() int64 {
	var total int64
	for _, segment := range e.Segments {
		total += segment.OriginalSize
	}

	return total
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
	if bytes.HasPrefix(data, XP3Magic) {
		return true
	}
	if bytes.HasPrefix(data, XP3MagicTest) {
		return true
	}

	return bytes.HasPrefix(data, []byte("XP3"))
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

	if len(header) < 7 || !validateXP3Magic(header) {
		return fmt.Errorf("%w: %s", ErrInvalidXP3, a.archivePath)
	}

	a.parseFileIndex(f, header)

	return nil
}

// parseFileIndex はファイルインデックスをパースする。
//
// テスト用の最小限のXP3ファイル（ヘッダー19バイト未満、または完全な
// 11バイトマジックを持たない簡易形式）の場合、Python版と同様に
// パースエラーとはせず空のファイル一覧のまま処理を終える。
func (a *XP3Archive) parseFileIndex(f io.ReadSeeker, header []byte) {
	if len(header) < 19 {
		return
	}

	if bytes.HasPrefix(header, XP3Magic) {
		a.parseStandardIndex(f, header)
	}
}

func (a *XP3Archive) parseStandardIndex(f io.ReadSeeker, header []byte) {
	infoOffset, ok := safeInt64(binary.LittleEndian.Uint64(header[11:19]))
	if !ok {
		return
	}

	if _, err := f.Seek(infoOffset, io.SeekStart); err != nil {
		return
	}

	flagByte := make([]byte, 1)
	if _, err := io.ReadFull(f, flagByte); err != nil {
		return
	}
	flag := flagByte[0]

	if flag&0x80 != 0 {
		// バージョン2: フラグの後に(table_size, table_offset)がある。
		tableSize, ok := readUint64(f)
		if !ok {
			return
		}
		tableOffset, ok := readUint64(f)
		if !ok {
			return
		}

		tableOffsetInt64, ok := safeInt64(tableOffset)
		if !ok {
			return
		}
		tableSizeInt64, ok := safeInt64(tableSize)
		if !ok {
			return
		}

		if _, err := f.Seek(tableOffsetInt64, io.SeekStart); err != nil {
			return
		}
		a.readFileTable(f, tableSizeInt64)

		return
	}

	// バージョン1: フラグの後に(compressed_size, original_size)がある。
	// original_sizeは解凍後サイズの検証に使える可能性があるが、
	// Python版と同様に現時点では読み飛ばすのみで使用しない。
	compressedSize, ok := readUint64(f)
	if !ok {
		return
	}
	if _, ok := readUint64(f); !ok {
		return
	}

	compressedSizeInt64, ok := safeInt64(compressedSize)
	if !ok {
		return
	}

	a.readFileTable(f, compressedSizeInt64)
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
// 符号が反転し負値になる（seekの意図しない巻き戻り等につながる）。
// Python版はunbounded intのためこの問題が起きないが、Go移植では
// 変換前に範囲チェックし、超過時はパース失敗として扱う。
func safeInt64(v uint64) (int64, bool) {
	if v > math.MaxInt64 {
		return 0, false
	}

	return int64(v), true //nolint:gosec // 直前のv > math.MaxInt64チェックによりオーバーフローしないことを保証済み
}

func (a *XP3Archive) readFileTable(f io.Reader, tableSize int64) {
	if tableSize < 0 {
		return
	}

	compressed := make([]byte, tableSize)
	n, err := io.ReadFull(f, compressed)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return
	}
	compressed = compressed[:n]
	if len(compressed) == 0 {
		return
	}

	tableData, err := decompressZlib(compressed)
	if err != nil {
		// 圧縮されていない場合はそのまま使用する（Python版の挙動を踏襲）。
		tableData = compressed
	}

	a.parseFileEntries(tableData)
}

func decompressZlib(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("zlib解凍に失敗しました: %w", err)
	}
	defer func() { _ = r.Close() }()

	decompressed, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("zlib解凍に失敗しました: %w", err)
	}

	return decompressed, nil
}

func (a *XP3Archive) parseFileEntries(tableData []byte) {
	stream := bytes.NewReader(tableData)

	for {
		chunkName := make([]byte, 4)
		if _, err := io.ReadFull(stream, chunkName); err != nil {
			return
		}

		if !bytes.Equal(chunkName, []byte("File")) {
			chunkSize, ok := readUint64(stream)
			if !ok {
				return
			}
			if chunkSize > uint64(len(tableData)) { //nolint:gosec // len()は非負でuint64との比較として安全
				return
			}
			skipChunk(stream, chunkSize)

			continue
		}

		chunkSize, ok := readUint64(stream)
		if !ok {
			return
		}
		if chunkSize > uint64(len(tableData)) { //nolint:gosec // len()は非負でuint64との比較として安全
			return
		}

		entryData := make([]byte, chunkSize)
		n, err := io.ReadFull(stream, entryData)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return
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
// 有効なセグメントを構成できない場合）はokにfalseを返す。Python版の
// `if name and segments:` と同じ判定条件。
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
				// ここでは読み飛ばす（Python版delta 3a17127と同じ判断）。
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
// これによりPython版の`len(segm_data) // segment_size`と同じく、末尾の
// 28バイト未満の断片は自然に無視され、宣言セグメント数がどれほど巨大でも
// 実データ長を超えて処理することはない。ただしこれは「セグメントレコードの
// パース時」に確保する[]XP3Segmentのメモリ量に関する主張に過ぎず、展開時に
// 同一オフセットを指す大量のセグメントを積み重ねる攻撃までは防げない
// （そちらの対策はextractEntryのbudgetに関するwhy not参照）。
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
		// 本来無関係なアーカイブヘッダーのバイト列を展開結果に混入させてしまう
		// （reviewer実測）。Size/OriginalSizeのゼロ値フォールバックは最悪でも
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
// why not: sizeはXP3インデックス内の宣言値であり信頼できない。Pythonの
// BytesIO.read(n)はnがどれほど大きくても実際にバッファ内に残っているバイト数
// までしか読まず安全だが、Go版でmake([]byte, size)を素朴に呼ぶと巨大な
// sizeでOOM（回復不能なfatal error）を起こしうる（実際に数十バイトの
// 細工ファイルで再現する）。そのためstream.Len()（残りバイト数）でsizeを
// クランプしてから確保し、Python版と同じ「残っている分だけ読む」挙動にする。
func readChunk(stream *bytes.Reader, size uint64) []byte {
	remaining := stream.Len()
	if remaining < 0 {
		remaining = 0
	}
	if size > uint64(remaining) { //nolint:gosec // remainingはbytes.Reader.Len()の戻り値で常に非負
		size = uint64(remaining)
	}

	buf := make([]byte, size)
	n, err := io.ReadFull(stream, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil
	}

	return buf[:n]
}

// skipChunk はstreamの現在位置からsizeバイト分を前方へスキップする。
//
// why not: Pythonの BytesIO.seek(size, 1) はsizeがどれほど大きくても
// バッファ終端を越えて（エラーにならず）位置を進めるだけであり、次の
// stream.read(4) が空を返してループが自然終了するだけで済む。一方
// bytes.Reader.Seek はint64変換後の絶対位置が負になる場合にエラーを返す
// ため、sizeをuint64のまま素朴にint64変換すると（math.MaxInt64超で符号が
// 反転し）Seekが失敗し、呼び出し元でエントリ全体を破棄してしまう
// （情報チャンクを先に読んでいた場合、正常に得られたはずのデータまで
// 失われる）。そのためsizeをstream.Len()でクランプし、Seekが常に成功する
// ようにしてPython版の「バッファ終端で止まるだけ」という挙動に合わせる。
func skipChunk(stream *bytes.Reader, size uint64) {
	remaining := stream.Len()
	if remaining < 0 {
		remaining = 0
	}
	if size > uint64(remaining) { //nolint:gosec // remainingはbytes.Reader.Len()の戻り値で常に非負
		size = uint64(remaining)
	}

	_, _ = stream.Seek(int64(size), io.SeekCurrent) //nolint:gosec // 直前にremaining(int)以下へクランプ済みでint64へ安全に変換可能
}

// decodeUTF16LE はUTF-16LEバイト列をデコードする。
//
// Pythonのdecode("utf-16-le")は不正なサロゲート列で例外を送出し空文字列に
// フォールバックする。utf16.Decodeは不正なサロゲートを置換文字に変換して
// 継続するため完全に同一の挙動ではないが、正常なファイル名では結果が一致する。
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
		outputPath, err := safeJoin(outputDir, entry.Name)
		if err != nil {
			return err
		}
		if err := extractEntry(f, entry, outputPath); err != nil {
			return err
		}
	}

	return nil
}

// ExtractFile は指定ファイルを展開する。
//
// アーカイブ内に該当ファイルが存在しない場合はErrFileNotInArchiveを返す。
func (a *XP3Archive) ExtractFile(filename, outputPath string) error {
	entry, ok := a.findEntry(filename)
	if !ok {
		return fmt.Errorf("%w: %s", ErrFileNotInArchive, filename)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	f, err := os.Open(a.archivePath) //nolint:gosec // コンストラクタでexists検証済みのユーザー指定パスを読む用途のため妥当
	if err != nil {
		return fmt.Errorf("XP3ファイルを開けません: %w", err)
	}
	defer func() { _ = f.Close() }()

	return extractEntry(f, entry, outputPath)
}

func (a *XP3Archive) findEntry(filename string) (XP3FileEntry, bool) {
	for _, entry := range a.fileEntries {
		if entry.Name == filename {
			return entry, true
		}
	}

	normalized := strings.ReplaceAll(filename, `\`, "/")
	for _, entry := range a.fileEntries {
		if strings.ReplaceAll(entry.Name, `\`, "/") == normalized {
			return entry, true
		}
	}

	return XP3FileEntry{}, false
}

// extractEntry はentryの全セグメントを順に読み取り・解凍し、連結して
// outputPathへ書き出す（Python版delta 3a17127の複数セグメント連結展開に相当）。
//
// why not: 個々のセグメントはreadSegment内でfileSizeによりオフセット以降の
// 実際の残量へクランプされるが、それだけでは「同一オフセットを指す大量の
// セグメント」を積み重ねる攻撃を防げない（各セグメントは独立にfileSize近くまで
// 読めてしまうため、セグメント数×fileSizeでアロケーション総量が膨れ上がる。
// reviewer実測: 52KBの細工アーカイブ・同一オフセットのセグメント20,000件で
// 5.1GB RSS）。正当なアーカイブでは各バイトは高々1つのセグメントにしか
// 属さないため、エントリ全体で読み取れる生バイト数の総量をbudgetとして
// fileSizeを上限に管理し、セグメントをまたいで消費させる。
func extractEntry(f io.ReadSeeker, entry XP3FileEntry, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	fileSize, err := streamSize(f)
	if err != nil {
		return err
	}

	budget := fileSize

	var data []byte
	for _, segment := range entry.Segments {
		segmentData, consumed, err := readSegment(f, segment, fileSize, budget)
		if err != nil {
			return err
		}
		budget -= consumed
		data = append(data, segmentData...)
	}

	if err := os.WriteFile(outputPath, data, 0o600); err != nil {
		return fmt.Errorf("ファイルの書き込みに失敗しました: %w", err)
	}

	return nil
}

// streamSize はfの総バイト数を返す。
//
// why not: 各セグメントのSize宣言値はアーカイブバイナリ由来で信頼できず、
// safeInt64の範囲チェックを通過していても実ファイルサイズを大幅に超える
// 値になりうる（int64範囲内の巨大値の宣言は防げない）。事前にファイル全体の
// サイズを取得しておき、readSegmentでオフセット以降の実際の残量・エントリ
// 全体のbudgetにSizeをクランプすることで、巨大なSize宣言や大量セグメントの
// 積み重ねによるmake([]byte, size)でのOOMを防ぐ（詳細はextractEntry・
// readSegmentのwhy not参照）。
func streamSize(f io.ReadSeeker) (int64, error) {
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("ファイルサイズの取得に失敗しました: %w", err)
	}

	return size, nil
}

// readSegment は1セグメント分のデータを読み取り、圧縮されていれば解凍する。
// 戻り値のconsumedは実際にファイルから読み取った生バイト数（解凍前）であり、
// 呼び出し元はこれをbudgetから差し引いてエントリ全体の累積読み取り量を管理する
// （budgetの必要性はextractEntryのwhy not参照）。
//
// fileSizeでセグメントのSize宣言値をクランプする理由はstreamSizeのwhy not参照。
// readSizeはsegment.Size（safeInt64通過済みで非負）とremaining（fileSize由来で
// 非負にクランプ済み）の小さい方であり常に非負のため、負値ガードは不要。
func readSegment(f io.ReadSeeker, segment XP3Segment, fileSize, budget int64) ([]byte, int64, error) {
	if _, err := f.Seek(segment.Offset, io.SeekStart); err != nil {
		return nil, 0, fmt.Errorf("セグメントオフセットへのシークに失敗しました: %w", err)
	}

	remaining := fileSize - segment.Offset
	if remaining < 0 {
		remaining = 0
	}
	if remaining > budget {
		remaining = budget
	}

	readSize := segment.Size
	if readSize > remaining {
		readSize = remaining
	}

	buf := make([]byte, readSize)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, 0, fmt.Errorf("セグメントデータの読み込みに失敗しました: %w", err)
	}
	buf = buf[:n]
	consumed := int64(n)

	if segment.IsCompressed && segment.Size != segment.OriginalSize {
		if decompressed, err := decompressZlib(buf); err == nil {
			buf = decompressed
		}
	}

	return buf, consumed, nil
}

// safeJoin はbaseDir配下にentryNameを結合する。
//
// why not: Python版はPath結合をそのまま行いパストラバーサル対策をしていないが、
// エントリ名はアーカイブ内データに由来し外部入力として信頼できないため、
// Go移植では展開先がbaseDir外に脱出しないことを検証する（zip slip対策）。
//
// もう1点、Python版との既知の差分として、entryName中の"\"はここで"/"へ
// 正規化してからOS区切り文字へ変換するためディレクトリ階層として扱われる。
// Python版（PurePosixPath上でのパス結合）では"\"はパス区切りとして解釈
// されず、"data\script.ks"のような名前のエントリはリテラルに"\"を含む
// 単一ファイル名として書き出される。XP3アーカイブ内のエントリ名はWindows
// 由来で"\"区切りのケースが実際にあり得るため、Go移植ではこちらを正規化する
// 挙動を意図的に選んでいる。
func safeJoin(baseDir, entryName string) (string, error) {
	cleanedName := filepath.FromSlash(strings.ReplaceAll(entryName, `\`, "/"))
	joined := filepath.Join(baseDir, cleanedName)

	base, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("出力ディレクトリの絶対パス解決に失敗しました: %w", err)
	}
	target, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("展開先パスの絶対パス解決に失敗しました: %w", err)
	}

	if target != base && !strings.HasPrefix(target, base+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: 展開先が出力ディレクトリの外を指しています: %s", ErrInvalidXP3, entryName)
	}

	return target, nil
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
// 不正なXP3ファイル形式でパースに失敗した場合は、Python版と同様に
// 暗号化されていないとみなしたEncryptionInfoを返す（エラーにしない）。
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
