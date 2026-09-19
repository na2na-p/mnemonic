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
