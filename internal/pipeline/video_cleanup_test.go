package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/converter"
)

// TestRemoveStaleVideoSourceFiles はVideoConverterが拡張子を変更して変換に
// 成功した場合、copyTree由来の旧拡張子ファイルが削除されることを検証する。
//
// MIDIがT-220で解決した「変換成功後に旧ファイルを消す」のと同じ方法
// （convertMidiFileList内のos.Remove(midiFile)、失敗はビルドを落とさない
// best-effort）を、ConversionManager経由で処理される動画にも適用する。
func TestRemoveStaleVideoSourceFiles(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 拡張子が変わった変換成功時は旧ファイルを削除する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		staleFile := filepath.Join(dir, "movie.wmv")
		convertedFile := filepath.Join(dir, "movie.mpg")
		require.NoError(t, os.WriteFile(staleFile, []byte("raw copy from copyTree"), 0o600))
		require.NoError(t, os.WriteFile(convertedFile, []byte("converted mpeg-ps"), 0o600))

		summary := converter.ConversionSummary{Results: []converter.ConversionResult{
			{
				SourcePath: filepath.Join(dir, "extract", "movie.wmv"),
				DestPath:   convertedFile,
				Status:     converter.StatusSuccess,
			},
		}}

		removeStaleVideoSourceFiles(summary)

		assert.NoFileExists(t, staleFile)
		assert.FileExists(t, convertedFile)
	})

	t.Run("正常系: パススルー(拡張子が変わらない)場合は何もしない", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		mpgFile := filepath.Join(dir, "movie.mpg")
		require.NoError(t, os.WriteFile(mpgFile, []byte("mpeg-ps passthrough"), 0o600))

		summary := converter.ConversionSummary{Results: []converter.ConversionResult{
			{
				SourcePath: filepath.Join(dir, "extract", "movie.mpg"),
				DestPath:   mpgFile,
				Status:     converter.StatusSuccess,
			},
		}}

		removeStaleVideoSourceFiles(summary)

		assert.FileExists(t, mpgFile)
	})

	t.Run("異常系: 変換が失敗した結果は旧ファイルを消さない", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		staleFile := filepath.Join(dir, "movie.wmv")
		require.NoError(t, os.WriteFile(staleFile, []byte("raw copy from copyTree"), 0o600))

		summary := converter.ConversionSummary{Results: []converter.ConversionResult{
			{
				SourcePath: filepath.Join(dir, "extract", "movie.wmv"),
				DestPath:   filepath.Join(dir, "movie.mpg"),
				Status:     converter.StatusFailed,
			},
		}}

		removeStaleVideoSourceFiles(summary)

		assert.FileExists(t, staleFile)
	})

	t.Run("正常系: 動画変換器の対象外拡張子は削除しない（スコープ外）", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		staleFile := filepath.Join(dir, "image.tlg")
		require.NoError(t, os.WriteFile(staleFile, []byte("tlg raw copy"), 0o600))

		summary := converter.ConversionSummary{Results: []converter.ConversionResult{
			{
				SourcePath: filepath.Join(dir, "extract", "image.tlg"),
				DestPath:   filepath.Join(dir, "image.png"),
				Status:     converter.StatusSuccess,
			},
		}}

		removeStaleVideoSourceFiles(summary)

		assert.FileExists(t, staleFile)
	})

	t.Run("正常系: 旧ファイルが既に存在しなくてもエラーにならない", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		summary := converter.ConversionSummary{Results: []converter.ConversionResult{
			{
				SourcePath: filepath.Join(dir, "extract", "movie.avi"),
				DestPath:   filepath.Join(dir, "movie.mpg"),
				Status:     converter.StatusSuccess,
			},
		}}

		assert.NotPanics(t, func() { removeStaleVideoSourceFiles(summary) })
	})

	t.Run("正常系: 大文字拡張子(.WMV)の旧ファイルもケースを保持して削除できる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// why: withSuffixはSourcePathに記録された拡張子のケースをそのまま使って
		// 旧ファイルパスを再構築する。小文字化して再構築すると、大文字小文字を
		// 区別するファイルシステム(Android向けビルドを行うLinux CI/DevContainer)
		// 上では実際にcopyTreeが複製した"OP.WMV"を見つけられず削除に失敗する。
		skipIfCaseInsensitiveFS(t, dir)

		staleFile := filepath.Join(dir, "OP.WMV")
		convertedFile := filepath.Join(dir, "OP.mpg")
		require.NoError(t, os.WriteFile(staleFile, []byte("raw copy from copyTree"), 0o600))
		require.NoError(t, os.WriteFile(convertedFile, []byte("converted mpeg-ps"), 0o600))

		summary := converter.ConversionSummary{Results: []converter.ConversionResult{
			{
				SourcePath: filepath.Join(dir, "extract", "OP.WMV"),
				DestPath:   convertedFile,
				Status:     converter.StatusSuccess,
			},
		}}

		removeStaleVideoSourceFiles(summary)

		assert.NoFileExists(t, staleFile)
		assert.FileExists(t, convertedFile)
	})

	t.Run("正常系: 大文字綴り(.MPG)でも別実体の生ファイルは削除する（ケースセンシティブFS）", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// why: SourcePathとDestPathの拡張子がEqualFold(".MPG",".mpg")で
		// 一致するからといって同一実体とは限らない。ケースセンシティブFS
		// (Android向けビルドを行うLinux CI/DevContainer)では"OP.MPG"と
		// "OP.mpg"は別ファイルであり、copyTreeが複製した未変換の"OP.MPG"を
		// 拡張子文字列の一致だけで見逃すと、変換済みの"OP.mpg"と共存したまま
		// 残ってしまう（normalizeCriticalFilenamesの不安定ソートにより、
		// どちらが最終的な"op.mpg"になるか未規定になる）。
		skipIfCaseInsensitiveFS(t, dir)

		staleFile := filepath.Join(dir, "OP.MPG")
		convertedFile := filepath.Join(dir, "OP.mpg")
		require.NoError(t, os.WriteFile(staleFile, []byte("raw unconverted copy from copyTree"), 0o600))
		require.NoError(t, os.WriteFile(convertedFile, []byte("converted mpeg-ps"), 0o600))

		summary := converter.ConversionSummary{Results: []converter.ConversionResult{
			{
				SourcePath: filepath.Join(dir, "extract", "OP.MPG"),
				DestPath:   convertedFile,
				Status:     converter.StatusSuccess,
			},
		}}

		removeStaleVideoSourceFiles(summary)

		assert.NoFileExists(t, staleFile)
		assert.FileExists(t, convertedFile)

		got, err := os.ReadFile(convertedFile) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Equal(t, "converted mpeg-ps", string(got))
	})

	t.Run("正常系: 大文字綴り(.MPG)がdestと同一実体を指す場合は削除しない（ケース非区別FSの保護）", func(t *testing.T) {
		t.Parallel()

		// why: このケースにskipIfCaseInsensitiveFSは付けない。ケース非区別FS
		// (macOS既定)では"OP.MPG"と"op.mpg"が同一実体を指すため、
		// os.SameFileによる同一実体判定が正しく機能していれば
		// destを消してしまわない。ケースセンシティブFSでは"OP.MPG"は
		// 実在しないため削除自体が空振りするだけで、いずれの環境でも
		// destが生き残ることを検証できる。
		dir := t.TempDir()
		convertedFile := filepath.Join(dir, "op.mpg")
		require.NoError(t, os.WriteFile(convertedFile, []byte("converted mpeg-ps"), 0o600))

		summary := converter.ConversionSummary{Results: []converter.ConversionResult{
			{
				SourcePath: filepath.Join(dir, "extract", "OP.MPG"),
				DestPath:   convertedFile,
				Status:     converter.StatusSuccess,
			},
		}}

		removeStaleVideoSourceFiles(summary)

		assert.FileExists(t, convertedFile)

		got, err := os.ReadFile(convertedFile) //nolint:gosec // テストで自身が書き出した一時ファイルを読む用途のため妥当
		require.NoError(t, err)
		assert.Equal(t, "converted mpeg-ps", string(got))
	})

	t.Run("正常系: 動画結果を含まないsummaryでは何もしない（--skip-video相当）", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		scriptDest := filepath.Join(dir, "script.ks")
		require.NoError(t, os.WriteFile(scriptDest, []byte("script content"), 0o600))

		// why: --skip-video時はVideoConverterがconvertersに登録されないため
		// (phases.go executeConvert参照)、ConversionManagerのsummaryには
		// 動画由来の結果が一切含まれない。空のsummaryと非動画結果のみの
		// summaryの両方で無害であることを確認し、「自然に無害なはず」という
		// 前提を実際に検証する。
		assert.NotPanics(t, func() { removeStaleVideoSourceFiles(converter.ConversionSummary{}) })

		summary := converter.ConversionSummary{Results: []converter.ConversionResult{
			{
				SourcePath: filepath.Join(dir, "extract", "script.ks"),
				DestPath:   scriptDest,
				Status:     converter.StatusSuccess,
			},
		}}

		removeStaleVideoSourceFiles(summary)

		assert.FileExists(t, scriptDest)
	})
}
