"""TLG5デコーダーモジュール

TLG5形式の画像をデコードする機能を提供する。
TLG5はLZSS圧縮を使用したカラープレーン分離方式の画像フォーマット。
"""

from dataclasses import dataclass
from typing import Protocol

from PIL import Image

from mnemonic.converter.tlg.lzss import LZSSDecoder


class TLG5DecoderProtocol(Protocol):
    """TLG5デコーダーインターフェース

    TLG5形式の画像をデコードするためのプロトコル定義。
    """

    def decode(self, data: bytes) -> Image.Image:
        """TLG5形式のバイト列をデコードする

        Args:
            data: TLG5形式の画像バイト列

        Returns:
            デコードされたPIL.Imageオブジェクト

        Raises:
            ValueError: 不正なTLG5データの場合
        """
        ...


@dataclass(frozen=True)
class TLG5Header:
    """TLG5ヘッダー情報

    TLG5画像ファイルのヘッダーから読み取った情報を保持する不変データクラス。

    Attributes:
        width: 画像の幅（ピクセル）
        height: 画像の高さ（ピクセル）
        colors: 色数（3=RGB、4=RGBA）
        block_height: ブロックの高さ（ピクセル）
    """

    width: int
    height: int
    colors: int
    block_height: int


class TLG5Decoder:
    """TLG5画像デコーダー

    TLG5形式の画像をデコードしてPIL.Imageオブジェクトに変換する。
    TLG5はLZSS圧縮を使用し、カラープレーン分離方式でデータを格納する。

    TLG5形式の構造:
    - ヘッダー: マジック(11) + 色深度(1) + width(4) + height(4) + block_height(4)
    - データ: 各ブロックごとに mark(1) + block_size(4) + compressed_data
    - 各ブロックは各色チャンネル(B,G,R,A)ごとに格納
    - デルタエンコーディング（行差分）を使用
    """

    MAGIC: bytes = b"TLG5.0\x00raw\x1a"
    """TLG5形式のマジックバイト"""

    HEADER_SIZE: int = 24
    """ヘッダーサイズ: マジック(11) + 色深度(1) + width(4) + height(4) + block_height(4)"""

    def __init__(self) -> None:
        """TLG5デコーダーを初期化する"""
        self._lzss = LZSSDecoder()

    def is_valid(self, data: bytes) -> bool:
        """指定されたデータがTLG5形式かどうかを判定する

        Args:
            data: 判定対象のバイト列

        Returns:
            TLG5形式の場合True、そうでない場合False
        """
        return data.startswith(self.MAGIC)

    def parse_header(self, data: bytes) -> TLG5Header:
        """TLG5ヘッダーを解析する

        Args:
            data: TLG5形式の画像バイト列

        Returns:
            解析されたヘッダー情報

        Raises:
            ValueError: TLG5形式でないデータ、またはデータが短すぎる場合
        """
        if not self.is_valid(data):
            raise ValueError("TLG5形式ではありません")

        if len(data) < self.HEADER_SIZE:
            raise ValueError("データが短すぎます")

        offset = len(self.MAGIC)

        # 色深度: 24=RGB(3色), 32=RGBA(4色)
        color_depth = data[offset]
        colors = 4 if color_depth == 32 else 3
        offset += 1

        # 画像サイズ（リトルエンディアン）
        width = int.from_bytes(data[offset : offset + 4], "little")
        offset += 4
        height = int.from_bytes(data[offset : offset + 4], "little")
        offset += 4

        # ブロック高さ（リトルエンディアン）
        block_height = int.from_bytes(data[offset : offset + 4], "little")

        return TLG5Header(
            width=width,
            height=height,
            colors=colors,
            block_height=block_height,
        )

    def decode(self, data: bytes) -> Image.Image:
        """TLG5形式のバイト列をデコードする

        Args:
            data: TLG5形式の画像バイト列

        Returns:
            デコードされたPIL.Imageオブジェクト

        Raises:
            ValueError: TLG5形式でないデータ、または不正なデータの場合
        """
        if not self.is_valid(data):
            raise ValueError("TLG5形式ではありません")

        header = self.parse_header(data)
        width = header.width
        height = header.height
        colors = header.colors
        block_height = header.block_height

        # ブロック数を計算
        block_count = (height + block_height - 1) // block_height

        # 各チャンネルのピクセルデータを格納
        # TLG5はBGRA順で格納される
        channels: list[bytearray] = [bytearray(width * height) for _ in range(colors)]

        # データ読み取り開始位置
        offset = self.HEADER_SIZE

        # 各ブロックを処理
        for block_idx in range(block_count):
            # このブロックの開始行と行数
            block_y_start = block_idx * block_height
            block_rows = min(block_height, height - block_y_start)
            block_pixel_count = width * block_rows

            # 各チャンネルのブロックデータを読み取り
            for channel_idx in range(colors):
                # ブロックヘッダーを読み取り: mark(1) + block_size(4)
                if offset + 5 > len(data):
                    raise ValueError("ブロックデータが不完全です")

                # mark は通常0（未使用フラグ）
                offset += 1

                block_size = int.from_bytes(data[offset : offset + 4], "little")
                offset += 4

                # 圧縮データを読み取り
                if offset + block_size > len(data):
                    raise ValueError("ブロックデータが不完全です")

                compressed_data = data[offset : offset + block_size]
                offset += block_size

                # LZSS解凍
                decompressed = self._lzss.decode(compressed_data, block_pixel_count)

                # デルタエンコーディングを復元
                self._apply_delta_decoding(
                    channels[channel_idx],
                    decompressed,
                    width,
                    block_y_start,
                    block_rows,
                )

        # BGRAからRGBA/RGBに変換してPIL Imageを作成
        return self._create_image(channels, width, height, colors)

    def _apply_delta_decoding(
        self,
        channel: bytearray,
        delta_data: bytes,
        width: int,
        y_start: int,
        rows: int,
    ) -> None:
        """デルタエンコーディングされたデータを復元する

        TLG5のデルタエンコーディング:
        - 各行の最初のピクセルは前の行の最初のピクセルとの差分
        - 行内の各ピクセルは同じ行の前のピクセルとの差分

        Args:
            channel: 復元先のチャンネルデータ
            delta_data: デルタエンコーディングされたデータ
            width: 画像の幅
            y_start: ブロックの開始行
            rows: ブロックの行数
        """
        delta_pos = 0

        for row in range(rows):
            y = y_start + row
            row_offset = y * width

            for x in range(width):
                delta_value = delta_data[delta_pos]
                delta_pos += 1

                if x == 0:
                    # 各行の最初のピクセル: 前の行の最初のピクセルとの差分
                    if y == 0:
                        # 最初の行は直接値
                        channel[row_offset] = delta_value
                    else:
                        prev_row_offset = (y - 1) * width
                        channel[row_offset] = (channel[prev_row_offset] + delta_value) & 0xFF
                else:
                    # 行内の他のピクセル: 同じ行の前のピクセルとの差分
                    channel[row_offset + x] = (channel[row_offset + x - 1] + delta_value) & 0xFF

    def _create_image(
        self,
        channels: list[bytearray],
        width: int,
        height: int,
        colors: int,
    ) -> Image.Image:
        """チャンネルデータからPIL Imageを作成する

        TLG5はBGRA順で格納されているため、RGBA/RGB順に変換する。

        Args:
            channels: 各チャンネルのピクセルデータ（BGRA順）
            width: 画像の幅
            height: 画像の高さ
            colors: 色数（3=RGB、4=RGBA）

        Returns:
            作成されたPIL Imageオブジェクト
        """
        pixel_count = width * height

        if colors == 4:
            # BGRA -> RGBA
            rgba_data = bytearray(pixel_count * 4)
            for i in range(pixel_count):
                rgba_data[i * 4] = channels[2][i]  # R <- B channel position 2
                rgba_data[i * 4 + 1] = channels[1][i]  # G <- G channel position 1
                rgba_data[i * 4 + 2] = channels[0][i]  # B <- R channel position 0
                rgba_data[i * 4 + 3] = channels[3][i]  # A <- A channel position 3
            return Image.frombytes("RGBA", (width, height), bytes(rgba_data))
        else:
            # BGR -> RGB
            rgb_data = bytearray(pixel_count * 3)
            for i in range(pixel_count):
                rgb_data[i * 3] = channels[2][i]  # R <- B channel position 2
                rgb_data[i * 3 + 1] = channels[1][i]  # G <- G channel position 1
                rgb_data[i * 3 + 2] = channels[0][i]  # B <- R channel position 0
            return Image.frombytes("RGB", (width, height), bytes(rgb_data))
