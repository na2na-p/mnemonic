package parser_test

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/parser"
)

// minimalXP3Bytes はPython版テストで使われる最小限のXP3ファイル
// （簡易マジック + パディング、ファイルインデックスは持たない）を返す。
func minimalXP3Bytes() []byte {
	return []byte{'X', 'P', '3', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00}
}

func TestEncryptionType_Values(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		encryptionType parser.EncryptionType
		expected       string
	}{
		"正常系: NONE型の値":       {parser.EncryptionNone, "none"},
		"正常系: SIMPLE_XOR型の値": {parser.EncryptionSimpleXOR, "simple_xor"},
		"正常系: CUSTOM型の値":     {parser.EncryptionCustom, "custom"},
		"正常系: UNKNOWN型の値":    {parser.EncryptionUnknown, "unknown"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, string(tc.encryptionType))
		})
	}
}

func TestEncryptionInfo_Creation(t *testing.T) {
	t.Parallel()

	cases := map[string]parser.EncryptionInfo{
		"正常系: 非暗号化・詳細なし": {
			IsEncrypted: false, EncryptionType: parser.EncryptionNone, Details: "",
		},
		"正常系: XOR暗号化・詳細あり": {
			IsEncrypted: true, EncryptionType: parser.EncryptionSimpleXOR, Details: "XORキー: 0xFF",
		},
		"正常系: カスタム暗号化・詳細あり": {
			IsEncrypted: true, EncryptionType: parser.EncryptionCustom, Details: "カスタム暗号化検出",
		},
		"正常系: 未知の暗号化・詳細なし": {
			IsEncrypted: true, EncryptionType: parser.EncryptionUnknown, Details: "",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			info := parser.EncryptionInfo{
				IsEncrypted:    tc.IsEncrypted,
				EncryptionType: tc.EncryptionType,
				Details:        tc.Details,
			}

			assert.Equal(t, tc.IsEncrypted, info.IsEncrypted)
			assert.Equal(t, tc.EncryptionType, info.EncryptionType)
			assert.Equal(t, tc.Details, info.Details)
		})
	}
}

func TestXP3EncryptionError(t *testing.T) {
	t.Parallel()

	t.Run("正常系: エラーメッセージに暗号化タイプが含まれる", func(t *testing.T) {
		t.Parallel()

		info := parser.EncryptionInfo{
			IsEncrypted:    true,
			EncryptionType: parser.EncryptionSimpleXOR,
		}
		err := &parser.XP3EncryptionError{Info: info}

		assert.Equal(t, info, err.Info)
		assert.Contains(t, err.Error(), "暗号化されています")
		assert.Contains(t, err.Error(), "simple_xor")
	})

	t.Run("正常系: 詳細情報を含むエラーメッセージ", func(t *testing.T) {
		t.Parallel()

		info := parser.EncryptionInfo{
			IsEncrypted:    true,
			EncryptionType: parser.EncryptionCustom,
			Details:        "特殊な暗号化方式",
		}
		err := &parser.XP3EncryptionError{Info: info}

		assert.Contains(t, err.Error(), "特殊な暗号化方式")
	})

	t.Run("正常系: errorインターフェースを満たす", func(t *testing.T) {
		t.Parallel()

		var err error = &parser.XP3EncryptionError{Info: parser.EncryptionInfo{}}
		assert.Error(t, err)
	})
}

func TestNewXP3Archive(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 有効なXP3ファイルを開ける", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "valid.xp3")
		writeFile(t, path, minimalXP3Bytes())

		archive, err := parser.NewXP3Archive(path)

		require.NoError(t, err)
		assert.NotNil(t, archive)
	})

	t.Run("異常系: 存在しないファイルでErrXP3NotFound", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "nonexistent.xp3")

		_, err := parser.NewXP3Archive(path)

		require.ErrorIs(t, err, parser.ErrXP3NotFound)
	})

	t.Run("異常系: 不正なXP3ファイルでErrInvalidXP3", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "invalid.xp3")
		writeFile(t, path, []byte("NOT_XP3_FILE"))

		_, err := parser.NewXP3Archive(path)

		require.ErrorIs(t, err, parser.ErrInvalidXP3)
	})
}

func TestXP3Archive_ListFiles(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 最小限のXP3ファイルで空のファイル一覧を返す", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "test.xp3")
		writeFile(t, path, minimalXP3Bytes())

		archive, err := parser.NewXP3Archive(path)
		require.NoError(t, err)

		files := archive.ListFiles()

		assert.Empty(t, files)
	})
}

func TestXP3Archive_ExtractAll(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 空のアーカイブでも出力ディレクトリを作成する", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "valid.xp3")
		writeFile(t, path, minimalXP3Bytes())
		outputDir := filepath.Join(tmpDir, "output")

		archive, err := parser.NewXP3Archive(path)
		require.NoError(t, err)

		err = archive.ExtractAll(outputDir)

		require.NoError(t, err)
		assert.DirExists(t, outputDir)
	})
}

func TestXP3Archive_ExtractFile(t *testing.T) {
	t.Parallel()

	cases := []string{
		"data/script.ks",
		"image/bg/title.png",
		"sound/bgm/main.ogg",
	}

	for _, filename := range cases {
		t.Run("異常系: 存在しないファイル "+filename, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "test.xp3")
			writeFile(t, path, minimalXP3Bytes())
			outputPath := filepath.Join(tmpDir, "output", filepath.Base(filename))

			archive, err := parser.NewXP3Archive(path)
			require.NoError(t, err)

			err = archive.ExtractFile(filename, outputPath)

			require.ErrorIs(t, err, parser.ErrFileNotInArchive)
		})
	}
}

func TestXP3Archive_IsEncrypted(t *testing.T) {
	t.Parallel()

	t.Run("正常系: テスト用ファイルは暗号化されていない", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "valid.xp3")
		writeFile(t, path, minimalXP3Bytes())

		archive, err := parser.NewXP3Archive(path)
		require.NoError(t, err)

		assert.False(t, archive.IsEncrypted())
	})
}

