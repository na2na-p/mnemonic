// Package logger は進捗表示およびログ出力を提供する。
//
// Python版 (src/mnemonic/logger.py) をGoへ移植したもの。
// VerboseLevelに応じた出力制御を行い、CLIでのビルド進捗をユーザーにわかりやすく
// 表示するために使用される。
package logger

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/na2na-p/mnemonic/internal/pipeline"
)

// VerboseLevel は詳細ログレベルを表す。
//
//   - Quiet: エラーのみ出力
//   - Normal: 進捗バーとサマリ出力
//   - Verbose: 変換ファイル一覧も出力（-vオプション）
//   - Debug: 外部コマンド実行ログも出力（-vvオプション）
type VerboseLevel int

// VerboseLevelの各段階。値の大小関係で出力するログの詳細度を比較する。
const (
	Quiet   VerboseLevel = -1
	Normal  VerboseLevel = 0
	Verbose VerboseLevel = 1
	Debug   VerboseLevel = 2
)

// ProgressDisplay はビルドパイプラインの各フェーズの進捗を表示するインターフェース。
type ProgressDisplay interface {
	// Start はフェーズ開始を表示する。
	Start(phase pipeline.Phase, total int)
	// Update は進捗を更新する。messageは追加の進捗メッセージ（不要な場合は空文字列）。
	Update(current int, message string)
	// Finish はフェーズ終了を表示する。messageは終了メッセージ（不要な場合は空文字列）。
	Finish(success bool, message string)
}

// LogConfig はログ出力の動作を制御する設定。
type LogConfig struct {
	VerboseLevel VerboseLevel
	// LogFile はログ出力先ファイルパス。空文字列の場合はファイル出力なし
	// （Pythonの `log_file: Path | None = None` に相当）。
	LogFile  string
	UseColor bool
	UseEmoji bool
}

// DefaultLogConfig はデフォルトのログ設定を返す。
func DefaultLogConfig() LogConfig {
	return LogConfig{
		VerboseLevel: Normal,
		UseColor:     true,
		UseEmoji:     true,
	}
}

