package converter_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/na2na-p/mnemonic/internal/converter"
)

func TestVideoInfo_Fields(t *testing.T) {
	t.Parallel()

	t.Run("正常系: VideoInfoの作成", func(t *testing.T) {
		t.Parallel()

		info := converter.VideoInfo{
			Width: 1920, Height: 1080, DurationSeconds: 120.5,
			HasAudio: true, VideoCodec: "h264", AudioCodec: "aac", Bitrate: 5000000,
			FrameRate: 30, ContainerFormat: "mov,mp4,m4a,3gp,3g2,mj2",
		}

		assert.Equal(t, 1920, info.Width)
		assert.Equal(t, 1080, info.Height)
		assert.InDelta(t, 120.5, info.DurationSeconds, 1e-9)
		assert.True(t, info.HasAudio)
		assert.Equal(t, "h264", info.VideoCodec)
		assert.Equal(t, "aac", info.AudioCodec)
		assert.Equal(t, int64(5000000), info.Bitrate)
		assert.InDelta(t, 30.0, info.FrameRate, 1e-9)
		assert.Equal(t, "mov,mp4,m4a,3gp,3g2,mj2", info.ContainerFormat)
	})

	t.Run("正常系: 音声なしのVideoInfoの作成", func(t *testing.T) {
		t.Parallel()

		info := converter.VideoInfo{
			Width: 640, Height: 480, DurationSeconds: 30.0,
			HasAudio: false, VideoCodec: "mpeg1video", AudioCodec: "", Bitrate: 1500000,
		}

		assert.False(t, info.HasAudio)
		assert.Empty(t, info.AudioCodec)
	})
}

func TestVideoConverter_SupportedExtensions(t *testing.T) {
	t.Parallel()

	c := converter.NewVideoConverter(0, nil)
	extensions := c.SupportedExtensions()

	for _, ext := range []string{".mpg", ".mpeg", ".wmv", ".avi"} {
		assert.Contains(t, extensions, ext)
	}
	for _, ext := range extensions {
		assert.True(t, filepathHasDotPrefix(ext))
	}
}

func filepathHasDotPrefix(s string) bool {
	return len(s) > 0 && s[0] == '.'
}

func TestVideoConverter_CanConvert(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		filename string
		expected bool
	}{
		"正常系: MPGファイル":      {"video.mpg", true},
		"正常系: MPEGファイル":     {"video.mpeg", true},
		"正常系: WMVファイル":      {"video.wmv", true},
		"正常系: AVIファイル":      {"video.avi", true},
		"正常系: 大文字拡張子":       {"video.MPG", true},
		"異常系: MP4ファイル(非対応)": {"video.mp4", false},
		"異常系: 画像ファイル":       {"image.png", false},
		"異常系: テキストファイル":     {"document.txt", false},
	}

	c := converter.NewVideoConverter(0, nil)
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, c.CanConvert(filepath.Join(t.TempDir(), tc.filename)))
		})
	}
}

func TestVideoConverter_GetOutputExtension(t *testing.T) {
	t.Parallel()

	c := converter.NewVideoConverter(0, nil)

	cases := map[string]string{
		"正常系: mpg入力でも.mpgを返す":  "video.mpg",
		"正常系: wmv入力でも.mpgを返す":  "video.wmv",
		"正常系: avi入力でも.mpgを返す":  "video.avi",
		"正常系: mpeg入力でも.mpgを返す": "video.mpeg",
	}

	for name, filename := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, ".mpg", c.GetOutputExtension(filename))
		})
	}
}

func TestVideoConverter_IsFFmpegAvailable(t *testing.T) {
	t.Parallel()

	t.Run("正常系: FFmpegが利用可能な場合trueを返す", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().Run(gomock.Any(), "ffmpeg", "-version").Return([]byte("ffmpeg version 6.0"), nil)

		c := converter.NewVideoConverter(0, runner)
		assert.True(t, c.IsFFmpegAvailable())
	})

	t.Run("異常系: FFmpegが見つからない場合falseを返す", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().Run(gomock.Any(), "ffmpeg", "-version").Return(nil, errors.New("executable file not found"))

		c := converter.NewVideoConverter(0, runner)
		assert.False(t, c.IsFFmpegAvailable())
	})

	t.Run("異常系: FFmpegがエラーを返す場合falseを返す", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().Run(gomock.Any(), "ffmpeg", "-version").Return(nil, errors.New("exit status 1"))

		c := converter.NewVideoConverter(0, runner)
		assert.False(t, c.IsFFmpegAvailable())
	})
}

