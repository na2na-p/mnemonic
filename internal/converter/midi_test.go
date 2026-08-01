package converter_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/na2na-p/mnemonic/internal/converter"
)

// TestGetDefaultSoundfontPath はGetDefaultSoundfontPathの探索優先順位をテストする。
//
// converter.MuseScoreSoundfontPath / FluidR3SoundfontPath はパッケージ変数
// （Python版のクラス属性モンキーパッチに相当）を書き換えるため、このテストは
// 他のテストとの並行実行を避けてt.Parallel()を呼ばない
// （goimportsやgo test -race -shuffle=onでの競合を避けるため）。
//
// Python版 TestMidiConverterGetDefaultSoundfontPath の移植。
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
//
// Python版 TestMidiConverterInit の一部（デフォルト初期化）の移植。
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
//
// Python版 TestMidiConverterInit の一部（カスタム初期化）の移植。
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
//
// Python版 TestMidiConverterSupportedExtensions の移植。
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
// why not: baseline Python版はBaseConverter.get_output_extensionの
// デフォルト（拡張子を変更しない）のままだが、Go版Converterインターフェースは
// PR9(T-209)でGetOutputExtensionを追加した際にManagerがこれを使い出力先の
// 拡張子をリネームする設計になった。MIDI変換の出力実体は常にOGGであるため、
// ".ogg"を返すのがConverterインターフェースの契約として正しい
// （Python版はconvert()呼び出し側がdestを明示的に.oggで渡す前提の設計だった）。
func TestMidiConverter_GetOutputExtension(t *testing.T) {
	t.Parallel()

	c := converter.NewMidiConverter(filepath.Join(t.TempDir(), "sf.sf2"), 0, "", 0, 0, nil)
	assert.Equal(t, ".ogg", c.GetOutputExtension("bgm/title.mid"))
}

// TestMidiConverter_CanConvert はCanConvertをテストする。
//
// Python版 TestMidiConverterCanConvert の移植。
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
//
// Python版 TestMidiConverterIsFluidSynthAvailable の移植。
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
//
// Python版 TestMidiConverterConvert の移植。
func TestMidiConverter_Convert(t *testing.T) {
	t.Parallel()

	t.Run("異常系: 変換元ファイルが存在しない場合FAILEDを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "non_existent.mid")
		dest := filepath.Join(dir, "output.ogg")

		c := converter.NewMidiConverter(filepath.Join(dir, "sf.sf2"), 0, "", 0, 0, nil)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusFailed, result.Status)
		assert.Equal(t, source, result.SourcePath)
		assert.Contains(t, result.Message, "見つかりません")
	})

	t.Run("異常系: サウンドフォントが存在しない場合FAILEDを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "output.ogg")

		c := converter.NewMidiConverter(filepath.Join(dir, "non_existent.sf2"), 0, "", 0, 0, nil)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusFailed, result.Status)
		assert.Contains(t, result.Message, "サウンドフォント")
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

	t.Run("異常系: FluidSynthがエラーを返す場合FAILEDを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		soundfont := filepath.Join(dir, "test.sf2")
		writeFile(t, soundfont, []byte("soundfont data"))

		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "output.ogg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "fluidsynth", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("FluidSynth error"))

		c := converter.NewMidiConverter(soundfont, 0, "", 0, 0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusFailed, result.Status)
		assert.Contains(t, result.Message, "FluidSynth")
	})

	t.Run("異常系: FFmpegがエラーを返す場合FAILEDを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		soundfont := filepath.Join(dir, "test.sf2")
		writeFile(t, soundfont, []byte("soundfont data"))

		source := filepath.Join(dir, "input.mid")
		writeFile(t, source, []byte("MThd"+string(make([]byte, 100))))
		dest := filepath.Join(dir, "output.ogg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
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

		require.NoError(t, err)
		assert.Equal(t, converter.StatusFailed, result.Status)
		assert.Contains(t, result.Message, "FFmpeg")
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
