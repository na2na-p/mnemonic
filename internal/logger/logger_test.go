package logger_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/logger"
)

func TestDefaultLogConfig(t *testing.T) {
	t.Parallel()

	cfg := logger.DefaultLogConfig()

	assert.Equal(t, logger.Normal, cfg.VerboseLevel)
	assert.Empty(t, cfg.LogFile)
	assert.True(t, cfg.UseColor)
	assert.True(t, cfg.UseEmoji)
}

func TestVerboseLevel_Ordering(t *testing.T) {
	t.Parallel()

	assert.Less(t, int(logger.Quiet), int(logger.Normal))
	assert.Less(t, int(logger.Normal), int(logger.Verbose))
	assert.Less(t, int(logger.Verbose), int(logger.Debug))
}

// newLogger はos.Stdout/os.Stderrを介さないテスト用BuildLoggerを生成する。
// テスト間でt.Parallel()を安全に使うため、実プロセスの標準出力をcapsys相当で
// 捕捉するのではなく、io.Writerを直接注入する設計とした。
func newLogger(t *testing.T, cfg logger.LogConfig) (*logger.BuildLogger, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	l, err := logger.NewWithWriters(cfg, stdout, stderr)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = l.Close()
	})

	return l, stdout, stderr
}

func TestBuildLogger_Info(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		level       logger.VerboseLevel
		wantOutput  bool
		wantMessage string
	}{
		{name: "正常系: NORMALレベルでinfoメッセージが出力される", level: logger.Normal, wantOutput: true},
		{name: "異常系: QUIETレベルでinfoメッセージが出力されない", level: logger.Quiet, wantOutput: false},
		{name: "正常系: VERBOSEレベルでinfoメッセージが出力される", level: logger.Verbose, wantOutput: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := logger.DefaultLogConfig()
			cfg.VerboseLevel = tt.level
			l, stdout, _ := newLogger(t, cfg)

			l.Info("テストメッセージ")

			if tt.wantOutput {
				assert.Contains(t, stdout.String(), "テストメッセージ")
			} else {
				assert.Empty(t, stdout.String())
			}
		})
	}
}

func TestBuildLogger_Verbose(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		level      logger.VerboseLevel
		wantOutput bool
	}{
		{name: "正常系: VERBOSEレベルでverboseメッセージが出力される", level: logger.Verbose, wantOutput: true},
		{name: "異常系: NORMALレベルでverboseメッセージが出力されない", level: logger.Normal, wantOutput: false},
		{name: "正常系: DEBUGレベルでverboseメッセージが出力される", level: logger.Debug, wantOutput: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := logger.DefaultLogConfig()
			cfg.VerboseLevel = tt.level
			l, stdout, _ := newLogger(t, cfg)

			l.Verbose("詳細メッセージ")

			if tt.wantOutput {
				assert.Contains(t, stdout.String(), "詳細メッセージ")
			} else {
				assert.Empty(t, stdout.String())
			}
		})
	}
}

func TestBuildLogger_Debug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		level      logger.VerboseLevel
		wantOutput bool
	}{
		{name: "正常系: DEBUGレベルでdebugメッセージが出力される", level: logger.Debug, wantOutput: true},
		{name: "異常系: VERBOSEレベルでdebugメッセージが出力されない", level: logger.Verbose, wantOutput: false},
		{name: "異常系: NORMALレベルでdebugメッセージが出力されない", level: logger.Normal, wantOutput: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := logger.DefaultLogConfig()
			cfg.VerboseLevel = tt.level
			l, stdout, _ := newLogger(t, cfg)

			l.Debug("デバッグメッセージ")

			if tt.wantOutput {
				assert.Contains(t, stdout.String(), "デバッグメッセージ")
			} else {
				assert.Empty(t, stdout.String())
			}
		})
	}
}

