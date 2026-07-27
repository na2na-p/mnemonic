package pipeline_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/na2na-p/mnemonic/internal/pipeline"
)

func TestResult_Success(t *testing.T) {
	t.Parallel()

	outputPath := "/path/to/output.apk"
	result := pipeline.Result{
		Success:    true,
		OutputPath: &outputPath,
		PhasesCompleted: []pipeline.Phase{
			pipeline.PhaseAnalyze,
			pipeline.PhaseExtract,
			pipeline.PhaseConvert,
			pipeline.PhaseBuild,
			pipeline.PhaseSign,
		},
		Statistics: map[string]any{
			"total_time_seconds": 120.5,
			"files_processed":    100,
		},
	}

	assert.True(t, result.Success)
	assert.Equal(t, &outputPath, result.OutputPath)
	assert.Empty(t, result.ErrorMessage)
	assert.Len(t, result.PhasesCompleted, 5)
	assert.Contains(t, result.PhasesCompleted, pipeline.PhaseAnalyze)
	assert.Contains(t, result.PhasesCompleted, pipeline.PhaseSign)
	assert.InEpsilon(t, 120.5, result.Statistics["total_time_seconds"], 0.001)
	assert.Equal(t, 100, result.Statistics["files_processed"])
}

func TestResult_Failure(t *testing.T) {
	t.Parallel()

	result := pipeline.Result{
		Success:         false,
		OutputPath:      nil,
		ErrorMessage:    "Parser failed: Invalid EXE format",
		PhasesCompleted: []pipeline.Phase{pipeline.PhaseAnalyze},
	}

	assert.False(t, result.Success)
	assert.Nil(t, result.OutputPath)
	assert.Contains(t, result.ErrorMessage, "Parser failed")
	assert.Equal(t, []pipeline.Phase{pipeline.PhaseAnalyze}, result.PhasesCompleted)
}

func TestResult_DefaultValues(t *testing.T) {
	t.Parallel()

	result := pipeline.Result{Success: false, OutputPath: nil}

	assert.False(t, result.Success)
	assert.Nil(t, result.OutputPath)
	assert.Empty(t, result.ErrorMessage)
	assert.Empty(t, result.PhasesCompleted)
	assert.Empty(t, result.Statistics)
}

func TestProgress_Creation(t *testing.T) {
	t.Parallel()

	progress := pipeline.Progress{
		Phase:   pipeline.PhaseConvert,
		Current: 50,
		Total:   100,
		Message: "Converting images...",
	}

	assert.Equal(t, pipeline.PhaseConvert, progress.Phase)
	assert.Equal(t, 50, progress.Current)
	assert.Equal(t, 100, progress.Total)
	assert.Equal(t, "Converting images...", progress.Message)
}

func TestResult_Statistics(t *testing.T) {
	t.Parallel()

	statistics := map[string]any{
		"total_time_seconds":   300.0,
		"analyze_time_seconds": 10.5,
		"extract_time_seconds": 30.2,
		"convert_time_seconds": 150.8,
		"build_time_seconds":   100.3,
		"sign_time_seconds":    8.2,
		"files_processed":      500,
		"images_converted":     200,
		"videos_converted":     5,
		"scripts_converted":    50,
	}

	result := pipeline.Result{
		Success:         true,
		PhasesCompleted: pipeline.AllPhases(),
		Statistics:      statistics,
	}

	assert.InEpsilon(t, 300.0, result.Statistics["total_time_seconds"], 0.001)
	assert.Equal(t, 500, result.Statistics["files_processed"])
	assert.Equal(t, 200, result.Statistics["images_converted"])
	assert.Equal(t, 5, result.Statistics["videos_converted"])
	assert.Equal(t, 50, result.Statistics["scripts_converted"])
	assert.InEpsilon(t, 10.5, result.Statistics["analyze_time_seconds"], 0.001)
	assert.InEpsilon(t, 30.2, result.Statistics["extract_time_seconds"], 0.001)
	assert.InEpsilon(t, 150.8, result.Statistics["convert_time_seconds"], 0.001)
	assert.InEpsilon(t, 100.3, result.Statistics["build_time_seconds"], 0.001)
	assert.InEpsilon(t, 8.2, result.Statistics["sign_time_seconds"], 0.001)
}

func TestProgress_DefaultMessage(t *testing.T) {
	t.Parallel()

	progress := pipeline.Progress{Phase: pipeline.PhaseBuild, Current: 0, Total: 1}

	assert.Empty(t, progress.Message)
}
