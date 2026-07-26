package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/parser"
)

func TestAssetType_Count(t *testing.T) {
	t.Parallel()

	types := []parser.AssetType{
		parser.AssetScript, parser.AssetImage, parser.AssetAudio, parser.AssetVideo, parser.AssetOther,
	}
	assert.Len(t, types, 5)
}

func TestConversionAction_Values(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		action   parser.ConversionAction
		expected string
	}{
		"正常系: UTF-8エンコード": {parser.ConvertEncodeUTF8, "encode_utf8"},
		"正常系: WebP変換":     {parser.ConvertWebP, "convert_webp"},
		"正常系: OGG変換":      {parser.ConvertOgg, "convert_ogg"},
		"正常系: MP4変換":      {parser.ConvertMP4, "convert_mp4"},
		"正常系: コピー":        {parser.ConvertCopy, "copy"},
		"正常系: スキップ":       {parser.ConvertSkip, "skip"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, string(tc.action))
		})
	}
}

func sampleAssetFiles() []parser.AssetFile {
	return []parser.AssetFile{
		{
			Path: "scenario/first.ks", AssetType: parser.AssetScript, Action: parser.ConvertEncodeUTF8,
			SourceFormat: ".ks",
		},
		{
			Path: "scenario/system.tjs", AssetType: parser.AssetScript, Action: parser.ConvertEncodeUTF8,
			SourceFormat: ".tjs",
		},
		{
			Path: "image/bg01.tlg", AssetType: parser.AssetImage, Action: parser.ConvertWebP,
			SourceFormat: ".tlg", TargetFormat: ".webp",
		},
		{
			Path: "image/chara.bmp", AssetType: parser.AssetImage, Action: parser.ConvertWebP,
			SourceFormat: ".bmp", TargetFormat: ".webp",
		},
		{
			Path: "bgm/title.wav", AssetType: parser.AssetAudio, Action: parser.ConvertOgg,
			SourceFormat: ".wav", TargetFormat: ".ogg",
		},
		{
			Path: "voice/v001.ogg", AssetType: parser.AssetAudio, Action: parser.ConvertCopy,
			SourceFormat: ".ogg",
		},
		{
			Path: "movie/op.mpg", AssetType: parser.AssetVideo, Action: parser.ConvertMP4,
			SourceFormat: ".mpg", TargetFormat: ".mp4",
		},
		{
			Path: "data/config.txt", AssetType: parser.AssetOther, Action: parser.ConvertCopy,
			SourceFormat: ".txt",
		},
	}
}

func TestAssetManifest_Filters(t *testing.T) {
	t.Parallel()

	manifest := parser.AssetManifest{GameDir: "/games/mygame", Files: sampleAssetFiles()}

	t.Run("正常系: FilterByTypeでスクリプトファイルがフィルタされる", func(t *testing.T) {
		t.Parallel()

		result := manifest.FilterByType(parser.AssetScript)

		assert.Len(t, result, 2)
	})

	t.Run("正常系: FilterByTypeで画像ファイルがフィルタされる", func(t *testing.T) {
		t.Parallel()

		result := manifest.FilterByType(parser.AssetImage)

		assert.Len(t, result, 2)
	})

	t.Run("正常系: FilterByTypeで音声ファイルがフィルタされる", func(t *testing.T) {
		t.Parallel()

		result := manifest.FilterByType(parser.AssetAudio)

		assert.Len(t, result, 2)
	})

	t.Run("正常系: FilterByTypeで動画ファイルがフィルタされる", func(t *testing.T) {
		t.Parallel()

		result := manifest.FilterByType(parser.AssetVideo)

		assert.Len(t, result, 1)
	})

	t.Run("正常系: FilterByTypeでその他ファイルがフィルタされる", func(t *testing.T) {
		t.Parallel()

		result := manifest.FilterByType(parser.AssetOther)

		assert.Len(t, result, 1)
	})

	t.Run("正常系: FilterByActionでENCODE_UTF8アクションがフィルタされる", func(t *testing.T) {
		t.Parallel()

		result := manifest.FilterByAction(parser.ConvertEncodeUTF8)

		assert.Len(t, result, 2)
	})

	t.Run("正常系: FilterByActionでCONVERT_WEBPアクションがフィルタされる", func(t *testing.T) {
		t.Parallel()

		result := manifest.FilterByAction(parser.ConvertWebP)

		assert.Len(t, result, 2)
	})

	t.Run("正常系: FilterByActionでCONVERT_OGGアクションがフィルタされる", func(t *testing.T) {
		t.Parallel()

		result := manifest.FilterByAction(parser.ConvertOgg)

		assert.Len(t, result, 1)
	})

	t.Run("正常系: FilterByActionでCONVERT_MP4アクションがフィルタされる", func(t *testing.T) {
		t.Parallel()

		result := manifest.FilterByAction(parser.ConvertMP4)

		assert.Len(t, result, 1)
	})

	t.Run("正常系: FilterByActionでCOPYアクションがフィルタされる", func(t *testing.T) {
		t.Parallel()

		result := manifest.FilterByAction(parser.ConvertCopy)

		assert.Len(t, result, 2)
	})
}

