package converter_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/na2na-p/mnemonic/internal/converter"
)

// TestGetDefaultSoundfontPath はGetDefaultSoundfontPathの探索優先順位をテストする。
//
// converter.MuseScoreSoundfontPath / FluidR3SoundfontPath はパッケージ変数を
// 書き換えるため、このテストは他のテストとの並行実行を避けてt.Parallel()を
// 呼ばない（goimportsやgo test -race -shuffle=onでの競合を避けるため）。
func TestGetDefaultSoundfontPath(t *testing.T) {
	t.Run("正常系: MuseScore Generalが存在する場合はそれを返す", func(t *testing.T) {
		dir := t.TempDir()
		museScorePath := filepath.Join(dir, "MuseScore_General.sf3")
		writeFile(t, museScorePath, []byte("soundfont"))

		origMuseScore := converter.MuseScoreSoundfontPath
		defer func() { converter.MuseScoreSoundfontPath = origMuseScore }()
		converter.MuseScoreSoundfontPath = museScorePath

		assert.Equal(t, museScorePath, converter.GetDefaultSoundfontPath())
	})

	t.Run("正常系: MuseScore Generalが存在しない場合はFluidR3を返す", func(t *testing.T) {
		dir := t.TempDir()
		nonExistent := filepath.Join(dir, "non_existent.sf3")
		fluidR3Path := filepath.Join(dir, "FluidR3_GM.sf2")

		origMuseScore := converter.MuseScoreSoundfontPath
		origFluidR3 := converter.FluidR3SoundfontPath
		defer func() {
			converter.MuseScoreSoundfontPath = origMuseScore
			converter.FluidR3SoundfontPath = origFluidR3
		}()
		converter.MuseScoreSoundfontPath = nonExistent
		converter.FluidR3SoundfontPath = fluidR3Path

		assert.Equal(t, fluidR3Path, converter.GetDefaultSoundfontPath())
	})
}

// TestNewMidiConverter_DefaultValues はNewMidiConverterのデフォルト値解決を
// テストする。
//
// why not: soundfontPath=""はGetDefaultSoundfontPath()を経由してパッケージ変数
// MuseScoreSoundfontPath/FluidR3SoundfontPathを読む。TestGetDefaultSoundfontPath
// がこれらを書き換えるため、このテストはt.Parallel()を呼ばず競合を避ける
// （tparallelは「サブテストがParallelなら親も」を要求するため、混在させず
// カスタム値のテスト(TestNewMidiConverter_CustomValues)を関数ごと分離した）。
func TestNewMidiConverter_DefaultValues(t *testing.T) {
	c := converter.NewMidiConverter("", 0, "", 0, 0, nil)

	assert.Equal(t, converter.GetDefaultSoundfontPath(), c.SoundfontPath())
	assert.Equal(t, 44100, c.SampleRate())
	assert.Equal(t, "libvorbis", c.AudioCodec())
	assert.Equal(t, 4, c.AudioQuality())
	assert.Equal(t, 300*time.Second, c.Timeout())
}

// TestNewMidiConverter_CustomValues はNewMidiConverterへのカスタム値指定を
// テストする。
func TestNewMidiConverter_CustomValues(t *testing.T) {
	t.Parallel()

	customSF := filepath.Join(t.TempDir(), "custom.sf2")
	c := converter.NewMidiConverter(customSF, 48000, "libopus", 6, 600*time.Second, nil)

	assert.Equal(t, customSF, c.SoundfontPath())
	assert.Equal(t, 48000, c.SampleRate())
	assert.Equal(t, "libopus", c.AudioCodec())
	assert.Equal(t, 6, c.AudioQuality())
	assert.Equal(t, 600*time.Second, c.Timeout())
}

// TestMidiConverter_SupportedExtensions はSupportedExtensionsをテストする。
func TestMidiConverter_SupportedExtensions(t *testing.T) {
	t.Parallel()

	c := converter.NewMidiConverter(filepath.Join(t.TempDir(), "sf.sf2"), 0, "", 0, 0, nil)
	extensions := c.SupportedExtensions()

	assert.Contains(t, extensions, ".mid")
	assert.Contains(t, extensions, ".midi")
	for _, ext := range extensions {
		assert.True(t, filepathHasDotPrefix(ext))
	}
}