func TestXP3EncryptionChecker_Check(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 非暗号化XP3を正しく判定できる", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "valid.xp3")
		writeFile(t, path, minimalXP3Bytes())

		checker := parser.NewXP3EncryptionChecker(path)
		result, err := checker.Check()

		require.NoError(t, err)
		assert.False(t, result.IsEncrypted)
		assert.Equal(t, parser.EncryptionNone, result.EncryptionType)
	})

	t.Run("異常系: 存在しないファイルでErrXP3NotFound", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "nonexistent.xp3")

		checker := parser.NewXP3EncryptionChecker(path)
		_, err := checker.Check()

		require.ErrorIs(t, err, parser.ErrXP3NotFound)
	})

	t.Run("正常系: 不正なXP3ファイルはパース失敗として非暗号化扱いになる", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "invalid.xp3")
		writeFile(t, path, append([]byte("NOT_AN_XP3_FILE"), make([]byte, 100)...))

		checker := parser.NewXP3EncryptionChecker(path)
		result, err := checker.Check()

		require.NoError(t, err)
		assert.False(t, result.IsEncrypted)
		assert.Equal(t, parser.EncryptionNone, result.EncryptionType)
	})
}

func TestXP3EncryptionChecker_RaiseIfEncrypted(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 非暗号化XP3では例外を発生させない", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "valid.xp3")
		writeFile(t, path, minimalXP3Bytes())

		checker := parser.NewXP3EncryptionChecker(path)

		assert.NoError(t, checker.RaiseIfEncrypted())
	})

	t.Run("異常系: 存在しないファイルでErrXP3NotFound", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "nonexistent.xp3")

		checker := parser.NewXP3EncryptionChecker(path)

		err := checker.RaiseIfEncrypted()

		require.ErrorIs(t, err, parser.ErrXP3NotFound)
	})
}

// --- 標準インデックス（バージョン1）を持つ実XP3アーカイブのビルダー ---
//
// 単一セグメントの往復検証（構築→解析→展開→内容一致）に関しては、Python版の
// テストスイートには対応するフィクスチャヘルパーが存在しない（簡易マジック
// のみの最小ファイルで検証している）。しかしXP3Archiveの本体ロジック
// （zlib解凍・チャンク解析・オフセット算出）はこのパスを通らないと一切
// 検証されないため、Go移植では独自にビルダーを追加している。
// 複数セグメント関連（xp3SegmentSpec.segments経由）についても、同様の
// 理由からこのビルダーへ統合した。
//
// xp3SegmentSpec は1セグメント分の実データと圧縮有無を表す。
// compressFlagがtrueの場合、dataはzlib圧縮した上でデータ領域に書き込まれる
// （segmチャンクのsize/original_sizeは圧縮後/圧縮前の実サイズを反映する）。
type xp3SegmentSpec struct {
	data         []byte
	compressFlag bool
}

type xp3EntrySpec struct {
	name         string
	data         []byte
	encryptFlag  bool
	compressFlag bool
	// segments を指定すると複数セグメントの明示的なレイアウトを構築できる。
	// 空の場合はdata/compressFlagから単一セグメントのエントリを合成する
	// （既存の単一セグメントテストとの後方互換のため）。
	segments []xp3SegmentSpec
}

// resolvedSegments はentryが表す実効的なセグメント列を返す。
func (e xp3EntrySpec) resolvedSegments() []xp3SegmentSpec {
	if len(e.segments) > 0 {
		return e.segments
	}

	return []xp3SegmentSpec{{data: e.data, compressFlag: e.compressFlag}}
}

// totalOriginalSize は全セグメントの元サイズ合計を返す（infoチャンク用）。
func (e xp3EntrySpec) totalOriginalSize() uint64 {
	var total uint64
	for _, seg := range e.resolvedSegments() {
		total += uint64(len(seg.data)) //nolint:gosec // テストヘルパーであり非負であることが既知
	}

	return total
}

