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

	err := p.finalizeConvertedTree(dir, converter.ConversionSummary{}, midiConverter)

	require.ErrorIs(t, err, ErrMidiConversionUnavailable)

	// MIDI変換より前にスクリプトが書き換えられていないこと。書き換えられて
	// いれば、実体の無いbgm/opening.oggを指す参照が残ったAPKが出来上がる。
	content, readErr := os.ReadFile(scriptPath) //nolint:gosec // テストが直前に書き込んだ一時ファイルの読み戻し
	require.NoError(t, readErr)
	assert.Equal(t, scriptSource, string(content))
	assert.NotContains(t, string(content), ".ogg")
}

// TestBuildPipeline_FinalizeConvertedTree_RemoveStaleVideoSourceFilesPrecedesNormalize は
// CONVERTフェーズの後処理において、動画の旧拡張子ファイル削除
// (removeStaleVideoSourceFiles)がファイル名正規化(normalizeCriticalFilenames、
// finalizeConvertedTreeの最後のステップ)より先に実行されるという順序不変
// 条件を固定する。MIDI変換とスクリプト調整の順序を固定する既存テスト
// （本ファイル上部）と同じくfinalizeConvertedTree自体を呼び出して検証する。
//
// why: normalizeCriticalFilenamesが先に走ると、大文字拡張子の旧ファイル
// (例: OP.WMV)が小文字にリネームされる。removeStaleVideoSourceFilesは
// summaryが記録した元のケース(.WMV)で旧ファイルパスを再構築するため、
// 既にリネーム済みだと削除に失敗し(best-effortで握りつぶされ)、旧ファイルが
// 永続的に残ってしまう。
func TestBuildPipeline_FinalizeConvertedTree_RemoveStaleVideoSourceFilesPrecedesNormalize(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skipIfCaseInsensitiveFS(t, dir)

	staleFile := filepath.Join(dir, "OP.WMV")
	convertedFile := filepath.Join(dir, "OP.mpg")
	require.NoError(t, os.WriteFile(staleFile, []byte("raw copy from copyTree"), 0o600))
	require.NoError(t, os.WriteFile(convertedFile, []byte("converted mpeg-ps"), 0o600))

	// why not: copyPolyfillFiles(finalizeConvertedTreeの一部)は
	// system/font.ttfが無い場合に実ネットワークへフォントダウンロードを試みる
	// (copyFontFile参照)。あらかじめsystem/font.ttfを用意しその既存ファイル
	// ガードを通すことで、finalizeConvertedTreeを実ネットワークに触れず最後
	// まで実行できるようにする（本ファイル冒頭のalwaysFailRoundTripper/
	// offlineFontFetcherが実ネットワークを避けているのと同じ方針）。
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "system"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "system", "font.ttf"), []byte("stub font"), 0o600))

	summary := converter.ConversionSummary{Results: []converter.ConversionResult{
		{
			SourcePath: filepath.Join(dir, "extract", "OP.WMV"),
			DestPath:   convertedFile,
			Status:     converter.StatusSuccess,
		},
	}}

	p := newTestPipeline(t)
	midiConverter := converter.NewMidiConverter("", 0, "", 0, time.Second, fakeCommandRunner{})

	require.NoError(t, p.finalizeConvertedTree(dir, summary, midiConverter))

	// removeStaleVideoSourceFilesがnormalizeCriticalFilenamesより後に走ると、
	// 大文字のOP.WMVは既にnormalizeCriticalFilenamesによって小文字の
	// "op.wmv"へリネームされている。summaryが記録した元のケース(.WMV)で
	// 削除を試みても"OP.WMV"はもう存在せず削除に失敗し(best-effortで
	// 握りつぶされ)、リネーム後の"op.wmv"が残ってしまう。両方の表記を
	// 確認することで、順序が入れ替わった場合にこのテストが実際に落ちる
	// ことを保証する。
	assert.NoFileExists(t, staleFile)
	assert.NoFileExists(t, filepath.Join(dir, "op.wmv"))
	assert.FileExists(t, filepath.Join(dir, "op.mpg"))
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
