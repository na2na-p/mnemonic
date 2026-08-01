package converter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrVideoSourceNotFound はget_video_info対象の動画ファイルが存在しない場合のエラー。
var ErrVideoSourceNotFound = errors.New("ファイルが見つかりません")

// ErrVideoInfoUnavailable はffprobeの実行・パースに失敗した場合のエラー。
var ErrVideoInfoUnavailable = errors.New("動画情報を取得できません")

// ErrNoVideoStream はffprobe結果に動画ストリームが含まれない場合のエラー。
var ErrNoVideoStream = errors.New("動画ストリームが見つかりません")

// mpeg1videoCodecName / mp2CodecName / mpegPSFormatName はAndroid側ランタイム
// (krkrsdl2 fork、pl_mpeg採用)が再生できる唯一の組み合わせ
// (MPEG-PSコンテナ + mpeg1video + mp2)を表す定数。
const (
	mpeg1videoCodecName = "mpeg1video"
	mp2CodecName        = "mp2"
	mpegPSFormatName    = "mpeg"
)

// fallbackFrameRate はffprobeでフレームレートを取得できなかった場合に使う
// 既定値(fps)。
//
// why not: mpeg1videoエンコーダは規格上定められた値以外のフレームレートを
// 拒否する(例: 15fps/20fpsの入力はエンコーダ初期化に失敗し出力が0バイトになる)。
// 取得不能時に入力側のフレームレートをそのまま使うと同じ失敗を再現するため、
// PAL(25fps)を中立な既定値として採用する。
const fallbackFrameRate = 25.0

// legalMpeg1FrameRates はMPEG-1/2規格が許容するフレームレートの一覧。
// ffmpegが認識する引数表記(分数形式含む)で保持する。
var legalMpeg1FrameRates = []struct {
	Arg   string
	Value float64
}{
	{"24000/1001", 24000.0 / 1001.0},
	{"24", 24},
	{"25", 25},
	{"30000/1001", 30000.0 / 1001.0},
	{"30", 30},
	{"50", 50},
	{"60000/1001", 60000.0 / 1001.0},
	{"60", 60},
}

// CommandRunner は外部コマンド実行を抽象化する。
//
// why not: os/exec.Cmdを直接VideoConverterから呼ぶとユニットテストが実際に
// ffmpeg/ffprobeプロセスの起動を要求し、CI環境依存かつ低速になる。実行結果を
// 差し替え可能にするためインターフェース化し、go.uber.org/mock(gomock)で
// モックする（Go Library SSOT）。
type CommandRunner interface {
	// Run はnameコマンドをargsで実行し、標準出力を返す。
	// 非ゼロ終了やコマンド未検出の場合はerrを返す。
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// execCommandRunner はos/execを使った既定のCommandRunner実装。
type execCommandRunner struct{}

// NewExecCommandRunner はos/execベースのCommandRunnerを返す。
func NewExecCommandRunner() CommandRunner {
	return execCommandRunner{}
}

func (execCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // ffmpeg/ffprobeを呼び出す用途のため妥当

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("%s実行に失敗しました: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

// VideoInfo は動画ファイルのメタデータを表す不変値。
//
// AudioCodecが空文字列の場合、音声トラックなしを表す。
type VideoInfo struct {
	Width           int
	Height          int
	DurationSeconds float64
	HasAudio        bool
	VideoCodec      string
	AudioCodec      string
	Bitrate         int64
	FrameRate       float64
	ContainerFormat string
}

// VideoConverter はFFmpeg/ffprobeを使い動画ファイルをAndroid側ランタイム
// (krkrsdl2 fork、pl_mpeg採用)が再生できるMPEG-PS(mpeg1video+mp2)形式へ
// 変換するConverter。
type VideoConverter struct {
	timeout time.Duration
	runner  CommandRunner
}

// NewVideoConverter はVideoConverterを初期化する。
// timeoutが0以下の場合はデフォルト値(300秒)を使用する。
// runnerがnilの場合はos/execベースの既定実装を使用する。
func NewVideoConverter(timeout time.Duration, runner CommandRunner) *VideoConverter {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	if runner == nil {
		runner = NewExecCommandRunner()
	}

	return &VideoConverter{
		timeout: timeout,
		runner:  runner,
	}
}

// SupportedExtensions は対応する拡張子の一覧を返す。
func (c *VideoConverter) SupportedExtensions() []string {
	return []string{".mpg", ".mpeg", ".wmv", ".avi"}
}

// GetOutputExtension は出力ファイルの拡張子".mpg"を返す。
//
// why not: 変換後の実体は常にMPEG-PS(mpeg1video+mp2)であり、入力が.wmv/.aviで
// あっても出力コンテナは.mpgになる。拡張子を保持したままにすると
// ConversionManagerが元の拡張子(.wmv等)のまま変換後ファイルを配置してしまい、
// 実体と拡張子が食い違ったファイルが生成される。
func (c *VideoConverter) GetOutputExtension(_ string) string { return ".mpg" }

// CanConvert はfilePathが変換可能かを拡張子で判定する。
func (c *VideoConverter) CanConvert(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))

	return containsString(c.SupportedExtensions(), ext)
}

// IsFFmpegAvailable はffmpegコマンドが利用可能かを確認する。
func (c *VideoConverter) IsFFmpegAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	_, err := c.runner.Run(ctx, "ffmpeg", "-version")

	return err == nil
}