func TestBuildLogger_Error(t *testing.T) {
	t.Parallel()

	t.Run("正常系: エラーメッセージは常に出力される", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		cfg.VerboseLevel = logger.Quiet
		l, _, stderr := newLogger(t, cfg)

		l.Error("エラーメッセージ")

		assert.Contains(t, stderr.String(), "エラー")
		assert.Contains(t, stderr.String(), "エラーメッセージ")
	})

	t.Run("正常系: エラーメッセージは標準エラー出力に出力される", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		l, stdout, stderr := newLogger(t, cfg)

		l.Error("エラーメッセージ")

		assert.Empty(t, stdout.String())
		assert.Contains(t, stderr.String(), "エラーメッセージ")
	})
}

func TestBuildLogger_Warning(t *testing.T) {
	t.Parallel()

	t.Run("正常系: NORMALレベルで警告メッセージが出力される", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		l, stdout, _ := newLogger(t, cfg)

		l.Warning("警告メッセージ")

		assert.Contains(t, stdout.String(), "警告")
		assert.Contains(t, stdout.String(), "警告メッセージ")
	})

	t.Run("異常系: QUIETレベルで警告メッセージが出力されない", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		cfg.VerboseLevel = logger.Quiet
		l, stdout, _ := newLogger(t, cfg)

		l.Warning("警告メッセージ")

		assert.Empty(t, stdout.String())
	})
}

func TestBuildLogger_LogCommand(t *testing.T) {
	t.Parallel()

	t.Run("正常系: DEBUGレベルでコマンドログが出力される", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		cfg.VerboseLevel = logger.Debug
		l, stdout, _ := newLogger(t, cfg)

		l.LogCommand([]string{"ffmpeg", "-i", "input.mp4"}, "output line 1\noutput line 2")

		out := stdout.String()
		assert.Contains(t, out, "ffmpeg -i input.mp4")
		assert.Contains(t, out, "output line 1")
		assert.Contains(t, out, "output line 2")
	})

	t.Run("異常系: NORMALレベルでコマンドログが出力されない", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		l, stdout, _ := newLogger(t, cfg)

		l.LogCommand([]string{"ffmpeg", "-i", "input.mp4"}, "output")

		assert.Empty(t, stdout.String())
	})
}

func TestBuildLogger_LogConversion(t *testing.T) {
	t.Parallel()

	t.Run("正常系: VERBOSEレベルで変換ログが出力される", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		cfg.VerboseLevel = logger.Verbose
		l, stdout, _ := newLogger(t, cfg)

		l.LogConversion("input.ogg", "output.mp3", "OK")

		out := stdout.String()
		assert.Contains(t, out, "input.ogg")
		assert.Contains(t, out, "output.mp3")
		assert.Contains(t, out, "OK")
	})

	t.Run("異常系: NORMALレベルで変換ログが出力されない", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		l, stdout, _ := newLogger(t, cfg)

		l.LogConversion("input.ogg", "output.mp3", "OK")

		assert.Empty(t, stdout.String())
	})
}

func TestBuildLogger_LogSummary(t *testing.T) {
	t.Parallel()

	t.Run("正常系: emojiありでサマリが出力される", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		cfg.UseEmoji = true
		l, stdout, _ := newLogger(t, cfg)
		outputPath := "game.apk"

		l.LogSummary(logger.Statistics{OutputPath: &outputPath, OutputSize: 10485760})

		out := stdout.String()
		assert.Contains(t, out, "Build complete")
		assert.Contains(t, out, "game.apk")
		assert.Contains(t, out, "10.0 MB")
	})

	t.Run("正常系: emojiなしでサマリが出力される", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		cfg.UseEmoji = false
		l, stdout, _ := newLogger(t, cfg)
		outputPath := "game.apk"

		l.LogSummary(logger.Statistics{OutputPath: &outputPath, OutputSize: 10485760})

		out := stdout.String()
		assert.Contains(t, out, "[OK]")
		assert.Contains(t, out, "Build complete")
	})

	t.Run("正常系: パッケージ名が出力される", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		l, stdout, _ := newLogger(t, cfg)
		packageName := "com.example.game"

		l.LogSummary(logger.Statistics{PackageName: &packageName})

		assert.Contains(t, stdout.String(), "com.example.game")
	})

	t.Run("異常系: QUIETレベルでサマリが出力されない", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		cfg.VerboseLevel = logger.Quiet
		l, stdout, _ := newLogger(t, cfg)
		outputPath := "game.apk"

		l.LogSummary(logger.Statistics{OutputPath: &outputPath})

		assert.Empty(t, stdout.String())
	})
}

