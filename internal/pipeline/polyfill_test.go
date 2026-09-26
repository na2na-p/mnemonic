package pipeline

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/builder"
	"github.com/na2na-p/mnemonic/internal/resources"
)

func TestBuildPipeline_CopyPolyfillFiles(t *testing.T) {
	t.Parallel()

	t.Run("正常系: systemディレクトリが作成される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		require.NoError(t, copyPolyfillFilesUsing(dir, offlineFontFetcher(t)))

		assert.DirExists(t, filepath.Join(dir, "system"))
	})

	t.Run("正常系: 全ポリフィルファイルがコピーされる（SaveDataPath_patch.tjsを除く）", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		require.NoError(t, copyPolyfillFilesUsing(dir, offlineFontFetcher(t)))

		systemDir := filepath.Join(dir, "system")
		for _, name := range resources.SystemPolyfillFiles {
			want, err := resources.SystemPolyfillFS.ReadFile("system_polyfill/" + name)
			require.NoError(t, err)

			got, readErr := os.ReadFile(filepath.Join(systemDir, name)) //nolint:gosec // 埋め込みリソースをコピーしたテスト用ファイルを読む用途のため妥当
			require.NoError(t, readErr)
			assert.Equal(t, want, got)
		}

		assert.NoFileExists(t, filepath.Join(systemDir, "SaveDataPath_patch.tjs"))
	})

	t.Run("正常系: フォント取得に失敗してもビルドは継続する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		fetcher := builder.NewFontFetcher(t.TempDir(), &http.Client{Transport: alwaysFailRoundTripper{}})

		err := copyPolyfillFilesUsing(dir, fetcher)

		require.NoError(t, err)
		assert.NoFileExists(t, filepath.Join(dir, "system", "font.ttf"))
	})
}

// TestCopyFontFile はcopyFontFile単体の挙動を検証する。
func TestCopyFontFile(t *testing.T) {
	t.Parallel()

	t.Run("正常系: キャッシュ済みフォントをsystem/font.ttfとしてコピーする", func(t *testing.T) {
		t.Parallel()

		systemDir := t.TempDir()

		require.NoError(t, copyFontFile(systemDir, offlineFontFetcher(t)))

		got, err := os.ReadFile(filepath.Join(systemDir, "font.ttf")) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Equal(t, []byte("fake koruri ttf content"), got)
	})

	t.Run("正常系: 既にfont.ttfが存在する場合は上書きしない", func(t *testing.T) {
		t.Parallel()

		systemDir := t.TempDir()
		fontDest := filepath.Join(systemDir, "font.ttf")
		require.NoError(t, os.WriteFile(fontDest, []byte("existing"), 0o600))

		require.NoError(t, copyFontFile(systemDir, offlineFontFetcher(t)))

		got, err := os.ReadFile(fontDest) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Equal(t, []byte("existing"), got)
	})

	t.Run("正常系: フォント取得に失敗した場合はエラーを返さずスキップする", func(t *testing.T) {
		t.Parallel()

		systemDir := t.TempDir()
		fetcher := builder.NewFontFetcher(t.TempDir(), &http.Client{Transport: alwaysFailRoundTripper{}})

		err := copyFontFile(systemDir, fetcher)

		require.NoError(t, err)
		assert.NoFileExists(t, filepath.Join(systemDir, "font.ttf"))
	})
}

// offlineFontFetcher はキャッシュ済みフォントを持つFontFetcherを返す
// （実ネットワークに触れずGetFont()が成功する）。
func offlineFontFetcher(t *testing.T) *builder.FontFetcher {
	t.Helper()

	cacheDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, builder.KoruriTTFFilename),
		[]byte("fake koruri ttf content"),
		0o600,
	))

	return builder.NewFontFetcher(cacheDir, &http.Client{Transport: alwaysFailRoundTripper{}})
}

// alwaysFailRoundTripper は常にエラーを返すhttp.RoundTripper。
// テストが誤って実ネットワークへ到達しないことを保証するために使う。
type alwaysFailRoundTripper struct{}

func (alwaysFailRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("テストでは実ネットワークへのアクセスを許可しない")
}
