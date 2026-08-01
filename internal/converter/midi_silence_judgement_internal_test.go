package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTrailingSilenceStartFromFrames はtrailingSilenceStartFromFramesの
// white-boxテスト。ffprobeのIO(実行・JSONパース)を伴わず、パース済みの
// frame_tags列だけから「末尾が無音で終わっているか」の判定を検証する。
func TestTrailingSilenceStartFromFrames(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		frames        []silenceProbeFrame
		expectedStart float64
		expectedFound bool
	}{
		"正常系: silence_startのみで終わる場合は末尾無音として検出する": {
			frames: []silenceProbeFrame{
				{Tags: map[string]string{"lavfi.silence_start": "1.5"}},
			},
			expectedStart: 1.5,
			expectedFound: true,
		},
		"正常系: silence_start→silence_endの後にsilence_startで終わる場合は最後のstartを返す": {
			frames: []silenceProbeFrame{
				{Tags: map[string]string{"lavfi.silence_start": "0.4"}},
				{Tags: map[string]string{"lavfi.silence_end": "0.9"}},
				{Tags: map[string]string{"lavfi.silence_start": "2.0"}},
			},
			expectedStart: 2.0,
			expectedFound: true,
		},
		"異常系: silence_endで終わる場合は末尾無音とみなさない": {
			frames: []silenceProbeFrame{
				{Tags: map[string]string{"lavfi.silence_start": "0.4"}},
				{Tags: map[string]string{"lavfi.silence_end": "0.9"}},
			},
			expectedStart: 0,
			expectedFound: false,
		},
		"異常系: フレームが空の場合は末尾無音とみなさない": {
			frames:        nil,
			expectedStart: 0,
			expectedFound: false,
		},
		"異常系: silence_startの値が数値でない場合は無視される": {
			frames: []silenceProbeFrame{
				{Tags: map[string]string{"lavfi.silence_start": "not-a-number"}},
			},
			expectedStart: 0,
			expectedFound: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			start, found := trailingSilenceStartFromFrames(tc.frames)

			assert.Equal(t, tc.expectedFound, found)
			assert.InDelta(t, tc.expectedStart, start, 1e-9)
		})
	}
}
