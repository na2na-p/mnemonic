"""MIDI変換モジュール

MIDIファイルをAndroid互換のOGG Vorbis形式に変換するための機能を提供する。
FluidSynthを使用してMIDIをWAVにレンダリングし、FFmpegでOGGに変換する。
"""

import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path

from .base import BaseConverter, ConversionResult, ConversionStatus


@dataclass(frozen=True)
class MidiConversionConfig:
    """MIDI変換設定を表すデータクラス

    MIDI変換処理のパラメータを保持する不変データクラス。

    Attributes:
        soundfont_path: サウンドフォントファイルのパス
        sample_rate: 出力サンプルレート（Hz）
        audio_codec: 使用する音声コーデック
        audio_quality: 音声品質（0-10、VBR用）
    """

    soundfont_path: Path
    sample_rate: int
    audio_codec: str
    audio_quality: int


class MidiConverter(BaseConverter):
    """MIDIファイルをOGG Vorbis形式に変換するConverter

    FluidSynthを使用してMIDIをWAVにレンダリングし、
    FFmpegでOGG Vorbis形式に変換する。

    変換フロー:
    1. FluidSynth + SoundFont で MIDI → WAV
    2. FFmpeg で WAV → OGG Vorbis
    """

    DEFAULT_SOUNDFONT_PATH = Path("/usr/share/sounds/sf2/FluidR3_GM.sf2")

    def __init__(
        self,
        soundfont_path: Path | None = None,
        sample_rate: int = 44100,
        audio_codec: str = "libvorbis",
        audio_quality: int = 4,
        timeout: int = 300,
    ) -> None:
        """MidiConverterを初期化する

        Args:
            soundfont_path: サウンドフォントファイルのパス（デフォルト: FluidR3_GM.sf2）
            sample_rate: 出力サンプルレート（デフォルト: 44100 Hz）
            audio_codec: 使用する音声コーデック（デフォルト: libvorbis）
            audio_quality: 音声品質（デフォルト: 4、VBR用）
            timeout: 変換処理のタイムアウト秒数（デフォルト: 300秒）
        """
        self._soundfont_path = soundfont_path or self.DEFAULT_SOUNDFONT_PATH
        self._sample_rate = sample_rate
        self._audio_codec = audio_codec
        self._audio_quality = audio_quality
        self._timeout = timeout

    @property
    def supported_extensions(self) -> tuple[str, ...]:
        """対応する拡張子のタプルを返す

        Returns:
            対応するMIDIファイル拡張子のタプル
        """
        return (".mid", ".midi")

    def can_convert(self, file_path: Path) -> bool:
        """このConverterで変換可能なファイルかを判定する

        Args:
            file_path: 判定対象のファイルパス

        Returns:
            変換可能な場合True、そうでない場合False
        """
        return file_path.suffix.lower() in self.supported_extensions

    def convert(self, source: Path, dest: Path) -> ConversionResult:
        """MIDIファイルをOGG Vorbis形式に変換する

        Args:
            source: 変換元MIDIファイルのパス
            dest: 変換先ファイルのパス

        Returns:
            変換結果を表すConversionResultオブジェクト
        """
        if not source.exists():
            return ConversionResult(
                source_path=source,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message=f"変換元ファイルが見つかりません: {source}",
            )

        if not self._soundfont_path.exists():
            return ConversionResult(
                source_path=source,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message=f"サウンドフォントが見つかりません: {self._soundfont_path}",
            )

        bytes_before = self._get_file_size(source)

        dest.parent.mkdir(parents=True, exist_ok=True)

        try:
            # 一時WAVファイルを作成
            with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp_wav:
                tmp_wav_path = Path(tmp_wav.name)

            try:
                # Step 1: FluidSynth で MIDI → WAV
                fluidsynth_result = self._run_fluidsynth(source, tmp_wav_path)
                if fluidsynth_result is not None:
                    return fluidsynth_result

                # Step 2: FFmpeg で WAV → OGG
                ffmpeg_result = self._run_ffmpeg(tmp_wav_path, dest)
                if ffmpeg_result is not None:
                    return ffmpeg_result

            finally:
                # 一時ファイルをクリーンアップ
                if tmp_wav_path.exists():
                    tmp_wav_path.unlink()

            bytes_after = self._get_file_size(dest)

            return ConversionResult(
                source_path=source,
                dest_path=dest,
                status=ConversionStatus.SUCCESS,
                bytes_before=bytes_before,
                bytes_after=bytes_after,
            )

        except Exception as e:
            return ConversionResult(
                source_path=source,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message=f"MIDI変換に失敗しました: {e}",
            )

    def _run_fluidsynth(self, source: Path, wav_output: Path) -> ConversionResult | None:
        """FluidSynthを実行してMIDIをWAVに変換する

        Args:
            source: 入力MIDIファイルのパス
            wav_output: 出力WAVファイルのパス

        Returns:
            エラー時はConversionResult、成功時はNone
        """
        cmd = [
            "fluidsynth",
            "-ni",  # No interactive mode, no shell
            "-g",
            "1.0",  # Gain
            "-r",
            str(self._sample_rate),  # Sample rate
            "-F",
            str(wav_output),  # Output file
            str(self._soundfont_path),
            str(source),
        ]

        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=self._timeout,
            )

            if result.returncode != 0:
                return ConversionResult(
                    source_path=source,
                    dest_path=None,
                    status=ConversionStatus.FAILED,
                    message=f"FluidSynth変換に失敗しました: {result.stderr}",
                )

            return None

        except subprocess.TimeoutExpired:
            return ConversionResult(
                source_path=source,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message=f"FluidSynth処理がタイムアウトしました（{self._timeout}秒）",
            )
        except FileNotFoundError:
            return ConversionResult(
                source_path=source,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message="FluidSynthが見つかりません。インストールしてください。",
            )

    def _run_ffmpeg(self, wav_input: Path, ogg_output: Path) -> ConversionResult | None:
        """FFmpegを実行してWAVをOGGに変換する

        Args:
            wav_input: 入力WAVファイルのパス
            ogg_output: 出力OGGファイルのパス

        Returns:
            エラー時はConversionResult、成功時はNone
        """
        cmd = [
            "ffmpeg",
            "-y",  # Overwrite output
            "-i",
            str(wav_input),
            "-c:a",
            self._audio_codec,
            "-q:a",
            str(self._audio_quality),
            str(ogg_output),
        ]

        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=self._timeout,
            )

            if result.returncode != 0:
                return ConversionResult(
                    source_path=wav_input,
                    dest_path=None,
                    status=ConversionStatus.FAILED,
                    message=f"FFmpeg変換に失敗しました: {result.stderr}",
                )

            return None

        except subprocess.TimeoutExpired:
            return ConversionResult(
                source_path=wav_input,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message=f"FFmpeg処理がタイムアウトしました（{self._timeout}秒）",
            )
        except FileNotFoundError:
            return ConversionResult(
                source_path=wav_input,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message="FFmpegが見つかりません。インストールしてください。",
            )

    def is_fluidsynth_available(self) -> bool:
        """FluidSynthが利用可能かを確認する

        Returns:
            FluidSynthが利用可能な場合True、そうでない場合False
        """
        try:
            subprocess.run(
                ["fluidsynth", "--version"],
                capture_output=True,
                check=True,
            )
            return True
        except (FileNotFoundError, subprocess.SubprocessError):
            return False
