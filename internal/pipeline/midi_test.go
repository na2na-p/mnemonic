package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

		err := convertMidiFilesUsing(dir, midiConverter, nopLogger{})

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

		err := convertMidiFilesUsing(dir, midiConverter, nopLogger{})

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

		require.NoError(t, convertMidiFilesUsing(dir, midiConverter, nopLogger{}))

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

		require.NoError(t, convertMidiFilesUsing(dir, midiConverter, nopLogger{}))
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
		// wantCollision は出力先の重複として報告されるべき「出力先 ← 変換元, …」を
		// dir相対パスで並べたもの（先頭が出力先、以降が変換元のパス順）。
		wantCollision []string
		wantRemoved   []string
		wantKept      []string
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
		{
			name:          "異常系: 同じ.oggへ変換される.midと.midiはいずれも変換せず重複した変換元を全て報告する",
			sources:       []string{"bgm/foo.midi", "bgm/foo.mid"},
			wantRenders:   0,
			wantFailed:    []string{"bgm/foo.mid", "bgm/foo.midi"},
			wantMessage:   "再試行しても解消しない変換失敗です: 出力先が重複しています: ",
			wantCollision: []string{"bgm/foo.ogg", "bgm/foo.mid", "bgm/foo.midi"},
			wantKept:      []string{"bgm/foo.mid", "bgm/foo.midi"},
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

			err := convertMidiFileListWith(midiFiles, midiConverter, recorder.sleep, nopLogger{})

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

				if len(tt.wantCollision) > 0 {
					collided := make([]string, 0, len(tt.wantCollision)-1)
					for _, rel := range tt.wantCollision[1:] {
						collided = append(collided, abs(rel))
					}
					assert.Contains(t, msg, abs(tt.wantCollision[0])+" ← "+strings.Join(collided, ", "))
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

// TestConvertMidiFileListWith_RemovalFailure は変換に成功したMIDIファイルを
// 削除できなかった場合の扱いを検証する。
func TestConvertMidiFileListWith_RemovalFailure(t *testing.T) {
	t.Parallel()

	// why not: rootは書き込み権限の無いディレクトリからもファイルを削除できるため、
	// 削除失敗を再現できない。
	if os.Geteuid() == 0 {
		t.Skip("rootでは読み取り専用ディレクトリからの削除失敗を再現できないためスキップ")
	}

	dir := t.TempDir()
	bgmDir := filepath.Join(dir, "bgm")
	require.NoError(t, os.MkdirAll(bgmDir, 0o750))
	midiFile := filepath.Join(bgmDir, "sinone.mid")
	require.NoError(t, os.WriteFile(midiFile, []byte("MThd"), 0o600))
	soundfont := filepath.Join(dir, "soundfont.sf2")
	require.NoError(t, os.WriteFile(soundfont, []byte("sf2"), 0o600))

	require.NoError(t, os.Chmod(bgmDir, 0o500)) //nolint:gosec // 削除失敗を再現するため意図的に書き込み権限を外す
	// t.TempDirのRemoveAllより先に権限を戻すため、TempDirの後に登録する（Cleanupは逆順に走る）。
	t.Cleanup(func() { _ = os.Chmod(bgmDir, 0o750) }) //nolint:gosec // テスト用ディレクトリの権限を元に戻す

	runner := fakeCommandRunner{responses: map[string]fakeCommandResponse{
		"fluidsynth": {},
		"ffmpeg":     {},
	}}
	midiConverter := converter.NewMidiConverter(soundfont, 0, "", 0, time.Second, runner)
	logger := &recordingLogger{}

	err := convertMidiFileListWith([]string{midiFile}, midiConverter, (&sleepRecorder{}).sleep, logger)

	require.NoError(t, err, "削除失敗はビルドエラーに昇格させない")
	assert.FileExists(t, midiFile)
	warnings := logger.messages("WARNING")
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], midiFile)
	assert.Equal(t, 1, strings.Count(warnings[0], midiFile), "パスは1回だけ示す")
}

func TestMidiWorkerCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cpuCount int
		want     int
	}{
		{name: "境界値: CPU数が0以下でも1ワーカーは確保する", cpuCount: 0, want: 1},
		{name: "正常系: 1CPUなら1ワーカー", cpuCount: 1, want: 1},
		{name: "正常系: 2CPUなら2ワーカー", cpuCount: 2, want: 2},
		{name: "正常系: 8CPUでも2ワーカーに抑える", cpuCount: 8, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, midiWorkerCount(tt.cpuCount))
		})
	}
}

// concurrencyRecordingRunner はfakeCommandRunnerの応答を返しつつ、同時に
// 実行中のfluidsynthレンダリング数の最大値を記録する。
//
// why not: レンダリングを即座に返さない。一瞬で終わる呼び出しは並列ワーカー
// 同士が重なり合わず、上限を超える並列化をしていても最大同時実行数が1に
// 見えてしまうため、holdの間だけ呼び出しを留めて重なりを観測できるようにする。
type concurrencyRecordingRunner struct {
	fakeCommandRunner
	hold time.Duration

	mu     sync.Mutex
	active int
	peak   int
}

func (r *concurrencyRecordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "fluidsynth" && slices.Contains(args, "-F") {
		r.mu.Lock()
		r.active++
		r.peak = max(r.peak, r.active)
		r.mu.Unlock()

		<-time.After(r.hold)

		r.mu.Lock()
		r.active--
		r.mu.Unlock()
	}

	return r.fakeCommandRunner.Run(ctx, name, args...)
}

func (r *concurrencyRecordingRunner) peakConcurrency() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.peak
}

func TestConvertMidiFileListWith_Concurrency(t *testing.T) {
	t.Parallel()

	if runtime.NumCPU() <= 2 {
		t.Skip("CPU数が2以下ではCPU数由来のワーカー数が上限を超えないため、上限の検証にならない")
	}

	t.Run("正常系: CPU数が多くてもfluidsynthの同時レンダリングは2件までに抑える", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		midiFiles := make([]string, 0, 4)
		for _, name := range []string{"a.mid", "b.mid", "c.mid", "d.mid"} {
			path := filepath.Join(dir, name)
			require.NoError(t, os.WriteFile(path, []byte("MThd"), 0o600))
			midiFiles = append(midiFiles, path)
		}

		soundfont := filepath.Join(dir, "soundfont.sf2")
		require.NoError(t, os.WriteFile(soundfont, []byte("sf2"), 0o600))

		runner := &concurrencyRecordingRunner{
			fakeCommandRunner: fakeCommandRunner{responses: map[string]fakeCommandResponse{
				"fluidsynth": {},
				"ffmpeg":     {},
			}},
			hold: 50 * time.Millisecond,
		}
		midiConverter := converter.NewMidiConverter(soundfont, 0, "", 0, time.Second, runner)

		require.NoError(t, convertMidiFileListWith(midiFiles, midiConverter, (&sleepRecorder{}).sleep, nopLogger{}))

		assert.LessOrEqual(t, runner.peakConcurrency(), 2)
	})
}