func TestAssetManifest_GetSummary(t *testing.T) {
	t.Parallel()

	manifest := parser.AssetManifest{GameDir: "/games/mygame", Files: sampleAssetFiles()}

	summary := manifest.GetSummary()

	assert.Equal(t, 2, summary[parser.AssetScript])
	assert.Equal(t, 2, summary[parser.AssetImage])
	assert.Equal(t, 2, summary[parser.AssetAudio])
	assert.Equal(t, 1, summary[parser.AssetVideo])
	assert.Equal(t, 1, summary[parser.AssetOther])
}

func TestNewAssetScanner(t *testing.T) {
	t.Parallel()

	t.Run("異常系: 存在しないディレクトリでErrGameDirNotFoundForAssets", func(t *testing.T) {
		t.Parallel()

		_, err := parser.NewAssetScanner(filepath.Join(t.TempDir(), "nonexistent"), nil)

		require.ErrorIs(t, err, parser.ErrGameDirNotFoundForAssets)
	})

	t.Run("正常系: configなしで初期化できる", func(t *testing.T) {
		t.Parallel()

		scanner, err := parser.NewAssetScanner(t.TempDir(), nil)

		require.NoError(t, err)
		assert.NotNil(t, scanner)
	})

	t.Run("正常系: configありで初期化できる", func(t *testing.T) {
		t.Parallel()

		cfg := &parser.ScannerConfig{Exclude: []string{"*.bak"}}
		scanner, err := parser.NewAssetScanner(t.TempDir(), cfg)

		require.NoError(t, err)
		assert.NotNil(t, scanner)
	})
}

func TestAssetScanner_Classification(t *testing.T) {
	t.Parallel()

	classifyCases := []struct {
		extension string
		wantType  parser.AssetType
		wantAct   parser.ConversionAction
	}{
		{".ks", parser.AssetScript, parser.ConvertEncodeUTF8},
		{".tjs", parser.AssetScript, parser.ConvertEncodeUTF8},
		{".tlg", parser.AssetImage, parser.ConvertWebP},
		{".bmp", parser.AssetImage, parser.ConvertWebP},
		{".jpg", parser.AssetImage, parser.ConvertWebP},
		{".png", parser.AssetImage, parser.ConvertWebP},
		{".ogg", parser.AssetAudio, parser.ConvertCopy},
		{".wav", parser.AssetAudio, parser.ConvertOgg},
		{".mpg", parser.AssetVideo, parser.ConvertMP4},
		{".wmv", parser.AssetVideo, parser.ConvertMP4},
		{".txt", parser.AssetOther, parser.ConvertCopy},
		{".ini", parser.AssetOther, parser.ConvertCopy},
		{".dat", parser.AssetOther, parser.ConvertCopy},
	}

	for _, tc := range classifyCases {
		t.Run("正常系: "+tc.extension+"の種別・アクション分類", func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "test"+tc.extension), []byte{0x00})

			scanner, err := parser.NewAssetScanner(dir, nil)
			require.NoError(t, err)

			manifest, err := scanner.Scan()
			require.NoError(t, err)

			require.Len(t, manifest.Files, 1)
			assert.Equal(t, tc.wantType, manifest.Files[0].AssetType)
			assert.Equal(t, tc.wantAct, manifest.Files[0].Action)
		})
	}
}

func buildFullGameDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	scenarioDir := filepath.Join(dir, "scenario")
	require.NoError(t, os.Mkdir(scenarioDir, 0o750))
	writeFile(t, filepath.Join(scenarioDir, "first.ks"), []byte("@bg storage=bg01"))
	writeFile(t, filepath.Join(scenarioDir, "system.tjs"), []byte("var x = 1;"))

	imageDir := filepath.Join(dir, "image")
	require.NoError(t, os.Mkdir(imageDir, 0o750))
	writeFile(t, filepath.Join(imageDir, "bg01.tlg"), []byte("\x00TLG5"))
	writeFile(t, filepath.Join(imageDir, "chara.bmp"), []byte("BM"))
	writeFile(t, filepath.Join(imageDir, "icon.jpg"), []byte("\xff\xd8\xff"))
	writeFile(t, filepath.Join(imageDir, "logo.png"), []byte("\x89PNG"))

	bgmDir := filepath.Join(dir, "bgm")
	require.NoError(t, os.Mkdir(bgmDir, 0o750))
	writeFile(t, filepath.Join(bgmDir, "title.wav"), []byte("RIFF"))
	writeFile(t, filepath.Join(bgmDir, "ending.ogg"), []byte("OggS"))

	movieDir := filepath.Join(dir, "movie")
	require.NoError(t, os.Mkdir(movieDir, 0o750))
	writeFile(t, filepath.Join(movieDir, "op.mpg"), []byte{0x00, 0x00, 0x01, 0xba})
	writeFile(t, filepath.Join(movieDir, "ed.wmv"), []byte("WMV"))

	writeFile(t, filepath.Join(dir, "readme.txt"), []byte("readme"))
	writeFile(t, filepath.Join(dir, "config.ini"), []byte("[config]"))

	return dir
}

func TestAssetScanner_FullGameDirectoryScan(t *testing.T) {
	t.Parallel()

	dir := buildFullGameDir(t)

	scanner, err := parser.NewAssetScanner(dir, nil)
	require.NoError(t, err)

	manifest, err := scanner.Scan()
	require.NoError(t, err)

	require.Len(t, manifest.Files, 12)

	summary := manifest.GetSummary()
	assert.Equal(t, 2, summary[parser.AssetScript])
	assert.Equal(t, 4, summary[parser.AssetImage])
	assert.Equal(t, 2, summary[parser.AssetAudio])
	assert.Equal(t, 2, summary[parser.AssetVideo])
	assert.Equal(t, 2, summary[parser.AssetOther])
}

func TestAssetScanner_ErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 空のディレクトリで空のマニフェストが返される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		scanner, err := parser.NewAssetScanner(dir, nil)
		require.NoError(t, err)

		manifest, err := scanner.Scan()

		require.NoError(t, err)
		assert.Empty(t, manifest.Files)
		assert.Equal(t, dir, manifest.GameDir)
	})
}

func TestAssetScanner_WithConfig(t *testing.T) {
	t.Parallel()

	t.Run("正常系: exclude設定でファイルがスキップされる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "test.ks"), []byte("test"))
		writeFile(t, filepath.Join(dir, "backup.bak"), []byte("backup"))

		cfg := &parser.ScannerConfig{Exclude: []string{"*.bak"}}
		scanner, err := parser.NewAssetScanner(dir, cfg)
		require.NoError(t, err)

		manifest, err := scanner.Scan()
		require.NoError(t, err)

		require.Len(t, manifest.Files, 1)
		assert.Equal(t, "test.ks", manifest.Files[0].Path)
	})

	t.Run("正常系: conversion_rules設定で変換ルールが上書きされる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		voiceDir := filepath.Join(dir, "voice")
		require.NoError(t, os.Mkdir(voiceDir, 0o750))
		writeFile(t, filepath.Join(voiceDir, "v001.ogg"), []byte("OggS"))

		cfg := &parser.ScannerConfig{
			ConversionRules: []parser.ConversionRule{{Pattern: "voice/*.ogg", Converter: "skip"}},
		}
		scanner, err := parser.NewAssetScanner(dir, cfg)
		require.NoError(t, err)

		manifest, err := scanner.Scan()
		require.NoError(t, err)

		require.Len(t, manifest.Files, 1)
		assert.Equal(t, parser.ConvertSkip, manifest.Files[0].Action)
	})

	t.Run("正常系: 隠しファイルが自動的に除外される", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "visible.ks"), []byte("test"))
		writeFile(t, filepath.Join(dir, ".hidden.ks"), []byte("hidden"))

		scanner, err := parser.NewAssetScanner(dir, nil)
		require.NoError(t, err)

		manifest, err := scanner.Scan()
		require.NoError(t, err)

		require.Len(t, manifest.Files, 1)
		assert.Equal(t, "visible.ks", manifest.Files[0].Path)
	})
}
