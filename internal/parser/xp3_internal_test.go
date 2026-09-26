package parser

import (
	"bytes"
	"compress/zlib"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// segmentDecompressionLimitはパッケージ非公開ヘルパーであり、1GiB近くまで実際に
// 膨張するフィクスチャを作らずに上限値を検証するため、ホワイトボックス
// （同一パッケージ）テストとして検証する。
func TestSegmentDecompressionLimit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		originalSize int64
		want         int64
	}{
		{"正常系: 宣言サイズが上限未満ならその値を上限にする", 16, 16},
		{"正常系: 宣言サイズがちょうど1GiBなら1GiBを上限にする", 1 << 30, 1 << 30},
		{"正常系: 宣言サイズが2GiBでも1GiBを上限にする", 2 << 30, 1 << 30},
		{"正常系: 宣言サイズが0ならファイルテーブルと同じ64MiBを上限にする", 0, 64 << 20},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := segmentDecompressionLimit(XP3Segment{OriginalSize: tc.originalSize, IsCompressed: true})

			assert.Equal(t, tc.want, got)
		})
	}
}

// chunkClampCase はreadChunk/skipChunkを、6バイトのデータのうちconsumedバイトを
// 読み進めた位置からsizeバイト分呼び出すケースを表す。
type chunkClampCase struct {
	name          string
	consumed      int
	size          uint64
	wantData      []byte
	wantRemaining int
}

var chunkClampCases = []chunkClampCase{
	{"正常系: sizeが残量未満なら先頭からsizeバイトだけ進む", 0, 2, []byte("ab"), 4},
	{"正常系: sizeが残量ちょうどなら末尾まで進む", 0, 6, []byte("abcdef"), 0},
	{"正常系: sizeが残量を超える場合は残量へクランプして末尾で止まる", 2, 100, []byte("cdef"), 0},
	{"正常系: sizeがuint64の最大値でも残量へクランプして末尾で止まる", 0, math.MaxUint64, []byte("abcdef"), 0},
	{"正常系: sizeが0なら位置を変えない", 3, 0, []byte{}, 3},
	{"正常系: 既に末尾にある場合は何も読まず末尾のまま", 6, 10, []byte{}, 0},
}

func newConsumedReader(t *testing.T, consumed int) *bytes.Reader {
	t.Helper()

	stream := bytes.NewReader([]byte("abcdef"))
	_, err := stream.Seek(int64(consumed), io.SeekStart)
	require.NoError(t, err)

	return stream
}

// readChunk/skipChunkはパッケージ非公開ヘルパーであり、宣言サイズと残量の大小関係
// ごとの挙動をアーカイブを組まずに直接検証するため、ホワイトボックステストとする。
func TestReadChunk(t *testing.T) {
	t.Parallel()

	for _, tc := range chunkClampCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stream := newConsumedReader(t, tc.consumed)

			got := readChunk(stream, tc.size)

			assert.Equal(t, tc.wantData, got)
			assert.Equal(t, tc.wantRemaining, stream.Len())
		})
	}
}

func TestSkipChunk(t *testing.T) {
	t.Parallel()

	for _, tc := range chunkClampCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stream := newConsumedReader(t, tc.consumed)

			skipChunk(stream, tc.size)

			assert.Equal(t, tc.wantRemaining, stream.Len())
		})
	}
}

// readSegmentはパッケージ非公開ヘルパーであり、セグメントのオフセットがファイル末尾より
// 先を指す場合の読み取り量をアーカイブを組まずに直接検証するため、ホワイトボックステストとする。
func TestReadSegment(t *testing.T) {
	t.Parallel()

	data := []byte("abcdef")
	fileSize := int64(len(data))

	cases := []struct {
		name         string
		segment      XP3Segment
		wantConsumed int64
	}{
		{"正常系: オフセットがファイル末尾より先なら何も読まずに空データを返す", XP3Segment{Offset: fileSize + 10, Size: 4, OriginalSize: 4}, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				got []byte
				err error
			)
			budget := entryBudget{raw: fileSize, inflated: maxSegmentDecompressedSize, inflatedLimit: maxSegmentDecompressedSize}
			require.NotPanics(t, func() {
				got, err = readSegment(bytes.NewReader(data), tc.segment, fileSize, &budget)
			})

			require.NoError(t, err)
			assert.Empty(t, got)
			assert.Equal(t, fileSize-tc.wantConsumed, budget.raw)
		})
	}
}

// entrySegmentSpec はextractEntryに渡すセグメント1件分の実データを表す。
// compressedがtrueならdataをzlib圧縮して配置し、OriginalSizeにはlen(data)を宣言する。
type entrySegmentSpec struct {
	data       []byte
	compressed bool
}

// buildEntrySource はspecsを先頭から順に配置したバイト列と、それを指すセグメント列を返す。
func buildEntrySource(t *testing.T, specs []entrySegmentSpec) ([]byte, []XP3Segment) {
	t.Helper()

	var (
		source   bytes.Buffer
		segments []XP3Segment
	)
	for _, spec := range specs {
		stored := spec.data
		if spec.compressed {
			var compressed bytes.Buffer
			w := zlib.NewWriter(&compressed)
			_, err := w.Write(spec.data)
			require.NoError(t, err)
			require.NoError(t, w.Close())
			stored = compressed.Bytes()
		}
		segments = append(segments, XP3Segment{
			Offset:       int64(source.Len()),
			Size:         int64(len(stored)),
			OriginalSize: int64(len(spec.data)),
			IsCompressed: spec.compressed,
		})
		source.Write(stored)
	}

	return source.Bytes(), segments
}