// TestMidiConverter_GetOutputExtension はGetOutputExtensionをテストする。
//
// why not: Converterインターフェースは、Managerが出力先の拡張子をリネーム
// する際にGetOutputExtensionを使う設計になっている。MIDI変換の出力実体は
// 常にOGGであるため、".ogg"を返すのがConverterインターフェースの契約として
// 正しい。
func TestMidiConverter_GetOutputExtension(t *testing.T) {
	t.Parallel()

	c := converter.NewMidiConverter(filepath.Join(t.TempDir(), "sf.sf2"), 0, "", 0, 0, nil)
	assert.Equal(t, ".ogg", c.GetOutputExtension("bgm/title.mid"))
}

// TestMidiConverter_CanConvert はCanConvertをテストする。
func TestMidiConverter_CanConvert(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		filename string
		expected bool
	}{
		"正常系: MIDファイル":      {"music.mid", true},
		"正常系: MIDIファイル":     {"music.midi", true},
		"正常系: 大文字MID拡張子":    {"music.MID", true},
		"正常系: 大文字MIDI拡張子":   {"music.MIDI", true},
		"異常系: MP3ファイル（非対応）": {"music.mp3", false},
		"異常系: OGGファイル":      {"music.ogg", false},
		"異常系: 画像ファイル":       {"image.png", false},
		"異常系: テキストファイル":     {"document.txt", false},
	}

	c := converter.NewMidiConverter(filepath.Join(t.TempDir(), "sf.sf2"), 0, "", 0, 0, nil)
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, c.CanConvert(filepath.Join(t.TempDir(), tc.filename)))
		})
	}
}

// TestMidiConverter_IsFluidsynthAvailable はIsFluidsynthAvailableをテストする。
func TestMidiConverter_IsFluidsynthAvailable(t *testing.T) {
	t.Parallel()

	t.Run("正常系: FluidSynthが利用可能な場合trueを返す", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().Run(gomock.Any(), "fluidsynth", "--version").Return([]byte("FluidSynth 2.3.0"), nil)

		c := converter.NewMidiConverter(filepath.Join(t.TempDir(), "sf.sf2"), 0, "", 0, 0, runner)
		assert.True(t, c.IsFluidsynthAvailable())
	})

	t.Run("異常系: FluidSynthが見つからない場合falseを返す", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().Run(gomock.Any(), "fluidsynth", "--version").Return(nil, errors.New("executable file not found"))

		c := converter.NewMidiConverter(filepath.Join(t.TempDir(), "sf.sf2"), 0, "", 0, 0, runner)
		assert.False(t, c.IsFluidsynthAvailable())
	})

	t.Run("異常系: FluidSynthがエラーを返す場合falseを返す", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().Run(gomock.Any(), "fluidsynth", "--version").Return(nil, errors.New("exit status 1"))

		c := converter.NewMidiConverter(filepath.Join(t.TempDir(), "sf.sf2"), 0, "", 0, 0, runner)
		assert.False(t, c.IsFluidsynthAvailable())
	})
}