// buildXP3Archive はバージョン1形式（フラグ直後にcompressed_size/original_size
// が続く形式）のXP3アーカイブをバイト列として構築する。
// 各エントリはresolvedSegments()が返す1つ以上のセグメントとしてデータ領域に
// 順に配置され、segmチャンクにも同じ順でセグメントレコードが書き込まれる
// （Python版delta 3a17127の複数segmレコード対応と対をなすテストフィクスチャ）。
func buildXP3Archive(t *testing.T, entries []xp3EntrySpec) []byte {
	t.Helper()

	// データ領域: ヘッダーの直後に各エントリの各セグメントの実データ
	// （圧縮対象なら圧縮後バイト列）を連結配置する。
	const headerSize = 19 // 11(magic) + 8(info_offset)

	type placedSegment struct {
		offset       int64
		size         int64
		originalSize uint64
		compressFlag bool
	}

	var dataSection bytes.Buffer
	entrySegments := make([][]placedSegment, len(entries))
	for i, e := range entries {
		for _, seg := range e.resolvedSegments() {
			offset := headerSize + int64(dataSection.Len())

			payload := seg.data
			if seg.compressFlag {
				payload = compressZlib(t, seg.data)
			}
			dataSection.Write(payload)

			entrySegments[i] = append(entrySegments[i], placedSegment{
				offset:       offset,
				size:         int64(len(payload)),
				originalSize: uint64(len(seg.data)), //nolint:gosec // テストヘルパーであり非負であることが既知
				compressFlag: seg.compressFlag,
			})
		}
	}

	// ファイルテーブル（"File"チャンクの列）を構築する。
	var table bytes.Buffer
	for i, e := range entries {
		var entryBody bytes.Buffer

		// infoサブチャンク（original_size/sizeはPython版delta 3a17127以降
		// 読み飛ばされる値のため、合計値をそのまま書けば十分）。
		var info bytes.Buffer
		var flags uint32
		if e.encryptFlag {
			flags |= 0x80000000
		}
		nameUTF16 := utf16.Encode([]rune(e.name))
		totalOriginalSize := e.totalOriginalSize()
		writeUint32(&info, flags)
		writeUint64(&info, totalOriginalSize)
		writeUint64(&info, totalOriginalSize)
		writeUint16(&info, uint16(len(nameUTF16))) //nolint:gosec // テストヘルパーであり名前長は既知の小さい値
		for _, u := range nameUTF16 {
			writeUint16(&info, u)
		}
		writeChunkHeader(&entryBody, "info", info.Bytes())

		// segmサブチャンク（セグメントごとに28バイトのレコードを連結）。
		// フラグは0x07（Python版delta 3a17127のテストフィクスチャ
		// `flags = 0x07 if is_comp else 0x00`に合わせたマルチビット値）。
		// パース側の判定はflags&0x07 != 0のため0x01でも判定結果は同じだが、
		// Python版フィクスチャとバイト列レベルで一致させる。
		var segm bytes.Buffer
		for _, seg := range entrySegments[i] {
			var segmFlags uint32
			if seg.compressFlag {
				segmFlags |= 0x07
			}
			writeUint32(&segm, segmFlags)
			writeUint64(&segm, uint64(seg.offset)) //nolint:gosec // テストヘルパーであり非負であることが既知
			writeUint64(&segm, uint64(seg.size))   //nolint:gosec // テストヘルパーであり非負であることが既知
			writeUint64(&segm, seg.originalSize)
		}
		writeChunkHeader(&entryBody, "segm", segm.Bytes())

		writeChunkHeader(&table, "File", entryBody.Bytes())
	}

	compressedTable := compressZlib(t, table.Bytes())

	// アーカイブ全体のレイアウト:
	//   [magic(11)] [info_offset(8)] [data...] [flag(1)+compressed_size(8)+original_size(8)+table]
	// info_offsetはdata section終端（=インデックス開始位置）を指す。
	var buf bytes.Buffer
	buf.Write(parser.XP3Magic)
	indexOffset := int64(headerSize) + int64(dataSection.Len())
	writeUint64(&buf, uint64(indexOffset)) //nolint:gosec // テストヘルパーであり非負であることが既知
	buf.Write(dataSection.Bytes())

	buf.WriteByte(0x00) // flag: バージョン1（0x80ビットなし）
	writeUint64(&buf, uint64(len(compressedTable)))
	writeUint64(&buf, uint64(table.Len()))
	buf.Write(compressedTable)

	return buf.Bytes()
}

func writeChunkHeader(buf *bytes.Buffer, name string, body []byte) {
	buf.WriteString(name)
	writeUint64(buf, uint64(len(body)))
	buf.Write(body)
}

func writeUint32(buf *bytes.Buffer, v uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	buf.Write(b)
}

func writeUint64(buf *bytes.Buffer, v uint64) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	buf.Write(b)
}

func writeUint16(buf *bytes.Buffer, v uint16) {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	buf.Write(b)
}

func compressZlib(t *testing.T, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	return buf.Bytes()
}

func TestXP3Archive_StandardIndexRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 単一ファイルの実インデックスを解析・展開できる", func(t *testing.T) {
		t.Parallel()

		content := []byte("hello kirikiri")
		archiveBytes := buildXP3Archive(t, []xp3EntrySpec{
			{name: "data/script.ks", data: content},
		})

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "game.xp3")
		writeFile(t, path, archiveBytes)

		archive, err := parser.NewXP3Archive(path)
		require.NoError(t, err)

		files := archive.ListFiles()
		require.Equal(t, []string{"data/script.ks"}, files)
		assert.False(t, archive.IsEncrypted())

		outputPath := filepath.Join(tmpDir, "out", "script.ks")
		require.NoError(t, archive.ExtractFile("data/script.ks", outputPath))

		extracted, err := os.ReadFile(outputPath) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
		require.NoError(t, err)
		assert.Equal(t, content, extracted)
	})

	t.Run("正常系: 複数ファイル・ネストしたパスをExtractAllできる", func(t *testing.T) {
		t.Parallel()

		archiveBytes := buildXP3Archive(t, []xp3EntrySpec{
			{name: "scenario/first.ks", data: []byte("*start")},
			{name: "image/bg/title.png", data: []byte("PNGDATA")},
		})

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "game.xp3")
		writeFile(t, path, archiveBytes)

		archive, err := parser.NewXP3Archive(path)
		require.NoError(t, err)

		outputDir := filepath.Join(tmpDir, "out")
		require.NoError(t, archive.ExtractAll(outputDir))

		got1, err := os.ReadFile(filepath.Join(outputDir, "scenario", "first.ks")) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
		require.NoError(t, err)
		assert.Equal(t, []byte("*start"), got1)

		got2, err := os.ReadFile(filepath.Join(outputDir, "image", "bg", "title.png")) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
		require.NoError(t, err)
		assert.Equal(t, []byte("PNGDATA"), got2)
	})

	t.Run("正常系: 暗号化フラグが立ったエントリがあるとIsEncryptedがtrueになる", func(t *testing.T) {
		t.Parallel()

		archiveBytes := buildXP3Archive(t, []xp3EntrySpec{
			{name: "data/secret.dat", data: []byte("secret"), encryptFlag: true},
		})

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "game.xp3")
		writeFile(t, path, archiveBytes)

		archive, err := parser.NewXP3Archive(path)
		require.NoError(t, err)

		assert.True(t, archive.IsEncrypted())

		checker := parser.NewXP3EncryptionChecker(path)
		result, err := checker.Check()
		require.NoError(t, err)
		assert.True(t, result.IsEncrypted)
		assert.Equal(t, parser.EncryptionUnknown, result.EncryptionType)

		err = checker.RaiseIfEncrypted()
		var encErr *parser.XP3EncryptionError
		require.ErrorAs(t, err, &encErr)
		assert.True(t, encErr.Info.IsEncrypted)
	})
}

