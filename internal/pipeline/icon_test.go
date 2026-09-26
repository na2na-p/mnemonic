package pipeline

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeIconExtractor はparser.IconExtractorのテスト用実装。
type fakeIconExtractor struct {
	path string
	err  error
}

func (f fakeIconExtractor) Extract(string, string) (string, error) {
	return f.path, f.err
}

func TestBuildPipeline_FindGameIcon_FallsBackToExeExtraction(t *testing.T) {
	t.Parallel()

	t.Run("正常系: アイコンファイルがない場合はEXEからアイコン抽出を試みる", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		extractDir := t.TempDir()
		p.extractDir = extractDir

		extractedIcon := filepath.Join(extractDir, "extracted_icon.png")
		require.NoError(t, os.WriteFile(extractedIcon, []byte("\x89PNG\r\n\x1a\n"), 0o600))

		result := p.findGameIconUsing(fakeIconExtractor{path: extractedIcon})

		assert.Equal(t, extractedIcon, result)
	})

	t.Run("正常系: EXE抽出も失敗した場合は空文字列を返す", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		extractDir := t.TempDir()
		p.extractDir = extractDir

		result := p.findGameIconUsing(fakeIconExtractor{err: errors.New("抽出失敗")})

		assert.Empty(t, result)
	})

	t.Run("正常系: 既存アイコンファイルが見つかった場合はEXE抽出を試みない", func(t *testing.T) {
		t.Parallel()

		p := newTestPipeline(t)
		extractDir := t.TempDir()
		p.extractDir = extractDir
		pngPath := filepath.Join(extractDir, "icon.png")
		require.NoError(t, os.WriteFile(pngPath, []byte("\x89PNG\r\n\x1a\n"), 0o600))

		// このエクストラクタが呼ばれた場合は必ずエラーになるため、
		// 呼ばれていないことを間接的に検証する。
		result := p.findGameIconUsing(fakeIconExtractor{err: errors.New("呼ばれてはいけない")})

		assert.Equal(t, pngPath, result)
	})
}