// TestMidiConverter_Convert はConvertをテストする。
func TestMidiConverter_Convert(t *testing.T) {
	t.Parallel()

	t.Run("異常系: 変換元ファイルが存在しない場合は再試行不要なエラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "non_existent.mid")
		dest := filepath.Join(dir, "output.ogg")

		c := converter.NewMidiConverter(filepath.Join(dir, "sf.sf2"), 0, "", 0, 0, nil)
		result, err := c.Convert(source, dest)

		require.ErrorIs(t, err, converter.ErrSourceNotFound)
		require.ErrorIs(t, err, converter.ErrPermanentFailure)
		assert.Contains(t, err.Error(), "変換元ファイルが見つかりません: "+source)
		assert.Equal(t, source, result.SourcePath)
	})

	t.Run("異常系: サウンドフォントが存在しない場合は再試行不要なエラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "output.ogg")
		soundfont := filepath.Join(dir, "non_existent.sf2")

		c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, nil)
		_, err := c.Convert(source, dest)

		require.ErrorIs(t, err, converter.ErrSoundfontNotFound)
		require.ErrorIs(t, err, converter.ErrPermanentFailure)
		assert.Contains(t, err.Error(), "サウンドフォントが見つかりません: "+soundfont)
	})

	soundfontStatFailures := []struct {
		name          string
		skipAsRoot    bool
		setup         func(t *testing.T) string
		wantOSErr     error
		wantPermanent bool
	}{
		{
			name:       "異常系: 探索権限の無いディレクトリ配下のサウンドフォントは見つからないとは報告せず再試行不要なエラー",
			skipAsRoot: true,
			setup: func(t *testing.T) string {
				t.Helper()

				return writeFileInLockedDir(t, "test.sf2", []byte("soundfont data"))
			},
			wantOSErr:     fs.ErrPermission,
			wantPermanent: true,
		},
		{
			name: "異常系: 親がファイルのサウンドフォントは再試行対象のエラー",
			setup: func(t *testing.T) string {
				t.Helper()

				parent := filepath.Join(t.TempDir(), "parent.sf2")
				writeFile(t, parent, []byte("not a directory"))

				return filepath.Join(parent, "test.sf2")
			},
			wantOSErr:     syscall.ENOTDIR,
			wantPermanent: false,
		},
	}

	for _, tt := range soundfontStatFailures {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.skipAsRoot && os.Geteuid() == 0 {
				t.Skip("rootはパーミッションに関係なく読み込めるため再現できない")
			}

			dir := t.TempDir()
			source := filepath.Join(dir, "input.mid")
			writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
			soundfont := tt.setup(t)

			c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, nil)
			_, err := c.Convert(source, filepath.Join(dir, "output.ogg"))

			require.ErrorIs(t, err, converter.ErrSoundfontUnreadable)
			require.ErrorIs(t, err, tt.wantOSErr)
			require.NotErrorIs(t, err, converter.ErrSoundfontNotFound)
			assert.NotContains(t, err.Error(), "見つかりません")
			assert.Contains(t, err.Error(), soundfont)
			if tt.wantPermanent {
				require.ErrorIs(t, err, converter.ErrPermanentFailure)
			} else {
				require.NotErrorIs(t, err, converter.ErrPermanentFailure)
			}
		})
	}

	t.Run("異常系: 出力先ディレクトリを作成できない場合は再試行対象のエラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		soundfont := filepath.Join(dir, "test.sf2")
		writeFile(t, soundfont, []byte("soundfont data"))
		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		blocker := filepath.Join(dir, "blocker")
		writeFile(t, blocker, []byte("not a directory"))

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)

		c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, runner)
		_, err := c.Convert(source, filepath.Join(blocker, "output.ogg"))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "MIDI変換に失敗しました: ")
		require.NotErrorIs(t, err, converter.ErrPermanentFailure)
	})

	t.Run("正常系: MIDI変換が成功する(末尾無音なし)", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		soundfont := filepath.Join(dir, "test.sf2")
		writeFile(t, soundfont, []byte("soundfont data"))

		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "output.ogg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		expectLegacyFluidsynthVersion(runner)
		runner.EXPECT().
			Run(gomock.Any(), "fluidsynth", "-ni", "-g", "1.0", "-r", "44100", "-F", gomock.Any(), soundfont, source).
			Return(nil, nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-v", "error", "-f", "lavfi", "-i", gomock.Any(), "-show_entries", "frame_tags=lavfi.silence_start,lavfi.silence_end", "-of", "json").
			Return([]byte(`{"frames":[{}]}`), nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", "-y", "-i", gomock.Any(), "-c:a", "libvorbis", "-q:a", "4", dest).
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				writeFile(t, dest, []byte("OggS"))

				return nil, nil
			})

		c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.Equal(t, source, result.SourcePath)
		assert.Equal(t, dest, result.DestPath)
		assert.Positive(t, result.BytesBefore)
		assert.Positive(t, result.BytesAfter)
	})

	t.Run("正常系: 末尾無音を検出した場合ffmpegに-tでトリム指定を渡す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		soundfont := filepath.Join(dir, "test.sf2")
		writeFile(t, soundfont, []byte("soundfont data"))

		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "output.ogg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		expectLegacyFluidsynthVersion(runner)
		runner.EXPECT().
			Run(gomock.Any(), "fluidsynth", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-v", "error", "-f", "lavfi", "-i", gomock.Any(), "-show_entries", "frame_tags=lavfi.silence_start,lavfi.silence_end", "-of", "json").
			Return([]byte(`{"frames":[{"tags":{"lavfi.silence_start":"10.5"}}]}`), nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", "-y", "-i", gomock.Any(), "-c:a", "libvorbis", "-q:a", "4", "-t", "10.800", dest).
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				writeFile(t, dest, []byte("OggS"))

				return nil, nil
			})

		c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
	})

	t.Run("正常系: 末尾が無音で終わらない場合トリムしない", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		soundfont := filepath.Join(dir, "test.sf2")
		writeFile(t, soundfont, []byte("soundfont data"))

		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "output.ogg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		expectLegacyFluidsynthVersion(runner)
		runner.EXPECT().
			Run(gomock.Any(), "fluidsynth", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-v", "error", "-f", "lavfi", "-i", gomock.Any(), "-show_entries", "frame_tags=lavfi.silence_start,lavfi.silence_end", "-of", "json").
			Return([]byte(`{"frames":[{"tags":{"lavfi.silence_start":"1.0"}},{"tags":{"lavfi.silence_end":"1.5"}}]}`), nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", "-y", "-i", gomock.Any(), "-c:a", "libvorbis", "-q:a", "4", dest).
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				writeFile(t, dest, []byte("OggS"))

				return nil, nil
			})

		c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
	})

	t.Run("正常系: 無音検出(ffprobe)が失敗してもトリムをスキップして変換は成功する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		soundfont := filepath.Join(dir, "test.sf2")
		writeFile(t, soundfont, []byte("soundfont data"))

		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "output.ogg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		expectLegacyFluidsynthVersion(runner)
		runner.EXPECT().
			Run(gomock.Any(), "fluidsynth", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-v", "error", "-f", "lavfi", "-i", gomock.Any(), "-show_entries", "frame_tags=lavfi.silence_start,lavfi.silence_end", "-of", "json").
			Return(nil, errors.New("ffprobe error"))
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", "-y", "-i", gomock.Any(), "-c:a", "libvorbis", "-q:a", "4", dest).
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				writeFile(t, dest, []byte("OggS"))

				return nil, nil
			})

		c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
	})

	t.Run("異常系: FluidSynthがエラーを返す場合は再試行対象のエラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		soundfont := filepath.Join(dir, "test.sf2")
		writeFile(t, soundfont, []byte("soundfont data"))

		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "output.ogg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		expectLegacyFluidsynthVersion(runner)
		runner.EXPECT().
			Run(gomock.Any(), "fluidsynth", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("FluidSynth error"))

		c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, runner)
		result, err := c.Convert(source, dest)

		require.ErrorIs(t, err, converter.ErrFluidsynthFailed)
		require.NotErrorIs(t, err, converter.ErrPermanentFailure)
		assert.Equal(t, "FluidSynth変換に失敗しました: FluidSynth error", err.Error())
		assert.Equal(t, source, result.SourcePath)
	})

	t.Run("異常系: FFmpegがエラーを返す場合は再試行対象のエラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		soundfont := filepath.Join(dir, "test.sf2")
		writeFile(t, soundfont, []byte("soundfont data"))

		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "output.ogg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		expectLegacyFluidsynthVersion(runner)
		runner.EXPECT().
			Run(gomock.Any(), "fluidsynth", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"frames":[{}]}`), nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("FFmpeg error"))

		c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, runner)
		result, err := c.Convert(source, dest)

		require.ErrorIs(t, err, converter.ErrMidiFFmpegFailed)
		require.NotErrorIs(t, err, converter.ErrPermanentFailure)
		assert.Equal(t, "FFmpeg変換に失敗しました: FFmpeg error", err.Error())
		assert.Equal(t, source, result.SourcePath)
	})

	t.Run("正常系: 出力先の親ディレクトリが存在しない場合作成する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		soundfont := filepath.Join(dir, "test.sf2")
		writeFile(t, soundfont, []byte("soundfont data"))

		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "subdir", "nested", "output.ogg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		expectLegacyFluidsynthVersion(runner)
		runner.EXPECT().
			Run(gomock.Any(), "fluidsynth", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"frames":[{}]}`), nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				writeFile(t, dest, []byte("OggS"))

				return nil, nil
			})

		c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.DirExists(t, filepath.Dir(dest))
	})

	t.Run("正常系: カスタムサンプルレート・コーデック・品質が実効的なコマンドに使われる", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		soundfont := filepath.Join(dir, "test.sf2")
		writeFile(t, soundfont, []byte("soundfont data"))

		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "output.ogg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		expectLegacyFluidsynthVersion(runner)
		runner.EXPECT().
			Run(gomock.Any(), "fluidsynth", "-ni", "-g", "1.0", "-r", "48000", "-F", gomock.Any(), soundfont, source).
			Return(nil, nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]byte(`{"frames":[{}]}`), nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", "-y", "-i", gomock.Any(), "-c:a", "libopus", "-q:a", "6", dest).
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				writeFile(t, dest, []byte("OggS"))

				return nil, nil
			})

		c := converter.NewMidiConverter(soundfont, 48000, "libopus", 6, 0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
	})
}

