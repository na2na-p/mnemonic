package converter_test

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/japanese"
)

// writeFile はcontentをpathへ書き込むテストヘルパー。
func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, content, 0o600))
}

// mkdirAll はpathをその親も含めて作成するテストヘルパー。
func mkdirAll(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, 0o750))
}

// writeFileInLockedDir はtempディレクトリ配下のディレクトリにnameのファイルを作成し、
// そのディレクトリの権限を0o000にしてファイルのパスを返すテストヘルパー。
// 返すパスはos.StatがEACCESで失敗する。
func writeFileInLockedDir(t *testing.T, name string, content []byte) string {
	t.Helper()

	locked := filepath.Join(t.TempDir(), "locked")
	mkdirAll(t, locked)
	path := filepath.Join(locked, name)
	writeFile(t, path, content)
	// t.TempDir()のクリーンアップは探索権限の無いディレクトリを削除できないため、
	// 権限を落とす前に戻す処理を登録する。
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) }) //nolint:gosec // テスト用の一時ディレクトリの権限を戻す用途のため妥当
	require.NoError(t, os.Chmod(locked, 0o000))

	return path
}

// readFile はpathの内容を読み込むテストヘルパー。
func readFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // テスト用の一時ファイルを読む用途のため妥当
	require.NoError(t, err)

	return data
}

// encodeSJIS はtextをShift_JISバイト列へエンコードするテストヘルパー。
func encodeSJIS(t *testing.T, text string) []byte {
	t.Helper()

	encoded, err := japanese.ShiftJIS.NewEncoder().String(text)
	require.NoError(t, err)

	return []byte(encoded)
}

// encodeEUCJP はtextをEUC-JPバイト列へエンコードするテストヘルパー。
func encodeEUCJP(t *testing.T, text string) []byte {
	t.Helper()

	encoded, err := japanese.EUCJP.NewEncoder().String(text)
	require.NoError(t, err)

	return []byte(encoded)
}

// writeSJIS はtextをShift_JISエンコードしてpathへ書き込むテストヘルパー。
func writeSJIS(t *testing.T, path, text string) {
	t.Helper()
	writeFile(t, path, encodeSJIS(t, text))
}

// writeEUCJP はtextをEUC-JPエンコードしてpathへ書き込むテストヘルパー。
func writeEUCJP(t *testing.T, path, text string) {
	t.Helper()
	writeFile(t, path, encodeEUCJP(t, text))
}

// assertFileUTF8Equals はpathの内容がUTF-8としてwantと一致することを検証する。
func assertFileUTF8Equals(t *testing.T, path, want string) {
	t.Helper()
	require.Equal(t, want, string(readFile(t, path)))
}

// encodeUTF16 はtextをUTF-16へ符号化する。bigEndianでバイト順を、withBOMでBOMの
// 有無を指定する。
//
// why not: golang.org/x/text/encoding/unicodeのエンコーダは使わない。
// 本番コードの復号に使う実装で期待値を作ると、実装の不具合がテストで打ち消される。
func encodeUTF16(text string, bigEndian, withBOM bool) []byte {
	var order binary.AppendByteOrder = binary.LittleEndian
	if bigEndian {
		order = binary.BigEndian
	}

	units := utf16.Encode([]rune(text))
	if withBOM {
		units = append([]uint16{0xfeff}, units...)
	}

	out := make([]byte, 0, len(units)*2)
	for _, u := range units {
		out = order.AppendUint16(out, u)
	}

	return out
}

// simpleCryptHeader は吉里吉里のsimple crypt形式の先頭（FE FE・モードバイト・
// UTF-16LEのBOM）を返す。
func simpleCryptHeader(mode byte) []byte {
	return []byte{0xfe, 0xfe, mode, 0xff, 0xfe}
}

// encodeSimpleCrypt はtextをBOM無しUTF-16LEへ符号化し、吉里吉里のsimple crypt
// mode 0/1で暗号化してsimpleCryptHeaderの後ろに続ける。
//
// why not: 本番コードの復号器で暗号文を作らない。復号器の不具合がテストで打ち消され
// ないよう式をここに独立して書く。吉里吉里本体の書き込み側（krkrz
// base/TextStream.cppのtTVPTextWriteStream）はmode 0を書けないため、暗号化にも
// 復号と同じ式を使う。mode 1の式は全符号単位で自身の逆変換だが、mode 0の式は
// 0x0202・0x0203・0x0404など30個の符号単位で逆変換にならない（暗号化すると0x20
// 未満になり復号で戻らない）ため、mode 0で渡すtextはそれらを含んではならない。
func encodeSimpleCrypt(t *testing.T, text string, mode byte) []byte {
	t.Helper()

	out := simpleCryptHeader(mode)
	for _, u := range utf16.Encode([]rune(text)) {
		switch mode {
		case 0:
			if u >= 0x20 {
				u ^= (u&0xfe)<<8 ^ 1
			}
		case 1:
			u = (u&0xaaaa)>>1 | (u&0x5555)<<1
		default:
			t.Fatalf("encodeSimpleCryptは mode 0/1 のみ扱う: %d", mode)
		}

		out = binary.LittleEndian.AppendUint16(out, u)
	}

	return out
}

// zlibCompress はdataをzlib形式で圧縮する。
func zlibCompress(t *testing.T, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, err := zw.Write(data)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return buf.Bytes()
}

// simpleCryptCompressed はsimple crypt mode 2の形式（先頭・圧縮後サイズ・展開後
// サイズ・zlibストリーム）のバイト列を組み立てる。サイズは宣言値をそのまま書くため、
// 実際のpayloadと食い違う壊れたデータも作れる。
func simpleCryptCompressed(compressed, uncompressed uint64, payload []byte) []byte {
	out := simpleCryptHeader(2)
	out = binary.LittleEndian.AppendUint64(out, compressed)
	out = binary.LittleEndian.AppendUint64(out, uncompressed)

	return append(out, payload...)
}

// encodeSimpleCryptCompressed はtextをBOM無しUTF-16LEへ符号化してzlibで圧縮し、
// simple crypt mode 2の形式で返す。
func encodeSimpleCryptCompressed(t *testing.T, text string) []byte {
	t.Helper()

	plain := encodeUTF16(text, false, false)
	compressed := zlibCompress(t, plain)

	return simpleCryptCompressed(uint64(len(compressed)), uint64(len(plain)), compressed)
}
