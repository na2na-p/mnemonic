package pipeline_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/pipeline"
)

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func TestBuildPipeline_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupFn    func(t *testing.T, dir string) pipeline.Config
		wantErr    bool
		errContain string
	}{
		{
			name: "異常系: 入力ファイルが存在しない",
			setupFn: func(_ *testing.T, dir string) pipeline.Config {
				return pipeline.NewConfig(filepath.Join(dir, "nonexistent.exe"), filepath.Join(dir, "output.apk"))
			},
			wantErr:    true,
			errContain: "見つかりません",
		},
		{
			name: "異常系: サポートされていないファイル形式",
			setupFn: func(t *testing.T, dir string) pipeline.Config {
				t.Helper()
				input := filepath.Join(dir, "invalid.txt")
				writeFile(t, input, []byte("invalid content"))

				return pipeline.NewConfig(input, filepath.Join(dir, "output.apk"))
			},
			wantErr:    true,
			errContain: ".txt",
		},
		{
			name: "異常系: キーストアファイルが存在しない",
			setupFn: func(t *testing.T, dir string) pipeline.Config {
				t.Helper()
				input := filepath.Join(dir, "game.exe")
				writeFile(t, input, make([]byte, 100))
				cfg := pipeline.NewConfig(input, filepath.Join(dir, "output.apk"))
				cfg.KeystorePath = filepath.Join(dir, "nonexistent.jks")

				return cfg
			},
			wantErr:    true,
			errContain: "キーストア",
		},
		{
			name: "正常系: 有効な設定",
			setupFn: func(t *testing.T, dir string) pipeline.Config {
				t.Helper()
				input := filepath.Join(dir, "game.exe")
				writeFile(t, input, make([]byte, 100))

				return pipeline.NewConfig(input, filepath.Join(dir, "output.apk"))
			},
			wantErr: false,
		},
		{
			name: "正常系: キーストア付きの有効な設定",
			setupFn: func(t *testing.T, dir string) pipeline.Config {
				t.Helper()
				input := filepath.Join(dir, "game.exe")
				writeFile(t, input, make([]byte, 100))
				keystore := filepath.Join(dir, "keystore.jks")
				writeFile(t, keystore, make([]byte, 100))
				cfg := pipeline.NewConfig(input, filepath.Join(dir, "output.apk"))
				cfg.KeystorePath = keystore

				return cfg
			},
			wantErr: false,
		},
		{
			name: "正常系: XP3ファイルを入力とした有効な設定",
			setupFn: func(t *testing.T, dir string) pipeline.Config {
				t.Helper()
				input := filepath.Join(dir, "game.xp3")
				writeFile(t, input, make([]byte, 100))

				return pipeline.NewConfig(input, filepath.Join(dir, "output.apk"))
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			config := tt.setupFn(t, dir)
			p := pipeline.NewBuildPipeline(config)

			errs := p.Validate()

			if tt.wantErr {
				require.NotEmpty(t, errs)
				assert.Contains(t, strings.Join(errs, " "), tt.errContain)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestNewBuildPipeline_ConfigProperty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	input := filepath.Join(dir, "game.exe")
	writeFile(t, input, make([]byte, 100))

	config := pipeline.NewConfig(input, filepath.Join(dir, "output.apk"))
	config.PackageName = "com.example.game"

	p := pipeline.NewBuildPipeline(config)

	assert.Equal(t, config, p.Config())
	assert.Equal(t, input, p.Config().InputPath)
	assert.Equal(t, "com.example.game", p.Config().PackageName)
}

func TestBuildPipeline_Run_ParserFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	config := pipeline.NewConfig(filepath.Join(dir, "nonexistent.exe"), filepath.Join(dir, "output.apk"))
	p := pipeline.NewBuildPipeline(config)

	result := p.Run(nil)

	assert.False(t, result.Success)
	assert.NotEmpty(t, result.ErrorMessage)
	assert.Nil(t, result.OutputPath)
}
