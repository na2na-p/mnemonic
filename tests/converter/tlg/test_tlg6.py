"""TLG6デコーダーテストモジュール

TLG6デコーダーの各機能をテストする。
"""

import struct

import pytest

from mnemonic.converter.tlg.tlg6 import TLG6Decoder, TLG6Header


class TestTLG6DecoderIsValid:
    """TLG6Decoder.is_valid()メソッドのテスト"""

    @pytest.mark.parametrize(
        "data, expected",
        [
            pytest.param(
                b"TLG6.0\x00raw\x1a" + b"\x00" * 20,
                True,
                id="正常系: 有効なTLG6マジックバイト",
            ),
            pytest.param(
                b"TLG5.0\x00raw\x1a" + b"\x00" * 20,
                False,
                id="異常系: TLG5形式のマジックバイト",
            ),
            pytest.param(
                b"PNG\x89\x50\x4e\x47",
                False,
                id="異常系: PNG形式のマジックバイト",
            ),
            pytest.param(
                b"TLG6.0",
                False,
                id="異常系: 不完全なマジックバイト",
            ),
            pytest.param(
                b"",
                False,
                id="異常系: 空のデータ",
            ),
        ],
    )
    def test_is_valid(self, data: bytes, expected: bool) -> None:
        """is_valid()がTLG6形式を正しく判定することを確認する"""
        decoder = TLG6Decoder()
        assert decoder.is_valid(data) == expected


class TestTLG6DecoderParseHeader:
    """TLG6Decoder.parse_header()メソッドのテスト"""

    def _create_tlg6_data(
        self,
        *,
        colors: int = 32,
        data_flags: int = 0,
        width: int = 640,
        height: int = 480,
        x_block_count: int = 80,
        y_block_count: int = 60,
    ) -> bytes:
        """TLG6形式のテストデータを生成する

        Args:
            colors: 色深度（24=RGB、32=RGBA）
            data_flags: データフラグ
            width: 画像の幅
            height: 画像の高さ
            x_block_count: 水平方向のブロック数
            y_block_count: 垂直方向のブロック数

        Returns:
            TLG6形式のヘッダーを含むバイト列
        """
        header = b"TLG6.0\x00raw\x1a"
        header += struct.pack("B", colors)
        header += struct.pack("B", data_flags)
        header += struct.pack("<I", width)
        header += struct.pack("<I", height)
        header += struct.pack("<I", x_block_count)
        header += struct.pack("<I", y_block_count)
        return header

    @pytest.mark.parametrize(
        "colors, data_flags, width, height, x_block_count, y_block_count",
        [
            pytest.param(
                32,
                0,
                640,
                480,
                80,
                60,
                id="正常系: RGBA画像ヘッダー解析",
            ),
            pytest.param(
                24,
                0,
                800,
                600,
                100,
                75,
                id="正常系: RGB画像ヘッダー解析",
            ),
            pytest.param(
                32,
                1,
                1920,
                1080,
                240,
                135,
                id="正常系: フラグ付きRGBA画像ヘッダー解析",
            ),
            pytest.param(
                24,
                0,
                1,
                1,
                1,
                1,
                id="正常系: 最小サイズ画像ヘッダー解析",
            ),
        ],
    )
    def test_parse_header_valid(
        self,
        colors: int,
        data_flags: int,
        width: int,
        height: int,
        x_block_count: int,
        y_block_count: int,
    ) -> None:
        """parse_header()が有効なヘッダーを正しく解析することを確認する"""
        data = self._create_tlg6_data(
            colors=colors,
            data_flags=data_flags,
            width=width,
            height=height,
            x_block_count=x_block_count,
            y_block_count=y_block_count,
        )
        decoder = TLG6Decoder()
        header = decoder.parse_header(data)

        expected_colors = 4 if colors == 32 else 3
        assert isinstance(header, TLG6Header)
        assert header.width == width
        assert header.height == height
        assert header.colors == expected_colors
        assert header.data_flags == data_flags
        assert header.x_block_count == x_block_count
        assert header.y_block_count == y_block_count

    def test_parse_header_invalid_magic(self) -> None:
        """parse_header()が無効なマジックバイトでValueErrorを発生させることを確認する"""
        data = b"INVALID_MAGIC" + b"\x00" * 20
        decoder = TLG6Decoder()

        with pytest.raises(ValueError, match="TLG6形式ではありません"):
            decoder.parse_header(data)

    def test_parse_header_too_short(self) -> None:
        """parse_header()がデータが短すぎる場合にValueErrorを発生させることを確認する"""
        data = b"TLG6.0\x00raw\x1a" + b"\x00" * 5
        decoder = TLG6Decoder()

        with pytest.raises(ValueError, match="データが短すぎます"):
            decoder.parse_header(data)


class TestTLG6DecoderDecode:
    """TLG6Decoder.decode()メソッドのテスト"""

    def test_decode_invalid_magic(self) -> None:
        """decode()が無効なマジックバイトでValueErrorを発生させることを確認する"""
        data = b"INVALID_MAGIC" + b"\x00" * 20
        decoder = TLG6Decoder()

        with pytest.raises(ValueError, match="TLG6形式ではありません"):
            decoder.decode(data)