// buildXP3ArchiveWithRawTable はrawTableをそのままファイルテーブル領域
// （非圧縮、zlib解凍失敗時のフォールバック経路）に配置したXP3アーカイブを
// 構築する。buildXP3Archive（xp3EntrySpec経由）では表現できない、
// 意図的に壊れた／攻撃者制御のサブチャンクサイズを持つエントリを直接
// 検証するための低レベルビルダー。
func buildXP3ArchiveWithRawTable(rawTable []byte) []byte {
	const headerSize = 19 // 11(magic) + 8(info_offset)

	var buf bytes.Buffer
	buf.Write(parser.XP3Magic)
	writeUint64(&buf, uint64(headerSize)) // info_offset: ヘッダー直後（データ領域なし）

	buf.WriteByte(0x00)                      // flag: バージョン1
	writeUint64(&buf, uint64(len(rawTable))) // compressed_size相当（実際は非圧縮のrawTableをそのまま使う）
	writeUint64(&buf, uint64(len(rawTable))) // original_size（読み飛ばされるのみ）
	buf.Write(rawTable)

	return buf.Bytes()
}

func TestXP3Archive_MalformedEntry_DoesNotOOM(t *testing.T) {
	t.Parallel()

	t.Run("異常系: infoサブチャンクが巨大サイズを宣言しても即時OOMしない", func(t *testing.T) {
		t.Parallel()

		// "info"サブチャンクがsubChunkSize=MaxUint64を宣言するが、実データは
		// 一切続かない（=数十バイトの細工ファイル）。修正前はreadChunkが
		// make([]byte, size)を素朴に呼び出しfatal error（回復不能なOOM）に
		// なっていた。修正後はstream.Len()でクランプされ、単に情報不足として
		// エントリが破棄されるだけになる。
		var entryBody bytes.Buffer
		entryBody.WriteString("info")
		writeUint64(&entryBody, math.MaxUint64)

		var table bytes.Buffer
		writeChunkHeader(&table, "File", entryBody.Bytes())

		archiveBytes := buildXP3ArchiveWithRawTable(table.Bytes())
		t.Logf("crafted archive size: %d bytes", len(archiveBytes))

		path := filepath.Join(t.TempDir(), "malicious.xp3")
		writeFile(t, path, archiveBytes)

		archive, err := parser.NewXP3Archive(path)

		require.NoError(t, err)
		assert.Empty(t, archive.ListFiles())
	})

	t.Run("異常系: segmサブチャンクが巨大サイズを宣言しても即時OOMしない", func(t *testing.T) {
		t.Parallel()

		// writeChunkHeaderは実データ長をそのままsubChunkSizeとして書き込むため
		// 使えない（「宣言サイズ ≠ 実データ長」という攻撃条件を作れない）。
		// ここではsubChunkSize=MaxUint64を宣言しつつ実データを一切続けない
		// バイト列を手動で組み立てる。
		var entryBody bytes.Buffer
		entryBody.WriteString("segm")
		writeUint64(&entryBody, math.MaxUint64)

		var table bytes.Buffer
		writeChunkHeader(&table, "File", entryBody.Bytes())

		archiveBytes := buildXP3ArchiveWithRawTable(table.Bytes())

		path := filepath.Join(t.TempDir(), "malicious_segm.xp3")
		writeFile(t, path, archiveBytes)

		archive, err := parser.NewXP3Archive(path)

		require.NoError(t, err)
		assert.Empty(t, archive.ListFiles())
	})
}

// TestXP3Archive_CorruptSegmOffset_DiscardsSegmentInsteadOfHeaderSplice は
// segmのoffsetがint64範囲を超える（safeInt64が失敗する）ケース。
//
// 修正前はOffsetもSize/OriginalSizeと同じくゼロ値（オフセット0）へ
// フォールバックしていたため、「オフセット0（=アーカイブヘッダー付近）から
// Size/OriginalSizeバイト読む」動作になり、無関係なアーカイブヘッダーの
// バイト列を展開結果に混入させていた（reviewer実測の情報漏えい）。
// 修正後はOffsetが範囲外の場合そのセグメントを丸ごと破棄する。この例では
// エントリが持つ唯一のセグメントが破棄されるため、name/segments判定により
// エントリ自体も破棄され、ヘッダーバイト列を含むファイルは一切生成されない。
func TestXP3Archive_CorruptSegmOffset_DiscardsSegmentInsteadOfHeaderSplice(t *testing.T) {
	t.Parallel()

	nameUTF16 := utf16.Encode([]rune("secret.dat"))

	var info bytes.Buffer
	writeUint32(&info, 0)                      // flags
	writeUint64(&info, 5)                      // originalSize（Python版delta以降読み飛ばされる）
	writeUint64(&info, 5)                      // size（同上）
	writeUint16(&info, uint16(len(nameUTF16))) //nolint:gosec // テストヘルパーであり名前長は既知の小さい値
	for _, u := range nameUTF16 {
		writeUint16(&info, u)
	}

	var segm bytes.Buffer
	writeUint32(&segm, 0)              // flags
	writeUint64(&segm, math.MaxUint64) // offset（int64範囲超過 → セグメント自体が破棄される）
	writeUint64(&segm, 5)              // size（範囲内だがoffset破棄により無関係）
	writeUint64(&segm, 5)              // originalSize（同上）

	var entryBody bytes.Buffer
	writeChunkHeader(&entryBody, "info", info.Bytes())
	writeChunkHeader(&entryBody, "segm", segm.Bytes())

	var table bytes.Buffer
	writeChunkHeader(&table, "File", entryBody.Bytes())

	archiveBytes := buildXP3ArchiveWithRawTable(table.Bytes())

	path := filepath.Join(t.TempDir(), "corrupt_segm_offset.xp3")
	writeFile(t, path, archiveBytes)

	archive, err := parser.NewXP3Archive(path)
	require.NoError(t, err)

	// セグメントが1つも残らないため、エントリ自体が破棄されている。
	assert.Empty(t, archive.ListFiles())

	// 展開してもパニックせず、（存在しないエントリの）ファイルも作られないこと。
	outputDir := t.TempDir()
	require.NoError(t, archive.ExtractAll(outputDir))
	assert.NoFileExists(t, filepath.Join(outputDir, "secret.dat"))
}

