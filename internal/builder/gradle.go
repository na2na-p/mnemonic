// Package builder はkrkrsdl2テンプレートからAndroidプロジェクトを生成し、
// Gradleでビルドするための機能を提供する。
package builder

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/na2na-p/mnemonic/internal/cmdrun"
)

// センチネルエラー群。
var (
	// ErrGradleBuildFailed はGradleビルドが非ゼロ終了コードで終了した場合のエラー。
	ErrGradleBuildFailed = errors.New("Gradleビルドに失敗しました")
	// ErrGradleTimeout はGradleコマンドがタイムアウトした場合のエラー。
	ErrGradleTimeout = errors.New("Gradleコマンドがタイムアウトしました")
	// ErrGradleWrapperNotFound はGradle wrapperが見つからない場合のエラー。
	ErrGradleWrapperNotFound = errors.New("gradle wrapperが見つかりません")
)

// DefaultGradleTimeout はGradleビルドのデフォルトタイムアウト（30分）。
const DefaultGradleTimeout = 1800 * time.Second

// RunResult は外部コマンドの実行結果を表す。
//
// why not: converter.CommandRunner（video.go）は非ゼロ終了コードを暗黙に
// errorへ畳み込む設計だが、Gradleのbuildは終了コード0以外を
// 「ビルド失敗」として自身で判定し、標準出力・標準エラーを結合したログを
// 保持し続ける必要がある。そのため終了コード・stdout・stderrをそのまま呼び出し元へ返す専用の結果型を
// 用意し、実行自体が失敗した場合（wrapper未検出・タイムアウト等）のみ
// errorを返すインターフェースにする。
type RunResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// CommandRunner はGradleコマンド実行を抽象化する。
//
// why not: os/exec.Cmdを直接GradleBuilderから呼ぶとユニットテストが実際に
// gradlewプロセスの起動を要求し、CI環境依存かつ低速になる。実行結果を
// 差し替え可能にするためインターフェース化し、go.uber.org/mock(gomock)で
// モックする（internal/converter.CommandRunnerと同じ設計方針）。
type CommandRunner interface {
	// Run はworkDirをカレントディレクトリとしてargsのコマンドをenv環境変数で実行する。
	// プロセスが起動し完了した場合は終了コードにかかわらずRunResultを返す。
	// タイムアウトやコマンド未検出など、プロセスの実行自体に失敗した場合にerrorを返す。
	Run(ctx context.Context, workDir string, env []string, args []string) (RunResult, error)
}

// execCommandRunner はos/execを使った既定のCommandRunner実装。
type execCommandRunner struct{}

// NewExecCommandRunner はos/execベースのCommandRunnerを返す。
func NewExecCommandRunner() CommandRunner {
	return execCommandRunner{}
}

func (execCommandRunner) Run(ctx context.Context, workDir string, env []string, args []string) (RunResult, error) {
	res, err := cmdrun.Run(ctx, cmdrun.Options{Dir: workDir, Env: env}, args...)
	if errors.Is(err, cmdrun.ErrNoCommand) {
		return RunResult{}, err
	}
	// why not: コンテキストで強制終了されたgradlewはcmdrunからResult{ExitCode: -1}と
	// nil errorで返るため、errだけを見るとタイムアウトがビルド失敗として報告される。
	// 一方、成功時にctx.Err()を見ると、期限直前に正常終了したビルドがタイムアウト扱いに
	// なるため、失敗した場合に限って確認する。
	if err != nil || res.ExitCode != 0 {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return RunResult{}, fmt.Errorf("%w: %w", ErrGradleTimeout, ctxErr)
		}
	}
	if err != nil {
		return RunResult{}, fmt.Errorf("gradleコマンドの実行に失敗しました: %w", err)
	}

	return RunResult{ExitCode: res.ExitCode, Stdout: res.Stdout, Stderr: res.Stderr}, nil
}

// BuildResult はGradleビルド結果を表す不変値。
//
// APKPathがnilの場合、ビルドは成功したがAPKファイルが見つからないことを表す。
type BuildResult struct {
	Success   bool
	APKPath   *string
	BuildTime time.Duration
	OutputLog string
}

// GradleBuilder はAndroidプロジェクトのGradleビルドを実行する。
type GradleBuilder struct {
	projectPath string
	timeout     time.Duration
	runner      CommandRunner
}

