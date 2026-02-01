"""TLG5デコーダーテストモジュール

TLG5デコーダーの各機能をテストする。
"""

import struct

import pytest

from mnemonic.converter.tlg.tlg5 import TLG5Decoder, TLG5Header


class TestTLG5DecoderIsValid:
    """TLG5Decoder.is_valid()メソッドのテスト"""

    @pytest.mark.parametrize(
        "data, expected",
        [
            pytest.param(
                b"TLG5.0\x00raw\x1a" + b"\x00" * 20,
                True,
                id="正常系: 有効なTLG5マジックバイト",
            ),
            pytest.param(
                b"TLG6.0\x00raw\x1a" + b"\x00" * 20,
                False,
                id="異常系: TLG6形式のマジックバイト",
            ),
            pytest.param(
                b"PNG\x89\x50\x4e\x47",
                False,
                id="異常系: PNG形式のマジックバイト",
            ),
            pytest.param(
                b"TLG5.0",
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
        """is_valid()がTLG5形式を正しく判定することを確認する"""
        decoder = TLG5Decoder()
        assert decoder.is_valid(data) == expected


class TestTLG5DecoderParseHeader:
    """TLG5Decoder.parse_header()メソッドのテスト"""

    def _create_tlg5_header(
        self,
        *,
        color_depth: int = 32,
        width: int = 640,
        height: int = 480,
        block_height: int = 4,
    ) -> bytes:
        """TLG5形式のヘッダーを生成する

        Args:
            color_depth: 色深度（24=RGB、32=RGBA）
            width: 画像の幅
            height: 画像の高さ
            block_height: ブロックの高さ

        Returns:
            TLG5形式のヘッダーバイト列
        """
        header = b"TLG5.0\x00raw\x1a"
        header += struct.pack("B", color_depth)
        header += struct.pack("<I", width)
        header += struct.pack("<I", height)
        header += struct.pack("<I", block_height)
        return header

    @pytest.mark.parametrize(
        "color_depth, width, height, block_height, expected_colors",
        [
            pytest.param(
                32,
                640,
                480,
                4,
                4,
                id="正常系: RGBA画像ヘッダー解析",
            ),
            pytest.param(
                24,
                800,
                600,
                4,
                3,
                id="正常系: RGB画像ヘッダー解析",
            ),
            pytest.param(
                32,
                1920,
                1080,
                8,
                4,
                id="正常系: 大きな画像ヘッダー解析",
            ),
            pytest.param(
                24,
                1,
                1,
                1,
                3,
                id="正常系: 最小サイズ画像ヘッダー解析",
            ),
        ],
    )
    def test_parse_header_valid(
        self,
        color_depth: int,
        width: int,
        height: int,
        block_height: int,
        expected_colors: int,
    ) -> None:
        """parse_header()が有効なヘッダーを正しく解析することを確認する"""
        data = self._create_tlg5_header(
            color_depth=color_depth,
            width=width,
            height=height,
            block_height=block_height,
        )
        decoder = TLG5Decoder()
        header = decoder.parse_header(data)

        assert isinstance(header, TLG5Header)
        assert header.width == width
        assert header.height == height
        assert header.colors == expected_colors
        assert header.block_height == block_height

    def test_parse_header_invalid_magic(self) -> None:
        """parse_header()が無効なマジックバイトでValueErrorを発生させることを確認する"""
        data = b"INVALID_MAGIC" + b"\x00" * 20
        decoder = TLG5Decoder()

        with pytest.raises(ValueError, match="TLG5形式ではありません"):
            decoder.parse_header(data)

    def test_parse_header_too_short(self) -> None:
        """parse_header()がデータが短すぎる場合にValueErrorを発生させることを確認する"""
        data = b"TLG5.0\x00raw\x1a" + b"\x00" * 5
        decoder = TLG5Decoder()

        with pytest.raises(ValueError, match="データが短すぎます"):
            decoder.parse_header(data)


class TestTLG5DecoderDecode:
    """TLG5Decoder.decode()メソッドのテスト"""

    def _create_tlg5_header(
        self,
        *,
        color_depth: int = 32,
        width: int = 4,
        height: int = 4,
        block_height: int = 4,
    ) -> bytes:
        """TLG5形式のヘッダーを生成する"""
        header = b"TLG5.0\x00raw\x1a"
        header += struct.pack("B", color_depth)
        header += struct.pack("<I", width)
        header += struct.pack("<I", height)
        header += struct.pack("<I", block_height)
        return header

    def test_decode_invalid_magic(self) -> None:
        """decode()が無効なマジックバイトでValueErrorを発生させることを確認する"""
        data = b"INVALID_MAGIC" + b"\x00" * 20
        decoder = TLG5Decoder()

        with pytest.raises(ValueError, match="TLG5形式ではありません"):
            decoder.decode(data)

    def test_decode_simple_image(self) -> None:
        """decode()が単純なTLG5画像をデコードできることを確認する

        2x2ピクセル、RGBA、block_height=2の最小画像をテスト。
        各チャンネル(B,G,R,A)は別々のブロックとして格納される。
        """
        decoder = TLG5Decoder()

        # 2x2 RGBA画像、block_height=2
        width, height, block_height = 2, 2, 2

        # ヘッダー作成
        header = self._create_tlg5_header(
            color_depth=32,
            width=width,
            height=height,
            block_height=block_height,
        )

        # ブロックデータ作成
        # TLG5はBGRA順で、各チャンネルをブロック単位で格納
        # 2x2画像、block_height=2 → 1ブロック
        # 各チャンネルのデータ: 2x2 = 4バイト
        # デルタエンコーディング: 各行の最初のピクセルは前の行との差分

        # テスト用に単純な画像データを作成
        # 全ピクセルが同じ色 (R=255, G=128, B=64, A=255) のBGRA画像
        # BGRA順: B=64, G=128, R=255, A=255

        block_data = bytearray()

        # 各チャンネルのデータ（B, G, R, A順）
        # デルタエンコーディングでは、各行の最初のピクセルは前行との差分
        # 全ピクセル同じ値なら:
        # - 1行目: [値, 0] (最初のピクセルは直接値、2番目は前との差分=0)
        # - 2行目: [0, 0] (1行目との差分=0)

        # Bチャンネル (値=64)
        b_channel = bytes([64, 0, 0, 0])  # 行1: [64, 0], 行2: [0, 0]
        # Gチャンネル (値=128)
        g_channel = bytes([128, 0, 0, 0])
        # Rチャンネル (値=255)
        r_channel = bytes([255, 0, 0, 0])
        # Aチャンネル (値=255)
        a_channel = bytes([255, 0, 0, 0])

        # 各チャンネルをLZSS非圧縮形式（全リテラル）でエンコード
        # LZSS: フラグ=0xFF (8リテラル) + リテラルバイト
        def create_block(channel_data: bytes) -> bytes:
            """チャンネルデータをブロック形式にパック"""
            # LZSS圧縮データ作成（全リテラル）
            compressed = bytearray()
            pos = 0
            while pos < len(channel_data):
                chunk_size = min(8, len(channel_data) - pos)
                flag = (1 << chunk_size) - 1  # 下位chunk_sizeビットを1に
                compressed.append(flag)
                compressed.extend(channel_data[pos : pos + chunk_size])
                pos += chunk_size

            # mark + block_size + compressed_data
            mark = 0
            block_size = len(compressed)
            return bytes([mark]) + struct.pack("<I", block_size) + bytes(compressed)

        # 各チャンネルのブロックを連結
        block_data.extend(create_block(b_channel))
        block_data.extend(create_block(g_channel))
        block_data.extend(create_block(r_channel))
        block_data.extend(create_block(a_channel))

        # 完全なTLG5データ
        tlg5_data = header + bytes(block_data)

        # デコード
        image = decoder.decode(tlg5_data)

        # 検証
        assert image.width == width
        assert image.height == height
        assert image.mode == "RGBA"

        # ピクセル値を検証（RGBA順）
        expected_pixel = (255, 128, 64, 255)  # R, G, B, A
        for y in range(height):
            for x in range(width):
                pixel = image.getpixel((x, y))
                assert pixel == expected_pixel, f"Expected {expected_pixel}, got {pixel}"

    def test_decode_rgb_image(self) -> None:
        """decode()がRGB（3チャンネル）画像をデコードできることを確認する"""
        decoder = TLG5Decoder()

        # 2x2 RGB画像
        width, height, block_height = 2, 2, 2

        header = self._create_tlg5_header(
            color_depth=24,
            width=width,
            height=height,
            block_height=block_height,
        )

        # 各チャンネルのデータ（B, G, R順）
        b_channel = bytes([64, 0, 0, 0])
        g_channel = bytes([128, 0, 0, 0])
        r_channel = bytes([255, 0, 0, 0])

        def create_block(channel_data: bytes) -> bytes:
            """チャンネルデータをブロック形式にパック"""
            compressed = bytearray()
            pos = 0
            while pos < len(channel_data):
                chunk_size = min(8, len(channel_data) - pos)
                flag = (1 << chunk_size) - 1
                compressed.append(flag)
                compressed.extend(channel_data[pos : pos + chunk_size])
                pos += chunk_size

            mark = 0
            block_size = len(compressed)
            return bytes([mark]) + struct.pack("<I", block_size) + bytes(compressed)

        block_data = bytearray()
        block_data.extend(create_block(b_channel))
        block_data.extend(create_block(g_channel))
        block_data.extend(create_block(r_channel))

        tlg5_data = header + bytes(block_data)
        image = decoder.decode(tlg5_data)

        assert image.width == width
        assert image.height == height
        assert image.mode == "RGB"

        expected_pixel = (255, 128, 64)  # R, G, B
        for y in range(height):
            for x in range(width):
                pixel = image.getpixel((x, y))
                assert pixel == expected_pixel, f"Expected {expected_pixel}, got {pixel}"

    def test_decode_multi_block_image(self) -> None:
        """decode()が複数ブロックの画像をデコードできることを確認する

        4x4画像、block_height=2 → 2ブロック
        """
        decoder = TLG5Decoder()

        width, height, block_height = 4, 4, 2

        header = self._create_tlg5_header(
            color_depth=32,
            width=width,
            height=height,
            block_height=block_height,
        )

        # 各ブロックのチャンネルデータ
        # ブロック1: 行0-1 (4x2=8ピクセル)
        # ブロック2: 行2-3 (4x2=8ピクセル)
        block1_size = width * block_height  # 8バイト
        block2_size = width * block_height  # 8バイト

        # 全ピクセル同じ色 (R=100, G=150, B=200, A=255)
        # デルタエンコーディング: 最初のピクセルのみ値、残りは0
        def create_channel_data(value: int, size: int) -> bytes:
            """チャンネルデータを生成（デルタエンコーディング済み）"""
            data = bytearray(size)
            data[0] = value  # 1行目の最初のピクセル
            # 残りは全て0（同じ値なので差分は0）
            return bytes(data)

        def create_block(channel_data: bytes) -> bytes:
            """チャンネルデータをブロック形式にパック"""
            compressed = bytearray()
            pos = 0
            while pos < len(channel_data):
                chunk_size = min(8, len(channel_data) - pos)
                flag = (1 << chunk_size) - 1
                compressed.append(flag)
                compressed.extend(channel_data[pos : pos + chunk_size])
                pos += chunk_size

            mark = 0
            block_size = len(compressed)
            return bytes([mark]) + struct.pack("<I", block_size) + bytes(compressed)

        block_data = bytearray()

        # ブロック1のチャンネルデータ
        for value in [200, 150, 100, 255]:  # B, G, R, A
            channel = create_channel_data(value, block1_size)
            block_data.extend(create_block(channel))

        # ブロック2のチャンネルデータ
        # 2番目のブロックは前のブロックの最終行との差分
        # 同じ色なので全て0
        for _ in range(4):  # B, G, R, A
            channel = bytes(block2_size)  # 全て0
            block_data.extend(create_block(channel))

        tlg5_data = header + bytes(block_data)
        image = decoder.decode(tlg5_data)

        assert image.width == width
        assert image.height == height
        assert image.mode == "RGBA"

        expected_pixel = (100, 150, 200, 255)  # R, G, B, A
        for y in range(height):
            for x in range(width):
                pixel = image.getpixel((x, y))
                assert pixel == expected_pixel, f"Expected {expected_pixel}, got {pixel}"


class TestTLG5DecoderEdgeCases:
    """TLG5Decoderのエッジケーステスト"""

    def _create_tlg5_header(
        self,
        *,
        color_depth: int = 32,
        width: int = 4,
        height: int = 4,
        block_height: int = 4,
    ) -> bytes:
        """TLG5形式のヘッダーを生成する"""
        header = b"TLG5.0\x00raw\x1a"
        header += struct.pack("B", color_depth)
        header += struct.pack("<I", width)
        header += struct.pack("<I", height)
        header += struct.pack("<I", block_height)
        return header

    def test_decode_truncated_block_data(self) -> None:
        """decode()がブロックデータが不完全な場合にValueErrorを発生させることを確認する"""
        decoder = TLG5Decoder()

        header = self._create_tlg5_header(
            color_depth=32,
            width=2,
            height=2,
            block_height=2,
        )

        # 不完全なブロックデータ（block_sizeは10だが実際は5バイトしかない）
        mark = 0
        block_size = 10
        incomplete_data = bytes([mark]) + struct.pack("<I", block_size) + b"\x00" * 5

        tlg5_data = header + incomplete_data

        with pytest.raises(ValueError, match="ブロックデータが不完全|不正なブロックデータ"):
            decoder.decode(tlg5_data)

    def test_decode_invalid_block_mark(self) -> None:
        """decode()が無効なブロックマークを処理できることを確認する

        mark != 0 の場合の動作を確認
        """
        decoder = TLG5Decoder()

        header = self._create_tlg5_header(
            color_depth=32,
            width=2,
            height=2,
            block_height=2,
        )

        # mark = 1 のブロック（通常は0）
        # 仕様上、mark != 0 の場合は特別な処理が必要な可能性がある
        # ここでは警告なく処理されることを確認

        def create_block_with_mark(channel_data: bytes, mark: int = 0) -> bytes:
            compressed = bytearray()
            pos = 0
            while pos < len(channel_data):
                chunk_size = min(8, len(channel_data) - pos)
                flag = (1 << chunk_size) - 1
                compressed.append(flag)
                compressed.extend(channel_data[pos : pos + chunk_size])
                pos += chunk_size

            block_size = len(compressed)
            return bytes([mark]) + struct.pack("<I", block_size) + bytes(compressed)

        block_data = bytearray()
        for value in [64, 128, 255, 255]:  # B, G, R, A
            channel = bytes([value, 0, 0, 0])
            block_data.extend(create_block_with_mark(channel, mark=1))

        tlg5_data = header + bytes(block_data)

        # mark != 0 でもエラーにならないことを確認
        # 実際の動作は仕様による
        image = decoder.decode(tlg5_data)
        assert image.width == 2
        assert image.height == 2
