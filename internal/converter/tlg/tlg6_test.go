package tlg_test

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/converter/tlg"
)

// tlg6Header はTLG6形式のヘッダーバイト列を生成するテストヘルパー。
func tlg6Header(colors, dataFlags byte, width, height, xBlockCount, yBlockCount uint32) []byte {
	header := append([]byte{}, tlg.TLG6Magic...)
	header = append(header, colors, dataFlags)

	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, width)
	header = append(header, buf...)
	binary.LittleEndian.PutUint32(buf, height)
	header = append(header, buf...)
	binary.LittleEndian.PutUint32(buf, xBlockCount)
	header = append(header, buf...)
	binary.LittleEndian.PutUint32(buf, yBlockCount)
	header = append(header, buf...)

	return header
}

func TestTLG6Decoder_IsValid(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		data     []byte
		expected bool
	}{
		"正常系: 有効なTLG6マジックバイト": {append(append([]byte{}, tlg.TLG6Magic...), make([]byte, 20)...), true},
		"異常系: TLG5形式のマジックバイト": {append(append([]byte{}, tlg.TLG5Magic...), make([]byte, 20)...), false},
		"異常系: PNG形式のマジックバイト":  {[]byte("PNG\x89\x50\x4e\x47"), false},
		"異常系: 不完全なマジックバイト":    {[]byte("TLG6.0"), false},
		"異常系: 空のデータ":          {[]byte{}, false},
	}

	d := tlg.NewTLG6Decoder()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, d.IsValid(tc.data))
		})
	}
}

func TestTLG6Decoder_ParseHeader(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		colors, dataFlags        byte
		width, height            uint32
		xBlockCount, yBlockCount uint32
		expectedColors           int
	}{
		"正常系: RGBA画像ヘッダー解析":      {32, 0, 640, 480, 80, 60, 4},
		"正常系: RGB画像ヘッダー解析":       {24, 0, 800, 600, 100, 75, 3},
		"正常系: フラグ付きRGBA画像ヘッダー解析": {32, 1, 1920, 1080, 240, 135, 4},
		"正常系: 最小サイズ画像ヘッダー解析":     {24, 0, 1, 1, 1, 1, 3},
	}

	d := tlg.NewTLG6Decoder()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data := tlg6Header(tc.colors, tc.dataFlags, tc.width, tc.height, tc.xBlockCount, tc.yBlockCount)
			header, err := d.ParseHeader(data)

			require.NoError(t, err)
			assert.Equal(t, int(tc.width), header.Width)
			assert.Equal(t, int(tc.height), header.Height)
			assert.Equal(t, tc.expectedColors, header.Colors)
			assert.Equal(t, int(tc.dataFlags), header.DataFlags)
			assert.Equal(t, int(tc.xBlockCount), header.XBlockCount)
			assert.Equal(t, int(tc.yBlockCount), header.YBlockCount)
		})
	}

	t.Run("異常系: 無効なマジックバイトでErrTLG6InvalidMagicを返す", func(t *testing.T) {
		t.Parallel()

		data := append([]byte("INVALID_MAGIC"), make([]byte, 20)...)
		_, err := d.ParseHeader(data)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrTLG6InvalidMagic)
	})

	t.Run("異常系: データが短すぎる場合ErrTLG6DataTooShortを返す", func(t *testing.T) {
		t.Parallel()

		data := append(append([]byte{}, tlg.TLG6Magic...), make([]byte, 5)...)
		_, err := d.ParseHeader(data)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrTLG6DataTooShort)
	})
}

func TestTLG6Decoder_Decode(t *testing.T) {
	t.Parallel()

	t.Run("異常系: 無効なマジックバイトでErrTLG6InvalidMagicを返す", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewTLG6Decoder()
		data := append([]byte("INVALID_MAGIC"), make([]byte, 20)...)

		_, err := d.Decode(data)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrTLG6InvalidMagic)
	})

	t.Run("異常系: 有効なTLG6データはErrTLG6NotImplementedを返す（本体デコード未実装）", func(t *testing.T) {
		t.Parallel()

		d := tlg.NewTLG6Decoder()
		data := tlg6Header(32, 0, 8, 4, 1, 1)

		_, err := d.Decode(data)

		require.Error(t, err)
		assert.ErrorIs(t, err, tlg.ErrTLG6NotImplemented)
	})
}