func TestVideoConverter_GetVideoInfo(t *testing.T) {
	t.Parallel()

	t.Run("異常系: ファイルが存在しない場合エラーを返す", func(t *testing.T) {
		t.Parallel()

		c := converter.NewVideoConverter(0, nil)
		_, err := c.GetVideoInfo(filepath.Join(t.TempDir(), "non_existent.mpg"))

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrVideoSourceNotFound)
	})

	t.Run("正常系: 動画情報を正しく取得する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		videoFile := filepath.Join(dir, "test.mpg")
		writeFile(t, videoFile, []byte("dummy video content"))

		probeJSON := `{
			"streams": [
				{"codec_type": "video", "codec_name": "mpeg1video", "width": 640, "height": 480, "r_frame_rate": "25/1"},
				{"codec_type": "audio", "codec_name": "mp2"}
			],
			"format": {"duration": "30.5", "bit_rate": "1500000", "format_name": "mpeg"}
		}`

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", videoFile).
			Return([]byte(probeJSON), nil)

		c := converter.NewVideoConverter(0, runner)
		info, err := c.GetVideoInfo(videoFile)

		require.NoError(t, err)
		assert.Equal(t, 640, info.Width)
		assert.Equal(t, 480, info.Height)
		assert.InDelta(t, 30.5, info.DurationSeconds, 1e-9)
		assert.True(t, info.HasAudio)
		assert.Equal(t, "mpeg1video", info.VideoCodec)
		assert.Equal(t, "mp2", info.AudioCodec)
		assert.Equal(t, int64(1500000), info.Bitrate)
		assert.InDelta(t, 25.0, info.FrameRate, 1e-9)
		assert.Equal(t, "mpeg", info.ContainerFormat)
	})

	t.Run("正常系: 音声ストリームがない動画の情報を取得する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		videoFile := filepath.Join(dir, "test.avi")
		writeFile(t, videoFile, []byte("dummy video content"))

		probeJSON := `{
			"streams": [
				{"codec_type": "video", "codec_name": "msmpeg4v3", "width": 320, "height": 240, "r_frame_rate": "20/1"}
			],
			"format": {"duration": "15.0", "bit_rate": "800000", "format_name": "avi"}
		}`

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", videoFile).
			Return([]byte(probeJSON), nil)

		c := converter.NewVideoConverter(0, runner)
		info, err := c.GetVideoInfo(videoFile)

		require.NoError(t, err)
		assert.False(t, info.HasAudio)
		assert.Empty(t, info.AudioCodec)
		assert.InDelta(t, 20.0, info.FrameRate, 1e-9)
		assert.Equal(t, "avi", info.ContainerFormat)
	})

	t.Run("正常系: r_frame_rateが不正な形式の場合フレームレート0を返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		videoFile := filepath.Join(dir, "test.mpg")
		writeFile(t, videoFile, []byte("dummy video content"))

		probeJSON := `{
			"streams": [
				{"codec_type": "video", "codec_name": "mpeg1video", "width": 320, "height": 240, "r_frame_rate": "0/0"}
			],
			"format": {"duration": "1.0", "format_name": "mpeg"}
		}`

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", videoFile).
			Return([]byte(probeJSON), nil)

		c := converter.NewVideoConverter(0, runner)
		info, err := c.GetVideoInfo(videoFile)

		require.NoError(t, err)
		assert.InDelta(t, 0.0, info.FrameRate, 1e-9)
	})

	t.Run("異常系: 無効な動画ファイルの場合エラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		invalidFile := filepath.Join(dir, "invalid.mpg")
		writeFile(t, invalidFile, []byte("not a video"))

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", invalidFile).
			Return(nil, errors.New("Invalid video file"))

		c := converter.NewVideoConverter(0, runner)
		_, err := c.GetVideoInfo(invalidFile)

		require.ErrorIs(t, err, converter.ErrVideoInfoUnavailable)
		assert.Contains(t, err.Error(), "動画情報を取得できません")
	})

	t.Run("異常系: 動画ストリームが無い場合エラーを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		videoFile := filepath.Join(dir, "audio_only.mpg")
		writeFile(t, videoFile, []byte("dummy content"))

		probeJSON := `{"streams": [{"codec_type": "audio", "codec_name": "mp2"}], "format": {}}`

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", videoFile).
			Return([]byte(probeJSON), nil)

		c := converter.NewVideoConverter(0, runner)
		_, err := c.GetVideoInfo(videoFile)

		require.Error(t, err)
		assert.ErrorIs(t, err, converter.ErrNoVideoStream)
	})
}

