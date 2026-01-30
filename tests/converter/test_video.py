"""VideoConverterのテスト"""

import subprocess
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from mnemonic.converter import ConversionStatus, VideoConverter, VideoInfo

class TestVideoInfo:
    """VideoInfoデータクラスのテスト"""

    def test_video_info_creation(self) -> None:
        """正常系: VideoInfoの作成をテスト"""
        info = VideoInfo(
            width=1920,
            height=1080,
            duration_seconds=120.5,
            has_audio=True,
            video_codec="h264",
            audio_codec="aac",
            bitrate=5000000,
        )
        assert info.width == 1920
        assert info.height == 1080
        assert info.duration_seconds == 120.5
        assert info.has_audio is True
        assert info.video_codec == "h264"
        assert info.audio_codec == "aac"
        assert info.bitrate == 5000000

    def test_video_info_without_audio(self) -> None:
        """正常系: 音声なしのVideoInfoの作成をテスト"""
        info = VideoInfo(
            width=640,
            height=480,
            duration_seconds=30.0,
            has_audio=False,
            video_codec="mpeg1video",
            audio_codec=None,
            bitrate=1500000,
        )
        assert info.has_audio is False
        assert info.audio_codec is None

    def test_video_info_is_frozen(self) -> None:
        """正常系: VideoInfoが不変であることをテスト"""
        info = VideoInfo(
            width=1280,
            height=720,
            duration_seconds=60.0,
            has_audio=True,
            video_codec="h264",
            audio_codec="aac",
            bitrate=3000000,
        )
        with pytest.raises(AttributeError):
            info.width = 1920  # type: ignore[misc]

class TestVideoConverterInit:
    """VideoConverterの初期化テスト"""

    def test_default_initialization(self) -> None:
        """正常系: デフォルト値での初期化をテスト"""
        converter = VideoConverter()
        assert converter._video_codec == "libx264"
        assert converter._video_profile == "baseline"
        assert converter._audio_codec == "aac"
        assert converter._timeout == 300

    def test_custom_initialization(self) -> None:
        """正常系: カスタム値での初期化をテスト"""
        converter = VideoConverter(
            video_codec="libx265",
            video_profile="main",
            audio_codec="opus",
            timeout=600,
        )
        assert converter._video_codec == "libx265"
        assert converter._video_profile == "main"
        assert converter._audio_codec == "opus"
        assert converter._timeout == 600

class TestVideoConverterSupportedExtensions:
    """VideoConverter.supported_extensionsのテスト"""

    def test_supported_extensions(self) -> None:
        """正常系: 対応拡張子が正しいことをテスト"""
        converter = VideoConverter()
        extensions = converter.supported_extensions

        assert isinstance(extensions, tuple)
        assert ".mpg" in extensions
        assert ".mpeg" in extensions
        assert ".wmv" in extensions
        assert ".avi" in extensions

    def test_all_extensions_start_with_dot(self) -> None:
        """正常系: すべての拡張子がドットで始まることをテスト"""
        converter = VideoConverter()
        for ext in converter.supported_extensions:
            assert ext.startswith(".")

class TestVideoConverterCanConvert:
    """VideoConverter.can_convertのテスト"""

    @pytest.mark.parametrize(
        "filename,expected",
        [
            pytest.param("video.mpg", True, id="正常系: MPGファイル"),
            pytest.param("video.mpeg", True, id="正常系: MPEGファイル"),
            pytest.param("video.wmv", True, id="正常系: WMVファイル"),
            pytest.param("video.avi", True, id="正常系: AVIファイル"),
            pytest.param("video.MPG", True, id="正常系: 大文字拡張子"),
            pytest.param("video.mp4", False, id="異常系: MP4ファイル（非対応）"),
            pytest.param("image.png", False, id="異常系: 画像ファイル"),
            pytest.param("document.txt", False, id="異常系: テキストファイル"),
        ],
    )
    def test_can_convert(self, tmp_path: Path, filename: str, expected: bool) -> None:
        """can_convertが正しくファイル形式を判定することをテスト"""
        converter = VideoConverter()
        file_path = tmp_path / filename

        result = converter.can_convert(file_path)

        assert result is expected

