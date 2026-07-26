package parser_test

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
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
// Python版のテストスイートには実インデックスを構築するフィクスチャヘルパーが
// 存在しない（すべて簡易マジックのみの最小ファイルで検証している）。
// しかしXP3Archiveの本体ロジック（zlib解凍・チャンク解析・オフセット算出）は
// このパスを通らないと一切検証されないため、Go移植では独自にビルダーを追加し
// 実データでの往復（構築→解析→展開→内容一致）を検証する。
type xp3EntrySpec struct {
	name         string
	data         []byte
	encryptFlag  bool
	compressFlag bool
}

// buildXP3Archive はバージョン1形式（フラグ直後にcompressed_size/original_size
// が続く形式）のXP3アーカイブをバイト列として構築する。
// 返り値はアーカイブ全体のバイト列と、各エントリの実データオフセット一覧。
func buildXP3Archive(t *testing.T, entries []xp3EntrySpec) []byte {
	t.Helper()

	// データ領域: ヘッダーの直後に各エントリの実データを連結配置する。
	const headerSize = 19 // 11(magic) + 8(info_offset)
	dataOffsets := make([]int64, len(entries))
	var dataSection bytes.Buffer
	for i, e := range entries {
		dataOffsets[i] = headerSize + int64(dataSection.Len())
		dataSection.Write(e.data)
	}

	// ファイルテーブル（"File"チャンクの列）を構築する。
	var table bytes.Buffer
	for i, e := range entries {
		var entryBody bytes.Buffer

		// infoサブチャンク
		var info bytes.Buffer
		var flags uint32
		if e.encryptFlag {
			flags |= 0x80000000
		}
		nameUTF16 := utf16.Encode([]rune(e.name))
		originalSize := uint64(len(e.data))
		size := originalSize
		if e.compressFlag {
			size = originalSize + 1 // segmで別途上書きされるため実値は問わない
		}
		writeUint32(&info, flags)
		writeUint64(&info, originalSize)
		writeUint64(&info, size)
		writeUint16(&info, uint16(len(nameUTF16))) //nolint:gosec // テストヘルパーであり名前長は既知の小さい値
		for _, u := range nameUTF16 {
			writeUint16(&info, u)
		}
		writeChunkHeader(&entryBody, "info", info.Bytes())

		// segmサブチャンク（実データのオフセット・サイズを保持）。
		var segm bytes.Buffer
		var segmFlags uint32
		if e.compressFlag {
			segmFlags |= 0x01
		}
		writeUint32(&segm, segmFlags)
		writeUint64(&segm, uint64(dataOffsets[i])) //nolint:gosec // テストヘルパーであり非負であることが既知
		writeUint64(&segm, uint64(len(e.data)))
		writeUint64(&segm, originalSize)
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