// mpeg1Mp2ProbeJSON はAndroid側ランタイムが直接再生できる
// (MPEG-PS + mpeg1video + mp2)ffprobe出力の固定サンプル。
const mpeg1Mp2ProbeJSON = `{
	"streams": [
		{"codec_type": "video", "codec_name": "mpeg1video", "width": 320, "height": 240, "r_frame_rate": "25/1"},
		{"codec_type": "audio", "codec_name": "mp2"}
	],
	"format": {"duration": "1.0", "bit_rate": "100000", "format_name": "mpeg"}
}`

func TestVideoConverter_Convert(t *testing.T) {
	t.Parallel()

	t.Run("異常系: 変換元ファイルが存在しない場合FAILEDを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "non_existent.mpg")
		dest := filepath.Join(dir, "output.mpg")

		c := converter.NewVideoConverter(0, nil)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusFailed, result.Status)
		assert.Equal(t, source, result.SourcePath)
		assert.Contains(t, result.Message, "見つかりません")
	})

	t.Run("正常系: 既にmpeg1video+mp2の場合は再エンコードせずコピーする", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "input.mpg")
		sourceContent := []byte("already mpeg1video mp2 content")
		writeFile(t, source, sourceContent)
		dest := filepath.Join(dir, "output.mpg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", source).
			Return([]byte(mpeg1Mp2ProbeJSON), nil)
		// why: パススルー時はffmpegを一切呼び出さないことを保証する
		// (呼び出されたらこのテストはunexpected callで失敗する)。

		c := converter.NewVideoConverter(0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.Equal(t, dest, result.DestPath)
		assert.Equal(t, sourceContent, readFile(t, dest))
		assert.NoFileExists(t, dest+".tmp")
		assert.FileExists(t, source)
	})

	t.Run("正常系: mpeg1videoでも音声が無い場合はコピーする", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "silent.mpg")
		sourceContent := []byte("mpeg1video only, no audio")
		writeFile(t, source, sourceContent)
		dest := filepath.Join(dir, "output.mpg")

		probeJSON := `{
			"streams": [
				{"codec_type": "video", "codec_name": "mpeg1video", "width": 320, "height": 240, "r_frame_rate": "25/1"}
			],
			"format": {"duration": "1.0", "format_name": "mpeg"}
		}`

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", source).
			Return([]byte(probeJSON), nil)

		c := converter.NewVideoConverter(0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.Equal(t, sourceContent, readFile(t, dest))
	})

	t.Run("異常系: mpeg1video+mp2だがMPEG-PSコンテナでない場合は変換する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "input.avi")
		writeFile(t, source, []byte("mpeg1video mp2 but wrapped in avi"))
		dest := filepath.Join(dir, "output.mpg")

		probeJSON := `{
			"streams": [
				{"codec_type": "video", "codec_name": "mpeg1video", "width": 320, "height": 240, "r_frame_rate": "25/1"},
				{"codec_type": "audio", "codec_name": "mp2"}
			],
			"format": {"duration": "1.0", "format_name": "avi"}
		}`

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", source).
			Return([]byte(probeJSON), nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", "-y", "-i", source, "-r", "25", "-c:v", "mpeg1video",
				"-q:v", "4", "-c:a", "mp2", "-b:a", "224k", "-f", "mpeg", dest+".tmp").
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				writeFile(t, dest+".tmp", []byte("converted mpeg-ps content"))

				return nil, nil
			})

		c := converter.NewVideoConverter(0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.FileExists(t, dest)
		assert.NoFileExists(t, dest+".tmp")
	})

	t.Run("正常系: 非対応コーデックはffmpegでmpeg1video+mp2へ変換する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "input.wmv")
		writeFile(t, source, []byte("dummy wmv content"))
		dest := filepath.Join(dir, "output.mpg")

		probeJSON := `{
			"streams": [
				{"codec_type": "video", "codec_name": "wmv2", "width": 320, "height": 240, "r_frame_rate": "30/1"},
				{"codec_type": "audio", "codec_name": "wmav2"}
			],
			"format": {"duration": "1.0", "format_name": "asf"}
		}`

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", source).
			Return([]byte(probeJSON), nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", "-y", "-i", source, "-r", "30", "-c:v", "mpeg1video",
				"-q:v", "4", "-c:a", "mp2", "-b:a", "224k", "-f", "mpeg", dest+".tmp").
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				writeFile(t, dest+".tmp", []byte("converted video content larger than input"))

				return nil, nil
			})

		c := converter.NewVideoConverter(0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.Equal(t, source, result.SourcePath)
		assert.Equal(t, dest, result.DestPath)
		assert.Positive(t, result.BytesBefore)
		assert.Positive(t, result.BytesAfter)
	})

	t.Run("正常系: 規格外フレームレートは最も近い許容値へスナップする", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "input.avi")
		writeFile(t, source, []byte("15fps source"))
		dest := filepath.Join(dir, "output.mpg")

		probeJSON := `{
			"streams": [
				{"codec_type": "video", "codec_name": "msmpeg4v3", "width": 320, "height": 240, "r_frame_rate": "15/1"}
			],
			"format": {"duration": "1.0", "format_name": "avi"}
		}`

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", source).
			Return([]byte(probeJSON), nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", "-y", "-i", source, "-r", "24000/1001", "-c:v", "mpeg1video",
				"-q:v", "4", "-c:a", "mp2", "-b:a", "224k", "-f", "mpeg", dest+".tmp").
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				writeFile(t, dest+".tmp", []byte("converted content"))

				return nil, nil
			})

		c := converter.NewVideoConverter(0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
	})

	t.Run("正常系: ffprobe判定に失敗した場合は既定フレームレートで変換にフォールバックする", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "input.mpg")
		writeFile(t, source, []byte("dummy video content"))
		dest := filepath.Join(dir, "output.mpg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", source).
			Return(nil, errors.New("ffprobe not found"))
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", "-y", "-i", source, "-r", "25", "-c:v", "mpeg1video",
				"-q:v", "4", "-c:a", "mp2", "-b:a", "224k", "-f", "mpeg", dest+".tmp").
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				writeFile(t, dest+".tmp", []byte("converted content"))

				return nil, nil
			})

		c := converter.NewVideoConverter(0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
	})

	t.Run("異常系: FFmpegがエラーを返す場合FAILEDを返しdestに変更を残さない", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "input.wmv")
		writeFile(t, source, []byte("dummy video content"))
		dest := filepath.Join(dir, "output.mpg")
		writeFile(t, dest, []byte("pre-existing copied raw file"))

		probeJSON := `{
			"streams": [
				{"codec_type": "video", "codec_name": "wmv2", "width": 320, "height": 240, "r_frame_rate": "30/1"}
			],
			"format": {"duration": "1.0", "format_name": "asf"}
		}`

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", source).
			Return([]byte(probeJSON), nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", "-y", "-i", source, "-r", "30", "-c:v", "mpeg1video",
				"-q:v", "4", "-c:a", "mp2", "-b:a", "224k", "-f", "mpeg", dest+".tmp").
			Return(nil, errors.New("FFmpeg error"))

		c := converter.NewVideoConverter(0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusFailed, result.Status)
		assert.Contains(t, result.Message, "動画変換に失敗しました")
		// why: fail-loud設計の検証。ffmpeg失敗時にdestの既存内容(コピー済みの
		// 生ファイル)を0バイトへ破壊してはならない。
		assert.Equal(t, "pre-existing copied raw file", string(readFile(t, dest)))
		assert.NoFileExists(t, dest+".tmp")
	})

	t.Run("異常系: ffmpegが0バイト出力を書いた場合destへ反映せずFAILEDを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "input.wmv")
		writeFile(t, source, []byte("dummy video content"))
		dest := filepath.Join(dir, "output.mpg")
		writeFile(t, dest, []byte("pre-existing copied raw file"))

		probeJSON := `{
			"streams": [
				{"codec_type": "video", "codec_name": "wmv2", "width": 320, "height": 240, "r_frame_rate": "30/1"}
			],
			"format": {"duration": "1.0", "format_name": "asf"}
		}`

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", source).
			Return([]byte(probeJSON), nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", "-y", "-i", source, "-r", "30", "-c:v", "mpeg1video",
				"-q:v", "4", "-c:a", "mp2", "-b:a", "224k", "-f", "mpeg", dest+".tmp").
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				// why: エンコーダがストリームを開けず何も書き込まなかったケースを再現する
				// (実機で観測された0バイト出力バグの再現条件)。
				writeFile(t, dest+".tmp", []byte{})

				return nil, nil
			})

		c := converter.NewVideoConverter(0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusFailed, result.Status)
		assert.Contains(t, result.Message, "0バイト")
		assert.Equal(t, "pre-existing copied raw file", string(readFile(t, dest)))
		assert.NoFileExists(t, dest+".tmp")
	})

	t.Run("正常系: 出力先の親ディレクトリが存在しない場合作成する", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		source := filepath.Join(dir, "input.wmv")
		writeFile(t, source, []byte("dummy video content"))
		dest := filepath.Join(dir, "subdir", "nested", "output.mpg")

		probeJSON := `{
			"streams": [
				{"codec_type": "video", "codec_name": "wmv2", "width": 320, "height": 240, "r_frame_rate": "30/1"}
			],
			"format": {"duration": "1.0", "format_name": "asf"}
		}`

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", source).
			Return([]byte(probeJSON), nil)
		runner.EXPECT().
			Run(gomock.Any(), "ffmpeg", "-y", "-i", source, "-r", "30", "-c:v", "mpeg1video",
				"-q:v", "4", "-c:a", "mp2", "-b:a", "224k", "-f", "mpeg", dest+".tmp").
			DoAndReturn(func(context.Context, string, ...string) ([]byte, error) {
				writeFile(t, dest+".tmp", []byte("converted content"))

				return nil, nil
			})

		c := converter.NewVideoConverter(0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusSuccess, result.Status)
		assert.DirExists(t, filepath.Dir(dest))
	})
}