func TestBuildLogger_CreateProgress(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 進捗表示インスタンスを返す", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		l, _, _ := newLogger(t, cfg)

		progress := l.CreateProgress()

		assert.NotNil(t, progress)
	})

	t.Run("正常系: 設定が進捗表示に反映される", func(t *testing.T) {
		t.Parallel()

		cfg := logger.DefaultLogConfig()
		cfg.UseColor = false
		cfg.UseEmoji = false
		l, _, _ := newLogger(t, cfg)

		progress, ok := l.CreateProgress().(*logger.ConsoleProgressDisplay)

		require.True(t, ok)
		assert.False(t, progress.UseColor())
		assert.False(t, progress.UseEmoji())
	})
}

func TestBuildLogger_FileOutput(t *testing.T) {
	t.Parallel()

	t.Run("正常系: ファイルにログが出力される", func(t *testing.T) {
		t.Parallel()

		logFile := filepath.Join(t.TempDir(), "test.log")
		cfg := logger.DefaultLogConfig()
		cfg.LogFile = logFile
		l, err := logger.New(cfg)
		require.NoError(t, err)

		l.Info("テストメッセージ")
		require.NoError(t, l.Close())

		content, err := os.ReadFile(logFile) //nolint:gosec // テストコードで生成したパスを読むだけのため妥当
		require.NoError(t, err)
		assert.Contains(t, string(content), "INFO")
		assert.Contains(t, string(content), "テストメッセージ")
	})

	t.Run("正常系: ファイル出力はANSIエスケープシーケンスを除去する", func(t *testing.T) {
		t.Parallel()

		logFile := filepath.Join(t.TempDir(), "test.log")
		cfg := logger.DefaultLogConfig()
		cfg.LogFile = logFile
		l, err := logger.New(cfg)
		require.NoError(t, err)

		l.Info("\x1b[32mカラーメッセージ\x1b[0m")
		require.NoError(t, l.Close())

		content, err := os.ReadFile(logFile) //nolint:gosec // テストコードで生成したパスを読むだけのため妥当
		require.NoError(t, err)
		assert.NotContains(t, string(content), "\x1b[")
		assert.Contains(t, string(content), "カラーメッセージ")
	})

	t.Run("正常系: Closeがファイルを閉じる", func(t *testing.T) {
		t.Parallel()

		logFile := filepath.Join(t.TempDir(), "test.log")
		cfg := logger.DefaultLogConfig()
		cfg.LogFile = logFile
		l, err := logger.New(cfg)
		require.NoError(t, err)

		l.Info("テスト")
		require.NoError(t, l.Close())

		assert.True(t, l.Closed())
		// Close後の再Closeはエラーにならない（べき等な設計）
		assert.NoError(t, l.Close())
	})

	t.Run("正常系: 全てのログレベルがファイルに書き込まれる", func(t *testing.T) {
		t.Parallel()

		logFile := filepath.Join(t.TempDir(), "test.log")
		cfg := logger.DefaultLogConfig()
		cfg.VerboseLevel = logger.Quiet
		cfg.LogFile = logFile
		l, err := logger.New(cfg)
		require.NoError(t, err)

		l.Info("INFO message")
		l.Verbose("VERBOSE message")
		l.Debug("DEBUG message")
		l.Warning("WARNING message")
		l.Error("ERROR message")
		require.NoError(t, l.Close())

		content, err := os.ReadFile(logFile) //nolint:gosec // テストコードで生成したパスを読むだけのため妥当
		require.NoError(t, err)
		text := string(content)
		assert.Contains(t, text, "INFO: INFO message")
		assert.Contains(t, text, "VERBOSE: VERBOSE message")
		assert.Contains(t, text, "DEBUG: DEBUG message")
		assert.Contains(t, text, "WARNING: WARNING message")
		assert.Contains(t, text, "ERROR: ERROR message")
	})
}
