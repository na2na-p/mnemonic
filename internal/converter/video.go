package converter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
// AudioCodecが空文字列の場合、Python版のNone（音声トラックなし）に相当する。
type VideoInfo struct {
	Width           int
	Height          int
	DurationSeconds float64
	HasAudio        bool
	VideoCodec      string
	AudioCodec      string
	Bitrate         int64
}

// VideoConverter はFFmpegを使い動画ファイルをAndroid互換形式(MP4)に変換するConverter。
type VideoConverter struct {
	videoCodec   string
	videoProfile string
	audioCodec   string
	timeout      time.Duration
	runner       CommandRunner
}

// NewVideoConverter はVideoConverterを初期化する。
// 各引数が空文字列/0の場合はPython版と同じデフォルト値
// (videoCodec="libx264", videoProfile="baseline", audioCodec="aac", timeout=300秒)を使用する。
// runnerがnilの場合はos/execベースの既定実装を使用する。
func NewVideoConverter(videoCodec, videoProfile, audioCodec string, timeout time.Duration, runner CommandRunner) *VideoConverter {
	if videoCodec == "" {
		videoCodec = "libx264"
	}
	if videoProfile == "" {
		videoProfile = "baseline"
	}
	if audioCodec == "" {
		audioCodec = "aac"
	}
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	if runner == nil {
		runner = NewExecCommandRunner()
	}

	return &VideoConverter{
		videoCodec:   videoCodec,
		videoProfile: videoProfile,
		audioCodec:   audioCodec,
		timeout:      timeout,
		runner:       runner,
	}
}

// SupportedExtensions は対応する拡張子の一覧を返す。
func (c *VideoConverter) SupportedExtensions() []string {
	return []string{".mpg", ".mpeg", ".wmv", ".avi"}
}

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

// Convert は動画ファイルをMP4形式に変換し、destへ出力する。
//
// 実行される実効的なffmpegコマンドはPython版(ffmpeg-python)が生成する
// 引数と等価にしてある: `ffmpeg -i <source> -acodec <audioCodec>
// -profile <videoProfile> -vcodec <videoCodec> <dest> -y`
// （ffmpeg-pythonはoutput()のkwargsをキー名の辞書順でCLI引数化するため、
// acodec/profile/vcodecの順になる）。
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

	// why not: Python版はtimeoutをコンストラクタ引数として保持するが、
	// stream.run(overwrite_output=True, quiet=True)呼び出しには一切渡しておらず
	// 実際には何の効果も持たないdead configuration。Go版ではcontext.WithTimeoutで
	// timeoutを実際に有効化しており、これはPython版の未配線設定に対する意図的な
	// 改善（フリーズしたffmpegプロセスを無期限に待ち続けない）である。
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	args := []string{
		"-i", source,
		"-acodec", c.audioCodec,
		"-profile", c.videoProfile,
		"-vcodec", c.videoCodec,
		dest,
		"-y",
	}

	if _, err := c.runner.Run(ctx, "ffmpeg", args...); err != nil {
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
		BytesAfter:  getFileSize(dest),
	}, nil
}

// ffprobeOutput はffprobe -of json の出力を表す。
type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type ffprobeFormat struct {
	Duration string `json:"duration"`
	BitRate  string `json:"bit_rate"`
}

// GetVideoInfo は動画ファイルの情報をffprobeで取得する。
//
// 実行される実効的なffprobeコマンドはPython版(ffmpeg.probe)と同一:
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

func defaultString(s, fallback string) string {
	if s == "" {
		return fallback
	}

	return s
}