func TestNewVideoConverter_Defaults(t *testing.T) {
	t.Parallel()

	t.Run("正常系: runnerがnilの場合os/execベースの既定実装が使われクラッシュしない", func(t *testing.T) {
		t.Parallel()

		// why: 既定実装(execCommandRunner)は実プロセスを起動する。CI環境に
		// ffmpegが無い場合もあるため戻り値の真偽は問わず、nilを渡しても
		// パニックせず正常にbool値が返ることのみを確認する
		// （実効的なコマンド呼び出しの検証はCommandRunner注入テストで別途行う）。
		c := converter.NewVideoConverter(3*time.Second, nil)
		assert.NotPanics(t, func() { c.IsFFmpegAvailable() })
	})

	t.Run("正常系: カスタムタイムアウトが利用される", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().Run(gomock.Any(), "ffmpeg", "-version").
			DoAndReturn(func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				assert.WithinDuration(t, time.Now().Add(10*time.Second), deadline, 2*time.Second)

				return []byte("ffmpeg"), nil
			})

		c := converter.NewVideoConverter(10*time.Second, runner)
		assert.True(t, c.IsFFmpegAvailable())
	})
}

func TestCopyFile_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("異常系: 変換元が読み取れない場合はFAILEDを返す", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// why: 読み取り不能な変換元を再現するため、ファイルではなくディレクトリを
		// 変換元として渡す(os.Openはディレクトリを開けるがio.Copyで読み取りエラーになる)。
		source := filepath.Join(dir, "source_dir")
		require.NoError(t, os.Mkdir(source, 0o750))
		dest := filepath.Join(dir, "output.mpg")

		ctrl := gomock.NewController(t)
		runner := NewMockCommandRunner(ctrl)
		runner.EXPECT().
			Run(gomock.Any(), "ffprobe", "-show_format", "-show_streams", "-of", "json", source).
			Return([]byte(mpeg1Mp2ProbeJSON), nil)

		c := converter.NewVideoConverter(0, runner)
		result, err := c.Convert(source, dest)

		require.NoError(t, err)
		assert.Equal(t, converter.StatusFailed, result.Status)
	})
}
