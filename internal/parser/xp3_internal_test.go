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

// plannedOutputSizeはパッケージ非公開ヘルパーであり、GiB級やint64上限近くの宣言
// サイズを、実データを1バイトも書かずに索引だけで検証するため、ホワイトボックス
// テストとする。
func TestPlannedOutputSize(t *testing.T) {
	t.Parallel()

	const fileSize = 1000

	stored := func(offset, size int64) XP3Segment {
		return XP3Segment{Offset: offset, Size: size, OriginalSize: size}
	}
	compressed := func(size, originalSize int64) XP3Segment {
		return XP3Segment{Offset: 0, Size: size, OriginalSize: originalSize, IsCompressed: true}
	}
	entry := func(segments ...XP3Segment) XP3FileEntry {
		return XP3FileEntry{Name: "entry.bin", Segments: segments}
	}

	cases := []struct {
		name     string
		entries  []XP3FileEntry
		fileSize int64
		want     int64
	}{
		{"正常系: エントリが無ければ0", nil, fileSize, 0},
		{"正常系: 非圧縮セグメントは宣言サイズを数える", []XP3FileEntry{entry(stored(100, 200))}, fileSize, 200},
		{"正常系: 非圧縮セグメントがオフセット以降の残量を超える場合は残量へクランプする", []XP3FileEntry{entry(stored(100, 5000))}, fileSize, 900},
		{"正常系: オフセットがファイル末尾より先の非圧縮セグメントは0", []XP3FileEntry{entry(stored(fileSize+10, 10))}, fileSize, 0},
		{"正常系: 圧縮セグメントは宣言した解凍後サイズを数える", []XP3FileEntry{entry(compressed(100, 500))}, fileSize, 500},
		{"正常系: 圧縮セグメントの解凍後サイズが0なら生データの1032倍を数える", []XP3FileEntry{entry(compressed(100, 0))}, fileSize, 100 * 1032},
		{"正常系: 圧縮セグメントの解凍後サイズが0でも64MiBを上限にする", []XP3FileEntry{entry(compressed(1<<20, 0))}, 1 << 20, 64 << 20},
		{"正常系: 圧縮セグメントの宣言した解凍後サイズが生データの1032倍を超えれば1032倍を数える", []XP3FileEntry{entry(compressed(100, 1<<20))}, fileSize, 100 * 1032},
		{"正常系: 圧縮セグメントの解凍後サイズは1GiBを上限にする", []XP3FileEntry{entry(compressed(2<<20, 5<<30))}, 4 << 20, 1 << 30},
		{"正常系: 生データが0バイトの圧縮セグメントは0", []XP3FileEntry{entry(compressed(0, 500))}, fileSize, 0},
		{"正常系: オフセットがファイル末尾より先の圧縮セグメントは0", []XP3FileEntry{entry(XP3Segment{Offset: fileSize + 10, Size: 8, OriginalSize: 0, IsCompressed: true})}, fileSize, 0},
		{"正常系: 末尾で途切れた圧縮セグメントは残量の1032倍を数える", []XP3FileEntry{entry(XP3Segment{Offset: fileSize - 10, Size: 500, OriginalSize: 0, IsCompressed: true})}, fileSize, 10 * 1032},
		{"正常系: 4KiBのスクリプトと空の圧縮スクリプト1件", emptyScriptsProbe(1), 1 << 20, 4096 + 1*8*1032},
		{"正常系: 4KiBのスクリプトと空の圧縮スクリプト10件", emptyScriptsProbe(10), 1 << 20, 4096 + 10*8*1032},
		{"正常系: 4KiBのスクリプトと空の圧縮スクリプト100件でも1MiBに満たない", emptyScriptsProbe(100), 1 << 20, 4096 + 100*8*1032},
		{"正常系: 圧縮セグメントの生データが解凍後サイズより大きければ生データを数える", []XP3FileEntry{entry(compressed(800, 1))}, fileSize, 800},
		{"正常系: 圧縮フラグでも圧縮前後のサイズが等しいセグメントは非圧縮として残量へクランプする", []XP3FileEntry{entry(compressed(5000, 5000))}, fileSize, fileSize},
		{"正常系: 同じ範囲を共有する複数エントリはエントリごとに数える", []XP3FileEntry{entry(stored(0, 400)), entry(stored(0, 400)), entry(stored(0, 400))}, fileSize, 1200},
		{"正常系: 同じ範囲を指す複数の非圧縮セグメントもセグメントごとに数える", []XP3FileEntry{entry(stored(0, 400), stored(0, 400))}, fileSize, 800},
		{"正常系: 1エントリの合計はアーカイブサイズと解凍上限1GiBの和を上限にする", []XP3FileEntry{entry(compressed(2<<20, 1<<30), compressed(2<<20, 1<<30), compressed(2<<20, 1<<30))}, 4 << 20, 4<<20 + 1<<30},
		{"正常系: 1エントリ内の合計がint64を超える場合は最大値で飽和する", []XP3FileEntry{entry(stored(0, math.MaxInt64), stored(0, math.MaxInt64))}, math.MaxInt64 - 10, math.MaxInt64},
		{"正常系: エントリの合計がint64を超える場合は最大値で飽和する", []XP3FileEntry{entry(stored(0, math.MaxInt64)), entry(stored(0, math.MaxInt64))}, math.MaxInt64 - 10, math.MaxInt64},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := plannedOutputSize(tc.entries, tc.fileSize)

			assert.Equal(t, tc.want, got)
		})
	}
}