// TestXP3Archive_CorruptSegmSize_PreservesEntryAndAvoidsNegativeSlice は
// offsetは範囲内だがsize/original_sizeがint64範囲を超えるケース。
//
// 修正前はint64(uint64)の素朴なキャストで符号が反転して負値になり、
// ExtractAll時のmake([]byte, entry.Size)がmakeslice: len out of rangeで
// パニックしていた。また、そのpanicを避けるために（誤って）Seek失敗時に
// エントリ全体を破棄していたため、既に読み取れていたnameまで失われていた。
// 修正後はsafeInt64で範囲外の値のみゼロ値へフォールバックする。Size/
// OriginalSizeのフォールバックは（Offsetと異なり）最悪でも空読みになる
// だけで実害がないため、Offsetのように破棄はしない
// （詳細はparseSegmentsのwhy not参照）。
func TestXP3Archive_CorruptSegmSize_PreservesEntryAndAvoidsNegativeSlice(t *testing.T) {
	t.Parallel()

	nameUTF16 := utf16.Encode([]rune("secret.dat"))

	var info bytes.Buffer
	writeUint32(&info, 0)                      // flags
	writeUint64(&info, 5)                      // originalSize（Python版delta以降読み飛ばされる）
	writeUint64(&info, 5)                      // size（同上）
	writeUint16(&info, uint16(len(nameUTF16))) //nolint:gosec // テストヘルパーであり名前長は既知の小さい値
	for _, u := range nameUTF16 {
		writeUint16(&info, u)
	}

	var segm bytes.Buffer
	writeUint32(&segm, 0)              // flags
	writeUint64(&segm, 0)              // offset（範囲内・破棄されない）
	writeUint64(&segm, math.MaxUint64) // size（int64範囲超過）
	writeUint64(&segm, math.MaxUint64) // originalSize（int64範囲超過）

	var entryBody bytes.Buffer
	writeChunkHeader(&entryBody, "info", info.Bytes())
	writeChunkHeader(&entryBody, "segm", segm.Bytes())

	var table bytes.Buffer
	writeChunkHeader(&table, "File", entryBody.Bytes())

	archiveBytes := buildXP3ArchiveWithRawTable(table.Bytes())

	path := filepath.Join(t.TempDir(), "corrupt_segm.xp3")
	writeFile(t, path, archiveBytes)

	archive, err := parser.NewXP3Archive(path)
	require.NoError(t, err)

	// オフセットが範囲内のためセグメント・エントリともに破棄されず、nameが保持されていること。
	require.Equal(t, []string{"secret.dat"}, archive.ListFiles())

	// 展開してもmakesliceパニックせずに完了すること
	// （size/originalSizeがint64安全域外のためゼロ値にフォールバックし、空データとして書き出される）。
	outputDir := t.TempDir()
	err = archive.ExtractAll(outputDir)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(outputDir, "secret.dat"))
}

// --- 複数セグメント対応（Python版delta 3a17127相当）のテスト ---

func TestXP3FileEntry_TotalSize(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		segments []parser.XP3Segment
		expected int64
	}{
		"正常系: セグメントなしなら合計は0": {
			segments: nil,
			expected: 0,
		},
		"正常系: 単一セグメントの元サイズがそのまま合計になる": {
			segments: []parser.XP3Segment{
				{Offset: 1000, Size: 500, OriginalSize: 800, IsCompressed: true},
			},
			expected: 800,
		},
		"正常系: 複数セグメントの元サイズが合算される": {
			segments: []parser.XP3Segment{
				{Offset: 100, Size: 50, OriginalSize: 101, IsCompressed: true},
				{Offset: 200, Size: 100, OriginalSize: 192, IsCompressed: true},
				{Offset: 300, Size: 5000, OriginalSize: 11905, IsCompressed: true},
			},
			expected: 101 + 192 + 11905,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			entry := parser.XP3FileEntry{Name: "test.bin", Segments: tc.segments}

			assert.Equal(t, tc.expected, entry.TotalSize())
		})
	}
}

