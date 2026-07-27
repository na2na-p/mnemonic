package signer_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/signer"
)

func TestExecCommandRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 終了コード0でstdoutを返す", func(t *testing.T) {
		t.Parallel()

		runner := signer.NewExecCommandRunner()

		result, err := runner.Run(context.Background(), []string{"echo", "-n", "ok"})

		require.NoError(t, err)
		assert.Equal(t, 0, result.ExitCode)
		assert.Equal(t, "ok", result.Stdout)
	})

	t.Run("正常系: 非ゼロ終了コードはerrorではなくRunResultで返す", func(t *testing.T) {
		t.Parallel()

		runner := signer.NewExecCommandRunner()

		result, err := runner.Run(context.Background(), []string{"sh", "-c", "echo fail 1>&2; exit 3"})

		require.NoError(t, err)
		assert.Equal(t, 3, result.ExitCode)
		assert.Contains(t, result.Stderr, "fail")
	})

	t.Run("異常系: コマンドが空の場合にerror", func(t *testing.T) {
		t.Parallel()

		runner := signer.NewExecCommandRunner()

		_, err := runner.Run(context.Background(), nil)

		assert.Error(t, err)
	})

	t.Run("異常系: コマンドが見つからない場合にerror", func(t *testing.T) {
		t.Parallel()

		runner := signer.NewExecCommandRunner()

		_, err := runner.Run(context.Background(), []string{"mnemonic-signer-nonexistent-command-xyz"})

		assert.Error(t, err)
	})
}
