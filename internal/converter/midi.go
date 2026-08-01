package converter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MuseScoreSoundfontPath / FluidR3SoundfontPath はGetDefaultSoundfontPathが
// 探索するデフォルトサウンドフォントのパス。
//
// why: テストで存在判定を差し替えられるよう、パッケージ変数として公開する
// （テストは一時的な差し替えと復元をdeferで行う）。
var (
	MuseScoreSoundfontPath = "/usr/share/sounds/sf3/MuseScore_General.sf3"
	FluidR3SoundfontPath   = "/usr/share/sounds/sf2/FluidR3_GM.sf2"
)

// GetDefaultSoundfontPath は利用可能なデフォルトサウンドフォントのパスを返す。
// MuseScoreSoundfontPathが存在すればそれを、そうでなければFluidR3SoundfontPathを
// 返す（存在確認自体は行わないFluidR3SoundfontPathへの最終フォールバック）。
func GetDefaultSoundfontPath() string {
	if _, err := os.Stat(MuseScoreSoundfontPath); err == nil {
		return MuseScoreSoundfontPath
	}

	return FluidR3SoundfontPath
}

// MidiConverter はFluidSynth+FFmpegを使いMIDIファイルをOGG Vorbis形式に
// 変換するConverter。
//
// 変換フロー:
//  1. FluidSynth + SoundFont で MIDI → WAV
//  2. FFmpeg で WAV → OGG Vorbis
type MidiConverter struct {
	soundfontPath string
	sampleRate    int
	audioCodec    string
	audioQuality  int
	timeout       time.Duration
	runner        CommandRunner
}

// NewMidiConverter はMidiConverterを初期化する。
// soundfontPathが空文字列の場合はGetDefaultSoundfontPath()を使用する。
// sampleRate/audioQualityが0以下の場合、audioCodecが空文字列の場合、
// timeoutが0以下の場合はデフォルト値
// (sampleRate=44100, audioCodec="libvorbis", audioQuality=4, timeout=300秒)を
// 使用する。runnerがnilの場合はos/execベースの既定実装を使用する。
//
// why not: audioQuality=0（ffmpeg -q:a 0、libvorbisにおける最高品質指定）を
// 有効な値として区別することもできるが、VideoConverter等の既存の
// 「0以下はセンチネル」規約に合わせて0もデフォルト値(4)へフォールバックする
// （既存コンストラクタ群との規約一貫性を優先した設計）。
func NewMidiConverter(
	soundfontPath string,
	sampleRate int,
	audioCodec string,
	audioQuality int,
	timeout time.Duration,
	runner CommandRunner,
) *MidiConverter {
	if soundfontPath == "" {
		soundfontPath = GetDefaultSoundfontPath()
	}
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	if audioCodec == "" {
		audioCodec = "libvorbis"
	}
	if audioQuality <= 0 {
		audioQuality = 4
	}
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	if runner == nil {
		runner = NewExecCommandRunner()
	}

	return &MidiConverter{
		soundfontPath: soundfontPath,
		sampleRate:    sampleRate,
		audioCodec:    audioCodec,
		audioQuality:  audioQuality,
		timeout:       timeout,
		runner:        runner,
	}
}

// SoundfontPath は使用するサウンドフォントのパスを返す。
func (c *MidiConverter) SoundfontPath() string { return c.soundfontPath }

// SampleRate は出力サンプルレート（Hz）を返す。
func (c *MidiConverter) SampleRate() int { return c.sampleRate }

// AudioCodec は使用する音声コーデックを返す。
func (c *MidiConverter) AudioCodec() string { return c.audioCodec }

// AudioQuality は音声品質（VBR用、0-10）を返す。
func (c *MidiConverter) AudioQuality() int { return c.audioQuality }

// Timeout は変換処理のタイムアウトを返す。
func (c *MidiConverter) Timeout() time.Duration { return c.timeout }

// SupportedExtensions は対応する拡張子の一覧を返す。
func (c *MidiConverter) SupportedExtensions() []string {
	return []string{".mid", ".midi"}
}

// GetOutputExtension は変換後ファイルの拡張子".ogg"を返す。
//
// why not: Converterインターフェースが持つGetOutputExtensionは出力パスの
// 拡張子を書き換えるために使われる。MIDI変換の出力実体は常にOGGであるため、
// 空文字列（拡張子不変）を返すと出力先が".mid"のままになり誤りとなる。
func (c *MidiConverter) GetOutputExtension(_ string) string { return ".ogg" }

