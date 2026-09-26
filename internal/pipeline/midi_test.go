package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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

	t.Run("正常系: 変換成功時は.oggへ変換し元のMIDIファイルを削除する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		bgmDir := filepath.Join(dir, "bgm")
		require.NoError(t, os.MkdirAll(bgmDir, 0o750))
		midiFile := filepath.Join(bgmDir, "sinone.mid")
		require.NoError(t, os.WriteFile(midiFile, []byte("MThd"), 0o600))
		longExtMidiFile := filepath.Join(bgmDir, "ending.midi")
		require.NoError(t, os.WriteFile(longExtMidiFile, []byte("MThd"), 0o600))

		soundfont := filepath.Join(dir, "soundfont.sf2")
		require.NoError(t, os.WriteFile(soundfont, []byte("sf2"), 0o600))

		runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
			"fluidsynth": {},
			"ffmpeg":     {},
		}}
		midiConverter := converter.NewMidiConverter(soundfont, 0, "", 0, time.Second, runner)

		require.NoError(t, convertMidiFilesUsing(dir, midiConverter))

		assert.NoFileExists(t, midiFile)
		assert.NoFileExists(t, longExtMidiFile)
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

// renderCountingRunner はfakeCommandRunnerの応答を返しつつ、fluidsynthの
// レンダリング呼び出し回数を数える。
//
// why not: コマンド名だけで数えない。fluidsynthは可用性確認(--version)でも
// 呼ばれるため、レンダリング専用の引数(-F)を持つ呼び出しに限って数える。
// ConversionManagerのワーカーから並行に呼ばれるためatomicで数える。
type renderCountingRunner struct {
	fakeCommandRunner
	renders atomic.Int32
}

func (r *renderCountingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "fluidsynth" && slices.Contains(args, "-F") {
		r.renders.Add(1)
	}

	return r.fakeCommandRunner.Run(ctx, name, args...)
}

// sleepRecorder はリトライ待機を実際には待たずに記録する。
type sleepRecorder struct {
	mu        sync.Mutex
	durations []time.Duration
}

func (r *sleepRecorder) sleep(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.durations = append(r.durations, d)
}

// TestConvertMidiFileListWith はMIDIファイル群をConversionManager経由で変換する
// 際のリトライと失敗集約の仕様を検証する。
func TestConvertMidiFileListWith(t *testing.T) {
	t.Parallel()

	const transientMessage = "最大リトライ回数超過: FFmpeg変換に失敗しました: ffmpeg exited with 1"

	tests := []struct {
		name string
		// sources はconvertMidiFileListWithへ渡す順に並べたdir相対パス。
		sources []string
		// vanished は列挙後・変換前に削除するdir相対パス。
		vanished    []string
		ffmpegErr   error
		wantRenders int32
		wantSleeps  []time.Duration
		// wantFailed はエラーメッセージに現れるべきdir相対パスを、現れる順に並べたもの。
		wantFailed []string
		// wantMessage は失敗した各ファイルのパスに続く「: 」の直後に現れるべき文言の先頭部分。
		wantMessage string
		wantRemoved []string
		wantKept    []string
	}{
		{
			name:        "正常系: 変換に成功したMIDIファイルは削除し待機しない",
			sources:     []string{"bgm/sinone.mid"},
			wantRenders: 1,
			wantRemoved: []string{"bgm/sinone.mid"},
		},
		{
			name:        "異常系: 一時的な失敗は3回試行したうえで最大リトライ回数超過として1件だけ報告する",
			sources:     []string{"opening.mid"},
			ffmpegErr:   errors.New("ffmpeg exited with 1"),
			wantRenders: 3,
			wantSleeps:  []time.Duration{time.Second, 2 * time.Second},
			wantFailed:  []string{"opening.mid"},
			wantMessage: transientMessage,
			wantKept:    []string{"opening.mid"},
		},
		{
			name:        "異常系: 列挙後に消えた変換元は恒久的な失敗として再試行せず接頭辞なしで報告する",
			sources:     []string{"opening.mid"},
			vanished:    []string{"opening.mid"},
			wantRenders: 0,
			wantFailed:  []string{"opening.mid"},
			wantMessage: "再試行しても解消しない変換失敗です: 変換元ファイルが見つかりません: ",
		},
		{
			// why: 2件seedするのは「最初の失敗でbreakせず全件試す」という仕様と
			// 報告順がパス順であることを固定するため。入力順と逆のパス順を期待する
			// ことで、完了順や入力順のまま並べる実装を検出する。
			name:        "異常系: 複数の失敗は全件を試しパス順に並べて報告する",
			sources:     []string{"opening.mid", "ending.midi"},
			ffmpegErr:   errors.New("ffmpeg exited with 1"),
			wantRenders: 6,
			wantSleeps:  []time.Duration{time.Second, 2 * time.Second, time.Second, 2 * time.Second},
			wantFailed:  []string{"ending.midi", "opening.mid"},
			wantMessage: transientMessage,
			wantKept:    []string{"opening.mid", "ending.midi"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			abs := func(rel string) string { return filepath.Join(dir, filepath.FromSlash(rel)) }

			midiFiles := make([]string, 0, len(tt.sources))
			for _, rel := range tt.sources {
				path := abs(rel)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
				require.NoError(t, os.WriteFile(path, []byte("MThd"), 0o600))
				midiFiles = append(midiFiles, path)
			}
			for _, rel := range tt.vanished {
				require.NoError(t, os.Remove(abs(rel)))
			}

			soundfont := filepath.Join(dir, "soundfont.sf2")
			require.NoError(t, os.WriteFile(soundfont, []byte("sf2"), 0o600))

			runner := &renderCountingRunner{fakeCommandRunner: fakeCommandRunner{responses: map[string]fakeCommandResponse{
				"fluidsynth": {},
				"ffmpeg":     {err: tt.ffmpegErr},
			}}}
			midiConverter := converter.NewMidiConverter(soundfont, 0, "", 0, time.Second, runner)
			recorder := &sleepRecorder{}

			err := convertMidiFileListWith(midiFiles, midiConverter, recorder.sleep)

			assert.Equal(t, tt.wantRenders, runner.renders.Load())
			assert.ElementsMatch(t, tt.wantSleeps, recorder.durations)

			if len(tt.wantFailed) == 0 {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrMidiConversionFailed)
				msg := err.Error()

				lastIndex := -1
				for _, rel := range tt.wantFailed {
					assert.Contains(t, msg, abs(rel)+": "+tt.wantMessage)
					entry := abs(rel) + ": "
					assert.Equal(t, 1, strings.Count(msg, entry), "失敗は1ファイルにつき1件だけ報告する: %s", rel)
					index := strings.Index(msg, entry)
					assert.Greater(t, index, lastIndex, "失敗はパス順に並ぶ: %s", rel)
					lastIndex = index
				}
			}

			for _, rel := range tt.wantRemoved {
				assert.NoFileExists(t, abs(rel))
			}
			for _, rel := range tt.wantKept {
				assert.FileExists(t, abs(rel))
			}
		})
	}
}
