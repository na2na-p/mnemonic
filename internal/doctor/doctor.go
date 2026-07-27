// Package doctor はビルドに必要な依存ツールのインストール状況を確認する。
package doctor

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"regexp"
	"time"
)

// checkTimeout は依存ツールのバージョン確認コマンドのタイムアウト。
const checkTimeout = 10 * time.Second

// CheckResult はチェック結果を表す値。
type CheckResult struct {
	Name     string
	Required bool
	Found    bool
	// Versionが空文字列の場合、バージョン未検出を表す。
	Version string
	// Messageが空文字列の場合、メッセージなしを表す。
	Message string
}

// DependencyInfo は依存ツール情報を表す値。
type DependencyInfo struct {
	Name        string
	Command     string
	VersionFlag string
	Required    bool
	// MinVersionが空文字列の場合、最小バージョン指定なしを表す。
	MinVersion string
}

// Dependencies はビルドに必要な依存ツールの一覧。
// mnemonic自体はGoの単一バイナリとして動作するためランタイム依存を持たない。
// ここに列挙するのはAPKビルドパイプラインが実行時に呼び出す外部ツール。
var Dependencies = []DependencyInfo{
	{Name: "Java JDK", Command: "java", VersionFlag: "-version", Required: true, MinVersion: "17"},
	{Name: "Android SDK", Command: "sdkmanager", VersionFlag: "--version", Required: true},
	{Name: "Android NDK", Command: "ndk-build", VersionFlag: "--version", Required: true},
	{Name: "FFmpeg", Command: "ffmpeg", VersionFlag: "-version", Required: true},
}

// versionPatterns はコマンド出力からバージョン番号を抽出する正規表現の候補。
// 先頭から順に試し、最初にマッチしたものを採用する。
var versionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(\d+\.\d+\.\d+)`),
	regexp.MustCompile(`(\d+\.\d+)`),
	regexp.MustCompile(`(?i)version\s+(\d+)`),
	regexp.MustCompile(`(\d+)`),
}

// ExtractVersion はコマンド出力からバージョン番号を抽出する。
// 抽出できない場合は空文字列を返す。
func ExtractVersion(output string) string {
	for _, pattern := range versionPatterns {
		if m := pattern.FindStringSubmatch(output); m != nil {
			return m[1]
		}
	}

	return ""
}

// CheckDependency は単一の依存ツールをチェックする。
func CheckDependency(info DependencyInfo) CheckResult {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, info.Command, info.VersionFlag) //nolint:gosec // doctorコマンドで固定の依存ツール一覧を検査する用途のため妥当

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	switch {
	case err == nil:
		output := stdout.String() + stderr.String()

		return CheckResult{
			Name:     info.Name,
			Required: info.Required,
			Found:    true,
			Version:  ExtractVersion(output),
		}
	case ctx.Err() != nil:
		return CheckResult{
			Name:     info.Name,
			Required: info.Required,
			Found:    false,
			Message:  "コマンド '" + info.Command + "' がタイムアウトしました",
		}
	default:
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return CheckResult{
				Name:     info.Name,
				Required: info.Required,
				Found:    false,
				Message:  "コマンド '" + info.Command + "' が見つかりません",
			}
		}

		// 非ゼロ終了コード（例: javaは-versionの出力を標準エラーへ書きつつ
		// 正常終了コード以外を返すことがある）でもバージョン情報自体は
		// 取得できている場合があるため、foundとして扱いバージョン抽出を試みる。
		output := stdout.String() + stderr.String()

		return CheckResult{
			Name:     info.Name,
			Required: info.Required,
			Found:    true,
			Version:  ExtractVersion(output),
		}
	}
}

// CheckAllDependencies は全ての依存ツールをチェックする。
func CheckAllDependencies() []CheckResult {
	results := make([]CheckResult, 0, len(Dependencies))
	for _, dep := range Dependencies {
		results = append(results, CheckDependency(dep))
	}

	return results
}
