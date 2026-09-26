// Package logger は進捗表示およびログ出力を提供する。
//
// VerboseLevelに応じた出力制御を行い、CLIでのビルド進捗をユーザーにわかりやすく
// 表示するために使用される。
package logger

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"sync"
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

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// secretFlags は値が秘密情報になる引数名。apksignerの--ks-pass / --key-passと、
// keytoolの-storepass / -keypass。
var secretFlags = []string{"--ks-pass", "--key-pass", "-storepass", "-keypass"}

// BuildLogger はコンソールと注入されたログ出力先へビルドログを書き込む。
type BuildLogger struct {
	level  VerboseLevel
	stdout io.Writer
	stderr io.Writer
	file   io.Writer

	mu  sync.Mutex
	err error
}

// New は出力先を注入してBuildLoggerを生成する。fileがnilの場合はファイル出力を行わない。
//
// why not: ログファイルのパスを受け取ってここで開く形にはしない。追記するか切り詰めるかの
// 選択と書き込み完了後のCloseは、ファイルを開いた側が一貫して責任を持つ。
func New(level VerboseLevel, stdout, stderr, file io.Writer) *BuildLogger {
	return &BuildLogger{level: level, stdout: stdout, stderr: stderr, file: file}
}

func (l *BuildLogger) write(writer io.Writer, message string) {
	if _, err := fmt.Fprintln(writer, message); err != nil && l.err == nil {
		l.err = fmt.Errorf("ログの書き込みに失敗しました: %w", err)
	}
}

func (l *BuildLogger) log(level, consoleMessage, fileMessage string, console io.Writer, enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if enabled {
		l.write(console, consoleMessage)
	}
	if l.file != nil {
		clean := ansiEscapePattern.ReplaceAllString(fileMessage, "")
		l.write(l.file, prefixLines(fmt.Sprintf("[%s] %s: ", time.Now().Format("2006-01-02 15:04:05"), level), clean))
	}
}

// prefixLines はmessageの各行の先頭にprefixを付けて改行で連結する。messageの末尾の
// 改行は1つだけ行の終端として扱い、改行で区切った各行の末尾からCRを1つ除く。
//
// why not: 2行目以降を接頭辞無しで続けて書かない。grep ERRORのような行単位の抽出で
// 続きの行が落ち、別のエントリにも見える。
func prefixLines(prefix, message string) string {
	lines := make([]string, 0, strings.Count(message, "\n")+1)
	for line := range strings.SplitSeq(strings.TrimSuffix(message, "\n"), "\n") {
		lines = append(lines, prefix+strings.TrimSuffix(line, "\r"))
	}

	return strings.Join(lines, "\n")
}

// Info は情報メッセージを出力する（Normal以上）。
func (l *BuildLogger) Info(message string) {
	l.log("INFO", message, message, l.stdout, l.level >= Normal)
}

// Verbose は詳細メッセージを出力する（Verbose以上）。
func (l *BuildLogger) Verbose(message string) {
	l.log("VERBOSE", message, message, l.stdout, l.level >= Verbose)
}

// Debug はデバッグメッセージを出力する（Debug以上）。
func (l *BuildLogger) Debug(message string) {
	l.log("DEBUG", message, message, l.stdout, l.level >= Debug)
}

// Warning は警告メッセージを出力する（Quietより上のレベル）。
func (l *BuildLogger) Warning(message string) {
	l.log("WARNING", fmt.Sprintf("警告: %s", message), message, l.stdout, l.level > Quiet)
}

// Error はエラーメッセージを常に標準エラー出力へ出力する。
func (l *BuildLogger) Error(message string) {
	l.log("ERROR", fmt.Sprintf("エラー: %s", message), message, l.stderr, true)
}

// LogConversion はファイル変換の結果を記録する（Verbose以上）。
//
// why not: filepath.Baseで短くしない。bg/a.pngとfg/a.pngのように同名のファイルを
// 区別できなくなるため、呼び出し側が渡したパスをそのまま出す。
func (l *BuildLogger) LogConversion(source, dest, status string) {
	l.Verbose(fmt.Sprintf("変換: %s -> %s [%s]", source, dest, status))
}

// redactCommand はsecretFlagsの値を伏字にしたargvの写しを返す。
//
// why not: argvを書き換えない。呼び出し側は同じスライスをコマンドの実行にも使う。
func redactCommand(argv []string) []string {
	redacted := slices.Clone(argv)
	for i, arg := range redacted {
		for _, flag := range secretFlags {
			if arg == flag {
				if i+1 < len(redacted) {
					redacted[i+1] = "***"
				}
				break
			}
			prefix := flag + "="
			if strings.HasPrefix(arg, prefix) {
				redacted[i] = prefix + "***"
				break
			}
		}
	}

	return redacted
}

// LogCommand は外部コマンドの引数と成否を記録する（Debug以上）。
//
// why not: コマンドの出力とエラー値は受け取らない。署名ツールの出力や環境由来の
// 文字列は秘密を含み得るため、伏字化した引数と成否だけを記録する。
func (l *BuildLogger) LogCommand(argv []string, succeeded bool) {
	status := "失敗"
	if succeeded {
		status = "成功"
	}
	l.Debug(fmt.Sprintf("実行: %s [%s]", strings.Join(redactCommand(argv), " "), status))
}

// Err は最初に発生した書き込みエラーを返す。失敗がなければnil。
//
// why not: 書き込みに失敗しても以降の書き込みは止めない。ログファイルの障害で
// コンソール出力まで失わないよう、エラーは保持して扱いを呼び出し側に委ねる。
func (l *BuildLogger) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.err
}
