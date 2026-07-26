package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/config"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	def := config.Default()

	assert.Nil(t, def.PackageName)
	assert.Nil(t, def.AppName)
	assert.Equal(t, 1, def.VersionCode)
	assert.Equal(t, "1.0.0", def.VersionName)

	assert.Equal(t, "webp", def.Image.Format)
	assert.Equal(t, config.Quality{Preset: "high"}, def.Image.Quality)
	assert.True(t, def.Image.LosslessAlpha)

	assert.Equal(t, "h264", def.Video.Codec)
	assert.Equal(t, "baseline", def.Video.Profile)
	assert.Equal(t, "aac", def.Video.AudioCodec)

	assert.Nil(t, def.Encoding.Source)
	assert.Equal(t, "utf-8", def.Encoding.Target)

	assert.Equal(t, 300, def.Timeouts.Ffmpeg)
	assert.Equal(t, 1800, def.Timeouts.Gradle)

	assert.Empty(t, def.ConversionRules)
	assert.Empty(t, def.Exclude)
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "mnemonic.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestLoad(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 有効な設定ファイルが読み込める", func(t *testing.T) {
		t.Parallel()

		path := writeConfig(t, "package_name: com.example.test")

		got, err := config.Load(path)

		require.NoError(t, err)
		require.NotNil(t, got.PackageName)
		assert.Equal(t, "com.example.test", *got.PackageName)
	})

	t.Run("異常系: 存在しないファイルはErrNotFound", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		_, err := config.Load(filepath.Join(dir, "nonexistent.yml"))

		require.Error(t, err)
		assert.ErrorIs(t, err, config.ErrNotFound)
	})

	t.Run("異常系: 無効なYAMLはErrInvalidYAML", func(t *testing.T) {
		t.Parallel()

		path := writeConfig(t, "this is not valid yaml: [")

		_, err := config.Load(path)

		require.Error(t, err)
		assert.ErrorIs(t, err, config.ErrInvalidYAML)
	})

	t.Run("正常系: 空ファイルはデフォルト設定を返す", func(t *testing.T) {
		t.Parallel()

		path := writeConfig(t, "")

		got, err := config.Load(path)

		require.NoError(t, err)
		assert.Equal(t, config.Default(), got)
	})

	t.Run("正常系: 部分的な設定がデフォルト値とマージされる", func(t *testing.T) {
		t.Parallel()

		path := writeConfig(t, "package_name: com.example.partial")

		got, err := config.Load(path)

		require.NoError(t, err)
		require.NotNil(t, got.PackageName)
		assert.Equal(t, "com.example.partial", *got.PackageName)
		assert.Equal(t, 1, got.VersionCode)
		assert.Equal(t, "webp", got.Image.Format)
	})

	t.Run("正常系: ネストされた設定が正しく読み込まれる", func(t *testing.T) {
		t.Parallel()

		path := writeConfig(t, `
image:
  format: png
  quality: 90
video:
  codec: h265
`)

		got, err := config.Load(path)

		require.NoError(t, err)
		assert.Equal(t, "png", got.Image.Format)
		assert.Equal(t, config.Quality{Level: 90, IsInt: true}, got.Image.Quality)
		assert.True(t, got.Image.LosslessAlpha)
		assert.Equal(t, "h265", got.Video.Codec)
		assert.Equal(t, "baseline", got.Video.Profile)
	})

	t.Run("正常系: 変換ルールが正しく読み込まれる", func(t *testing.T) {
		t.Parallel()

		path := writeConfig(t, `
conversion_rules:
  - pattern: "*.png"
    converter: image
  - pattern: "*.mp4"
    converter: video
`)

		got, err := config.Load(path)

		require.NoError(t, err)
		require.Len(t, got.ConversionRules, 2)
		assert.Equal(t, "*.png", got.ConversionRules[0].Pattern)
		assert.Equal(t, "image", got.ConversionRules[0].Converter)
		assert.Equal(t, "*.mp4", got.ConversionRules[1].Pattern)
	})

	t.Run("正常系: 除外リストが正しく読み込まれる", func(t *testing.T) {
		t.Parallel()

		path := writeConfig(t, `
exclude:
  - "*.bak"
  - "temp/*"
`)

		got, err := config.Load(path)

		require.NoError(t, err)
		assert.Equal(t, []string{"*.bak", "temp/*"}, got.Exclude)
	})

	t.Run("正常系: 全ての設定が正しく読み込まれる", func(t *testing.T) {
		t.Parallel()

		path := writeConfig(t, `
package_name: com.example.full
app_name: Full Test App
version_code: 42
version_name: "2.0.0"
image:
  format: webp
  quality: high
  lossless_alpha: false
video:
  codec: h264
  profile: main
  audio_codec: opus
encoding:
  source: shift_jis
  target: utf-8
timeouts:
  ffmpeg: 600
  gradle: 3600
conversion_rules:
  - pattern: "*.txt"
    converter: text
exclude:
  - "debug/*"
`)

		got, err := config.Load(path)

		require.NoError(t, err)
		require.NotNil(t, got.PackageName)
		assert.Equal(t, "com.example.full", *got.PackageName)
		require.NotNil(t, got.AppName)
		assert.Equal(t, "Full Test App", *got.AppName)
		assert.Equal(t, 42, got.VersionCode)
		assert.Equal(t, "2.0.0", got.VersionName)
		assert.Equal(t, "webp", got.Image.Format)
		assert.False(t, got.Image.LosslessAlpha)
		assert.Equal(t, "main", got.Video.Profile)
		assert.Equal(t, "opus", got.Video.AudioCodec)
		require.NotNil(t, got.Encoding.Source)
		assert.Equal(t, "shift_jis", *got.Encoding.Source)
		assert.Equal(t, 600, got.Timeouts.Ffmpeg)
		assert.Equal(t, 3600, got.Timeouts.Gradle)
		require.Len(t, got.ConversionRules, 1)
		assert.Equal(t, []string{"debug/*"}, got.Exclude)
	})

	t.Run("異常系: マッピング形式でないYAMLはErrInvalidFormat", func(t *testing.T) {
		t.Parallel()

		path := writeConfig(t, "- item1\n- item2")

		_, err := config.Load(path)

		require.Error(t, err)
		assert.ErrorIs(t, err, config.ErrInvalidFormat)
	})
}

func TestConversionRule(t *testing.T) {
	t.Parallel()

	rule := config.ConversionRule{Pattern: "*.png", Converter: "image"}

	assert.Equal(t, "*.png", rule.Pattern)
	assert.Equal(t, "image", rule.Converter)
}

func TestVideoConfig(t *testing.T) {
	t.Parallel()

	video := config.VideoConfig{Codec: "h265", Profile: "main", AudioCodec: "opus"}

	assert.Equal(t, "h265", video.Codec)
	assert.Equal(t, "main", video.Profile)
	assert.Equal(t, "opus", video.AudioCodec)
}

func TestTimeoutConfig(t *testing.T) {
	t.Parallel()

	timeouts := config.TimeoutConfig{Ffmpeg: 600, Gradle: 3600}

	assert.Equal(t, 600, timeouts.Ffmpeg)
	assert.Equal(t, 3600, timeouts.Gradle)
}

func TestErrorSentinels_AreDistinct(t *testing.T) {
	t.Parallel()

	assert.False(t, errors.Is(config.ErrNotFound, config.ErrInvalidYAML))
	assert.False(t, errors.Is(config.ErrInvalidYAML, config.ErrInvalidFormat))
}
