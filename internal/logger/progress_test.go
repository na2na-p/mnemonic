package logger_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/na2na-p/mnemonic/internal/logger"
	"github.com/na2na-p/mnemonic/internal/pipeline"
)

func newProgress(useColor, useEmoji bool) (*logger.ConsoleProgressDisplay, *bytes.Buffer) {
	buf := &bytes.Buffer{}

	return logger.NewConsoleProgressDisplayWithWriter(useColor, useEmoji, buf), buf
}

func TestConsoleProgressDisplay_Start(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 絵文字ありでフェーズ開始を表示", func(t *testing.T) {
		t.Parallel()

		display, buf := newProgress(true, true)

		display.Start(pipeline.PhaseAnalyze, 10)

		assert.Contains(t, buf.String(), "Analyzing game structure")
	})

	t.Run("正常系: 絵文字なしでフェーズ開始を表示", func(t *testing.T) {
		t.Parallel()

		display, buf := newProgress(true, false)

		display.Start(pipeline.PhaseAnalyze, 10)

		assert.Contains(t, buf.String(), "Analyzing game structure")
	})

	tests := []struct {
		name         string
		phase        pipeline.Phase
		expectedName string
	}{
		{
			name:         "正常系: ANALYZEフェーズ名が表示される",
			phase:        pipeline.PhaseAnalyze,
			expectedName: "Analyzing game structure",
		},
		{
			name:         "正常系: EXTRACTフェーズ名が表示される",
			phase:        pipeline.PhaseExtract,
			expectedName: "Extracting assets",
		},
		{
			name:         "正常系: CONVERTフェーズ名が表示される",
			phase:        pipeline.PhaseConvert,
			expectedName: "Converting assets",
		},
		{
			name:         "正常系: BUILDフェーズ名が表示される",
			phase:        pipeline.PhaseBuild,
			expectedName: "Building APK",
		},
		{
			name:         "正常系: SIGNフェーズ名が表示される",
			phase:        pipeline.PhaseSign,
			expectedName: "Signing APK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			display, buf := newProgress(true, false)

			display.Start(tt.phase, 10)

			assert.Contains(t, buf.String(), tt.expectedName)
		})
	}
}

func TestConsoleProgressDisplay_Update(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 進捗が更新される", func(t *testing.T) {
		t.Parallel()

		display, buf := newProgress(true, true)
		display.Start(pipeline.PhaseConvert, 100)
		buf.Reset()

		display.Update(50, "processing...")

		out := buf.String()
		assert.Contains(t, out, "50%")
		assert.Contains(t, out, "processing")
	})

	t.Run("正常系: 進捗バーが表示される", func(t *testing.T) {
		t.Parallel()

		display, buf := newProgress(true, true)
		display.Start(pipeline.PhaseConvert, 100)
		buf.Reset()

		display.Update(25, "")

		// 25%なので、約10個のバーがあるはず（40 * 0.25 = 10）
		assert.Equal(t, 10, strings.Count(buf.String(), "█"))
	})

	t.Run("境界値: currentがtotalを超える場合は100%にクランプされpanicしない", func(t *testing.T) {
		t.Parallel()

		display, buf := newProgress(true, true)
		display.Start(pipeline.PhaseConvert, 100)
		buf.Reset()

		assert.NotPanics(t, func() {
			display.Update(150, "")
		})

		out := buf.String()
		assert.Contains(t, out, "100%")
		assert.Equal(t, 40, strings.Count(out, "█"))
	})

	t.Run("境界値: currentが負の場合は0%にクランプされpanicしない", func(t *testing.T) {
		t.Parallel()

		display, buf := newProgress(true, true)
		display.Start(pipeline.PhaseConvert, 100)
		buf.Reset()

		assert.NotPanics(t, func() {
			display.Update(-10, "")
		})

		out := buf.String()
		assert.Contains(t, out, "0%")
		assert.Equal(t, 0, strings.Count(out, "█"))
	})

	t.Run("境界値: totalが0の場合は何も描画せずpanicしない", func(t *testing.T) {
		t.Parallel()

		display, buf := newProgress(true, true)
		display.Start(pipeline.PhaseConvert, 0)
		buf.Reset()

		assert.NotPanics(t, func() {
			display.Update(5, "")
		})

		assert.Empty(t, buf.String())
	})
}

func TestConsoleProgressDisplay_Finish(t *testing.T) {
	t.Parallel()

	t.Run("正常系: 成功時の終了表示", func(t *testing.T) {
		t.Parallel()

		display, buf := newProgress(true, true)
		display.Start(pipeline.PhaseBuild, 10)
		buf.Reset()

		display.Finish(true, "")

		assert.Contains(t, buf.String(), "100%")
	})

	t.Run("正常系: 絵文字なしの成功時の終了表示", func(t *testing.T) {
		t.Parallel()

		display, buf := newProgress(true, false)
		display.Start(pipeline.PhaseBuild, 10)
		buf.Reset()

		display.Finish(true, "")

		assert.Contains(t, buf.String(), "done")
	})

	t.Run("異常系: 失敗時の終了表示", func(t *testing.T) {
		t.Parallel()

		display, buf := newProgress(true, true)
		display.Start(pipeline.PhaseBuild, 10)
		buf.Reset()

		display.Finish(false, "Build failed")

		assert.Contains(t, buf.String(), "Build failed")
	})

	t.Run("異常系: 絵文字なしの失敗時の終了表示", func(t *testing.T) {
		t.Parallel()

		display, buf := newProgress(true, false)
		display.Start(pipeline.PhaseBuild, 10)
		buf.Reset()

		display.Finish(false, "Build failed")

		out := buf.String()
		assert.Contains(t, out, "failed")
		assert.Contains(t, out, "Build failed")
	})
}

func TestConsoleProgressDisplay_StateTracking(t *testing.T) {
	t.Parallel()

	display, _ := newProgress(true, true)

	display.Start(pipeline.PhaseConvert, 100)
	assert.Equal(t, pipeline.PhaseConvert, display.Phase())
	assert.Equal(t, 100, display.Total())
	assert.Equal(t, 0, display.Current())

	display.Update(50, "")
	assert.Equal(t, 50, display.Current())
}