// emptyScriptsProbe は、4KiBのスクリプト1件（1500バイトへzlib圧縮）と、空の
// スクリプトn件からなるエントリ列を返す。krkrrel（krdevui RelSettingsUnit.cpp）は
// 圧縮対象の拡張子のファイルを空でもzlibで圧縮し（格納サイズ8バイト・解凍後
// サイズ0の圧縮セグメントになる）、内容が同一のファイルは格納し直さず先の
// セグメント情報を複写するため、空のスクリプトはすべて同じ8バイトを指す。
// 見積もりはエントリごとに数えるため、範囲を共有していても別々でも同じ値になる。
func emptyScriptsProbe(n int) []XP3FileEntry {
	entries := []XP3FileEntry{{Name: "first.ks", Segments: []XP3Segment{{Offset: 0, Size: 1500, OriginalSize: 4096, IsCompressed: true}}}}
	for i := range n {
		entries = append(entries, XP3FileEntry{
			Name:     "empty" + strconv.Itoa(i) + ".ks",
			Segments: []XP3Segment{{Offset: 1500, Size: 8, OriginalSize: 0, IsCompressed: true}},
		})
	}

	return entries
}

// maxDeflateRatioは物理的な上限であり、実際のzlibの膨張率がこれを超えないことを
// 大きな膨張率を生むゼロ列の最大圧縮で確かめる。
func TestMaxDeflateRatio(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		size  int
		level int
	}{
		{"正常系: 16MiBのゼロ列を最大圧縮しても膨張率は上限以下", 16 << 20, zlib.BestCompression},
		{"正常系: 16MiBのゼロ列を既定の圧縮レベルで圧縮しても膨張率は上限以下", 16 << 20, zlib.DefaultCompression},
		{"正常系: 1MiBのゼロ列を最大圧縮しても膨張率は上限以下", 1 << 20, zlib.BestCompression},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var compressed bytes.Buffer
			w, err := zlib.NewWriterLevel(&compressed, tc.level)
			require.NoError(t, err)
			_, err = w.Write(make([]byte, tc.size))
			require.NoError(t, err)
			require.NoError(t, w.Close())

			assert.GreaterOrEqual(t, int64(compressed.Len())*maxDeflateRatio, int64(tc.size))
		})
	}
}

func TestSaturatingMul(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a    int64
		b    int64
		want int64
	}{
		{"正常系: 0との積は0", 0, maxDeflateRatio, 0},
		{"正常系: int64に収まる積はそのまま返す", 8, maxDeflateRatio, 8 * 1032},
		{"正常系: 積がちょうどint64に収まる境界ではそのまま返す", math.MaxInt64 / maxDeflateRatio, maxDeflateRatio, math.MaxInt64 / maxDeflateRatio * maxDeflateRatio},
		{"正常系: 積がint64を超える場合は最大値で飽和する", math.MaxInt64/maxDeflateRatio + 1, maxDeflateRatio, math.MaxInt64},
		{"正常系: 最大値同士の積も最大値で飽和する", math.MaxInt64, math.MaxInt64, math.MaxInt64},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, saturatingMul(tc.a, tc.b))
		})
	}
}
