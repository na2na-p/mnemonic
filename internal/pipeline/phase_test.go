package pipeline_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/na2na-p/mnemonic/internal/pipeline"
)

func TestPhase_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		phase pipeline.Phase
		want  string
	}{
		{name: "正常系: analyzeフェーズの文字列表現", phase: pipeline.PhaseAnalyze, want: "analyze"},
		{name: "正常系: extractフェーズの文字列表現", phase: pipeline.PhaseExtract, want: "extract"},
		{name: "正常系: convertフェーズの文字列表現", phase: pipeline.PhaseConvert, want: "convert"},
		{name: "正常系: buildフェーズの文字列表現", phase: pipeline.PhaseBuild, want: "build"},
		{name: "正常系: signフェーズの文字列表現", phase: pipeline.PhaseSign, want: "sign"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, string(tt.phase))
		})
	}
}

func TestAllPhases(t *testing.T) {
	t.Parallel()

	want := []pipeline.Phase{
		pipeline.PhaseAnalyze,
		pipeline.PhaseExtract,
		pipeline.PhaseConvert,
		pipeline.PhaseBuild,
		pipeline.PhaseSign,
	}

	assert.Equal(t, want, pipeline.AllPhases())
}