// extractEntryはエントリ全体の解凍後サイズの上限を引数で受け取る。1GiBの既定値を
// 超えるフィクスチャを作らずに上限を検証するため、KiB単位の上限を渡せる
// ホワイトボックステストとする。
func TestExtractEntry_InflatedLimit(t *testing.T) {
	t.Parallel()

	const segmentSize = 1024

	compressedA := entrySegmentSpec{data: bytes.Repeat([]byte{'A'}, segmentSize), compressed: true}
	compressedB := entrySegmentSpec{data: bytes.Repeat([]byte{'B'}, segmentSize), compressed: true}
	raw := entrySegmentSpec{data: bytes.Repeat([]byte{'R'}, 100)}

	cases := []struct {
		name    string
		specs   []entrySegmentSpec
		limit   int64
		wantErr bool
	}{
		{"正常系: 解凍後サイズの合計が上限ちょうどなら展開できる", []entrySegmentSpec{compressedA, compressedB}, 2 * segmentSize, false},
		{"正常系: 非圧縮セグメントの生バイトは解凍後サイズの合計に数えない", []entrySegmentSpec{compressedA, raw, compressedB}, 2 * segmentSize, false},
		{"異常系: 各セグメントは宣言サイズ内でも合計が上限を1バイト超えればErrDecompressedTooLarge", []entrySegmentSpec{compressedA, compressedB}, 2*segmentSize - 1, true},
		{"異常系: 先頭セグメントで上限を使い切った後に膨張するセグメントはErrDecompressedTooLarge", []entrySegmentSpec{compressedA, compressedB}, segmentSize, true},
		{"異常系: 単独のセグメントが上限を超えて膨張する場合はErrDecompressedTooLarge", []entrySegmentSpec{compressedA}, segmentSize / 2, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, segments := buildEntrySource(t, tc.specs)
			outputPath := filepath.Join(t.TempDir(), "entry.bin")

			err := extractEntry(bytes.NewReader(source), XP3FileEntry{Name: "entry.bin", Segments: segments}, outputPath, tc.limit)

			if tc.wantErr {
				require.ErrorIs(t, err, ErrInvalidXP3)
				require.ErrorIs(t, err, ErrDecompressedTooLarge)
				assert.Contains(t, err.Error(), strconv.FormatInt(tc.limit, 10))
				assert.NoFileExists(t, outputPath)
				return
			}
			require.NoError(t, err)
			var want []byte
			for _, spec := range tc.specs {
				want = append(want, spec.data...)
			}
			got, err := os.ReadFile(outputPath) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestExtractEntry_OutputFileError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
	}{
		{"異常系: 出力先が既存の空ディレクトリならファイルを開けずにエラーを返し、ディレクトリは削除しない"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, segments := buildEntrySource(t, []entrySegmentSpec{{data: []byte("payload")}})
			outputPath := filepath.Join(t.TempDir(), "entry.bin")
			require.NoError(t, os.Mkdir(outputPath, 0o750))

			err := extractEntry(bytes.NewReader(source), XP3FileEntry{Name: "entry.bin", Segments: segments}, outputPath, maxSegmentDecompressedSize)

			require.Error(t, err)
			require.NotErrorIs(t, err, ErrInvalidXP3)
			assert.DirExists(t, outputPath)
		})
	}
}

// TestExtractEntry_ExistingOutputFile は、同名エントリの上書きなどで出力先に既に
// ファイルがある場合の挙動を検証する。
func TestExtractEntry_ExistingOutputFile(t *testing.T) {
	t.Parallel()

	const segmentSize = 1024

	compressedA := entrySegmentSpec{data: bytes.Repeat([]byte{'A'}, segmentSize), compressed: true}
	compressedB := entrySegmentSpec{data: bytes.Repeat([]byte{'B'}, segmentSize), compressed: true}
	existing := bytes.Repeat([]byte{'X'}, 4*segmentSize)

	cases := []struct {
		name    string
		specs   []entrySegmentSpec
		limit   int64
		wantErr bool
	}{
		{"正常系: 既存のより長いファイルは切り詰められ、展開結果だけが残る", []entrySegmentSpec{compressedA}, segmentSize, false},
		{"異常系: 展開に失敗した場合は既存のファイルも残さない", []entrySegmentSpec{compressedA, compressedB}, 2*segmentSize - 1, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, segments := buildEntrySource(t, tc.specs)
			outputPath := filepath.Join(t.TempDir(), "entry.bin")
			require.NoError(t, os.WriteFile(outputPath, existing, 0o600))

			err := extractEntry(bytes.NewReader(source), XP3FileEntry{Name: "entry.bin", Segments: segments}, outputPath, tc.limit)

			if tc.wantErr {
				require.ErrorIs(t, err, ErrDecompressedTooLarge)
				assert.NoFileExists(t, outputPath)
				return
			}
			require.NoError(t, err)
			var want []byte
			for _, spec := range tc.specs {
				want = append(want, spec.data...)
			}
			got, err := os.ReadFile(outputPath) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}
