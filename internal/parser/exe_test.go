package parser_test

import (
	"bytes"
	"encoding/binary"
	"hash/adler32"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/parser"
)

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, content, 0o600))
}

// storedXP3ForEXE は非圧縮のエントリ（name, data）を1件だけ持ち、索引も非圧縮で
// 書いたXP3アーカイブのバイト列を返す。Fileチャンクにはinfo、segm、adlrを書く。
// 索引とセグメントのオフセットはアーカイブ先頭からの相対値で、EXEのどこに
// 置いても読める。
func storedXP3ForEXE(name string, data []byte) []byte {
	const headerSize = 19 // マジック(11) + 索引オフセット(8)

	nameUTF16 := utf16.Encode([]rune(name))
	le := binary.LittleEndian

	var info []byte
	info = le.AppendUint32(info, 0)
	info = le.AppendUint64(info, uint64(len(data)))
	info = le.AppendUint64(info, uint64(len(data)))
	info = le.AppendUint16(info, uint16(len(nameUTF16))) //nolint:gosec // テストで渡す名前は短い既知の値
	for _, u := range nameUTF16 {
		info = le.AppendUint16(info, u)
	}

	var segm []byte
	segm = le.AppendUint32(segm, 0)
	segm = le.AppendUint64(segm, headerSize)
	segm = le.AppendUint64(segm, uint64(len(data)))
	segm = le.AppendUint64(segm, uint64(len(data)))

	chunk := func(name string, body []byte) []byte {
		return append(le.AppendUint64([]byte(name), uint64(len(body))), body...)
	}
	var adlr bytes.Buffer
	writeAdlrChunk(&adlr, adler32.Checksum(data))
	fileBody := append(chunk("info", info), chunk("segm", segm)...)
	table := chunk("File", append(fileBody, adlr.Bytes()...))

	archive := append([]byte{}, parser.XP3Magic...)
	archive = le.AppendUint64(archive, uint64(headerSize+len(data)))
	archive = append(archive, data...)
	archive = append(archive, 0x00)
	archive = le.AppendUint64(archive, uint64(len(table)))

	return append(archive, table...)
}

// exeBytes は"MZ"で始まり、parts[i].offsetの位置にparts[i].dataを置いた
// バイト列を返す。隙間は0で埋める。
func exeBytes(parts ...exePart) []byte {
	content := []byte("MZ")
	for _, p := range parts {
		if len(content) < p.offset {
			content = append(content, make([]byte, p.offset-len(content))...)
		}
		content = append(content[:p.offset], p.data...)
	}

	return content
}

type exePart struct {
	offset int
	data   []byte
}

func TestNewEmbeddedXP3Extractor(t *testing.T) {
	t.Parallel()

	t.Run("異常系: 存在しないEXEファイルでErrEXENotFound", func(t *testing.T) {
		t.Parallel()

		nonexistent := filepath.Join(t.TempDir(), "nonexistent.exe")

		_, err := parser.NewEmbeddedXP3Extractor(nonexistent)

		require.ErrorIs(t, err, parser.ErrEXENotFound)
	})

	t.Run("正常系: 存在するEXEファイルで初期化できる", func(t *testing.T) {
		t.Parallel()

		exeFile := filepath.Join(t.TempDir(), "game.exe")
		writeFile(t, exeFile, []byte("MZ"))

		extractor, err := parser.NewEmbeddedXP3Extractor(exeFile)

		require.NoError(t, err)
		_, found, err := extractor.FindEmbeddedXP3()
		require.NoError(t, err)
		assert.False(t, found)
	})
}