func TestXP3Archive_MultipleSegments(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 複数の圧縮セグメントを持つエントリがListFilesに現れる", func(t *testing.T) {
		t.Parallel()

		archiveBytes := buildXP3Archive(t, []xp3EntrySpec{
			{
				name: "test.bin",
				segments: []xp3SegmentSpec{
					{data: bytes.Repeat([]byte{'A'}, 101), compressFlag: true},
					{data: bytes.Repeat([]byte{'B'}, 192), compressFlag: true},
					{data: bytes.Repeat([]byte{'C'}, 11905), compressFlag: true},
				},
			},
		})

		path := filepath.Join(t.TempDir(), "multi_segment.xp3")
		writeFile(t, path, archiveBytes)

		archive, err := parser.NewXP3Archive(path)
		require.NoError(t, err)

		assert.Equal(t, []string{"test.bin"}, archive.ListFiles())
	})

	t.Run("正常系: 複数の非圧縮セグメントが連結されて展開される", func(t *testing.T) {
		t.Parallel()

		expected := bytes.Join([][]byte{
			[]byte("FIRST_SEGMENT_DATA_"),
			[]byte("SECOND_SEGMENT_DATA_"),
			[]byte("THIRD_SEGMENT_DATA"),
		}, nil)

		archiveBytes := buildXP3Archive(t, []xp3EntrySpec{
			{
				name: "test.bin",
				segments: []xp3SegmentSpec{
					{data: []byte("FIRST_SEGMENT_DATA_")},
					{data: []byte("SECOND_SEGMENT_DATA_")},
					{data: []byte("THIRD_SEGMENT_DATA")},
				},
			},
		})

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "multi_segment.xp3")
		writeFile(t, path, archiveBytes)

		archive, err := parser.NewXP3Archive(path)
		require.NoError(t, err)

		outputPath := filepath.Join(tmpDir, "output", "test.bin")
		require.NoError(t, archive.ExtractFile("test.bin", outputPath))

		actual, err := os.ReadFile(outputPath) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	})

	t.Run("正常系: 複数の圧縮セグメントが正しく解凍・連結される", func(t *testing.T) {
		t.Parallel()

		expected := bytes.Join([][]byte{
			bytes.Repeat([]byte{'X'}, 100),
			bytes.Repeat([]byte{'Y'}, 200),
			bytes.Repeat([]byte{'Z'}, 300),
		}, nil)

		archiveBytes := buildXP3Archive(t, []xp3EntrySpec{
			{
				name: "test.bin",
				segments: []xp3SegmentSpec{
					{data: bytes.Repeat([]byte{'X'}, 100), compressFlag: true},
					{data: bytes.Repeat([]byte{'Y'}, 200), compressFlag: true},
					{data: bytes.Repeat([]byte{'Z'}, 300), compressFlag: true},
				},
			},
		})

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "multi_segment.xp3")
		writeFile(t, path, archiveBytes)

		archive, err := parser.NewXP3Archive(path)
		require.NoError(t, err)

		outputPath := filepath.Join(tmpDir, "output", "test.bin")
		require.NoError(t, archive.ExtractFile("test.bin", outputPath))

		actual, err := os.ReadFile(outputPath) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
		require.NoError(t, err)
		require.Len(t, actual, 600)
		assert.Equal(t, expected, actual)
	})

	t.Run("正常系: 圧縮・非圧縮が混在するセグメントが正しく処理される", func(t *testing.T) {
		t.Parallel()

		expected := bytes.Join([][]byte{
			[]byte("UNCOMPRESSED_1_"),
			bytes.Repeat([]byte{'C'}, 100),
			[]byte("UNCOMPRESSED_2"),
		}, nil)

		archiveBytes := buildXP3Archive(t, []xp3EntrySpec{
			{
				name: "test.bin",
				segments: []xp3SegmentSpec{
					{data: []byte("UNCOMPRESSED_1_")},
					{data: bytes.Repeat([]byte{'C'}, 100), compressFlag: true},
					{data: []byte("UNCOMPRESSED_2")},
				},
			},
		})

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "multi_segment.xp3")
		writeFile(t, path, archiveBytes)

		archive, err := parser.NewXP3Archive(path)
		require.NoError(t, err)

		outputPath := filepath.Join(tmpDir, "output", "test.bin")
		require.NoError(t, archive.ExtractFile("test.bin", outputPath))

		actual, err := os.ReadFile(outputPath) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	})
}

// TestXP3Archive_HostileSegmentCount_TruncatedToActualData はGoal 4で要求された
// 「宣言セグメント数（宣言サイズ）が実データより大きい悪意あるケース」の検証。
//
// segmサブチャンクがsubChunkSize=math.MaxUint64（事実上無制限のセグメント数）を
// 宣言する一方、実際に後続するデータは28バイトレコード2件＋末尾に28バイト
// 未満の断片（10バイト、不完全なレコード）だけという、宣言と実データが
// 乖離したケース。parseSegmentsは宣言値ではなくreadChunkでstream残量に
// クランプ済みの実データ長（=readChunkが持つハードニング、詳細はreadChunkの
// why not参照）からセグメント数を算出するため、末尾の断片は無視され、
// ちょうど2セグメントだけが安全にパースされる（OOMもパニックもしない）。
func TestXP3Archive_HostileSegmentCount_TruncatedToActualData(t *testing.T) {
	t.Parallel()

	const headerSize = 19 // 11(magic) + 8(info_offset)

	seg1 := []byte("FIRST_SEGMENT_DATA_")
	seg2 := []byte("SECOND_SEGMENT_DATA")
	offset1 := int64(headerSize)
	offset2 := offset1 + int64(len(seg1))

	nameUTF16 := utf16.Encode([]rune("hostile.bin"))

	var info bytes.Buffer
	writeUint32(&info, 0)                           // flags
	writeUint64(&info, uint64(len(seg1)+len(seg2))) //nolint:gosec // テストヘルパーであり非負であることが既知
	writeUint64(&info, uint64(len(seg1)+len(seg2))) //nolint:gosec // テストヘルパーであり非負であることが既知
	writeUint16(&info, uint16(len(nameUTF16)))      //nolint:gosec // テストヘルパーであり名前長は既知の小さい値
	for _, u := range nameUTF16 {
		writeUint16(&info, u)
	}

	var segm bytes.Buffer
	writeUint32(&segm, 0)                 // セグメント1: flags（非圧縮）
	writeUint64(&segm, uint64(offset1))   //nolint:gosec // テストヘルパーであり非負であることが既知
	writeUint64(&segm, uint64(len(seg1))) //nolint:gosec // テストヘルパーであり非負であることが既知
	writeUint64(&segm, uint64(len(seg1))) //nolint:gosec // テストヘルパーであり非負であることが既知
	writeUint32(&segm, 0)                 // セグメント2: flags（非圧縮）
	writeUint64(&segm, uint64(offset2))   //nolint:gosec // テストヘルパーであり非負であることが既知
	writeUint64(&segm, uint64(len(seg2))) //nolint:gosec // テストヘルパーであり非負であることが既知
	writeUint64(&segm, uint64(len(seg2))) //nolint:gosec // テストヘルパーであり非負であることが既知
	segm.WriteString("GARBAGE___")        // 28バイト未満の不完全な断片（無視されるべき）

	var entryBody bytes.Buffer
	writeChunkHeader(&entryBody, "info", info.Bytes())
	entryBody.WriteString("segm")
	writeUint64(&entryBody, math.MaxUint64) // 宣言サイズ: 事実上無制限のセグメント数を主張
	entryBody.Write(segm.Bytes())           // 実データ: 66バイト（28*2+10）のみ

	var table bytes.Buffer
	writeChunkHeader(&table, "File", entryBody.Bytes())

	compressedTable := compressZlib(t, table.Bytes())

	dataSection := append(append([]byte{}, seg1...), seg2...)

	var buf bytes.Buffer
	buf.Write(parser.XP3Magic)
	indexOffset := int64(headerSize) + int64(len(dataSection))
	writeUint64(&buf, uint64(indexOffset)) //nolint:gosec // テストヘルパーであり非負であることが既知
	buf.Write(dataSection)

	buf.WriteByte(0x00) // flag: バージョン1
	writeUint64(&buf, uint64(len(compressedTable)))
	writeUint64(&buf, uint64(table.Len()))
	buf.Write(compressedTable)

	path := filepath.Join(t.TempDir(), "hostile_segment_count.xp3")
	writeFile(t, path, buf.Bytes())

	archive, err := parser.NewXP3Archive(path)
	require.NoError(t, err)
	require.Equal(t, []string{"hostile.bin"}, archive.ListFiles())

	outputDir := t.TempDir()
	require.NoError(t, archive.ExtractAll(outputDir))

	extracted, err := os.ReadFile(filepath.Join(outputDir, "hostile.bin")) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
	require.NoError(t, err)
	assert.Equal(t, dataSection, extracted)
}