// legacyFluidsynthVersion は動的サンプル読み込みの対象外（2.4.4未満）となる
// FluidSynthの--version出力。レンダリング引数を検証するテストが、
// 動的サンプル読み込みの指定を含まない引数列を期待できるようにする。
const legacyFluidsynthVersion = "FluidSynth runtime version 2.3.7\n"

// expectLegacyFluidsynthVersion はMidiConverterが行うFluidSynthのバージョン確認
// （fluidsynth --version）にlegacyFluidsynthVersionを返す期待を登録する。
func expectLegacyFluidsynthVersion(runner *MockCommandRunner) {
	runner.EXPECT().Run(gomock.Any(), "fluidsynth", "--version").Return([]byte(legacyFluidsynthVersion), nil)
}

// expectRenderedOgg はfluidsynth以降の無音検出（ffprobe）とOGG変換（ffmpeg）の
// 呼び出しを、末尾無音なし・出力ファイル作成成功として受け付ける期待を登録する。
func expectRenderedOgg(runner *MockCommandRunner, times int) {
	runner.EXPECT().
		Run(gomock.Any(), "ffprobe", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]byte(`{"frames":[{}]}`), nil).
		Times(times)
	runner.EXPECT().
		Run(gomock.Any(), "ffmpeg", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		// why not: writeFile（require）を使わない。並行変換のテストではこの関数が
		// テスト本体とは別のgoroutineで呼ばれ、そこでのFailNowは許されない。
		// 書き込みの失敗は変換エラーとして呼び出し側の検証に委ねる。
		DoAndReturn(func(_ context.Context, _ string, args ...string) ([]byte, error) {
			return nil, os.WriteFile(args[len(args)-1], []byte("OggS"), 0o600)
		}).
		Times(times)
}