// NewGradleBuilder はGradleBuilderを初期化する。
//
// timeoutが0以下の場合はDefaultGradleTimeout（30分）を使用する。
// runnerがnilの場合はos/execベースの既定実装を使用する。
// 初期化時にgradle.propertiesへキャッシュ無効化設定を書き込む。
func NewGradleBuilder(projectPath string, timeout time.Duration, runner CommandRunner) (*GradleBuilder, error) {
	if timeout <= 0 {
		timeout = DefaultGradleTimeout
	}
	if runner == nil {
		runner = NewExecCommandRunner()
	}

	b := &GradleBuilder{
		projectPath: projectPath,
		timeout:     timeout,
		runner:      runner,
	}

	if err := b.disableGradleCaching(); err != nil {
		return nil, err
	}

	return b, nil
}

// disableGradleCaching はgradle.propertiesにキャッシュ無効化設定を追加する。
//
// 一時ディレクトリでのビルドで発生するincremental build問題を回避するため、
// Gradleのキャッシュ機能とファイルシステムウォッチングを無効化する。
func (b *GradleBuilder) disableGradleCaching() error {
	gradleProps := filepath.Join(b.projectPath, "gradle.properties")
	settings := []string{
		"org.gradle.caching=false",
		"org.gradle.vfs.watch=false",
	}

	existing, err := os.ReadFile(gradleProps) //nolint:gosec // プロジェクトディレクトリ配下の固定ファイル名を読む用途のため妥当
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("gradle.propertiesの読み込みに失敗しました: %w", err)
		}

		content := strings.Join(settings, "\n") + "\n"
		if err := os.WriteFile(gradleProps, []byte(content), 0o600); err != nil {
			return fmt.Errorf("gradle.propertiesの作成に失敗しました: %w", err)
		}

		return nil
	}

	content := string(existing)

	var additions []string
	for _, setting := range settings {
		key, _, _ := strings.Cut(setting, "=")
		if !strings.Contains(content, key) {
			additions = append(additions, setting)
		}
	}

	if len(additions) == 0 {
		return nil
	}

	f, err := os.OpenFile(gradleProps, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // プロジェクトディレクトリ配下の固定ファイル名を開く用途のため妥当
	if err != nil {
		return fmt.Errorf("gradle.propertiesの更新に失敗しました: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString("\n" + strings.Join(additions, "\n") + "\n"); err != nil {
		return fmt.Errorf("gradle.propertiesの更新に失敗しました: %w", err)
	}

	return nil
}

// gradlewPath はプラットフォームに応じたGradle Wrapperのパスを返す。
func (b *GradleBuilder) gradlewPath() string {
	name := "gradlew"
	if runtime.GOOS == "windows" {
		name = "gradlew.bat"
	}

	return filepath.Join(b.projectPath, name)
}

// resolveGradleCommand はGradle Wrapperのパスを検証して返す。
//
// Gradle wrapperが見つからない場合はErrGradleWrapperNotFoundを返す。
func (b *GradleBuilder) resolveGradleCommand() (string, error) {
	gradlew := b.gradlewPath()
	if _, err := os.Stat(gradlew); err != nil {
		return "", fmt.Errorf("%w: %s", ErrGradleWrapperNotFound, gradlew)
	}

	if runtime.GOOS != "windows" {
		if err := ensureExecutable(gradlew); err != nil {
			return "", err
		}
	}

	return gradlew, nil
}

// ensureExecutable はfileに実行権限を付与する（ZIPから展開した場合など
// 実行権限がないことがあるため）。
func ensureExecutable(file string) error {
	info, err := os.Stat(file)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrGradleWrapperNotFound, file)
	}

	const execBits = 0o111
	if info.Mode().Perm()&0o100 != 0 {
		return nil
	}

	if err := os.Chmod(file, info.Mode().Perm()|execBits); err != nil {
		return fmt.Errorf("gradle wrapperへの実行権限付与に失敗しました: %w", err)
	}

	return nil
}

