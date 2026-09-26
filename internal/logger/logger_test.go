package logger_test

import (
	"bytes"
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/logger"
)

func TestVerboseLevel_Ordering(t *testing.T) {
	t.Parallel()

	assert.Less(t, int(logger.Quiet), int(logger.Normal))
	assert.Less(t, int(logger.Normal), int(logger.Verbose))
	assert.Less(t, int(logger.Verbose), int(logger.Debug))
}

func newLogger(t *testing.T, level logger.VerboseLevel) (*logger.BuildLogger, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	file := &bytes.Buffer{}

	return logger.New(level, stdout, stderr, file), stdout, stderr, file
}

func TestBuildLogger_Info(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level logger.VerboseLevel
		want  bool
	}{
		{
			name:  "正常系: NORMALレベルでinfoメッセージが出力される",
			level: logger.Normal,
			want:  true,
		},
		{
			name:  "異常系: QUIETレベルでinfoメッセージが出力されない",
			level: logger.Quiet,
			want:  false,
		},
		{
			name:  "正常系: VERBOSEレベルでinfoメッセージが出力される",
			level: logger.Verbose,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l, stdout, _, _ := newLogger(t, tt.level)

			l.Info("テストメッセージ")

			if tt.want {
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
		name  string
		level logger.VerboseLevel
		want  bool
	}{
		{
			name:  "正常系: VERBOSEレベルでverboseメッセージが出力される",
			level: logger.Verbose,
			want:  true,
		},
		{
			name:  "異常系: NORMALレベルでverboseメッセージが出力されない",
			level: logger.Normal,
			want:  false,
		},
		{
			name:  "正常系: DEBUGレベルでverboseメッセージが出力される",
			level: logger.Debug,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l, stdout, _, _ := newLogger(t, tt.level)

			l.Verbose("詳細メッセージ")

			if tt.want {
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
		name  string
		level logger.VerboseLevel
		want  bool
	}{
		{
			name:  "正常系: DEBUGレベルでdebugメッセージが出力される",
			level: logger.Debug,
			want:  true,
		},
		{
			name:  "異常系: VERBOSEレベルでdebugメッセージが出力されない",
			level: logger.Verbose,
			want:  false,
		},
		{
			name:  "異常系: NORMALレベルでdebugメッセージが出力されない",
			level: logger.Normal,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l, stdout, _, _ := newLogger(t, tt.level)

			l.Debug("デバッグメッセージ")

			if tt.want {
				assert.Contains(t, stdout.String(), "デバッグメッセージ")
			} else {
				assert.Empty(t, stdout.String())
			}
		})
	}
}

func TestBuildLogger_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level logger.VerboseLevel
	}{
		{
			name:  "正常系: QUIETでも出力される",
			level: logger.Quiet,
		},
		{
			name:  "正常系: NORMALでも標準エラーへ出力される",
			level: logger.Normal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l, stdout, stderr, _ := newLogger(t, tt.level)

			l.Error("エラーメッセージ")

			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), "エラー: エラーメッセージ")
		})
	}
}

func TestBuildLogger_Warning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level logger.VerboseLevel
		want  bool
	}{
		{
			name:  "正常系: NORMALレベルで警告メッセージが出力される",
			level: logger.Normal,
			want:  true,
		},
		{
			name:  "異常系: QUIETレベルで警告メッセージが出力されない",
			level: logger.Quiet,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l, stdout, _, _ := newLogger(t, tt.level)

			l.Warning("警告メッセージ")

			if tt.want {
				assert.Contains(t, stdout.String(), "警告: 警告メッセージ")
			} else {
				assert.Empty(t, stdout.String())
			}
		})
	}
}