// fluidsynthRenderArgs はfluidsynthのレンダリング呼び出しに期待する引数列を返す。
// dynamicSampleLoadingがtrueの場合、-Fの直前に動的サンプル読み込みの指定が入る。
func fluidsynthRenderArgs(soundfont, source string, dynamicSampleLoading bool) []any {
	args := []any{"-ni", "-g", "1.0", "-r", "44100"}
	if dynamicSampleLoading {
		args = append(args, "-o", "synth.dynamic-sample-loading=1")
	}

	return append(args, "-F", gomock.Any(), soundfont, source)
}

// TestMidiConverter_Convert_DynamicSampleLoading はFluidSynthのバージョンに応じて
// レンダリング時に動的サンプル読み込みを有効にするかを検証する。
func TestMidiConverter_Convert_DynamicSampleLoading(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		versionOutput string
		versionErr    error
		wantDynamic   bool
	}{
		{
			name: "正常系: 2.4.6なら動的サンプル読み込みを有効にする",
			versionOutput: "FluidSynth runtime version 2.4.6\n" +
				"Copyright (C) 2000-2025 Peter Hanappe and others.\n" +
				"FluidSynth executable version 2.4.6\n",
			wantDynamic: true,
		},
		{name: "正常系: 下限の2.4.4ちょうどなら有効にする", versionOutput: "2.4.4", wantDynamic: true},
		{name: "正常系: パッケージ接尾辞付きの2.4.4-1も有効にする", versionOutput: "2.4.4-1", wantDynamic: true},
		{name: "正常系: runtimeの語を含まない出力の2.5.0も有効にする", versionOutput: "FluidSynth version 2.5.0", wantDynamic: true},
		{name: "正常系: マイナー番号を数値として比較し2.10.0を有効にする", versionOutput: "FluidSynth runtime version 2.10.0", wantDynamic: true},
		{name: "正常系: メジャー番号が上がった3.0.0も有効にする", versionOutput: "FluidSynth runtime version 3.0.0", wantDynamic: true},
		{name: "正常系: 下限未満の2.4.3なら有効にしない", versionOutput: "FluidSynth runtime version 2.4.3", wantDynamic: false},
		{name: "正常系: 2.3.7なら有効にしない", versionOutput: legacyFluidsynthVersion, wantDynamic: false},
		{name: "異常系: バージョンを読み取れない出力なら有効にしない", versionOutput: "fluidsynth: unknown option", wantDynamic: false},
		{name: "異常系: 出力が空なら有効にしない", versionOutput: "", wantDynamic: false},
		{name: "異常系: バージョン確認が失敗しても変換は続行し有効にしない", versionErr: errors.New("exit status 1"), wantDynamic: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			soundfont := filepath.Join(dir, "test.sf2")
			writeFile(t, soundfont, []byte("soundfont data"))
			source := filepath.Join(dir, "input.mid")
			writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
			dest := filepath.Join(dir, "output.ogg")

			ctrl := gomock.NewController(t)
			runner := NewMockCommandRunner(ctrl)
			runner.EXPECT().Run(gomock.Any(), "fluidsynth", "--version").Return([]byte(tt.versionOutput), tt.versionErr)
			runner.EXPECT().
				Run(gomock.Any(), "fluidsynth", fluidsynthRenderArgs(soundfont, source, tt.wantDynamic)...).
				Return(nil, nil)
			expectRenderedOgg(runner, 1)

			c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, runner)
			result, err := c.Convert(source, dest)

			require.NoError(t, err)
			assert.Equal(t, converter.StatusSuccess, result.Status)
		})
	}
}

