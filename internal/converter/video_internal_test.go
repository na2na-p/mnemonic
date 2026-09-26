package converter

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "dstの親が通常ファイルの場合はコピー失敗として包んだstatエラーを返す"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			src := filepath.Join(dir, "src.mpg")
			require.NoError(t, os.WriteFile(src, []byte("content"), 0o644))
			notADir := filepath.Join(dir, "not-a-dir")
			require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o644))

			err := copyFile(src, filepath.Join(notADir, "dst.mpg"))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "ファイルのコピーに失敗しました")
			pathErr, ok := errors.AsType[*fs.PathError](err)
			require.True(t, ok, "want *fs.PathError, got %v", err)
			assert.Equal(t, "stat", pathErr.Op)
		})
	}
}

func TestSummarizeStderr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stderr string
		want   string
	}{
		{
			name:   "正常系: 空でない行が上限を超える場合は先頭3行を「 | 」でつなぎ空でない行の総数を添える",
			stderr: "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\n",
			want:   "l1 | l2 | l3（全 10 行）",
		},
		{
			name:   "正常系: 空でない行が上限ちょうどなら省略せず件数も添えない",
			stderr: "l1\nl2\nl3\n",
			want:   "l1 | l2 | l3",
		},
		{
			name:   "正常系: 空でない行が1行ならその行だけを返す",
			stderr: "  l1  \n",
			want:   "l1",
		},
		{
			name:   "正常系: 空行と空白だけの行は数えずに飛ばす",
			stderr: "\n\nl1\n   \nl2\r\n\nl3\n\nl4\n",
			want:   "l1 | l2 | l3（全 4 行）",
		},
		{
			name:   "正常系: 空のstderrは空文字列を返す",
			stderr: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, summarizeStderr(tt.stderr, 3))
		})
	}
}

func TestExecCommandRunner_Run(t *testing.T) {
	t.Parallel()

	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("shが無いためスキップ")
	}

	const script = `for i in 1 2 3 4 5 6 7 8 9 10; do echo "line$i" >&2; done; exit 1`

	tests := []struct {
		name        string
		runner      CommandRunner
		wantContain string
		wantAbsent  string
	}{
		{
			name:        "異常系: 行数の上限があればstderrの先頭の行だけと総行数をエラーに含める",
			runner:      execCommandRunner{stderrLineLimit: 3},
			wantContain: ": line1 | line2 | line3（全 10 行）",
			wantAbsent:  "line4",
		},
		{
			name:        "異常系: 行数の上限が無ければstderrをすべてエラーに含める",
			runner:      execCommandRunner{},
			wantContain: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10",
			wantAbsent:  "（全",
		},
		{
			name:        "異常系: NewExecCommandRunnerの実装はstderrを絞らずすべてエラーに含める",
			runner:      NewExecCommandRunner(),
			wantContain: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10",
			wantAbsent:  "（全",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.runner.Run(t.Context(), sh, "-c", script)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantContain)
			assert.NotContains(t, err.Error(), tt.wantAbsent)
		})
	}
}

func TestNewVideoConverter_DefaultRunnerLimitsStderr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want CommandRunner
	}{
		{
			name: "正常系: runnerがnilなら動画変換用にstderrを先頭3行へ絞る既定実装を使う",
			want: execCommandRunner{stderrLineLimit: 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, NewVideoConverter(0, nil).runner)
		})
	}
}
