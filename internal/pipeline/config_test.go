package pipeline_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/na2na-p/mnemonic/internal/pipeline"
)

func TestNewConfig_DefaultValues(t *testing.T) {
	t.Parallel()

	config := pipeline.NewConfig("game.exe", "game.apk")

	assert.Equal(t, "game.exe", config.InputPath)
	assert.Equal(t, "game.apk", config.OutputPath)
	assert.Empty(t, config.PackageName)
	assert.Empty(t, config.AppName)
	assert.Empty(t, config.KeystorePath)
	assert.False(t, config.SkipVideo)
	assert.Equal(t, "high", config.Quality)
	assert.False(t, config.CleanCache)
	assert.Equal(t, 0, config.VerboseLevel)
	assert.Empty(t, config.LogFile)
	assert.Equal(t, 300, config.FFmpegTimeoutSeconds)
	assert.Equal(t, 1800, config.GradleTimeoutSeconds)
	assert.Nil(t, config.TemplateVersion)
	assert.Equal(t, 7, config.TemplateRefreshDays)
	assert.False(t, config.TemplateOffline)
}

func TestConfig_CustomValues(t *testing.T) {
	t.Parallel()

	version := "1.0.0"
	config := pipeline.NewConfig("/path/to/game.exe", "/path/to/output.apk")
	config.PackageName = "com.example.game"
	config.AppName = "My Game"
	config.KeystorePath = "/path/to/keystore.jks"
	config.SkipVideo = true
	config.Quality = "low"
	config.CleanCache = true
	config.VerboseLevel = 2
	config.LogFile = "/path/to/log.txt"
	config.FFmpegTimeoutSeconds = 600
	config.GradleTimeoutSeconds = 3600
	config.TemplateVersion = &version
	config.TemplateRefreshDays = 14
	config.TemplateOffline = true

	assert.Equal(t, "/path/to/game.exe", config.InputPath)
	assert.Equal(t, "/path/to/output.apk", config.OutputPath)
	assert.Equal(t, "com.example.game", config.PackageName)
	assert.Equal(t, "My Game", config.AppName)
	assert.Equal(t, "/path/to/keystore.jks", config.KeystorePath)
	assert.True(t, config.SkipVideo)
	assert.Equal(t, "low", config.Quality)
	assert.True(t, config.CleanCache)
	assert.Equal(t, 2, config.VerboseLevel)
	assert.Equal(t, "/path/to/log.txt", config.LogFile)
	assert.Equal(t, 600, config.FFmpegTimeoutSeconds)
	assert.Equal(t, 3600, config.GradleTimeoutSeconds)
	assert.Equal(t, &version, config.TemplateVersion)
	assert.Equal(t, 14, config.TemplateRefreshDays)
	assert.True(t, config.TemplateOffline)
}
