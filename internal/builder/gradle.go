// Package builder はkrkrsdl2テンプレートからAndroidプロジェクトを生成し、
// Gradleでビルドするための機能を提供する。
package builder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// センチネルエラー群。
var (
	// ErrGradleBuildFailed はGradleビルド/クリーンが非ゼロ終了コードで終了した場合のエラー。
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
// errorへ畳み込む設計だが、Gradleのbuild/cleanは終了コード0以外を
// 「ビルド失敗」として自身で判定し、標準出力・標準エラーを結合したログを
// 保持し続ける必要がある（Python版のsubprocess.run(check=False)と同じ設計）。
// そのため終了コード・stdout・stderrをそのまま呼び出し元へ返す専用の結果型を
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
	if len(args) == 0 {
		return RunResult{}, errors.New("実行するコマンドが指定されていません")
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // gradlewを呼び出す用途のため妥当
	cmd.Dir = workDir
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return RunResult{}, fmt.Errorf("%w: %w", ErrGradleTimeout, ctxErr)
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return RunResult{
				ExitCode: exitErr.ExitCode(),
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
			}, nil
		}

		return RunResult{}, fmt.Errorf("gradleコマンドの実行に失敗しました: %w", err)
	}

	return RunResult{ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

// BuildResult はGradleビルド結果を表す不変値。
//
// APKPathがnilの場合、Python版のNone（ビルドは成功したがAPKファイルが
// 見つからない）に相当する。
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
// 初期化時にgradle.propertiesへキャッシュ無効化設定を書き込む
// （Python版__init__の_disable_gradle_caching呼び出しに相当）。
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

	// why not: ロケール関連の問題を回避するため、Python版と同様にLC_ALL/LANGを
	// C.utf8へ強制する。
	env := append(os.Environ(), "LC_ALL=C.utf8", "LANG=C.utf8")

	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	return b.runner.Run(ctx, b.projectPath, env, fullArgs)
}

// capitalize はPythonのstr.capitalize()と等価な変換を行う
// （先頭文字を大文字化し、残りを小文字化する）。
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
// ビルドが非ゼロ終了コードで終わった場合はErrGradleBuildFailedを返す。
func (b *GradleBuilder) Build(buildType string) (BuildResult, error) {
	if buildType == "" {
		buildType = "release"
	}

	task := "assemble" + capitalize(buildType)

	start := time.Now()
	result, err := b.runGradle(task)
	buildTime := time.Since(start)

	if err != nil {
		return BuildResult{}, err
	}

	outputLog := result.Stdout + result.Stderr

	if result.ExitCode != 0 {
		return BuildResult{}, fmt.Errorf("%w: exit code %d: %s", ErrGradleBuildFailed, result.ExitCode, outputLog)
	}

	return BuildResult{
		Success:   true,
		APKPath:   b.GetAPKPath(buildType),
		BuildTime: buildTime,
		OutputLog: outputLog,
	}, nil
}

// Clean はGradleのcleanタスクを実行してビルドキャッシュを削除する。
func (b *GradleBuilder) Clean() error {
	result, err := b.runGradle("clean")
	if err != nil {
		return err
	}

	if result.ExitCode != 0 {
		outputLog := result.Stdout + result.Stderr

		return fmt.Errorf("%w: exit code %d: %s", ErrGradleBuildFailed, result.ExitCode, outputLog)
	}

	return nil
}

// CheckGradleWrapper はプロジェクトディレクトリにGradle Wrapperが存在するかを確認する。
func (b *GradleBuilder) CheckGradleWrapper() bool {
	_, err := os.Stat(b.gradlewPath())

	return err == nil
}

// GetAPKPath は生成されたAPKファイルのパスを取得する。
// buildTypeが空文字列の場合は"release"を使用する。
// ファイルが存在しない場合はnilを返す。
func (b *GradleBuilder) GetAPKPath(buildType string) *string {
	if buildType == "" {
		buildType = "release"
	}

	apkName := fmt.Sprintf("app-%s.apk", buildType)
	if buildType == "release" {
		apkName = "app-release-unsigned.apk"
	}

	apkPath := filepath.Join(b.projectPath, "app", "build", "outputs", "apk", buildType, apkName)

	if _, err := os.Stat(apkPath); err != nil {
		return nil
	}

	return &apkPath
}
