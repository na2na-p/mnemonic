package tlg_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/converter/tlg"
)

func TestLZSSDecoder_Constants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 4096, tlg.WindowSize)
	assert.Equal(t, 18, tlg.MatchMaxLength)
	assert.Equal(t, 3, tlg.MatchMinLength)
}

func TestLZSSDecoder_Decode(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 空データの解凍は空バイト列を返す", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewLZSSDecoder()
		result, err := d.Decode([]byte{}, 0)

		require.NoError(t, err)
		assert.Equal(t, []byte{}, result)
	})

	cases := map[string]struct {
		data       []byte
		outputSize int
		expected   []byte
	}{
		"正常系: 全リテラル-フラグ0x00-8バイト": {
			data:       append([]byte{0x00}, []byte("ABCDEFGH")...),
			outputSize: 8,
			expected:   []byte("ABCDEFGH"),
		},
		"正常系: 全リテラル-フラグ0x00-数字8バイト": {
			data:       append([]byte{0x00}, []byte("12345678")...),
			outputSize: 8,
			expected:   []byte("12345678"),
		},
		"正常系: 3バイトリテラル": {
			data:       append([]byte{0x00}, []byte("ABC")...),
			outputSize: 3,
			expected:   []byte("ABC"),
		},
		"正常系: 単純なマッチ-繰り返しパターン": {
			// フラグ=0x08: ビット0,1,2=0(リテラル), ビット3=1(マッチ)
			// リテラル: "ABC" (3バイト)、マッチ: position=0, length=0 (実際は3バイト)
			data:       append(append([]byte{0x08}, []byte("ABC")...), 0x00, 0x00),
			outputSize: 6,
			expected:   []byte("ABCABC"),
		},
		"正常系: 長いマッチパターン": {
			// フラグ=0x02: ビット0=0(リテラル), ビット1=1(マッチ)
			// リテラル: "A"、マッチ: position=0, length=6 (実際は9バイト=6+3)
			data:       append([]byte{0x02, 'A'}, 0x00, 0x60),
			outputSize: 10,
			expected:   []byte("AAAAAAAAAA"),
		},
		"正常系: リテラルとマッチの混合パターン": {
			// フラグ=0x10: ビット0-3=0(リテラル4つ), ビット4=1(マッチ)
			// リテラル: "ABCD"、マッチ: position=0, length=0 (実際は3バイト)
			data:       append(append([]byte{0x10}, []byte("ABCD")...), 0x00, 0x00),
			outputSize: 7,
			expected:   []byte("ABCDABC"),
		},
		"正常系: 複数のフラグバイトを含むデータ": {
			data: append(
				append(append([]byte{0x00}, []byte("12345678")...), 0x00),
				[]byte("AB")...,
			),
			outputSize: 10,
			expected:   []byte("12345678AB"),
		},
		"正常系: position指定のマッチ": {
			// フラグ=0x10: リテラル"ABCD"、マッチ: position=2(C), length=0(3バイト)
			data:       append(append([]byte{0x10}, []byte("ABCD")...), 0x02, 0x00),
			outputSize: 7,
			expected:   []byte("ABCDCDC"),
		},
		"正常系: 重複するマッチ": {
			// フラグ=0x02: リテラル"X"、マッチ: position=0, length=2(実際は5バイト=2+3)
			data:       append([]byte{0x02, 'X'}, 0x00, 0x20),
			outputSize: 6,
			expected:   []byte("XXXXXX"),
		},
		"正常系: 最大マッチ長18バイト": {
			// フラグ=0x02: リテラル"A"、マッチ: position=0, length=15(mlen=15+3=18)
			// mlen==18のため追加バイト(0x00)を読み、最終長=18+0=18
			data:       append([]byte{0x02, 'A'}, 0x00, 0xf0, 0x00),
			outputSize: 19,
			expected:   []byte("AAAAAAAAAAAAAAAAAAA"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			d := tlg.NewLZSSDecoder()
			result, err := d.Decode(tc.data, tc.outputSize)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
			assert.Len(t, result, tc.outputSize)
		})
	}
}

func TestLZSSDecoder_Decode_DictionaryPersistsAcrossCalls(t *testing.T) {
	t.Parallel()

	// DEFECT 2の回帰防止: krkrz TVPLoadTLG5はスライド辞書を画像1枚のデコード
	// （全チャンク）を通じて持続させる。同一LZSSDecoderへの2回目のDecodeは、
	// 1回目が辞書へ書き込んだバイトをバックリファレンスで参照できなければ
	// ならない。旧実装（チャンクごとに辞書をゼロ初期化）ではここで
	// []byte{0,0,0}が返り、このテストは失敗する。
	d := tlg.NewLZSSDecoder()

	first, err := d.Decode(append([]byte{0x00}, []byte("ABC")...), 3)
	require.NoError(t, err)
	assert.Equal(t, []byte("ABC"), first)

	// フラグ0x01(ビット0=マッチ)、mpos=0・mlen=3。持続する辞書のslide[0..2]は
	// 1回目で書き込んだ"ABC"を保持しているため、出力は再び"ABC"になる。
	second, err := d.Decode([]byte{0x01, 0x00, 0x00}, 3)
	require.NoError(t, err)
	assert.Equal(t, []byte("ABC"), second)
}

func TestLZSSDecoder_Decode_FreshDecoderStartsFromZeroDictionary(t *testing.T) {
	t.Parallel()

	// 新しいLZSSDecoderは常にゼロ初期化された辞書から始まる（画像間で辞書が
	// 汚染されないことの担保。tlg5.goが画像ごとにNewLZSSDecoderを生成する前提）。
	d := tlg.NewLZSSDecoder()

	// フラグ0x01(マッチ)、mpos=0・mlen=3。辞書が未書き込み(ゼロ)のため出力は
	// ゼロ3バイトになる。
	result, err := d.Decode([]byte{0x01, 0x00, 0x00}, 3)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 0x00, 0x00}, result)
}

func TestLZSSDecoder_Decode_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("異常系: 不完全なリテラルデータでエラーを返す", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewLZSSDecoder()
		data := append([]byte{0x00}, []byte("ABC")...)

		_, err := d.Decode(data, 8)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrLZSSIncompleteData)
	})

	t.Run("異常系: 不完全なマッチ情報でエラーを返す", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewLZSSDecoder()
		// フラグ=0x01(ビット0がマッチ)だがマッチ情報が1バイトしかない
		data := []byte{0x01, 0x00}

		_, err := d.Decode(data, 3)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrLZSSIncompleteData)
	})

	t.Run("異常系: フラグバイトが不足している場合エラーを返す", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewLZSSDecoder()

		_, err := d.Decode([]byte{}, 1)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrLZSSIncompleteData)
	})

	t.Run("異常系: 追加長バイトが不足している場合エラーを返す", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewLZSSDecoder()
		// mlen==18になるがその後の追加長バイトが無い
		data := []byte{0x02, 'A', 0x00, 0xf0}

		_, err := d.Decode(data, 19)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrLZSSIncompleteData)
	})
}
