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

// XP3FileEntry はXP3アーカイブ内のファイルエントリ情報を表す。
type XP3FileEntry struct {
	// Name はファイル名（パス含む）。
	Name string
	// Offset はファイルデータのオフセット。
	Offset int64
	// Size は圧縮後サイズ。
	Size int64
	// OriginalSize は元のサイズ。
	OriginalSize int64
	// IsCompressed は圧縮されているか。
	IsCompressed bool
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
// nameが取得できなかった場合（infoチャンクを欠くなど）はokにfalseを返す。
func parseSingleEntry(entryData []byte) (XP3FileEntry, bool) {
	stream := bytes.NewReader(entryData)

	var (
		name         string
		offset       int64
		size         int64
		originalSize int64
		isCompressed bool
		isEncrypted  bool
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
				// safeInt64がfalseの場合、対応フィールドは0のまま
				// （エントリ全体は破棄せず、パース可能な範囲の情報を活かす）。
				if v, ok := safeInt64(binary.LittleEndian.Uint64(infoData[4:12])); ok {
					originalSize = v
				}
				if v, ok := safeInt64(binary.LittleEndian.Uint64(infoData[12:20])); ok {
					size = v
				}
				nameLen := int(binary.LittleEndian.Uint16(infoData[20:22]))

				if len(infoData) >= 22+nameLen*2 {
					name = decodeUTF16LE(infoData[22 : 22+nameLen*2])
				}

				isEncrypted = flags&0x80000000 != 0
				isCompressed = size != originalSize
			}
		case bytes.Equal(subChunkName, []byte("segm")):
			segmData := readChunk(stream, subChunkSize)
			if len(segmData) >= 28 {
				flags := binary.LittleEndian.Uint32(segmData[0:4])
				// safeInt64がfalseの場合、対応フィールドは0のまま（infoケースと同様）。
				if v, ok := safeInt64(binary.LittleEndian.Uint64(segmData[4:12])); ok {
					offset = v
				}
				if v, ok := safeInt64(binary.LittleEndian.Uint64(segmData[12:20])); ok {
					size = v
				}
				if v, ok := safeInt64(binary.LittleEndian.Uint64(segmData[20:28])); ok {
					originalSize = v
				}
				isCompressed = flags&0x07 != 0
			}
		default:
			// adlr（Adler32チェックサム）を含む未知のサブチャンクは、既知チャンクと
			// 同じくskipChunkでスキップする（詳細はskipChunkのwhy not参照）。
			skipChunk(stream, subChunkSize)
		}
	}

	if name == "" {
		return XP3FileEntry{}, false
	}

	return XP3FileEntry{
		Name:         name,
		Offset:       offset,
		Size:         size,
		OriginalSize: originalSize,
		IsCompressed: isCompressed,
		IsEncrypted:  isEncrypted,
	}, true
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

func extractEntry(f io.ReadSeeker, entry XP3FileEntry, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	if _, err := f.Seek(entry.Offset, io.SeekStart); err != nil {
		return fmt.Errorf("ファイルオフセットへのシークに失敗しました: %w", err)
	}

	data := make([]byte, entry.Size)
	n, err := io.ReadFull(f, data)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("ファイルデータの読み込みに失敗しました: %w", err)
	}
	data = data[:n]

	if entry.IsCompressed && entry.Size != entry.OriginalSize {
		if decompressed, err := decompressZlib(data); err == nil {
			data = decompressed
		}
	}

	if err := os.WriteFile(outputPath, data, 0o600); err != nil {
		return fmt.Errorf("ファイルの書き込みに失敗しました: %w", err)
	}

	return nil
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