// Convert は動画ファイルをAndroid側ランタイムが再生可能なMPEG-PS
// (mpeg1video+mp2)形式に変換し、destへ出力する。
//
// 入力が既にmpeg1video+mp2(MPEG-PSコンテナ)であれば再エンコードせずコピーする
// (パススルー)。それ以外はffmpegで変換する。出力は一時ファイルへ書き込み、
// サイズが0より大きいことを確認してからdestへrenameする(fail-loud)。
// 変換元ファイルが存在しない・ffmpegが失敗する・出力が0バイトの場合は
// StatusFailedのConversionResultを返す(err=nil、既知の失敗を自身で
// ConversionResultへ変換する設計)。
func (c *VideoConverter) Convert(source, dest string) (ConversionResult, error) {
	if _, err := os.Stat(source); err != nil {
		return ConversionResult{
			SourcePath: source,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("変換元ファイルが見つかりません: %s", source),
		}, nil
	}

	bytesBefore := getFileSize(source)

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return ConversionResult{}, fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	// why not: destと同じディレクトリの一時ファイルへ書いてからrenameする。
	// os.CreateTemp等でOS標準の一時ディレクトリ(/tmp)に書くとdestとは別の
	// ファイルシステムになることがあり、renameがEXDEVで失敗し得るため避ける。
	tempDest := dest + ".tmp"

	info, probeErr := c.GetVideoInfo(source)

	var convertErr error
	if isPassthroughEligible(info, probeErr) {
		convertErr = copyFile(source, tempDest)
	} else {
		convertErr = c.runFFmpegConvert(source, tempDest, info, probeErr)
	}

	if convertErr != nil {
		_ = os.Remove(tempDest)

		return ConversionResult{
			SourcePath: source,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("動画変換に失敗しました: %s", convertErr),
		}, nil
	}

	bytesAfter, err := finalizeTempOutput(tempDest, dest)
	if err != nil {
		return ConversionResult{
			SourcePath: source,
			Status:     StatusFailed,
			Message:    fmt.Sprintf("動画変換に失敗しました: %s", err),
		}, nil
	}

	return ConversionResult{
		SourcePath:  source,
		DestPath:    dest,
		Status:      StatusSuccess,
		BytesBefore: bytesBefore,
		BytesAfter:  bytesAfter,
	}, nil
}

// isPassthroughEligible はprobeErrがnilかつinfoがAndroid側ランタイムの
// 再生要件(MPEG-PS + mpeg1video + mp2、音声トラック無しも許容)を満たすかを返す。
// probeErr != nilの場合(ffprobe判定失敗)は常にfalseを返し、呼び出し元が
// ffmpeg変換経路にフォールバックできるようにする。
func isPassthroughEligible(info VideoInfo, probeErr error) bool {
	if probeErr != nil {
		return false
	}

	return info.VideoCodec == mpeg1videoCodecName &&
		info.ContainerFormat == mpegPSFormatName &&
		(info.AudioCodec == "" || info.AudioCodec == mp2CodecName)
}

// runFFmpegConvert はsourceをmpeg1video+mp2のMPEG-PS(tempDest)へ変換する。
func (c *VideoConverter) runFFmpegConvert(source, tempDest string, info VideoInfo, probeErr error) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	rate := fallbackFrameRate
	if probeErr == nil && info.FrameRate > 0 {
		rate = info.FrameRate
	}

	args := []string{
		"-y",
		"-i", source,
		"-r", nearestLegalFrameRateArg(rate),
		"-c:v", mpeg1videoCodecName,
		"-q:v", "4",
		"-c:a", mp2CodecName,
		"-b:a", "224k",
		"-f", "mpeg",
		tempDest,
	}

	if _, err := c.runner.Run(ctx, "ffmpeg", args...); err != nil {
		return err
	}

	return nil
}

// nearestLegalFrameRateArg はfpsに最も近いMPEG-1/2許容フレームレートの
// ffmpeg引数表記を返す。
func nearestLegalFrameRateArg(fps float64) string {
	best := legalMpeg1FrameRates[0]
	bestDiff := math.Abs(fps - best.Value)

	for _, candidate := range legalMpeg1FrameRates[1:] {
		diff := math.Abs(fps - candidate.Value)
		if diff < bestDiff {
			best = candidate
			bestDiff = diff
		}
	}

	return best.Arg
}

