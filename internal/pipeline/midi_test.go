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

// TestBuildPipeline_ConvertMidiFiles はconvertMidiFilesUsingの仕様を検証する
// （converter.MidiConverterのCommandRunner注入口を使い、実プロセスの
// fluidsynth/ffmpegに触れずに検証する）。
//
// MIDIが実在するのに変換できない場合はビルドを失敗させ、MIDIを含まない
// ゲームはFluidSynth無しでも成功する、という2点が本テストの主眼。
func TestBuildPipeline_ConvertMidiFiles(t *testing.T) {
	t.Parallel()

	t.Run("異常系: MIDIが存在しfluidsynthが利用不可ならセンチネルエラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		midiFile := filepath.Join(dir, "bgm.mid")
		require.NoError(t, os.WriteFile(midiFile, []byte("MThd"), 0o600))

		runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
			"fluidsynth": {err: errors.New("not found")},
		}}
		midiConverter := converter.NewMidiConverter("", 0, "", 0, time.Second, runner)

		err := convertMidiFilesUsing(dir, midiConverter)

		require.ErrorIs(t, err, ErrMidiConversionUnavailable)
		assert.Contains(t, err.Error(), "fluidsynth")
		assert.Contains(t, err.Error(), "apt-get install fluidsynth fluid-soundfont-gm")
		assert.Contains(t, err.Error(), "brew install fluid-synth")
		assert.Contains(t, err.Error(), "--skip-video")
		assert.FileExists(t, midiFile)
		assert.NoFileExists(t, filepath.Join(dir, "bgm.ogg"))
	})

	t.Run("異常系: --soundfontで指定したサウンドフォントが実在しないならセンチネルエラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		midiFile := filepath.Join(dir, "bgm.mid")
		require.NoError(t, os.WriteFile(midiFile, []byte("MThd"), 0o600))

		missingSoundfont := filepath.Join(dir, "absent.sf2")

		runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
			"fluidsynth": {},
			"ffmpeg":     {},
		}}
		midiConverter := converter.NewMidiConverter(missingSoundfont, 0, "", 0, time.Second, runner)

		err := convertMidiFilesUsing(dir, midiConverter)

		require.ErrorIs(t, err, ErrMidiConversionUnavailable)
		assert.Contains(t, err.Error(), missingSoundfont)
		assert.Contains(t, err.Error(), "--soundfont")
		assert.FileExists(t, midiFile)
	})

	t.Run("異常系: 個別ファイルの変換失敗は全件分を集約したビルドエラーになる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// why: 2件seedするのは「最初の失敗でbreakせず全件試す」という仕様を
		// 固定するため。1件だけだとbreak/continueの差が観測できない。
		firstMidi := filepath.Join(dir, "opening.mid")
		secondMidi := filepath.Join(dir, "ending.midi")
		require.NoError(t, os.WriteFile(firstMidi, []byte("MThd"), 0o600))
		require.NoError(t, os.WriteFile(secondMidi, []byte("MThd"), 0o600))

		soundfont := filepath.Join(dir, "soundfont.sf2")
		require.NoError(t, os.WriteFile(soundfont, []byte("sf2"), 0o600))

		// fluidsynth --version（可用性確認）は成功させ、ffmpegだけを失敗させる
		// ことで、MIDI単体の変換失敗が握りつぶされないことを検証する。
		runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
			"fluidsynth": {},
			"ffmpeg":     {err: errors.New("ffmpeg exited with 1")},
		}}
		midiConverter := converter.NewMidiConverter(soundfont, 0, "", 0, time.Second, runner)

		err := convertMidiFilesUsing(dir, midiConverter)

		require.ErrorIs(t, err, ErrMidiConversionFailed)
		assert.Contains(t, err.Error(), "opening.mid")
		assert.Contains(t, err.Error(), "ending.midi")
		assert.FileExists(t, firstMidi)
		assert.FileExists(t, secondMidi)
	})

	t.Run("正常系: 変換成功時は.oggへ変換し元のMIDIファイルを削除する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		bgmDir := filepath.Join(dir, "bgm")
		require.NoError(t, os.MkdirAll(bgmDir, 0o750))
		midiFile := filepath.Join(bgmDir, "sinone.mid")
		require.NoError(t, os.WriteFile(midiFile, []byte("MThd"), 0o600))

		soundfont := filepath.Join(dir, "soundfont.sf2")
		require.NoError(t, os.WriteFile(soundfont, []byte("sf2"), 0o600))

		runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
			"fluidsynth": {},
			"ffmpeg":     {},
		}}
		midiConverter := converter.NewMidiConverter(soundfont, 0, "", 0, time.Second, runner)

		require.NoError(t, convertMidiFilesUsing(dir, midiConverter))

		assert.NoFileExists(t, midiFile)
	})

	t.Run("正常系: MIDIが無ければfluidsynth未インストールでも成功する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("readme"), 0o600))

		runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
			"fluidsynth": {err: errors.New("not found")},
		}}
		midiConverter := converter.NewMidiConverter("", 0, "", 0, time.Second, runner)

		require.NoError(t, convertMidiFilesUsing(dir, midiConverter))
	})
}