// CanConvert はfilePathが変換可能かを拡張子で判定する。
func (c *MidiConverter) CanConvert(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))

	return containsString(c.SupportedExtensions(), ext)
}

// IsFluidsynthAvailable はfluidsynthコマンドが利用可能かを確認する。
func (c *MidiConverter) IsFluidsynthAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	_, err := c.runner.Run(ctx, "fluidsynth", "--version")

	return err == nil
}

// Convert はMIDIファイルをOGG Vorbis形式に変換し、destへ出力する。
//
// 実行される実効的なコマンドは以下の通り:
// `fluidsynth -ni -g 1.0 -r <sampleRate> -F <一時WAV> <soundfont> <source>`
// に続けて
// `ffmpeg -y -i <一時WAV> -c:a <audioCodec> -q:a <audioQuality> <dest>`
//
// why not: FluidSynth/FFmpegそれぞれの未インストールとタイムアウトを区別した
// 専用メッセージを返すこともできるが、CommandRunner抽象化により両者とも
// 単一のerr値に収束する（video.goのVideoConverter.Convertと同じ簡略化）。
// 区別が必要になればCommandRunnerの実装側でセンチネルエラーを定義して
// 呼び出し元に伝播させる。
func (c *MidiConverter) Convert(source, dest string) (ConversionResult, error) {
	if _, err := os.Stat(source); err != nil {
		return ConversionResult{
			SourcePath: source,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("変換元ファイルが見つかりません: %s", source),
		}, nil
	}

	if _, err := os.Stat(c.soundfontPath); err != nil {
		return ConversionResult{
			SourcePath: source,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("サウンドフォントが見つかりません: %s", c.soundfontPath),
		}, nil
	}

	bytesBefore := getFileSize(source)

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return ConversionResult{
			SourcePath: source,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("MIDI変換に失敗しました: %s", err),
		}, nil
	}

	tmpWav, err := os.CreateTemp("", "*.wav")
	if err != nil {
		return ConversionResult{
			SourcePath: source,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("MIDI変換に失敗しました: %s", err),
		}, nil
	}
	tmpWavPath := tmpWav.Name()
	_ = tmpWav.Close()
	// why: FluidSynth/FFmpegどちらが失敗しても一時ファイルが残らないよう
	// deferで無条件に削除する。
	defer func() { _ = os.Remove(tmpWavPath) }()

	if result := c.runFluidsynth(source, tmpWavPath); result != nil {
		return *result, nil
	}

	if result := c.runFFmpeg(tmpWavPath, dest); result != nil {
		return *result, nil
	}

	return ConversionResult{
		SourcePath:  source,
		DestPath:    dest,
		Status:      StatusSuccess,
		BytesBefore: bytesBefore,
		BytesAfter:  getFileSize(dest),
	}, nil
}

// runFluidsynth はFluidSynthを実行してsourceをwavOutputへレンダリングする。
// エラー時は*ConversionResultを、成功時はnilを返す。
func (c *MidiConverter) runFluidsynth(source, wavOutput string) *ConversionResult {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	args := []string{
		"-ni",       // No interactive mode, no shell
		"-g", "1.0", // Gain
		"-r", strconv.Itoa(c.sampleRate), // Sample rate
		"-F", wavOutput, // Output file
		c.soundfontPath,
		source,
	}

	if _, err := c.runner.Run(ctx, "fluidsynth", args...); err != nil {
		return &ConversionResult{
			SourcePath: source,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("FluidSynth変換に失敗しました: %s", err),
		}
	}

	return nil
}

// runFFmpeg はFFmpegを実行してwavInputをoggOutputへ変換する。
// エラー時は*ConversionResultを、成功時はnilを返す。
func (c *MidiConverter) runFFmpeg(wavInput, oggOutput string) *ConversionResult {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	args := []string{
		"-y", // Overwrite output
		"-i", wavInput,
		"-c:a", c.audioCodec,
		"-q:a", strconv.Itoa(c.audioQuality),
		oggOutput,
	}

	if _, err := c.runner.Run(ctx, "ffmpeg", args...); err != nil {
		return &ConversionResult{
			SourcePath: wavInput,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("FFmpeg変換に失敗しました: %s", err),
		}
	}

	return nil
}
