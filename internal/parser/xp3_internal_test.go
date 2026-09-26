package parser

import (
	"bytes"
	"io"
	"math"
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
				got      []byte
				consumed int64
				err      error
			)
			require.NotPanics(t, func() {
				got, consumed, err = readSegment(bytes.NewReader(data), tc.segment, fileSize, fileSize)
			})

			require.NoError(t, err)
			assert.Empty(t, got)
			assert.Equal(t, tc.wantConsumed, consumed)
		})
	}
}