func TestBuildLogger_FileOutput(t *testing.T) {
	t.Parallel()

	t.Run("正常系: INFOとANSI除去の行形式", func(t *testing.T) {
		t.Parallel()

		l, _, _, file := newLogger(t, logger.Normal)

		l.Info("\x1b[32mテストメッセージ\x1b[0m")

		assert.Regexp(t, `^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] INFO: テストメッセージ\n$`, file.String())
		assert.NotContains(t, file.String(), "\x1b[")
	})

	t.Run("正常系: QUIETでも全レベルがファイルへ出力される", func(t *testing.T) {
		t.Parallel()

		l, _, _, file := newLogger(t, logger.Quiet)

		l.Info("INFO message")
		l.Verbose("VERBOSE message")
		l.Debug("DEBUG message")
		l.Warning("WARNING message")
		l.Error("ERROR message")

		for _, text := range []string{
			"INFO: INFO message", "VERBOSE: VERBOSE message", "DEBUG: DEBUG message",
			"WARNING: WARNING message", "ERROR: ERROR message",
		} {
			assert.Contains(t, file.String(), text)
		}
	})
}

func TestBuildLogger_MultiLineFileOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		logFunc     func(l *logger.BuildLogger, message string)
		level       string
		message     string
		wantConsole string
		wantLines   []string
	}{
		{
			name:        "正常系: ERRORの各行に同じ接頭辞を付ける",
			logFunc:     (*logger.BuildLogger).Error,
			level:       "ERROR",
			message:     "アセットの変換に失敗しました:\n  bg/a.png\n  fg/b.png",
			wantConsole: "エラー: アセットの変換に失敗しました:\n  bg/a.png\n  fg/b.png\n",
			wantLines:   []string{"アセットの変換に失敗しました:", "  bg/a.png", "  fg/b.png"},
		},
		{
			name:        "正常系: WARNINGの各行に同じ接頭辞を付ける",
			logFunc:     (*logger.BuildLogger).Warning,
			level:       "WARNING",
			message:     "1行目\n2行目",
			wantConsole: "警告: 1行目\n2行目\n",
			wantLines:   []string{"1行目", "2行目"},
		},
		{
			name:        "正常系: INFOの各行に同じ接頭辞を付ける",
			logFunc:     (*logger.BuildLogger).Info,
			level:       "INFO",
			message:     "1行目\n2行目",
			wantConsole: "1行目\n2行目\n",
			wantLines:   []string{"1行目", "2行目"},
		},
		{
			name:        "正常系: CRLFを行区切りとして扱う",
			logFunc:     (*logger.BuildLogger).Info,
			level:       "INFO",
			message:     "1行目\r\n2行目",
			wantConsole: "1行目\r\n2行目\n",
			wantLines:   []string{"1行目", "2行目"},
		},
		{
			name:        "正常系: 末尾の改行で空のエントリを作らない",
			logFunc:     (*logger.BuildLogger).Info,
			level:       "INFO",
			message:     "1行目\n2行目\n",
			wantConsole: "1行目\n2行目\n\n",
			wantLines:   []string{"1行目", "2行目"},
		},
		{
			name:        "正常系: 末尾のCRLFで空のエントリを作らない",
			logFunc:     (*logger.BuildLogger).Info,
			level:       "INFO",
			message:     "1行目\r\n",
			wantConsole: "1行目\r\n\n",
			wantLines:   []string{"1行目"},
		},
		{
			name:        "正常系: 途中の空行は接頭辞付きの空行として残す",
			logFunc:     (*logger.BuildLogger).Info,
			level:       "INFO",
			message:     "1行目\n\n3行目",
			wantConsole: "1行目\n\n3行目\n",
			wantLines:   []string{"1行目", "", "3行目"},
		},
		{
			name:        "正常系: 末尾の改行は1つだけ行の終端として扱い、残りは空行として記録する",
			logFunc:     (*logger.BuildLogger).Info,
			level:       "INFO",
			message:     "1行目\n\n",
			wantConsole: "1行目\n\n\n",
			wantLines:   []string{"1行目", ""},
		},
		{
			name:        "正常系: 末尾のCRLFは1つだけ行の終端として扱い、残りは空行として記録する",
			logFunc:     (*logger.BuildLogger).Info,
			level:       "INFO",
			message:     "1行目\r\n\r\n",
			wantConsole: "1行目\r\n\r\n\n",
			wantLines:   []string{"1行目", ""},
		},
		{
			name:        "正常系: 改行の無い末尾の単独のCRも除く",
			logFunc:     (*logger.BuildLogger).Info,
			level:       "INFO",
			message:     "abc\r",
			wantConsole: "abc\r\n",
			wantLines:   []string{"abc"},
		},
		{
			name:        "正常系: 行末のCRは1つだけ除く",
			logFunc:     (*logger.BuildLogger).Info,
			level:       "INFO",
			message:     "abc\r\r\n",
			wantConsole: "abc\r\r\n\n",
			wantLines:   []string{"abc\r"},
		},
		{
			name:        "正常系: 空のメッセージも1行記録する",
			logFunc:     (*logger.BuildLogger).Info,
			level:       "INFO",
			message:     "",
			wantConsole: "\n",
			wantLines:   []string{""},
		},
	}

	prefixPattern := regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] [A-Z]+: `)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			file := &bytes.Buffer{}
			l := logger.New(logger.Normal, stdout, stderr, file)

			tt.logFunc(l, tt.message)

			assert.Equal(t, tt.wantConsole, stdout.String()+stderr.String())

			require.True(t, strings.HasSuffix(file.String(), "\n"))
			fileLines := strings.Split(strings.TrimSuffix(file.String(), "\n"), "\n")
			require.Len(t, fileLines, len(tt.wantLines))

			firstPrefix := prefixPattern.FindString(fileLines[0])
			require.NotEmpty(t, firstPrefix)
			assert.True(t, strings.HasSuffix(firstPrefix, "] "+tt.level+": "))
			for i, line := range fileLines {
				assert.Equal(t, firstPrefix+tt.wantLines[i], line, "全ての行に1回の呼び出しで同じ接頭辞を付ける")
			}
		})
	}
}

func TestBuildLogger_LogConversion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level logger.VerboseLevel
		want  bool
	}{
		{
			name:  "正常系: VERBOSEレベルで変換ログが出力される",
			level: logger.Verbose,
			want:  true,
		},
		{
			name:  "異常系: NORMALレベルで変換ログが出力されない",
			level: logger.Normal,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l, stdout, _, _ := newLogger(t, tt.level)

			l.LogConversion("bg/a.png", "bg/a.png", "OK")
			l.LogConversion("fg/a.png", "fg/a.png", "OK")

			if tt.want {
				assert.Contains(t, stdout.String(), "変換: bg/a.png -> bg/a.png [OK]")
				assert.Contains(t, stdout.String(), "変換: fg/a.png -> fg/a.png [OK]")
			} else {
				assert.Empty(t, stdout.String())
			}
		})
	}
}

func TestBuildLogger_LogCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		level      logger.VerboseLevel
		wantOutput bool
		succeeded  bool
	}{
		{
			name:       "正常系: DEBUGレベルで成功コマンドログが出力される",
			level:      logger.Debug,
			wantOutput: true,
			succeeded:  true,
		},
		{
			name:       "正常系: DEBUGレベルで失敗コマンドログが出力される",
			level:      logger.Debug,
			wantOutput: true,
			succeeded:  false,
		},
		{
			name:       "異常系: NORMALレベルでコマンドログが出力されない",
			level:      logger.Normal,
			wantOutput: false,
			succeeded:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l, stdout, _, _ := newLogger(t, tt.level)

			l.LogCommand([]string{"ffmpeg", "-i", "input.mp4"}, tt.succeeded)

			if tt.wantOutput {
				assert.Contains(t, stdout.String(), "ffmpeg -i input.mp4")
				if tt.succeeded {
					assert.Contains(t, stdout.String(), "[成功]")
				} else {
					assert.Contains(t, stdout.String(), "[失敗]")
				}
			} else {
				assert.Empty(t, stdout.String())
			}
		})
	}
}

func TestBuildLogger_Redaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		argv    []string
		want    []string
		notWant []string
	}{
		{
			name: "正常系: apksignerの秘密引数を伏字化する",
			argv: []string{
				"apksigner",
				"sign",
				"--ks",
				"/k.jks",
				"--ks-key-alias",
				"a",
				"--ks-pass",
				"pass:S3cret!",
				"--key-pass",
				"pass:K3y!",
				"--out",
				"o.apk",
				"in.apk",
			},
			want: []string{
				"--ks-pass ***",
				"--key-pass ***",
				"--ks /k.jks",
				"--ks-key-alias a",
			},
			notWant: []string{"S3cret!", "K3y!"},
		},
		{
			name: "正常系: keytoolの秘密引数を伏字化する",
			argv: []string{
				"keytool",
				"-genkeypair",
				"-keystore",
				"/k.jks",
				"-storepass",
				"android",
				"-keypass",
				"android",
				"-alias",
				"a",
			},
			want: []string{
				"-storepass ***",
				"-keypass ***",
				"-keystore /k.jks",
			},
			notWant: []string{"android"},
		},
		{
			name:    "正常系: 等号形式の秘密引数を伏字化する",
			argv:    []string{"--ks-pass=pass:S3cret!"},
			want:    []string{"--ks-pass=***"},
			notWant: []string{"S3cret!"},
		},
		{
			name: "正常系: 末尾の秘密フラグをそのまま記録する",
			argv: []string{"--ks-pass"},
			want: []string{"--ks-pass"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := slices.Clone(tt.argv)
			l, stdout, _, file := newLogger(t, logger.Debug)

			l.LogCommand(tt.argv, true)

			for _, text := range tt.want {
				assert.Contains(t, stdout.String(), text)
				assert.Contains(t, file.String(), text)
			}
			for _, text := range tt.notWant {
				assert.NotContains(t, stdout.String(), text)
				assert.NotContains(t, file.String(), text)
			}
			assert.Equal(t, original, tt.argv)
		})
	}
}

func TestBuildLogger_NilFile(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	l := logger.New(logger.Debug, stdout, stderr, nil)

	l.Info("info")
	l.Verbose("verbose")
	l.Debug("debug")
	l.Warning("warning")
	l.Error("error")
	l.LogConversion("a", "b", "OK")
	l.LogCommand([]string{"cmd"}, true)

	assert.NotEmpty(t, stdout.String())
	assert.NotEmpty(t, stderr.String())
	assert.NoError(t, l.Err())
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestBuildLogger_WriteErrors(t *testing.T) {
	t.Parallel()

	first := errors.New("first")
	second := errors.New("second")

	t.Run("異常系: ファイル失敗でもコンソールへ書き込む", func(t *testing.T) {
		t.Parallel()

		stdout := &bytes.Buffer{}
		l := logger.New(logger.Normal, stdout, &bytes.Buffer{}, failingWriter{first})

		l.Info("message")

		require.Error(t, l.Err())
		require.ErrorIs(t, l.Err(), first)
		assert.Contains(t, stdout.String(), "message")
	})
	t.Run("異常系: 標準出力失敗でもファイルへ書き込む", func(t *testing.T) {
		t.Parallel()

		file := &bytes.Buffer{}
		l := logger.New(logger.Normal, failingWriter{first}, &bytes.Buffer{}, file)

		l.Info("message")

		require.Error(t, l.Err())
		require.ErrorIs(t, l.Err(), first)
		assert.Contains(t, file.String(), "INFO: message")
	})
	t.Run("異常系: 最初のエラーを保持する", func(t *testing.T) {
		t.Parallel()

		l := logger.New(logger.Normal, failingWriter{first}, failingWriter{second}, nil)

		l.Info("message")
		l.Error("error")

		require.ErrorIs(t, l.Err(), first)
		require.NotErrorIs(t, l.Err(), second)
	})
	t.Run("正常系: 失敗がなければnil", func(t *testing.T) {
		t.Parallel()

		l, _, _, _ := newLogger(t, logger.Normal)

		l.Info("message")

		assert.NoError(t, l.Err())
	})
}

func TestBuildLogger_ConcurrentWrites(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	file := &bytes.Buffer{}
	l := logger.New(logger.Normal, stdout, &bytes.Buffer{}, file)
	var wg sync.WaitGroup

	for range 16 {
		wg.Go(func() {
			for range 50 {
				l.Info("並行メッセージ")
			}
		})
	}
	wg.Wait()

	linePattern := regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] INFO: 並行メッセージ$`)
	fileLines := strings.Split(strings.TrimSuffix(file.String(), "\n"), "\n")
	stdoutLines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")

	require.Len(t, fileLines, 800)
	require.Len(t, stdoutLines, 800)
	for _, line := range fileLines {
		assert.Regexp(t, linePattern, line)
	}
	for _, line := range stdoutLines {
		assert.Equal(t, "並行メッセージ", line)
	}
}