// runGradle はGradleコマンドを実行する。
func (b *GradleBuilder) runGradle(args ...string) (RunResult, error) {
	gradlew, err := b.resolveGradleCommand()
	if err != nil {
		return RunResult{}, err
	}

	fullArgs := append([]string{gradlew}, args...)
	fullArgs = append(fullArgs, "--no-daemon", "--no-build-cache", "--rerun-tasks", "--stacktrace")

	// why not: ロケール関連の問題を回避するため、LC_ALL/LANGをC.utf8へ強制する。
	env := append(os.Environ(), "LC_ALL=C.utf8", "LANG=C.utf8")

	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	return b.runner.Run(ctx, b.projectPath, env, fullArgs)
}

// capitalize は先頭文字を大文字化し、残りを小文字化する変換を行う。
func capitalize(s string) string {
	if s == "" {
		return s
	}

	r := []rune(s)

	return strings.ToUpper(string(r[0])) + strings.ToLower(string(r[1:]))
}

// Build はGradleビルドを実行する。
//
// buildTypeが空文字列の場合は"release"を使用する。
// Gradle wrapperが見つからない場合はErrGradleWrapperNotFound、
// タイムアウトした場合はErrGradleTimeout、
// ビルドが非ゼロ終了コードで終わった場合はErrGradleBuildFailedをUnwrapする
// *GradleBuildErrorを返す。
func (b *GradleBuilder) Build(buildType string) (BuildResult, error) {
	buildType = cmp.Or(buildType, "release")

	task := "assemble" + capitalize(buildType)

	start := time.Now()
	result, err := b.runGradle(task)
	buildTime := time.Since(start)

	if err != nil {
		return BuildResult{}, err
	}

	outputLog := joinOutput(result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return BuildResult{}, &GradleBuildError{ExitCode: result.ExitCode, Output: outputLog}
	}

	return BuildResult{
		Success:   true,
		APKPath:   b.GetAPKPath(buildType),
		BuildTime: buildTime,
		OutputLog: outputLog,
	}, nil
}

// joinOutput は標準出力と標準エラーを1つのログへ結合する。
//
// why not: 単純に連結しない。標準出力が改行で終わらないと、標準エラーの先頭行が
// 標準出力の最終行の続きになり、行頭で探す"* What went wrong:"を見落とす。
func joinOutput(stdout, stderr string) string {
	if stdout != "" && stderr != "" && !strings.HasSuffix(stdout, "\n") {
		return stdout + "\n" + stderr
	}

	return stdout + stderr
}

// GetAPKPath は生成されたAPKファイルのパスを取得する。
// buildTypeが空文字列の場合は"release"を使用する。
// 出力ディレクトリが存在しない、またはAPKファイルが1つも無い場合はnilを返す。
//
// 標準的なファイル名（release: app-release-unsigned.apk → app-release.apkの順、
// それ以外: app-<buildType>.apk）を優先して探し、どれも無ければディレクトリ内の
// APKファイルを1つ返す（標準名優先→globフォールバックの設計）。
//
// why not: krkrsdl2テンプレートのapp/build.gradleはoutputFileNameを
// "${app_name}_${architecture}.apk"のようにカスタマイズしており、標準名では
// 見つからないAPKが生成されることがある（実ゲーム資産でのE2Eビルドで判明した
// 回帰）。標準名チェックのみに戻すとGradleビルド自体は成功しているのに
// nilが返り、パイプラインが誤って失敗扱いになる。
func (b *GradleBuilder) GetAPKPath(buildType string) *string {
	buildType = cmp.Or(buildType, "release")

	apkDir := filepath.Join(b.projectPath, "app", "build", "outputs", "apk", buildType)

	if _, err := os.Stat(apkDir); err != nil {
		return nil
	}

	standardNames := []string{fmt.Sprintf("app-%s.apk", buildType)}
	if buildType == "release" {
		standardNames = []string{"app-release-unsigned.apk", "app-release.apk"}
	}

	for _, name := range standardNames {
		apkPath := filepath.Join(apkDir, name)
		if _, err := os.Stat(apkPath); err == nil {
			return &apkPath
		}
	}

	// why not: filepath.Globが返す順序はOS/ファイルシステム依存で非決定的なため、
	// テストの再現性を保つ目的でソートしてから先頭を採用する（テストはAPKが1つの
	// みのケースしか要求しないため実害はない）。
	matches, err := filepath.Glob(filepath.Join(apkDir, "*.apk"))
	if err != nil || len(matches) == 0 {
		return nil
	}

	sort.Strings(matches)

	return &matches[0]
}

