package pipeline

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/na2na-p/mnemonic/internal/converter"
)

// TestBuildPipeline_FinalizeConvertedTree_MidiFailurePrecedesScriptRewrite は
// CONVERTフェーズの後処理において、MIDI変換がスクリプト調整より先に実行される
// という順序不変条件を固定する。
//
// why: ScriptAdjusterは.mid/.midi参照を無条件に.oggへ書き換える。MIDI変換の
// 失敗がスクリプト調整より後に判明する順序だと、実体の無い.oggを指す参照へ
// 書き換えられたツリーが出来上がる。この並びこそがT-220のBGM無音バグの原因で
// あり、コメントだけでは順序を入れ替えても誰も気付けないためテストで固定する。
func TestBuildPipeline_FinalizeConvertedTree_MidiFailurePrecedesScriptRewrite(t *testing.T) {
	t.Parallel()

	const scriptSource = `@bgm storage="bgm/opening.mid"` + "\n"

	dir := t.TempDir()
	bgmDir := filepath.Join(dir, "bgm")
	require.NoError(t, os.MkdirAll(bgmDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(bgmDir, "opening.mid"), []byte("MThd"), 0o600))

	scriptPath := filepath.Join(dir, "first.ks")
	require.NoError(t, os.WriteFile(scriptPath, []byte(scriptSource), 0o600))

	p := newTestPipeline(t)
	runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
		"fluidsynth": {err: errors.New("not found")},
	}}
	midiConverter := converter.NewMidiConverter("", 0, "", 0, time.Second, runner)

	err := p.finalizeConvertedTree(dir, midiConverter)

	require.ErrorIs(t, err, ErrMidiConversionUnavailable)

	// MIDI変換より前にスクリプトが書き換えられていないこと。書き換えられて
	// いれば、実体の無いbgm/opening.oggを指す参照が残ったAPKが出来上がる。
	content, readErr := os.ReadFile(scriptPath) //nolint:gosec // テストが直前に書き込んだ一時ファイルの読み戻し
	require.NoError(t, readErr)
	assert.Equal(t, scriptSource, string(content))
	assert.NotContains(t, string(content), ".ogg")
}

// TestBuildPipeline_NewMidiConverter はConfig.SoundfontPathがMIDI変換器へ
// 引き渡されることを検証する（--soundfontフラグの経路の終端）。
func TestBuildPipeline_NewMidiConverter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		soundfontPath string
		want          string
	}{
		{
			name:          "正常系: 明示指定したサウンドフォントがそのまま使われる",
			soundfontPath: "/tmp/custom/MyFont.sf2",
			want:          "/tmp/custom/MyFont.sf2",
		},
		{
			name:          "正常系: 未指定なら既定の探索結果が使われる",
			soundfontPath: "",
			want:          converter.GetDefaultSoundfontPath(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := NewConfig("game.exe", "game.apk")
			config.SoundfontPath = tt.soundfontPath
			p := NewBuildPipeline(config)

			assert.Equal(t, tt.want, p.newMidiConverter().SoundfontPath())
		})
	}
}
