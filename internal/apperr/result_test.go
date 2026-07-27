package apperr_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/na2na-p/mnemonic/internal/apperr"
)

func TestExitCode_Constants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code apperr.ExitCode
		want int
	}{
		{name: "正常系: ExitSuccessは0", code: apperr.ExitSuccess, want: 0},
		{name: "正常系: ExitErrorは1", code: apperr.ExitError, want: 1},
		{name: "正常系: ExitInvalidInputは2", code: apperr.ExitInvalidInput, want: 2},
		{name: "正常系: ExitDependencyErrorは3", code: apperr.ExitDependencyError, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, int(tt.code))
		})
	}
}

func TestResult_ZeroValueDefaultsToExitSuccess(t *testing.T) {
	t.Parallel()

	// Pythonの `exit_code: ExitCode = ExitCode.SUCCESS` に相当する挙動:
	// フィールドを明示しない場合はExitSuccess相当のゼロ値になる。
	result := apperr.Result{Success: true, Message: "ok"}

	assert.Equal(t, apperr.ExitSuccess, result.ExitCode)
}

func TestResult_Fields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result apperr.Result
	}{
		{
			name:   "正常系: 成功結果を保持する",
			result: apperr.Result{Success: true, Message: "done", ExitCode: apperr.ExitSuccess},
		},
		{
			name:   "異常系: 失敗結果を保持する",
			result: apperr.Result{Success: false, Message: "failed", ExitCode: apperr.ExitError},
		},
		{
			name: "境界値: DependencyErrorのexit_codeを保持する",
			result: apperr.Result{
				Success:  false,
				Message:  "missing dependency",
				ExitCode: apperr.ExitDependencyError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := apperr.Result{
				Success:  tt.result.Success,
				Message:  tt.result.Message,
				ExitCode: tt.result.ExitCode,
			}

			assert.Equal(t, tt.result, got)
		})
	}
}