class TestVideoConverterIsFFmpegAvailable:
    """VideoConverter.is_ffmpeg_availableのテスト"""

    def test_ffmpeg_available(self) -> None:
        """正常系: FFmpegが利用可能な場合Trueを返すことをテスト"""
        converter = VideoConverter()

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0)
            result = converter.is_ffmpeg_available()

        assert result is True
        mock_run.assert_called_once()

    def test_ffmpeg_not_available(self) -> None:
        """異常系: FFmpegが利用不可能な場合Falseを返すことをテスト"""
        converter = VideoConverter()

        with patch("subprocess.run") as mock_run:
            mock_run.side_effect = FileNotFoundError
            result = converter.is_ffmpeg_available()

        assert result is False

    def test_ffmpeg_returns_error(self) -> None:
        """異常系: FFmpegがエラーを返す場合Falseを返すことをテスト"""
        converter = VideoConverter()

        with patch("subprocess.run") as mock_run:
            mock_run.side_effect = subprocess.SubprocessError
            result = converter.is_ffmpeg_available()

        assert result is False

class TestVideoConverterGetVideoInfo:
    """VideoConverter.get_video_infoのテスト"""

    def test_get_video_info_file_not_found(self, tmp_path: Path) -> None:
        """異常系: ファイルが存在しない場合FileNotFoundErrorを発生させることをテスト"""
        converter = VideoConverter()
        non_existent = tmp_path / "non_existent.mpg"

        with pytest.raises(FileNotFoundError):
            converter.get_video_info(non_existent)

    def test_get_video_info_success(self, tmp_path: Path) -> None:
        """正常系: 動画情報を正しく取得することをテスト"""
        converter = VideoConverter()
        video_file = tmp_path / "test.mpg"
        video_file.write_bytes(b"dummy video content")

        mock_probe_result = {
            "streams": [
                {
                    "codec_type": "video",
                    "codec_name": "mpeg1video",
                    "width": 640,
                    "height": 480,
                },
                {
                    "codec_type": "audio",
                    "codec_name": "mp2",
                },
            ],
            "format": {
                "duration": "30.5",
                "bit_rate": "1500000",
            },
        }

        with patch("ffmpeg.probe") as mock_probe:
            mock_probe.return_value = mock_probe_result
            info = converter.get_video_info(video_file)

        assert info.width == 640
        assert info.height == 480
        assert info.duration_seconds == 30.5
        assert info.has_audio is True
        assert info.video_codec == "mpeg1video"
        assert info.audio_codec == "mp2"
        assert info.bitrate == 1500000

    def test_get_video_info_no_audio_stream(self, tmp_path: Path) -> None:
        """正常系: 音声ストリームがない動画の情報を取得することをテスト"""
        converter = VideoConverter()
        video_file = tmp_path / "test.avi"
        video_file.write_bytes(b"dummy video content")

        mock_probe_result = {
            "streams": [
                {
                    "codec_type": "video",
                    "codec_name": "msmpeg4v3",
                    "width": 320,
                    "height": 240,
                },
            ],
            "format": {
                "duration": "15.0",
                "bit_rate": "800000",
            },
        }

        with patch("ffmpeg.probe") as mock_probe:
            mock_probe.return_value = mock_probe_result
            info = converter.get_video_info(video_file)

        assert info.has_audio is False
        assert info.audio_codec is None

    def test_get_video_info_invalid_file(self, tmp_path: Path) -> None:
        """異常系: 無効な動画ファイルの場合ValueErrorを発生させることをテスト"""
        converter = VideoConverter()
        invalid_file = tmp_path / "invalid.mpg"
        invalid_file.write_bytes(b"not a video")

        with patch("ffmpeg.probe") as mock_probe:
            mock_probe.side_effect = Exception("Invalid video file")

            with pytest.raises(ValueError) as exc_info:
                converter.get_video_info(invalid_file)

            assert "動画情報を取得できません" in str(exc_info.value)