// Statistics はビルドサマリに表示する統計情報を表す。
//
// OutputPath / PackageName はポインタとし、未設定（Pythonでいう
// `"output_path" in statistics` が偽の状態）を表現する。
type Statistics struct {
	OutputPath *string
	// OutputSize はOutputPathが設定されている場合のみ意味を持つ（バイト単位）。
	OutputSize  int64
	PackageName *string
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// BuildLogger はビルドパイプラインのログ出力を管理する。
//
// Python版はコンテキストマネージャ（__enter__/__exit__）でログファイルの
// クローズを保証していたが、GoではCloseメソッドを公開しio.Closerとして扱う
// のが自然なため、呼び出し側は `defer logger.Close()` で同等の保証を得る。
type BuildLogger struct {
	config LogConfig
	stdout io.Writer
	stderr io.Writer

	logFile *os.File
	closed  bool
}

// New はconfigに従いBuildLoggerを生成する。標準出力・標準エラー出力には
// 実プロセスのos.Stdout/os.Stderrを使用する。
func New(config LogConfig) (*BuildLogger, error) {
	return NewWithWriters(config, os.Stdout, os.Stderr)
}

// NewWithWriters は出力先を指定してBuildLoggerを生成する。
//
// テストで実プロセスの標準出力を捕捉すると t.Parallel() のテスト間で
// 出力が混ざり合うため、io.Writerを直接注入できるようにしている
// （CLAUDE.mdの「外部依存は注入可能にする」方針に沿った設計）。
func NewWithWriters(config LogConfig, stdout, stderr io.Writer) (*BuildLogger, error) {
	l := &BuildLogger{config: config, stdout: stdout, stderr: stderr}

	if config.LogFile != "" {
		f, err := os.Create(filepath.Clean(config.LogFile))
		if err != nil {
			return nil, fmt.Errorf("ログファイルを開けません: %w", err)
		}
		l.logFile = f
	}

	return l, nil
}

// Close はログファイルを閉じる。ログファイルを使用していない場合、
// または既に閉じている場合は何もしない（複数回呼び出しても安全）。
func (l *BuildLogger) Close() error {
	if l.logFile == nil || l.closed {
		l.closed = true

		return nil
	}

	l.closed = true
	if err := l.logFile.Close(); err != nil {
		return fmt.Errorf("ログファイルのクローズに失敗しました: %w", err)
	}

	return nil
}

// Closed はログファイルが（存在する場合に）クローズ済みかどうかを返す。
func (l *BuildLogger) Closed() bool {
	return l.closed
}

// Config は現在のログ設定を返す。
func (l *BuildLogger) Config() LogConfig {
	return l.config
}

func (l *BuildLogger) print(w io.Writer, message string) {
	fmt.Fprintln(w, message) //nolint:errcheck // ログ出力の書き込み失敗は実用上ハンドリング不要
}

func (l *BuildLogger) logToFile(level, message string) {
	if l.logFile == nil || l.closed {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	clean := ansiEscapePattern.ReplaceAllString(message, "")
	fmt.Fprintf(l.logFile, "[%s] %s: %s\n", timestamp, level, clean) //nolint:errcheck // 同上
}

// Info は情報メッセージを出力する（Normal以上）。
func (l *BuildLogger) Info(message string) {
	if l.config.VerboseLevel >= Normal {
		l.print(l.stdout, message)
	}
	l.logToFile("INFO", message)
}

// Verbose は詳細メッセージを出力する（Verbose以上）。
func (l *BuildLogger) Verbose(message string) {
	if l.config.VerboseLevel >= Verbose {
		l.print(l.stdout, message)
	}
	l.logToFile("VERBOSE", message)
}

// Debug はデバッグメッセージを出力する（Debug以上）。
func (l *BuildLogger) Debug(message string) {
	if l.config.VerboseLevel >= Debug {
		l.print(l.stdout, message)
	}
	l.logToFile("DEBUG", message)
}

// Error はエラーメッセージを常に標準エラー出力へ出力する。
func (l *BuildLogger) Error(message string) {
	l.print(l.stderr, fmt.Sprintf("エラー: %s", message))
	l.logToFile("ERROR", message)
}

// Warning は警告メッセージを出力する（Quietより上のレベル）。
func (l *BuildLogger) Warning(message string) {
	if l.config.VerboseLevel > Quiet {
		l.print(l.stdout, fmt.Sprintf("警告: %s", message))
	}
	l.logToFile("WARNING", message)
}

// CreateProgress は進捗表示インスタンスを作成する。
func (l *BuildLogger) CreateProgress() ProgressDisplay {
	return NewConsoleProgressDisplayWithWriter(l.config.UseColor, l.config.UseEmoji, l.stdout)
}

// LogCommand は外部コマンド実行をログする（Debug以上）。
func (l *BuildLogger) LogCommand(command []string, output string) {
	l.Debug(fmt.Sprintf("実行: %s", strings.Join(command, " ")))
	if output == "" {
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		l.Debug(fmt.Sprintf("  > %s", scanner.Text()))
	}
}

// LogConversion はファイル変換をログする（Verbose以上）。
func (l *BuildLogger) LogConversion(source, dest, status string) {
	l.Verbose(fmt.Sprintf("変換: %s -> %s [%s]", filepath.Base(source), filepath.Base(dest), status))
}

// LogSummary はビルドサマリを出力する（Normal以上）。
func (l *BuildLogger) LogSummary(stats Statistics) {
	emoji := "✅"
	if !l.config.UseEmoji {
		emoji = "[OK]"
	}
	l.Info(fmt.Sprintf("%s Build complete!", emoji))

	if stats.OutputPath != nil {
		sizeMB := float64(stats.OutputSize) / (1024 * 1024)
		l.Info(fmt.Sprintf("   Output: %s (%.1f MB)", *stats.OutputPath, sizeMB))
	}
	if stats.PackageName != nil {
		l.Info(fmt.Sprintf("   Package: %s", *stats.PackageName))
	}
}