// TestXP3Archive_EntryWithoutSegments_IsDiscarded は、infoチャンクは正常に
// nameを確定できるがsegmチャンクを欠く（あるいは28バイト未満で1件も有効な
// セグメントを構成できない）場合に、エントリ自体が破棄されることをpinする。
//
// parseSingleEntryの「name == "" || len(segments) == 0」判定
// （Python版の`if name and segments:`に対応）はこれまでテストされておらず、
// reviewerがlen(segments)==0の条件を取り除くミューテーションを入れても
// 全テストが通過することを確認していた。
func TestXP3Archive_EntryWithoutSegments_IsDiscarded(t *testing.T) {
	t.Parallel()

	buildInfoOnlyEntry := func(name string) []byte {
		nameUTF16 := utf16.Encode([]rune(name))

		var info bytes.Buffer
		writeUint32(&info, 0) // flags
		writeUint64(&info, 0)
		writeUint64(&info, 0)
		writeUint16(&info, uint16(len(nameUTF16))) //nolint:gosec // テストヘルパーであり名前長は既知の小さい値
		for _, u := range nameUTF16 {
			writeUint16(&info, u)
		}

		return info.Bytes()
	}

	t.Run("異常系: infoは正常だがsegmチャンクが存在しない場合エントリは破棄される", func(t *testing.T) {
		t.Parallel()

		var entryBody bytes.Buffer
		writeChunkHeader(&entryBody, "info", buildInfoOnlyEntry("no_segm.bin"))
		// segmチャンクを意図的に書かない。

		var table bytes.Buffer
		writeChunkHeader(&table, "File", entryBody.Bytes())

		archiveBytes := buildXP3ArchiveWithRawTable(table.Bytes())

		path := filepath.Join(t.TempDir(), "no_segm.xp3")
		writeFile(t, path, archiveBytes)

		archive, err := parser.NewXP3Archive(path)
		require.NoError(t, err)

		assert.Empty(t, archive.ListFiles())
	})

	t.Run("異常系: segmチャンクが28バイト未満の場合エントリは破棄される", func(t *testing.T) {
		t.Parallel()

		var entryBody bytes.Buffer
		writeChunkHeader(&entryBody, "info", buildInfoOnlyEntry("short_segm.bin"))
		// 27バイトの不完全なsegmレコード（28バイト未満で1件も構成できない）。
		writeChunkHeader(&entryBody, "segm", make([]byte, 27))

		var table bytes.Buffer
		writeChunkHeader(&table, "File", entryBody.Bytes())

		archiveBytes := buildXP3ArchiveWithRawTable(table.Bytes())

		path := filepath.Join(t.TempDir(), "short_segm.xp3")
		writeFile(t, path, archiveBytes)

		archive, err := parser.NewXP3Archive(path)
		require.NoError(t, err)

		assert.Empty(t, archive.ListFiles())
	})
}