// copyFile はsourceの内容をそのままdestへコピーする(パススルー用)。
//
// why not: os.Renameではなくコピーを使う。sourceは展開済みゲームツリー内の
// ファイルであり、変換先ディレクトリへ移動すると元のツリーからファイルが
// 消え、リトライや他の後続処理がsourceを参照できなくなる。
func copyFile(source, dest string) error {
	in, err := os.Open(source) //nolint:gosec // 変換元パスは呼び出し元(ConversionManager)が決定する
	if err != nil {
		return fmt.Errorf("変換元ファイルを開けません: %w", err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dest) //nolint:gosec // 変換先パスは呼び出し元(ConversionManager)が決定する
	if err != nil {
		return fmt.Errorf("出力ファイルを作成できません: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("ファイルのコピーに失敗しました: %w", err)
	}

	return nil
}

// finalizeTempOutput はtempDestのサイズを検証し、destへrenameする。
// サイズが0の場合はtempDestを削除しエラーを返す(fail-loud)。
func finalizeTempOutput(tempDest, dest string) (int64, error) {
	size := getFileSize(tempDest)
	if size == 0 {
		_ = os.Remove(tempDest)

		return 0, fmt.Errorf("出力ファイルが0バイトです: %s", tempDest)
	}

	if err := os.Rename(tempDest, dest); err != nil {
		_ = os.Remove(tempDest)

		return 0, fmt.Errorf("出力ファイルのリネームに失敗しました: %w", err)
	}

	return size, nil
}

// ffprobeOutput はffprobe -of json の出力を表す。
type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	CodecType  string `json:"codec_type"`
	CodecName  string `json:"codec_name"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	RFrameRate string `json:"r_frame_rate"`
}

type ffprobeFormat struct {
	Duration   string `json:"duration"`
	BitRate    string `json:"bit_rate"`
	FormatName string `json:"format_name"`
}

// GetVideoInfo は動画ファイルの情報をffprobeで取得する。
//
// 実行される実効的なffprobeコマンドは以下の通り:
// `ffprobe -show_format -show_streams -of json <file>`
func (c *VideoConverter) GetVideoInfo(filePath string) (VideoInfo, error) {
	if _, err := os.Stat(filePath); err != nil {
		return VideoInfo{}, fmt.Errorf("%w: %s", ErrVideoSourceNotFound, filePath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	stdout, err := c.runner.Run(ctx, "ffprobe", "-show_format", "-show_streams", "-of", "json", filePath)
	if err != nil {
		return VideoInfo{}, fmt.Errorf("%w: %s: %w", ErrVideoInfoUnavailable, filePath, err)
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(stdout, &probe); err != nil {
		return VideoInfo{}, fmt.Errorf("%w: %s: %w", ErrVideoInfoUnavailable, filePath, err)
	}

	var videoStream, audioStream *ffprobeStream

	for i := range probe.Streams {
		s := &probe.Streams[i]
		switch s.CodecType {
		case "video":
			if videoStream == nil {
				videoStream = s
			}
		case "audio":
			if audioStream == nil {
				audioStream = s
			}
		}
	}

	if videoStream == nil {
		return VideoInfo{}, fmt.Errorf("%w: %w: %s", ErrVideoInfoUnavailable, ErrNoVideoStream, filePath)
	}

	info := VideoInfo{
		Width:           videoStream.Width,
		Height:          videoStream.Height,
		DurationSeconds: parseFloatOrZero(probe.Format.Duration),
		HasAudio:        audioStream != nil,
		VideoCodec:      defaultString(videoStream.CodecName, "unknown"),
		Bitrate:         parseIntOrZero(probe.Format.BitRate),
		FrameRate:       parseFrameRateOrZero(videoStream.RFrameRate),
		ContainerFormat: probe.Format.FormatName,
	}
	if audioStream != nil {
		info.AudioCodec = audioStream.CodecName
	}

	return info, nil
}

func parseFloatOrZero(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}

	return v
}

func parseIntOrZero(s string) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}

	return v
}

// parseFrameRateOrZero はffprobeのr_frame_rate("分子/分母"形式、例: "25/1")を
// float64へ変換する。不正な形式や分母0の場合は0を返す。
func parseFrameRateOrZero(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	if !ok {
		return 0
	}

	n, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}

	d, err := strconv.ParseFloat(den, 64)
	if err != nil || d == 0 {
		return 0
	}

	return n / d
}

func defaultString(s, fallback string) string {
	if s == "" {
		return fallback
	}

	return s
}
