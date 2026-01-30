"""TLGImageDecoderのテスト"""

import tempfile
from pathlib import Path

import pytest

from mnemonic.converter.image import TLGImageDecoder, TLGVersion

# TLGマジックバイト定数
TLG5_MAGIC = b"TLG5.0\x00raw\x1a"
TLG6_MAGIC = b"TLG6.0\x00raw\x1a"

class TestIsTlgFile:
    """is_tlg_fileメソッドのテスト"""

    @pytest.mark.parametrize(
        "file_content, expected",
        [
            pytest.param(
                TLG5_MAGIC + b"\x00" * 100,
                True,
                id="正常系: TLG5形式のファイルはTrueを返す",
            ),
            pytest.param(
                TLG6_MAGIC + b"\x00" * 100,
                True,
                id="正常系: TLG6形式のファイルはTrueを返す",
            ),
            pytest.param(
                b"PNG\x00\x00\x00" + b"\x00" * 100,
                False,
                id="異常系: PNG形式のファイルはFalseを返す",
            ),
            pytest.param(
                b"JPEG\x00\x00\x00" + b"\x00" * 100,
                False,
                id="異常系: JPEG形式のファイルはFalseを返す",
            ),
            pytest.param(
                b"TLG5",
                False,
                id="異常系: マジックバイトが不完全なファイルはFalseを返す",
            ),
            pytest.param(
                b"",
                False,
                id="異常系: 空ファイルはFalseを返す",
            ),
        ],
    )
    def test_is_tlg_file(self, file_content: bytes, expected: bool) -> None:
        """ファイルがTLG形式かどうかを正しく判定できることを確認"""
        decoder = TLGImageDecoder()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(file_content)
            temp_path = Path(f.name)

        try:
            result = decoder.is_tlg_file(temp_path)
            assert result == expected
        finally:
            temp_path.unlink()

    def test_is_tlg_file_nonexistent_file(self) -> None:
        """存在しないファイルに対してFalseを返すことを確認"""
        decoder = TLGImageDecoder()
        nonexistent_path = Path("/nonexistent/path/to/file.tlg")

        result = decoder.is_tlg_file(nonexistent_path)
        assert result is False

class TestGetInfo:
    """get_infoメソッドのテスト"""

    def test_get_info_raises_not_implemented(self) -> None:
        """get_infoがNotImplementedErrorを発生させることを確認"""
        decoder = TLGImageDecoder()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(TLG5_MAGIC + b"\x00" * 100)
            temp_path = Path(f.name)

        try:
            with pytest.raises(NotImplementedError):
                decoder.get_info(temp_path)
        finally:
            temp_path.unlink()

class TestDecode:
    """decodeメソッドのテスト"""

    def test_decode_raises_not_implemented(self) -> None:
        """decodeがNotImplementedErrorを発生させることを確認"""
        decoder = TLGImageDecoder()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(TLG5_MAGIC + b"\x00" * 100)
            temp_path = Path(f.name)

        try:
            with pytest.raises(NotImplementedError):
                decoder.decode(temp_path)
        finally:
            temp_path.unlink()

class TestDecodeToFile:
    """decode_to_fileメソッドのテスト"""

    def test_decode_to_file_raises_not_implemented(self) -> None:
        """decode_to_fileがNotImplementedErrorを発生させることを確認"""
        decoder = TLGImageDecoder()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(TLG5_MAGIC + b"\x00" * 100)
            temp_path = Path(f.name)

        try:
            dest_path = Path("/tmp/output.png")
            with pytest.raises(NotImplementedError):
                decoder.decode_to_file(temp_path, dest_path)
        finally:
            temp_path.unlink()

class TestTLGVersion:
    """TLGVersionのテスト"""

    def test_tlg_version_values(self) -> None:
        """TLGVersionの値が正しいことを確認"""
        assert TLGVersion.TLG5.value == "TLG5"
        assert TLGVersion.TLG6.value == "TLG6"
        assert TLGVersion.UNKNOWN.value == "UNKNOWN"