// TestXP3Archive_CompressedFlagWithMatchingSize_PassesThroughRawBytes は、
// segmのis_compressedフラグ（0x07）が立っていてもSize==OriginalSizeの場合は
// 「圧縮後サイズ==元サイズ」つまり実質未圧縮を意味するため解凍を試みず、
// アーカイブ上の生バイト列をそのまま採用すること（Python版の
// `is_compressed and size != original_size`ガードと同じ）をpinする。
//
// 生バイト列自体を「たまたま正当なzlibストリームだが別の内容を指す」もの
// にしておくことで、ガード条件（Size != OriginalSize）が取り除かれた場合
// （解凍が誤って実行された場合）に解凍が成功してしまい、期待値と異なる
// 内容（別内容の解凍結果）が出力されて検出できるようにしている
// （単なる非zlibバイト列だと、解凍失敗時にdecompressZlibのエラーが
// 握りつぶされ元のバイト列がそのまま残るため、ガード除去のミューテーションを
// 検出できない）。
func TestXP3Archive_CompressedFlagWithMatchingSize_PassesThroughRawBytes(t *testing.T) {
	t.Parallel()

	const headerSize = 19

	// 「別の内容」をzlib圧縮したバイト列を、そのままアーカイブ上の生データ
	// として配置する。ガードが正しく機能していれば、このzlib圧縮済みバイト列
	// 自体がそのまま展開結果になるはず（内部のinnerContentへ解凍されない）。
	innerContent := []byte("THIS_SHOULD_NOT_BE_DECOMPRESSED")
	rawPayload := compressZlib(t, innerContent)
	offset := int64(headerSize)

	nameUTF16 := utf16.Encode([]rune("raw_passthrough.bin"))

	var info bytes.Buffer
	writeUint32(&info, 0)
	writeUint64(&info, uint64(len(rawPayload)))
	writeUint64(&info, uint64(len(rawPayload)))
	writeUint16(&info, uint16(len(nameUTF16))) //nolint:gosec // テストヘルパーであり名前長は既知の小さい値
	for _, u := range nameUTF16 {
		writeUint16(&info, u)
	}

	var segm bytes.Buffer
	writeUint32(&segm, 0x07)                    // is_compressed = true（Python版フィクスチャと同じ多ビットフラグ）
	writeUint64(&segm, uint64(offset))          //nolint:gosec // テストヘルパーであり非負であることが既知
	writeUint64(&segm, uint64(len(rawPayload))) // size
	writeUint64(&segm, uint64(len(rawPayload))) // original_size（sizeと同一 → 解凍を試みてはいけない）

	var entryBody bytes.Buffer
	writeChunkHeader(&entryBody, "info", info.Bytes())
	writeChunkHeader(&entryBody, "segm", segm.Bytes())

	var table bytes.Buffer
	writeChunkHeader(&table, "File", entryBody.Bytes())

	compressedTable := compressZlib(t, table.Bytes())

	var buf bytes.Buffer
	buf.Write(parser.XP3Magic)
	indexOffset := int64(headerSize) + int64(len(rawPayload))
	writeUint64(&buf, uint64(indexOffset)) //nolint:gosec // テストヘルパーであり非負であることが既知
	buf.Write(rawPayload)

	buf.WriteByte(0x00) // flag: バージョン1
	writeUint64(&buf, uint64(len(compressedTable)))
	writeUint64(&buf, uint64(table.Len()))
	buf.Write(compressedTable)

	path := filepath.Join(t.TempDir(), "raw_passthrough.xp3")
	writeFile(t, path, buf.Bytes())

	archive, err := parser.NewXP3Archive(path)
	require.NoError(t, err)

	outputDir := t.TempDir()
	require.NoError(t, archive.ExtractAll(outputDir))

	extracted, err := os.ReadFile(filepath.Join(outputDir, "raw_passthrough.bin")) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
	require.NoError(t, err)
	assert.Equal(t, rawPayload, extracted)
	assert.NotEqual(t, innerContent, extracted)
}

// TestXP3Archive_ManySegmentsSameOffset_BoundedByFileSize は、悪意ある
// アーカイブが同一オフセットを指す大量のsegmレコードを持つケース。
//
// 修正前は各セグメントが個別に「オフセット以降の残量」でのみクランプされて
// おり、エントリ全体での累積読み取り量には上限がなかった。同一オフセットの
// セグメントを大量に積み重ねると、セグメント数×クランプ後サイズ分の
// アロケーションが発生しうる（reviewer実測: 52KBの細工アーカイブ・同一
// オフセットのセグメント20,000件で5.1GB RSS）。修正後はextractEntryが
// エントリ全体でbudget（=fileSize）を管理し、セグメントをまたいで消費させる
// ため、累積読み取り量はfileSizeを超えない。
func TestXP3Archive_ManySegmentsSameOffset_BoundedByFileSize(t *testing.T) {
	t.Parallel()

	const headerSize = 19
	const numSegments = 2000
	const segmentDeclaredSize = 500 // 実ファイルサイズよりずっと大きい宣言値

	payload := []byte("SHARED_OFFSET_PAYLOAD")
	offset := int64(headerSize)

	nameUTF16 := utf16.Encode([]rune("many_segments.bin"))

	var info bytes.Buffer
	writeUint32(&info, 0)
	writeUint64(&info, uint64(len(payload)))
	writeUint64(&info, uint64(len(payload)))
	writeUint16(&info, uint16(len(nameUTF16))) //nolint:gosec // テストヘルパーであり名前長は既知の小さい値
	for _, u := range nameUTF16 {
		writeUint16(&info, u)
	}

	var segm bytes.Buffer
	for range numSegments {
		writeUint32(&segm, 0)                           // flags（非圧縮）
		writeUint64(&segm, uint64(offset))              //nolint:gosec // テストヘルパーであり非負であることが既知
		writeUint64(&segm, uint64(segmentDeclaredSize)) //nolint:gosec // テストヘルパーであり非負であることが既知
		writeUint64(&segm, uint64(segmentDeclaredSize)) //nolint:gosec // テストヘルパーであり非負であることが既知
	}

	var entryBody bytes.Buffer
	writeChunkHeader(&entryBody, "info", info.Bytes())
	writeChunkHeader(&entryBody, "segm", segm.Bytes())

	var table bytes.Buffer
	writeChunkHeader(&table, "File", entryBody.Bytes())

	compressedTable := compressZlib(t, table.Bytes())

	var buf bytes.Buffer
	buf.Write(parser.XP3Magic)
	indexOffset := int64(headerSize) + int64(len(payload))
	writeUint64(&buf, uint64(indexOffset)) //nolint:gosec // テストヘルパーであり非負であることが既知
	buf.Write(payload)

	buf.WriteByte(0x00) // flag: バージョン1
	writeUint64(&buf, uint64(len(compressedTable)))
	writeUint64(&buf, uint64(table.Len()))
	buf.Write(compressedTable)

	path := filepath.Join(t.TempDir(), "many_segments_same_offset.xp3")
	writeFile(t, path, buf.Bytes())
	fileSize := int64(len(buf.Bytes()))

	archive, err := parser.NewXP3Archive(path)
	require.NoError(t, err)
	require.Equal(t, []string{"many_segments.bin"}, archive.ListFiles())

	outputDir := t.TempDir()
	require.NoError(t, archive.ExtractAll(outputDir))

	extracted, err := os.ReadFile(filepath.Join(outputDir, "many_segments.bin")) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
	require.NoError(t, err)

	// budgetがなければ最大 numSegments * segmentDeclaredSize ≈ 1,000,000バイト
	// 分読まれうるが、budget導入後はエントリ全体でfileSizeを超えて読まれない。
	assert.LessOrEqual(t, int64(len(extracted)), fileSize)
	assert.Less(t, len(extracted), numSegments*segmentDeclaredSize)
}
