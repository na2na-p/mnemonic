package info_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/info"
)

func touch(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, nil, 0o600))
}

func TestDetectEngine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{name: "正常系: data.xp3があればkirikiri", files: []string{"data.xp3"}, want: "kirikiri"},
		{name: "正常系: video.xp3があればkirikiri", files: []string{"video.xp3"}, want: "kirikiri"},
		{name: "正常系: 任意の.xp3があればkirikiri", files: []string{"game.xp3"}, want: "kirikiri"},
		{name: "正常系: .rgss3aがあればrpgmaker", files: []string{"Game.rgss3a"}, want: "rpgmaker"},
		{name: "正常系: .rgssadがあればrpgmaker", files: []string{"Game.rgssad"}, want: "rpgmaker"},
		{name: "正常系: .rgss2aがあればrpgmaker", files: []string{"Game.rgss2a"}, want: "rpgmaker"},
		{name: "正常系: 不明なファイルのみはunknown", files: []string{"readme.txt"}, want: "unknown"},
		{name: "正常系: 空ディレクトリはunknown", files: nil, want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for _, f := range tt.files {
				touch(t, filepath.Join(dir, f))
			}

			assert.Equal(t, tt.want, info.DetectEngine(dir))
		})
	}
}

func TestDetectEngine_KirikiriPriorityOverRPGMaker(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	touch(t, filepath.Join(dir, "data.xp3"))
	touch(t, filepath.Join(dir, "Game.rgss3a"))

	assert.Equal(t, "kirikiri", info.DetectEngine(dir))
}

func TestCollectFileStats_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	result := info.CollectFileStats(dir, []string{".txt"})

	assert.Equal(t, 0, result.Count)
	assert.Equal(t, int64(0), result.TotalSizeBytes)
}

func TestCollectFileStats_SingleExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("hello"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("world"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file3.png"), []byte("\x89PNG"), 0o600))

	result := info.CollectFileStats(dir, []string{".txt"})

	assert.Equal(t, 2, result.Count)
	assert.Contains(t, result.Extensions, ".txt")
	assert.Equal(t, int64(10), result.TotalSizeBytes)
}

func TestCollectFileStats_MultipleExtensions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image1.png"), []byte("\x89PNG1234"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image2.jpg"), []byte("\xff\xd8\xff"), 0o600))

	result := info.CollectFileStats(dir, []string{".png", ".jpg"})

	assert.Equal(t, 2, result.Count)
	assert.Contains(t, result.Extensions, ".png")
	assert.Contains(t, result.Extensions, ".jpg")
	assert.Equal(t, int64(11), result.TotalSizeBytes)
}

func TestCollectFileStats_Recursive(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("root"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "file2.txt"), []byte("subdir"), 0o600))

	result := info.CollectFileStats(dir, []string{".txt"})

	assert.Equal(t, 2, result.Count)
	assert.Equal(t, int64(10), result.TotalSizeBytes)
}

func TestCollectFileStats_CaseInsensitive(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.TXT"), []byte("upper"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("lower"), 0o600))

	result := info.CollectFileStats(dir, []string{".txt"})

	assert.Equal(t, 2, result.Count)
}

func TestCollectFileStats_ReturnsOnlyFoundExtensions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.png"), []byte("\x89PNG"), 0o600))

	result := info.CollectFileStats(dir, []string{".png", ".jpg", ".gif"})

	assert.Equal(t, 1, result.Count)
	assert.Contains(t, result.Extensions, ".png")
	assert.NotContains(t, result.Extensions, ".jpg")
	assert.NotContains(t, result.Extensions, ".gif")
}

func TestAnalyzeGame_Kirikiri(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	touch(t, filepath.Join(dir, "data.xp3"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "script.ks"), []byte("スクリプト内容"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image.png"), []byte("\x89PNG12345678"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sound.ogg"), []byte("OggS1234"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "movie.mp4"), []byte("ftyp1234"), 0o600))

	result := info.AnalyzeGame(dir)

	assert.Equal(t, "kirikiri", result.Engine)
	assert.Equal(t, 1, result.Scripts.Count)
	assert.Equal(t, 1, result.Images.Count)
	assert.Equal(t, 1, result.Audio.Count)
	assert.Equal(t, 1, result.Video.Count)
}

func TestAnalyzeGame_RPGMaker(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	touch(t, filepath.Join(dir, "Game.rgss3a"))

	result := info.AnalyzeGame(dir)

	assert.Equal(t, "rpgmaker", result.Engine)
}

func TestAnalyzeGame_Unknown(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("readme"), 0o600))

	result := info.AnalyzeGame(dir)

	assert.Equal(t, "unknown", result.Engine)
}

func TestAnalyzeGame_EncodingDetection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "script.ks"), []byte("日本語テキスト"), 0o600))

	result := info.AnalyzeGame(dir)

	assert.NotEmpty(t, result.DetectedEncoding)
}

func TestAnalyzeGame_NoScriptsNoEncoding(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	result := info.AnalyzeGame(dir)

	assert.Empty(t, result.DetectedEncoding)
}
