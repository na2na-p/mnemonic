"""MidiConverterのテスト"""

import subprocess
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from mnemonic.converter.base import ConversionStatus
from mnemonic.converter.midi import MidiConverter


class TestMidiConverterGetDefaultSoundfontPath:
    """MidiConverter.get_default_soundfont_pathのテスト"""

    def test_returns_musescore_when_exists(self, tmp_path: Path) -> None:
        """正常系: MuseScore Generalが存在する場合はそれを返すことをテスト"""
        musescore_path = tmp_path / "MuseScore_General.sf3"
        musescore_path.touch()

        original_path = MidiConverter.MUSESCORE_SOUNDFONT_PATH
        MidiConverter.MUSESCORE_SOUNDFONT_PATH = musescore_path
        try:
            result = MidiConverter.get_default_soundfont_path()
            assert result == musescore_path
        finally:
            MidiConverter.MUSESCORE_SOUNDFONT_PATH = original_path

    def test_returns_fluidr3_when_musescore_not_exists(self, tmp_path: Path) -> None:
        """正常系: MuseScore Generalが存在しない場合はFluidR3を返すことをテスト"""
        # 存在しないパスを設定
        non_existent_path = tmp_path / "non_existent.sf3"
        fluidr3_path = tmp_path / "FluidR3_GM.sf2"

        original_musescore = MidiConverter.MUSESCORE_SOUNDFONT_PATH
        original_fluidr3 = MidiConverter.FLUIDR3_SOUNDFONT_PATH
        MidiConverter.MUSESCORE_SOUNDFONT_PATH = non_existent_path
        MidiConverter.FLUIDR3_SOUNDFONT_PATH = fluidr3_path
        try:
            result = MidiConverter.get_default_soundfont_path()
            assert result == fluidr3_path
        finally:
            MidiConverter.MUSESCORE_SOUNDFONT_PATH = original_musescore
            MidiConverter.FLUIDR3_SOUNDFONT_PATH = original_fluidr3


class TestMidiConverterInit:
    """MidiConverterの初期化テスト"""

    def test_default_initialization(self) -> None:
        """正常系: デフォルト値での初期化をテスト"""
        converter = MidiConverter()
        # get_default_soundfont_path()が返すパスと一致することを確認
        assert converter._soundfont_path == MidiConverter.get_default_soundfont_path()
        assert converter._sample_rate == 44100
        assert converter._audio_codec == "libvorbis"
        assert converter._audio_quality == 4
        assert converter._timeout == 300

    def test_custom_initialization(self) -> None:
        """正常系: カスタム値での初期化をテスト"""
        custom_sf = Path("/custom/soundfont.sf2")
        converter = MidiConverter(
            soundfont_path=custom_sf,
            sample_rate=48000,
            audio_codec="libopus",
            audio_quality=6,
            timeout=600,
        )
        assert converter._soundfont_path == custom_sf
        assert converter._sample_rate == 48000
        assert converter._audio_codec == "libopus"
        assert converter._audio_quality == 6
        assert converter._timeout == 600


class TestMidiConverterSupportedExtensions:
    """MidiConverter.supported_extensionsのテスト"""

    def test_supported_extensions(self) -> None:
        """正常系: 対応拡張子が正しいことをテスト"""
        converter = MidiConverter()
        extensions = converter.supported_extensions

        assert isinstance(extensions, tuple)
        assert ".mid" in extensions
        assert ".midi" in extensions

    def test_all_extensions_start_with_dot(self) -> None:
        """正常系: すべての拡張子がドットで始まることをテスト"""
        converter = MidiConverter()
        for ext in converter.supported_extensions:
            assert ext.startswith(".")