// TestMidiConverter_Convert_ProbesFluidsynthVersionOnce は同一のMidiConverterで
// 複数ファイルを並行に変換しても、FluidSynthのバージョン確認が1回だけ行われ、
// その結果が全ての変換に使われることを検証する。
func TestMidiConverter_Convert_ProbesFluidsynthVersionOnce(t *testing.T) {
	t.Parallel()

	const conversions = 3

	dir := t.TempDir()
	soundfont := filepath.Join(dir, "test.sf2")
	writeFile(t, soundfont, []byte("soundfont data"))
	source := filepath.Join(dir, "input.mid")
	writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))

	ctrl := gomock.NewController(t)
	runner := NewMockCommandRunner(ctrl)
	runner.EXPECT().
		Run(gomock.Any(), "fluidsynth", "--version").
		Return([]byte("FluidSynth runtime version 2.4.6\n"), nil).
		Times(1)
	runner.EXPECT().
		Run(gomock.Any(), "fluidsynth", fluidsynthRenderArgs(soundfont, source, true)...).
		Return(nil, nil).
		Times(conversions)
	expectRenderedOgg(runner, conversions)

	c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, runner)

	var wg sync.WaitGroup
	for i := range conversions {
		wg.Go(func() {
			dest := filepath.Join(dir, "out", strconv.Itoa(i)+".ogg")
			_, err := c.Convert(source, dest)
			assert.NoError(t, err)
		})
	}
	wg.Wait()
}

// TestDynamicSampleLoadingMinVersion は動的サンプル読み込みを有効にする
// FluidSynthの下限バージョンを、doctor等の表示に使う文字列として返すことを検証する。
func TestDynamicSampleLoadingMinVersion(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "2.4.4", converter.DynamicSampleLoadingMinVersion())
}

// TestMidiConverter_Convert_RenderTimeout はFluidSynthのバージョン確認に要した時間が
// レンダリングの持ち時間から差し引かれないことを検証する。
func TestMidiConverter_Convert_RenderTimeout(t *testing.T) {
	t.Parallel()

	t.Run("正常系: レンダリングの期限はバージョン確認の終了後に起算される", func(t *testing.T) {
		t.Parallel()

		const timeout = time.Hour

		dir := t.TempDir()
		soundfont := filepath.Join(dir, "test.sf2")
		writeFile(t, soundfont, []byte("soundfont data"))
		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "output.ogg")

		var (
			probeEnd    time.Time
			deadline    time.Time
			hasDeadline bool
		)

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "fluidsynth", "--version").
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				time.Sleep(20 * time.Millisecond)
				probeEnd = time.Now()

				return []byte("FluidSynth runtime version 2.4.6\n"), nil
			})
		runner.EXPECT().
			Run(gomock.Any(), "fluidsynth", fluidsynthRenderArgs(soundfont, source, true)...).
			DoAndReturn(func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
				deadline, hasDeadline = ctx.Deadline()

				return nil, nil
			})
		expectRenderedOgg(runner, 1)

		c := converter.NewMidiConverter(soundfont, 0, "", 0, timeout, runner)
		_, err := c.Convert(source, dest)

		require.NoError(t, err)
		require.True(t, hasDeadline)
		assert.False(t, deadline.Before(probeEnd.Add(timeout)),
			"レンダリングの期限 %v がバージョン確認終了 %v からタイムアウトを数えた時刻より前になっている", deadline, probeEnd)
	})
}
