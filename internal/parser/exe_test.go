package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/parser"
)

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, content, 0o600))
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
		assert.Equal(t, exeFile, extractor.ExePath())
	})
}

func TestEmbeddedXP3Extractor_FindEmbeddedXP3(t *testing.T) {
	t.Parallel()

	t.Run("正常系: XP3が埋め込まれていないEXEでは空リスト", func(t *testing.T) {
		t.Parallel()

		exeFile := filepath.Join(t.TempDir(), "no_xp3.exe")
		writeFile(t, exeFile, append([]byte("MZ"), make([]byte, 100)...))

		extractor, err := parser.NewEmbeddedXP3Extractor(exeFile)
		require.NoError(t, err)

		result, err := extractor.FindEmbeddedXP3()

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("正常系: 埋め込みXP3のオフセットを検出", func(t *testing.T) {
		t.Parallel()

		content := append([]byte("MZ"), make([]byte, 100)...)
		content = append(content, parser.XP3Magic...)
		content = append(content, make([]byte, 50)...)

		exeFile := filepath.Join(t.TempDir(), "with_xp3.exe")
		writeFile(t, exeFile, content)

		extractor, err := parser.NewEmbeddedXP3Extractor(exeFile)
		require.NoError(t, err)

		result, err := extractor.FindEmbeddedXP3()

		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, int64(102), result[0].Offset)
		assert.Equal(t, int64(61), result[0].EstimatedSize)
	})

	t.Run("正常系: 複数のXP3が埋め込まれている場合", func(t *testing.T) {
		t.Parallel()

		content := append([]byte("MZ"), make([]byte, 100)...)
		content = append(content, parser.XP3Magic...)
		content = append(content, make([]byte, 50)...)
		content = append(content, parser.XP3Magic...)
		content = append(content, make([]byte, 30)...)

		exeFile := filepath.Join(t.TempDir(), "multi_xp3.exe")
		writeFile(t, exeFile, content)

		extractor, err := parser.NewEmbeddedXP3Extractor(exeFile)
		require.NoError(t, err)

		result, err := extractor.FindEmbeddedXP3()

		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, int64(102), result[0].Offset)
		assert.Equal(t, int64(163), result[1].Offset)
	})
}

func TestEmbeddedXP3Extractor_ExtractAll(t *testing.T) {
	t.Parallel()

	t.Run("正常系: XP3ファイルを抽出", func(t *testing.T) {
		t.Parallel()

		xp3Content := append(append([]byte{}, parser.XP3Magic...), make([]byte, 50)...)
		exeContent := append([]byte("MZ"), make([]byte, 100)...)
		exeContent = append(exeContent, xp3Content...)

		tmpDir := t.TempDir()
		exeFile := filepath.Join(tmpDir, "test.exe")
		writeFile(t, exeFile, exeContent)

		outputDir := filepath.Join(tmpDir, "extracted")
		require.NoError(t, os.Mkdir(outputDir, 0o750))

		extractor, err := parser.NewEmbeddedXP3Extractor(exeFile)
		require.NoError(t, err)

		result, err := extractor.ExtractAll(outputDir)

		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.FileExists(t, result[0])
		assert.Equal(t, ".xp3", filepath.Ext(result[0]))

		data, err := os.ReadFile(result[0]) //nolint:gosec // テストで生成した既知のパスを読むだけのため妥当
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(data), len(parser.XP3Magic))
		assert.Equal(t, parser.XP3Magic, data[:len(parser.XP3Magic)])
	})

	t.Run("正常系: 出力ディレクトリが存在しない場合は作成", func(t *testing.T) {
		t.Parallel()

		xp3Content := append(append([]byte{}, parser.XP3Magic...), make([]byte, 50)...)
		exeContent := append([]byte("MZ"), make([]byte, 100)...)
		exeContent = append(exeContent, xp3Content...)

		tmpDir := t.TempDir()
		exeFile := filepath.Join(tmpDir, "test.exe")
		writeFile(t, exeFile, exeContent)

		outputDir := filepath.Join(tmpDir, "new_dir")
		require.NoDirExists(t, outputDir)

		extractor, err := parser.NewEmbeddedXP3Extractor(exeFile)
		require.NoError(t, err)

		result, err := extractor.ExtractAll(outputDir)

		require.NoError(t, err)
		assert.DirExists(t, outputDir)
		assert.Len(t, result, 1)
	})

	t.Run("正常系: XP3が埋め込まれていない場合は空リスト", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		exeFile := filepath.Join(tmpDir, "no_xp3.exe")
		writeFile(t, exeFile, append([]byte("MZ"), make([]byte, 100)...))

		outputDir := filepath.Join(tmpDir, "extracted")

		extractor, err := parser.NewEmbeddedXP3Extractor(exeFile)
		require.NoError(t, err)

		result, err := extractor.ExtractAll(outputDir)

		require.NoError(t, err)
		assert.Empty(t, result)
	})
}