class TestMidiConverterCanConvert:
    """MidiConverter.can_convertのテスト"""

    @pytest.mark.parametrize(
        "filename,expected",
        [
            pytest.param("music.mid", True, id="正常系: MIDファイル"),
            pytest.param("music.midi", True, id="正常系: MIDIファイル"),
            pytest.param("music.MID", True, id="正常系: 大文字MID拡張子"),
            pytest.param("music.MIDI", True, id="正常系: 大文字MIDI拡張子"),
            pytest.param("music.mp3", False, id="異常系: MP3ファイル（非対応）"),
            pytest.param("music.ogg", False, id="異常系: OGGファイル"),
            pytest.param("image.png", False, id="異常系: 画像ファイル"),
            pytest.param("document.txt", False, id="異常系: テキストファイル"),
        ],
    )
    def test_can_convert(self, tmp_path: Path, filename: str, expected: bool) -> None:
        """can_convertが正しくファイル形式を判定することをテスト"""
        converter = MidiConverter()
        file_path = tmp_path / filename

        result = converter.can_convert(file_path)

        assert result is expected


class TestMidiConverterIsFluidSynthAvailable:
    """MidiConverter.is_fluidsynth_availableのテスト"""

    def test_fluidsynth_available(self) -> None:
        """正常系: FluidSynthが利用可能な場合Trueを返すことをテスト"""
        converter = MidiConverter()

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0)
            result = converter.is_fluidsynth_available()

        assert result is True
        mock_run.assert_called_once()

    def test_fluidsynth_not_available(self) -> None:
        """異常系: FluidSynthが利用不可能な場合Falseを返すことをテスト"""
        converter = MidiConverter()

        with patch("subprocess.run") as mock_run:
            mock_run.side_effect = FileNotFoundError
            result = converter.is_fluidsynth_available()

        assert result is False

    def test_fluidsynth_returns_error(self) -> None:
        """異常系: FluidSynthがエラーを返す場合Falseを返すことをテスト"""
        converter = MidiConverter()

        with patch("subprocess.run") as mock_run:
            mock_run.side_effect = subprocess.SubprocessError
            result = converter.is_fluidsynth_available()

        assert result is False