// Gradleの失敗出力から要約を作る際の上限と見出し。
const (
	// why not: ブロックを無制限に示さない。複数の並列処理が失敗すると
	// "Multiple task action failures occurred"の下に1件あたり3行ずつ並び、
	// AAPT2デーモン5個の起動失敗で17行になった。同じ内容が繰り返されるだけなので、
	// その2倍弱で打ち切る。
	gradleCauseMaxLines = 30
	// why not: 原因ブロックが無い出力を全て示さない。ブロックが無い出力は
	// Gradleの定型の失敗報告ではなく、原因行の位置を形式から決められないため、
	// 端末の1画面に収まる末尾だけを示し、全文はGradleBuildError.Outputから記録させる。
	gradleTailLines = 20

	gradleCauseHeader = "* What went wrong:"
	// gradleSectionPrefix はGradleの失敗報告の各節（"* Try:"、"* Exception is:"等）の行頭。
	gradleSectionPrefix = "* "
)

// GradleBuildError はGradleビルドが非ゼロ終了コードで終わったことを表す。
// errors.Is(err, ErrGradleBuildFailed)を満たす。
//
// why not: Gradleの全出力をエラー文へ入れない。全出力はダウンロード進捗や
// スタックトレースを含み、実ゲームのビルド失敗では1100行を超えた。エラー文は
// 端末へそのまま表示され、ログファイルにも全行へ接頭辞を付けて書かれるため、
// 原因が埋もれる。エラー文には原因の要約だけを入れ、全出力はOutputとして
// 呼び出し側に記録させる。
type GradleBuildError struct {
	ExitCode int
	// Output は標準出力と標準エラーを結合したGradleの全出力。
	Output string
}

// Error はGradleが報告した"* What went wrong:"ブロックを、無ければ出力の
// 末尾を、終了コードとともに返す。
func (e *GradleBuildError) Error() string {
	summary := summarizeGradleFailure(e.Output)
	if summary == "" {
		return fmt.Sprintf("%s: exit code %d: （Gradleの出力なし）", ErrGradleBuildFailed.Error(), e.ExitCode)
	}

	return fmt.Sprintf("%s: exit code %d:\n%s", ErrGradleBuildFailed.Error(), e.ExitCode, summary)
}

// Unwrap はErrGradleBuildFailedを返す。
func (e *GradleBuildError) Unwrap() error {
	return ErrGradleBuildFailed
}

// summarizeGradleFailure はGradleの出力から失敗の原因を示す行を取り出す。
func summarizeGradleFailure(output string) string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")

	cause := gradleCauseLines(lines)
	if len(cause) == 0 {
		return strings.Join(lastNonEmptyLines(lines, gradleTailLines), "\n")
	}

	if len(cause) > gradleCauseMaxLines {
		omitted := len(cause) - gradleCauseMaxLines
		cause = append(cause[:gradleCauseMaxLines], fmt.Sprintf("（ほか%d行を省略）", omitted))
	}

	return strings.Join(cause, "\n")
}

// gradleCauseLines は全ての"* What went wrong:"ブロックの本文を、見出しと
// ブロック前後の空行を除いて返す。ブロックの間は空行1つで区切る。
func gradleCauseLines(lines []string) []string {
	var (
		cause   []string
		block   []string
		inBlock bool
	)

	flush := func() {
		block = trimBlankLines(block)
		if len(block) == 0 {
			return
		}
		if len(cause) > 0 {
			cause = append(cause, "")
		}
		cause = append(cause, block...)
		block = nil
	}

	for _, line := range lines {
		switch {
		case line == gradleCauseHeader:
			flush()
			inBlock = true
		case inBlock && strings.HasPrefix(line, gradleSectionPrefix):
			flush()
			inBlock = false
		case inBlock:
			block = append(block, line)
		}
	}
	flush()

	return cause
}

// trimBlankLines は先頭と末尾の空行を除いたlinesを返す。
func trimBlankLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

// lastNonEmptyLines は空行を除いたlinesの末尾n行を返す。
func lastNonEmptyLines(lines []string, n int) []string {
	nonEmpty := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}

	return nonEmpty[max(len(nonEmpty)-n, 0):]
}