class TestVideoConverterConvert:
    """VideoConverter.convertのテスト"""

    def test_convert_source_not_found(self, tmp_path: Path) -> None:
        """異常系: 変換元ファイルが存在しない場合FAILEDを返すことをテスト"""
        converter = VideoConverter()
        source = tmp_path / "non_existent.mpg"
        dest = tmp_path / "output.mp4"

        result = converter.convert(source, dest)

        assert result.status == ConversionStatus.FAILED
        assert result.source_path == source
        assert "見つかりません" in result.message

    def test_convert_success(self, tmp_path: Path) -> None:
        """正常系: 動画変換が成功することをテスト"""
        converter = VideoConverter()
        source = tmp_path / "input.mpg"
        source.write_bytes(b"dummy video content")
        dest = tmp_path / "output.mp4"

        with (
            patch("ffmpeg.input") as mock_input,
            patch("ffmpeg.output") as mock_output,
        ):
            mock_stream = MagicMock()
            mock_input.return_value = mock_stream
            mock_output_stream = MagicMock()
            mock_output.return_value = mock_output_stream
            mock_output_stream.run.return_value = None

            # シミュレートされた出力ファイル
            dest.write_bytes(b"converted video content larger than input")

            result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert result.source_path == source
        assert result.dest_path == dest
        assert result.bytes_before > 0
        assert result.bytes_after > 0

    def test_convert_ffmpeg_error(self, tmp_path: Path) -> None:
        """異常系: FFmpegがエラーを返す場合FAILEDを返すことをテスト"""
        converter = VideoConverter()
        source = tmp_path / "input.mpg"
        source.write_bytes(b"dummy video content")
        dest = tmp_path / "output.mp4"

        with (
            patch("ffmpeg.input") as mock_input,
            patch("ffmpeg.output") as mock_output,
        ):
            mock_stream = MagicMock()
            mock_input.return_value = mock_stream
            mock_output_stream = MagicMock()
            mock_output.return_value = mock_output_stream
            mock_output_stream.run.side_effect = Exception("FFmpeg error")

            result = converter.convert(source, dest)

        assert result.status == ConversionStatus.FAILED
        assert "FFmpeg" in result.message or "変換に失敗" in result.message

    def test_convert_creates_parent_directory(self, tmp_path: Path) -> None:
        """正常系: 出力先の親ディレクトリが存在しない場合作成することをテスト"""
        converter = VideoConverter()
        source = tmp_path / "input.wmv"
        source.write_bytes(b"dummy video content")
        dest = tmp_path / "subdir" / "nested" / "output.mp4"

        with (
            patch("ffmpeg.input") as mock_input,
            patch("ffmpeg.output") as mock_output,
        ):
            mock_stream = MagicMock()
            mock_input.return_value = mock_stream
            mock_output_stream = MagicMock()
            mock_output.return_value = mock_output_stream
            mock_output_stream.run.return_value = None

            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_bytes(b"converted content")

            result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.parent.exists()

    def test_convert_with_custom_codec(self, tmp_path: Path) -> None:
        """正常系: カスタムコーデックでの変換をテスト"""
        converter = VideoConverter(
            video_codec="libx265",
            video_profile="main",
            audio_codec="opus",
        )
        source = tmp_path / "input.avi"
        source.write_bytes(b"dummy video content")
        dest = tmp_path / "output.mp4"

        with (
            patch("ffmpeg.input") as mock_input,
            patch("ffmpeg.output") as mock_output,
        ):
            mock_stream = MagicMock()
            mock_input.return_value = mock_stream
            mock_output_stream = MagicMock()
            mock_output.return_value = mock_output_stream
            mock_output_stream.run.return_value = None

            dest.write_bytes(b"converted content")

            result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        # 出力呼び出しでカスタムコーデックが使用されていることを確認
        mock_output.assert_called_once()
        call_kwargs = mock_output.call_args[1]
        assert call_kwargs.get("vcodec") == "libx265"
        assert call_kwargs.get("acodec") == "opus"

class TestVideoConverterIntegration:
    """VideoConverterの統合テスト（モックなし）"""

    @pytest.mark.skipif(
        not VideoConverter().is_ffmpeg_available(),
        reason="FFmpegが利用不可能",
    )
    def test_real_ffmpeg_check(self) -> None:
        """統合テスト: 実際のFFmpeg確認をテスト"""
        converter = VideoConverter()
        # このテストはFFmpegがインストールされている環境でのみ実行
        assert converter.is_ffmpeg_available() is True
