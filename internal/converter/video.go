package converter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/na2na-p/mnemonic/internal/fsutil"
)

// ErrVideoSourceNotFound はget_video_info対象の動画ファイルが存在しない場合のエラー。
var ErrVideoSourceNotFound = errors.New("ファイルが見つかりません")

// ErrVideoInfoUnavailable はffprobeの実行・パースに失敗した場合と、存在しない以外の
// 理由で動画ファイルを確認できない場合のエラー。
var ErrVideoInfoUnavailable = errors.New("動画情報を取得できません")

// ErrNoVideoStream はffprobe結果に動画ストリームが含まれない場合のエラー。
var ErrNoVideoStream = errors.New("動画ストリームが見つかりません")

// ErrVideoConversionFailed はVideoConverter.Convertがffmpeg変換・パススルー
// コピー・出力の確定のいずれかに失敗した場合のエラー。原因のエラーを%wで保持する。
var ErrVideoConversionFailed = errors.New("動画変換に失敗しました")

// ErrEmptyOutput は変換処理が成功を返したにもかかわらず出力ファイルが0バイトの場合のエラー。
var ErrEmptyOutput = errors.New("出力ファイルが0バイトです")

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
//
// stderrLineLimitが正の場合、失敗時のエラーに含めるstderrをsummarizeStderrで
// 先頭のその行数までに絞る。0の場合はstderr全体を含める。
//
// why not: NewExecCommandRunnerが返す実装（MidiConverterが使う）では絞らない。
// MIDI変換はバナーを抑止せずにffmpegを起動しており、ffmpeg 9.0.1で同じ形の
// 呼び出しに壊れた入力を与えると、先頭3行はバージョンとビルド構成のバナーだった。
type execCommandRunner struct {
	stderrLineLimit int
}

// videoStderrLineLimit はVideoConverterの既定のCommandRunnerが失敗時のエラーに
// 含めるstderrの行数。
//
// why not: 末尾の行ではなく先頭の行を残す。ffmpeg 9.0.1でlavfiのtestsrcが
// 生成した15fpsの映像をmpeg1videoへ変換させて測ったところ、バナー付きの
// stderrでは原因の「MPEG-1/2 does not support 15/1 fps」は出力の途中
// （28行中18行目）にあり、末尾の3行は「Nothing was written into output file…」
// 「frame=…」「Conversion failed!」という原因を含まない行だった。
// -hide_banner -loglevel errorを付けると、原因は1行目に出た。VideoConverterは
// これらを付けてffmpegを起動する。
const videoStderrLineLimit = 3

// NewExecCommandRunner はos/execベースのCommandRunnerを返す。
func NewExecCommandRunner() CommandRunner {
	return execCommandRunner{}
}

func (r execCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // ffmpeg/ffprobeを呼び出す用途のため妥当

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if r.stderrLineLimit > 0 {
			detail = summarizeStderr(stderr.String(), r.stderrLineLimit)
		}

		return stdout.Bytes(), fmt.Errorf("%s実行に失敗しました: %w: %s", name, err, detail)
	}

	return stdout.Bytes(), nil
}

// summarizeStderr はstderrの空でない行のうち先頭limit行を「 | 」でつないだ
// 1行を返す。空でない行がlimit行を超える場合は、末尾に空でない行の総数を添える。
func summarizeStderr(stderr string, limit int) string {
	var lines []string
	for line := range strings.Lines(stderr) {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	if len(lines) <= limit {
		return strings.Join(lines, " | ")
	}

	return fmt.Sprintf("%s（全 %d 行）", strings.Join(lines[:limit], " | "), len(lines))
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
// runnerがnilの場合は、失敗時のstderrを先頭videoStderrLineLimit行に絞る
// os/execベースの既定実装を使用する。
func NewVideoConverter(timeout time.Duration, runner CommandRunner) *VideoConverter {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	if runner == nil {
		runner = execCommandRunner{stderrLineLimit: videoStderrLineLimit}
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
//
// 失敗はerrとして返す。変換元が存在しない・権限不足で確認できない場合は
// ErrSourceNotFound/ErrSourceUnreadableをErrPermanentFailureでラップして返し、
// それ以外の理由で確認できない場合はErrSourceUnreadableを再試行対象として返す。
// ffmpeg変換・パススルーコピー・出力の確定(0バイト出力はErrEmptyOutput)の失敗は
// ErrVideoConversionFailedで、出力先ディレクトリの作成失敗はOSのエラーを%wで
// 保持して返し、いずれも再試行対象とする。errがnilのとき、StatusはStatusSuccessとなる。
func (c *VideoConverter) Convert(source, dest string) (ConversionResult, error) {
	if err := ensureSourceExists(source); err != nil {
		return ConversionResult{SourcePath: source}, err
	}

	bytesBefore := getFileSize(source)

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return ConversionResult{SourcePath: source}, fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
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

		return ConversionResult{SourcePath: source}, fmt.Errorf("%w: %w", ErrVideoConversionFailed, convertErr)
	}

	bytesAfter, err := finalizeTempOutput(tempDest, dest)
	if err != nil {
		return ConversionResult{SourcePath: source}, fmt.Errorf("%w: %w", ErrVideoConversionFailed, err)
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

	// why not: -hide_banner -loglevel errorを外さない。失敗時のエラーにはstderrの
	// 先頭videoStderrLineLimit行しか残らないが、これらが無いとstderrはバージョンや
	// ビルド構成のバナーで始まる（ffmpeg 9.0.1で壊れた入力を与えると13行中10行が
	// バナーだった）。
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
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
	if err := fsutil.CopyFile(source, dest); err != nil {
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

		return 0, fmt.Errorf("%w: %s", ErrEmptyOutput, tempDest)
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
// `ffprobe -hide_banner -show_format -show_streams -of json <file>`
//
// filePathが存在しない場合はErrVideoSourceNotFoundを返す。それ以外の理由
// （権限不足・親がファイルなど）でos.Statに失敗した場合と、ffprobeの実行・
// 出力の解析に失敗した場合はErrVideoInfoUnavailableを返し、前者はOSのエラーを
// %wで保持する。
func (c *VideoConverter) GetVideoInfo(filePath string) (VideoInfo, error) {
	if _, err := os.Stat(filePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return VideoInfo{}, fmt.Errorf("%w: %s", ErrVideoSourceNotFound, filePath)
		}

		return VideoInfo{}, fmt.Errorf("%w: %w", ErrVideoInfoUnavailable, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	stdout, err := c.runner.Run(ctx, "ffprobe", "-hide_banner", "-show_format", "-show_streams", "-of", "json", filePath)
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
