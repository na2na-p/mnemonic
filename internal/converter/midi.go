package converter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// trailingSilenceThresholdDB / trailingSilenceMinDuration / trailingSilenceKeepMargin
// はMIDI変換結果の末尾無音トリムに使う閾値・最小継続時間・トリム後に残す
// マージン(秒)。
//
// why not: FluidSynthは既定でリバーブ/コーラスが有効なため、MIDIファイル自体が
// 持つ末尾休符（実測で0〜3.3秒）に加え、最後のノートのリリースと残響減衰で
// さらに数秒分の可聴〜準無音区間をレンダリングする（実測: サンプルBGM 9曲で
// トレーリング無音3.2〜7.8秒）。-50dB/0.3秒という組み合わせは、この実測データで
// 「本当に鳴り終わった後の無音」と「曲中の短い休符」（実測データのsinone.midに
// 存在する0.49〜0.55秒の内部休符）を唯一正しく判別できた値。閾値を緩める
// （絶対値を下げる）と曲中の自然なディミヌエンドまで削れ、厳しくする
// （絶対値を上げる）とFluidSynthの残響テールを十分にトリムできない。
// stop_silenceに相当するkeepMarginは、無音区間を完全に0秒までカットすると
// ループ時に不自然なクリック音が乗るため、若干の無音を意図的に残す。
const (
	trailingSilenceThresholdDB = -50
	trailingSilenceMinDuration = 0.3
	trailingSilenceKeepMargin  = 0.3
)

// silenceProbeOutput はffprobe `-f lavfi -i "amovie=...,silencedetect=..."`
// `-show_entries frame_tags=lavfi.silence_start,lavfi.silence_end -of json`
// の出力を表す。
type silenceProbeOutput struct {
	Frames []silenceProbeFrame `json:"frames"`
}

type silenceProbeFrame struct {
	Tags map[string]string `json:"tags"`
}

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

	trimSeconds, hasTrim := c.detectTrailingSilenceTrimPoint(tmpWavPath)

	if result := c.runFFmpeg(tmpWavPath, dest, trimSeconds, hasTrim); result != nil {
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
// hasTrimがtrueの場合、trimSeconds秒で出力を打ち切ることで末尾無音をトリムする。
// エラー時は*ConversionResultを、成功時はnilを返す。
func (c *MidiConverter) runFFmpeg(wavInput, oggOutput string, trimSeconds float64, hasTrim bool) *ConversionResult {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	args := []string{
		"-y", // Overwrite output
		"-i", wavInput,
		"-c:a", c.audioCodec,
		"-q:a", strconv.Itoa(c.audioQuality),
	}
	if hasTrim {
		args = append(args, "-t", strconv.FormatFloat(trimSeconds, 'f', 3, 64))
	}
	args = append(args, oggOutput)

	if _, err := c.runner.Run(ctx, "ffmpeg", args...); err != nil {
		return &ConversionResult{
			SourcePath: wavInput,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("FFmpeg変換に失敗しました: %s", err),
		}
	}

	return nil
}

// detectTrailingSilenceTrimPoint はwavPathの末尾無音トリム位置(秒)を検出する。
// 末尾が無音で終わっていない場合や検出に失敗した場合は(0, false)を返し、
// 呼び出し側はトリムなしでffmpegを実行する（無音トリムはUX向上のための
// ベストエフォート処理であり、検出失敗を変換全体の失敗にはしない）。
func (c *MidiConverter) detectTrailingSilenceTrimPoint(wavPath string) (float64, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	start, found := c.detectTrailingSilenceStart(ctx, wavPath)
	if !found {
		return 0, false
	}

	return start + trailingSilenceKeepMargin, true
}

// detectTrailingSilenceStart はwavPathの末尾に付与された無音の開始秒を検出する。
//
// why not: ffmpegのsilencedetectフィルタはイベントをログ(stderr)へ出力するが、
// CommandRunner.Runは成功時に標準出力のみを返す（video.goのffprobe JSON
// パース処理へstderrの警告が混入しないようにするための既存契約であり、
// このMIDI変換専用の目的だけのために変更しない）。そこで
// ffprobe + lavfi(amovie+silencedetect) を使い、イベントを標準出力上の
// JSON(frame_tags)として取得する。
//
// 末尾の無音区間だけを安全に検出するため、直近のタグが"silence_start"のまま
// 終わっている（＝その後"silence_end"で終了していない）場合だけを「末尾まで
// 無音が続いている」とみなす。曲中に短い休符が複数回あっても、最後に観測される
// タグが"silence_end"であれば末尾は無音でないと判定してトリムしない。
func (c *MidiConverter) detectTrailingSilenceStart(ctx context.Context, wavPath string) (float64, bool) {
	filterInput := fmt.Sprintf(
		"amovie=%s,silencedetect=noise=%ddB:d=%s",
		escapeLavfiPathForAmovie(wavPath),
		trailingSilenceThresholdDB,
		strconv.FormatFloat(trailingSilenceMinDuration, 'f', -1, 64),
	)

	out, err := c.runner.Run(ctx, "ffprobe",
		"-v", "error",
		"-f", "lavfi",
		"-i", filterInput,
		"-show_entries", "frame_tags=lavfi.silence_start,lavfi.silence_end",
		"-of", "json",
	)
	if err != nil {
		return 0, false
	}

	var probe silenceProbeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return 0, false
	}

	var lastStart float64
	trailing := false
	for _, frame := range probe.Frames {
		if v, ok := frame.Tags["lavfi.silence_start"]; ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				lastStart = f
				trailing = true
			}

			continue
		}
		if _, ok := frame.Tags["lavfi.silence_end"]; ok {
			trailing = false
		}
	}

	if !trailing {
		return 0, false
	}

	return lastStart, true
}

// escapeLavfiPathForAmovie はamovieフィルタの引数として渡すパスをエスケープする。
//
// why not: amovie=<path>はavfilterのフィルタグラフ記述内に埋め込まれるため、
// シェルのクォート規則(閉じクォート+バックスラッシュ+クォート+開きクォート方式)は
// 通用しない。実機のffprobe(exec直接呼び出し、シェル非経由)で検証した結果、
// amovieのフィルタグラフ記述は「フィルタオプション値自体のエスケープ」と
// 「フィルタグラフ記述全体のエスケープ」の2段階が重なったエスケープを要求し、
// バックスラッシュ1文字は4文字(`\\\\`)、シングルクォート1文字はバックスラッシュ
// 3つ+クォート(`\\\'`)、コロン1文字はバックスラッシュ2つ+コロン(`\\:`)に
// それぞれ変換しないとamovieがパスを正しく解釈できない（文字ごとに必要な
// バックスラッシュ本数が異なる。パスにこれらの文字が含まれるのは主にWindows版の
// 一時ディレクトリ(例: C:\Users\...\AppData\Local\Temp)のケース）。バックスラッシュ
// の変換を最初に行うのは、後続のクォート・コロン置換で挿入するバックスラッシュ
// 自体が再度多重エスケープされるのを防ぐため。
func escapeLavfiPathForAmovie(path string) string {
	s := path
	s = strings.ReplaceAll(s, `\`, `\\\\`)
	s = strings.ReplaceAll(s, `'`, `\\\'`)
	s = strings.ReplaceAll(s, `:`, `\\:`)

	return s
}