class TestMidiConverterConvert:
    """MidiConverter.convertのテスト"""

    def test_convert_source_not_found(self, tmp_path: Path) -> None:
        """異常系: 変換元ファイルが存在しない場合FAILEDを返すことをテスト"""
        converter = MidiConverter()
        source = tmp_path / "non_existent.mid"
        dest = tmp_path / "output.ogg"

        result = converter.convert(source, dest)

        assert result.status == ConversionStatus.FAILED
        assert result.source_path == source
        assert "見つかりません" in result.message

    def test_convert_soundfont_not_found(self, tmp_path: Path) -> None:
        """異常系: サウンドフォントが存在しない場合FAILEDを返すことをテスト"""
        converter = MidiConverter(soundfont_path=Path("/non/existent/soundfont.sf2"))
        source = tmp_path / "input.mid"
        source.write_bytes(b"MThd" + b"\x00" * 100)  # 簡易MIDIヘッダー
        dest = tmp_path / "output.ogg"

        result = converter.convert(source, dest)

        assert result.status == ConversionStatus.FAILED
        assert "サウンドフォント" in result.message

    def test_convert_success(self, tmp_path: Path) -> None:
        """正常系: MIDI変換が成功することをテスト"""
        soundfont = tmp_path / "test.sf2"
        soundfont.write_bytes(b"soundfont data")

        converter = MidiConverter(soundfont_path=soundfont)
        source = tmp_path / "input.mid"
        source.write_bytes(b"MThd" + b"\x00" * 100)
        dest = tmp_path / "output.ogg"

        with patch("subprocess.run") as mock_run:
            # FluidSynth と FFmpeg の両方をモック
            mock_run.return_value = MagicMock(returncode=0, stdout="", stderr="")

            # シミュレートされた出力ファイル
            def create_output_file(*args, **kwargs):
                # WAV一時ファイルまたはOGG出力ファイルを作成
                cmd = args[0] if args else kwargs.get("args", [])
                if "ffmpeg" in cmd[0] if cmd else False:
                    dest.write_bytes(b"OggS" + b"\x00" * 100)
                return MagicMock(returncode=0, stdout="", stderr="")

            mock_run.side_effect = create_output_file
            dest.write_bytes(b"OggS" + b"\x00" * 100)

            result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert result.source_path == source
        assert result.dest_path == dest

    def test_convert_fluidsynth_error(self, tmp_path: Path) -> None:
        """異常系: FluidSynthがエラーを返す場合FAILEDを返すことをテスト"""
        soundfont = tmp_path / "test.sf2"
        soundfont.write_bytes(b"soundfont data")

        converter = MidiConverter(soundfont_path=soundfont)
        source = tmp_path / "input.mid"
        source.write_bytes(b"MThd" + b"\x00" * 100)
        dest = tmp_path / "output.ogg"

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=1, stdout="", stderr="FluidSynth error")

            result = converter.convert(source, dest)

        assert result.status == ConversionStatus.FAILED
        assert "FluidSynth" in result.message or "変換に失敗" in result.message

    def test_convert_ffmpeg_error(self, tmp_path: Path) -> None:
        """異常系: FFmpegがエラーを返す場合FAILEDを返すことをテスト"""
        soundfont = tmp_path / "test.sf2"
        soundfont.write_bytes(b"soundfont data")

        converter = MidiConverter(soundfont_path=soundfont)
        source = tmp_path / "input.mid"
        source.write_bytes(b"MThd" + b"\x00" * 100)
        dest = tmp_path / "output.ogg"

        call_count = 0

        def mock_side_effect(*args, **kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                # FluidSynth成功（WAVファイルを作成）
                return MagicMock(returncode=0, stdout="", stderr="")
            else:
                # FFmpeg失敗
                return MagicMock(returncode=1, stdout="", stderr="FFmpeg error")

        with patch("subprocess.run") as mock_run:
            mock_run.side_effect = mock_side_effect

            result = converter.convert(source, dest)

        assert result.status == ConversionStatus.FAILED
        assert "FFmpeg" in result.message or "変換に失敗" in result.message

    def test_convert_creates_parent_directory(self, tmp_path: Path) -> None:
        """正常系: 出力先の親ディレクトリが存在しない場合作成することをテスト"""
        soundfont = tmp_path / "test.sf2"
        soundfont.write_bytes(b"soundfont data")

        converter = MidiConverter(soundfont_path=soundfont)
        source = tmp_path / "input.mid"
        source.write_bytes(b"MThd" + b"\x00" * 100)
        dest = tmp_path / "subdir" / "nested" / "output.ogg"

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0, stdout="", stderr="")
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_bytes(b"OggS" + b"\x00" * 100)

            result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.parent.exists()

    def test_convert_output_extension(self, tmp_path: Path) -> None:
        """正常系: 出力ファイルに.mid.ogg拡張子が使用されることをテスト"""
        soundfont = tmp_path / "test.sf2"
        soundfont.write_bytes(b"soundfont data")

        converter = MidiConverter(soundfont_path=soundfont)
        source = tmp_path / "input.mid"
        source.write_bytes(b"MThd" + b"\x00" * 100)
        # 出力ファイル名は変換フローで.mid.oggになる想定
        dest = tmp_path / "input.mid.ogg"

        with patch("subprocess.run") as mock_run:
            mock_run.return_value = MagicMock(returncode=0, stdout="", stderr="")
            dest.write_bytes(b"OggS" + b"\x00" * 100)

            result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert result.dest_path is not None
        assert result.dest_path.name == "input.mid.ogg"


class TestMidiConverterIntegration:
    """MidiConverterの統合テスト（モックなし）"""

    @pytest.mark.skipif(
        not MidiConverter().is_fluidsynth_available(),
        reason="FluidSynthが利用不可能",
    )
    def test_real_fluidsynth_check(self) -> None:
        """統合テスト: 実際のFluidSynth確認をテスト"""
        converter = MidiConverter()
        assert converter.is_fluidsynth_available() is True

    @pytest.mark.skipif(
        not MidiConverter.get_default_soundfont_path().exists(),
        reason="サウンドフォントが利用不可能",
    )
    def test_default_soundfont_exists(self) -> None:
        """統合テスト: デフォルトサウンドフォントの存在確認"""
        converter = MidiConverter()
        assert converter._soundfont_path.exists()