func TestEmbeddedXP3Extractor_FindEmbeddedXP3(t *testing.T) {
	t.Parallel()

	magic := parser.XP3Magic
	tail := make([]byte, 50)
	withTail := append(append([]byte{}, magic...), tail...)

	tests := []struct {
		name       string
		content    []byte
		wantFound  bool
		wantOffset int64
		wantSize   int64
	}{
		{
			name:      "正常系: XP3が埋め込まれていないEXEでは見つからない",
			content:   exeBytes(exePart{100, []byte{0}}),
			wantFound: false,
		},
		{
			name:       "正常系: 16バイト境界にあるマジックを検出しEXE終端までをサイズとする",
			content:    exeBytes(exePart{32, withTail}),
			wantFound:  true,
			wantOffset: 32,
			wantSize:   int64(len(withTail)),
		},
		{
			name:      "正常系: 16バイト境界にないマジックは検出しない",
			content:   exeBytes(exePart{24, withTail}),
			wantFound: false,
		},
		{
			name:       "正常系: 16バイト境界にない偽のマジックを飛ばして後ろの境界上のマジックを検出する",
			content:    exeBytes(exePart{20, magic}, exePart{48, withTail}),
			wantFound:  true,
			wantOffset: 48,
			wantSize:   int64(len(withTail)),
		},
		{
			name:       "正常系: 境界上のマジックが複数あれば最初の1つだけを検出しEXE終端までをサイズとする",
			content:    exeBytes(exePart{16, magic}, exePart{64, withTail}),
			wantFound:  true,
			wantOffset: 16,
			wantSize:   64 - 16 + int64(len(withTail)),
		},
		{
			name:      "正常系: マジックの末尾がファイルの最終バイトなら検出しない",
			content:   exeBytes(exePart{16, magic}),
			wantFound: false,
		},
		{
			name:       "正常系: マジックの後ろに1バイトあれば検出する",
			content:    exeBytes(exePart{16, append(append([]byte{}, magic...), 0)}),
			wantFound:  true,
			wantOffset: 16,
			wantSize:   int64(len(magic)) + 1,
		},
		{
			name:       "正常系: MZで始まらずマジックで始まるファイルは先頭のXP3として扱う",
			content:    withTail,
			wantFound:  true,
			wantOffset: 0,
			wantSize:   int64(len(withTail)),
		},
		{
			name:      "正常系: MZでもマジックでも始まらないファイルは探さない",
			content:   append(make([]byte, 32), withTail...),
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			exeFile := filepath.Join(t.TempDir(), "game.exe")
			writeFile(t, exeFile, tt.content)

			extractor, err := parser.NewEmbeddedXP3Extractor(exeFile)
			require.NoError(t, err)

			got, found, err := extractor.FindEmbeddedXP3()

			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				assert.Equal(t, parser.EmbeddedXP3Info{Offset: tt.wantOffset, EstimatedSize: tt.wantSize}, got)
			}
		})
	}
}

func TestEmbeddedXP3Extractor_Extract(t *testing.T) {
	t.Parallel()

	// アーカイブ内のデータにマジックを含めることで、次のマジックの手前で
	// アーカイブを切り詰める読み方では索引に届かなくなる。
	payload := append(append([]byte("head"), parser.XP3Magic...), []byte("tail")...)
	archive := storedXP3ForEXE("startup.tjs", payload)

	tests := []struct {
		name        string
		content     []byte
		outputDir   func(tmpDir string) string
		wantFound   bool
		wantArchive []byte
	}{
		{
			name:        "正常系: 非整列の偽マジックを飛ばし境界上のアーカイブを内部のマジックで切らずに書き出す",
			content:     exeBytes(exePart{20, parser.XP3Magic}, exePart{64, archive}),
			outputDir:   func(tmpDir string) string { return filepath.Join(tmpDir, "extracted") },
			wantFound:   true,
			wantArchive: archive,
		},
		{
			name:        "正常系: 出力ディレクトリが存在しない場合は作成する",
			content:     exeBytes(exePart{16, archive}),
			outputDir:   func(tmpDir string) string { return filepath.Join(tmpDir, "new", "dir") },
			wantFound:   true,
			wantArchive: archive,
		},
		{
			name:      "正常系: XP3が埋め込まれていない場合は何も書き出さない",
			content:   exeBytes(exePart{100, []byte{0}}),
			outputDir: func(tmpDir string) string { return filepath.Join(tmpDir, "extracted") },
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			exeFile := filepath.Join(tmpDir, "game.exe")
			writeFile(t, exeFile, tt.content)
			outputDir := tt.outputDir(tmpDir)

			extractor, err := parser.NewEmbeddedXP3Extractor(exeFile)
			require.NoError(t, err)

			path, found, err := extractor.Extract(outputDir)

			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, found)
			if !tt.wantFound {
				assert.Empty(t, path)
				assert.NoDirExists(t, outputDir)

				return
			}
			assert.Equal(t, filepath.Join(outputDir, "game_0.xp3"), path)
			data, err := os.ReadFile(path) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
			require.NoError(t, err)
			assert.True(t, bytes.Equal(tt.wantArchive, data))

			xp3, err := parser.NewXP3Archive(path)
			require.NoError(t, err)
			contentDir := filepath.Join(tmpDir, "content")
			require.NoError(t, xp3.ExtractAll(contentDir))
			extracted, err := os.ReadFile(filepath.Join(contentDir, "startup.tjs")) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
			require.NoError(t, err)
			assert.Equal(t, payload, extracted)
		})
	}
}
