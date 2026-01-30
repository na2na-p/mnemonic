"""動画変換モジュール

Windows向けゲームの動画アセット(.mpg, .mpeg, .wmv, .avi)を
Android互換のMP4形式に変換するための機能を提供する。
"""

from dataclasses import dataclass
from pathlib import Path

from .base import BaseConverter, ConversionResult

@dataclass(frozen=True)
class VideoInfo:
    """動画情報を表すデータクラス

    動画ファイルのメタデータを保持する不変データクラス。

    Attributes:
        width: 動画の幅（ピクセル）
        height: 動画の高さ（ピクセル）
        duration_seconds: 動画の長さ（秒）
        has_audio: 音声トラックの有無
        video_codec: 動画コーデック名
        audio_codec: 音声コーデック名（音声なしの場合はNone）
        bitrate: ビットレート（bps）
    """

    width: int
    height: int
    duration_seconds: float
    has_audio: bool
    video_codec: str
    audio_codec: str | None
    bitrate: int

class VideoConverter(BaseConverter):
    """動画ファイルをAndroid互換形式に変換するConverter

    FFmpegを使用して、Windows向けゲームで使用される動画形式
    (.mpg, .mpeg, .wmv, .avi)をH.264/AACのMP4形式に変換する。
    """

    def __init__(
        self,
        video_codec: str = "libx264",
        video_profile: str = "baseline",
        audio_codec: str = "aac",
        timeout: int = 300,
    ) -> None:
        """VideoConverterを初期化する

        Args:
            video_codec: 使用する動画コーデック（デフォルト: libx264）
            video_profile: H.264のプロファイル（デフォルト: baseline）
            audio_codec: 使用する音声コーデック（デフォルト: aac）
            timeout: FFmpeg実行のタイムアウト秒数（デフォルト: 300秒）
        """
        self._video_codec = video_codec
        self._video_profile = video_profile
        self._audio_codec = audio_codec
        self._timeout = timeout

    @property
    def supported_extensions(self) -> tuple[str, ...]:
        """対応する拡張子のタプルを返す

        Returns:
            対応する動画ファイル拡張子のタプル
        """
        return (".mpg", ".mpeg", ".wmv", ".avi")

    def can_convert(self, file_path: Path) -> bool:
        """このConverterで変換可能なファイルかを判定する

        Args:
            file_path: 判定対象のファイルパス

        Returns:
            変換可能な場合True、そうでない場合False
        """
        raise NotImplementedError

    def convert(self, source: Path, dest: Path) -> ConversionResult:
        """動画ファイルをMP4形式に変換する

        Args:
            source: 変換元動画ファイルのパス
            dest: 変換先ファイルのパス

        Returns:
            変換結果を表すConversionResultオブジェクト
        """
        raise NotImplementedError

    def get_video_info(self, file_path: Path) -> VideoInfo:
        """動画ファイルの情報を取得する

        Args:
            file_path: 動画ファイルのパス

        Returns:
            動画情報を表すVideoInfoオブジェクト

        Raises:
            FileNotFoundError: ファイルが存在しない場合
            ValueError: 動画情報を取得できない場合
        """
        raise NotImplementedError

    def is_ffmpeg_available(self) -> bool:
        """FFmpegが利用可能かを確認する

        Returns:
            FFmpegが利用可能な場合True、そうでない場合False
        """
        raise NotImplementedError
