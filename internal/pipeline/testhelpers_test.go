package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// skipIfCaseInsensitiveFS はdirが大文字小文字を区別しないファイルシステム
// （例: macOSのAPFS既定設定）上にある場合、このテストをスキップする。
//
// why not: os.Rename("Foo.tmp", "foo.tmp")は大文字小文字を区別しない
// ファイルシステム上では単なる同一ファイルの表記変更になり、リネーム後も
// 旧表記のパスでos.Statが成功し続ける。これはリネーム処理自体の不具合では
// なくファイルシステムの性質であり、本番コードを変更すべき問題ではない。
// CI/DevContainerの実行環境（Linux、大文字小文字を区別する）では本テストは
// 実際にリネームの旧名不在を検証する。判定はconstではなく実際の
// t.TempDir()をprobeして行う（tmpディレクトリのマウント設定次第でホストの
// デフォルトと異なる場合があるため）。
func skipIfCaseInsensitiveFS(t *testing.T, dir string) {
	t.Helper()

	probe := filepath.Join(dir, "Foo.tmp")
	require.NoError(t, os.WriteFile(probe, []byte("probe"), 0o600))

	if _, err := os.Stat(filepath.Join(dir, "foo.tmp")); err == nil {
		t.Skip("大文字小文字を区別しないファイルシステムのためスキップ")
	}
}

// fakeCommandRunner はconverter.CommandRunnerの単純なテスト用実装。
// コマンド名(nameの末尾)ごとに固定の応答を返す。
type fakeCommandRunner struct {
	// responses はコマンド名(例: "fluidsynth")をキーとする応答設定。
	responses map[string]fakeCommandResponse
}

type fakeCommandResponse struct {
	output []byte
	err    error
}

func (r fakeCommandRunner) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	resp, ok := r.responses[name]
	if !ok {
		return nil, errors.New("予期しないコマンド: " + name)
	}

	return resp.output, resp.err
}

// logEntry はrecordingLoggerが記録した1件のログ。
type logEntry struct {
	level   string
	message string
}

// recordingLogger は受け取ったログを順に記録するLoggerのテスト用実装。
// BuildLogger（internal/logger）と同じくmutexで保護し、ワーカーから並行に
// ログを出す箇所が加わっても-raceでテストが壊れないようにする。
type recordingLogger struct {
	mu      sync.Mutex
	entries []logEntry
}

func (r *recordingLogger) record(level, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries = append(r.entries, logEntry{level: level, message: message})
}

func (r *recordingLogger) Info(message string)    { r.record("INFO", message) }
func (r *recordingLogger) Warning(message string) { r.record("WARNING", message) }
func (r *recordingLogger) Verbose(message string) { r.record("VERBOSE", message) }

// messages はlevelのログのメッセージを記録順に返す。
func (r *recordingLogger) messages(level string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var got []string
	for _, e := range r.entries {
		if e.level == level {
			got = append(got, e.message)
		}
	}

	return got
}
