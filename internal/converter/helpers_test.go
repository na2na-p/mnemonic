package converter_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/japanese"
)

// writeFile はcontentをpathへ書き込むテストヘルパー。
func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, content, 0o600))
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
