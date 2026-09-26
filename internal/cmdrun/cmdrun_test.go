package cmdrun_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/cmdrun"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts cmdrun.Options
		args []string
		want cmdrun.Result
	}{
		{
			name: "正常系: 終了コード0でstdoutを返す",
			args: []string{"echo", "-n", "ok"},
			want: cmdrun.Result{ExitCode: 0, Stdout: "ok", Stderr: ""},
		},
		{
			name: "正常系: 非ゼロ終了はerrorではなくResultで返す",
			args: []string{"sh", "-c", "echo fail 1>&2; exit 3"},
			want: cmdrun.Result{ExitCode: 3, Stdout: "", Stderr: "fail\n"},
		},
		{
			name: "正常系: Options.Envが環境変数になる",
			opts: cmdrun.Options{Env: []string{"MNEMONIC_CMDRUN_TEST=1"}},
			args: []string{"sh", "-c", "printf %s \"$MNEMONIC_CMDRUN_TEST\""},
			want: cmdrun.Result{ExitCode: 0, Stdout: "1", Stderr: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := cmdrun.Run(t.Context(), tt.opts, tt.args...)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRun_Dir(t *testing.T) {
	t.Parallel()

	t.Run("正常系: Options.Dirが作業ディレクトリになる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		got, err := cmdrun.Run(t.Context(), cmdrun.Options{Dir: dir}, "pwd")

		require.NoError(t, err)
		assert.Equal(t, 0, got.ExitCode)

		// macOSではt.TempDir()の/varが/private/varへのシンボリックリンクのため、
		// 両辺を実パスへ解決してから比較する。
		wantDir, err := filepath.EvalSymlinks(dir)
		require.NoError(t, err)
		gotDir, err := filepath.EvalSymlinks(strings.TrimRight(got.Stdout, "\n"))
		require.NoError(t, err)
		assert.Equal(t, wantDir, gotDir)
	})
}

// TestRun_InheritEnv はt.Setenvでプロセス環境変数を書き換えるため、
// 並列実行する他のテストへ影響しないようt.Parallel()を呼ばない。
func TestRun_InheritEnv(t *testing.T) {
	t.Run("正常系: Envがnilなら親の環境を継承する", func(t *testing.T) {
		t.Setenv("MNEMONIC_CMDRUN_INHERIT", "yes")

		got, err := cmdrun.Run(t.Context(), cmdrun.Options{}, "sh", "-c", "printf %s \"$MNEMONIC_CMDRUN_INHERIT\"")

		require.NoError(t, err)
		assert.Equal(t, cmdrun.Result{ExitCode: 0, Stdout: "yes", Stderr: ""}, got)
	})
}

func TestRun_ContextDeadline(t *testing.T) {
	t.Parallel()

	t.Run("正常系: コンテキスト期限超過で強制終了された場合はExitCode-1とnil error", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()

		got, err := cmdrun.Run(ctx, cmdrun.Options{}, "sleep", "5")

		// why not: os/execはコンテキストで強制終了したプロセスを*exec.ExitError
		// （ExitCode -1）として報告する。タイムアウトかどうかの判定は呼び出し元の
		// 責務のため、Runはerrorへ変換せずResultのまま返すことを固定する。
		require.NoError(t, err)
		assert.Equal(t, -1, got.ExitCode)
	})
}

func TestRun_Error(t *testing.T) {
	t.Parallel()

	t.Run("異常系: コマンドが空ならErrNoCommand", func(t *testing.T) {
		t.Parallel()

		got, err := cmdrun.Run(t.Context(), cmdrun.Options{})

		require.ErrorIs(t, err, cmdrun.ErrNoCommand)
		assert.Equal(t, cmdrun.Result{}, got)
	})

	t.Run("異常系: コマンドが見つからない場合は*exec.Errorをそのまま返す", func(t *testing.T) {
		t.Parallel()

		got, err := cmdrun.Run(t.Context(), cmdrun.Options{}, "mnemonic-cmdrun-nonexistent-command-xyz")

		var execErr *exec.Error
		require.ErrorAs(t, err, &execErr)
		assert.Equal(t, cmdrun.Result{}, got)
	})
}
