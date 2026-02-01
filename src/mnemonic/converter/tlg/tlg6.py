"""TLG6デコーダーモジュール

TLG6形式の画像をデコードする機能を提供する。
TLG6はゴロム・ライス符号とフィルタリングを使用した可逆圧縮形式。
"""

from dataclasses import dataclass
from typing import Protocol

from PIL import Image


class TLG6DecoderProtocol(Protocol):
    """TLG6デコーダーインターフェース

    TLG6形式の画像をデコードするためのプロトコル定義。
    """

    def decode(self, data: bytes) -> Image.Image:
        """TLG6形式のバイト列をデコードする

        Args:
            data: TLG6形式の画像バイト列

        Returns:
            デコードされたPIL.Imageオブジェクト

        Raises:
            ValueError: 不正なTLG6データの場合
        """
        ...


@dataclass(frozen=True)
class TLG6Header:
    """TLG6ヘッダー情報

    TLG6画像ファイルのヘッダーから読み取った情報を保持する不変データクラス。

    Attributes:
        width: 画像の幅（ピクセル）
        height: 画像の高さ（ピクセル）
        colors: 色数（3=RGB、4=RGBA）
        data_flags: データフラグ
        filter_types: フィルタタイプ配列長
        x_block_count: 水平方向のブロック数
        y_block_count: 垂直方向のブロック数
    """

    width: int
    height: int
    colors: int
    data_flags: int
    filter_types: int
    x_block_count: int
    y_block_count: int


class TLG6Decoder:
    """TLG6画像デコーダー

    TLG6形式の画像をデコードしてPIL.Imageオブジェクトに変換する。
    TLG6はゴロム・ライス符号によるエントロピー符号化と、
    複数のフィルタリング手法を組み合わせた可逆圧縮形式。
    """

    MAGIC: bytes = b"TLG6.0\x00raw\x1a"
    """TLG6形式のマジックバイト"""

    def is_valid(self, data: bytes) -> bool:
        """指定されたデータがTLG6形式かどうかを判定する

        Args:
            data: 判定対象のバイト列

        Returns:
            TLG6形式の場合True、そうでない場合False
        """
        return data.startswith(self.MAGIC)

    def decode(self, data: bytes) -> Image.Image:
        """TLG6形式のバイト列をデコードする

        Args:
            data: TLG6形式の画像バイト列

        Returns:
            デコードされたPIL.Imageオブジェクト

        Raises:
            NotImplementedError: このメソッドは未実装
            ValueError: TLG6形式でないデータの場合
        """
        if not self.is_valid(data):
            raise ValueError("TLG6形式ではありません")
        raise NotImplementedError("TLG6Decoder.decode()は未実装です")

    def parse_header(self, data: bytes) -> TLG6Header:
        """TLG6ヘッダーを解析する

        Args:
            data: TLG6形式の画像バイト列

        Returns:
            解析されたヘッダー情報

        Raises:
            ValueError: TLG6形式でないデータ、またはデータが短すぎる場合
        """
        if not self.is_valid(data):
            raise ValueError("TLG6形式ではありません")

        # ヘッダーの最小サイズ = 29バイト
        # マジック(11) + 色深度(1) + フラグ(1) + width(4) + height(4) + x_block(4) + y_block(4)
        min_header_size = 11 + 1 + 1 + 4 + 4 + 4 + 4
        if len(data) < min_header_size:
            raise ValueError("データが短すぎます")

        offset = len(self.MAGIC)

        # 色深度: 24=RGB(3色), 32=RGBA(4色)
        color_depth = data[offset]
        colors = 4 if color_depth == 32 else 3
        offset += 1

        # データフラグ
        data_flags = data[offset]
        offset += 1

        # 画像サイズ（リトルエンディアン）
        width = int.from_bytes(data[offset : offset + 4], "little")
        offset += 4
        height = int.from_bytes(data[offset : offset + 4], "little")
        offset += 4

        # ブロック数（リトルエンディアン）
        x_block_count = int.from_bytes(data[offset : offset + 4], "little")
        offset += 4
        y_block_count = int.from_bytes(data[offset : offset + 4], "little")

        return TLG6Header(
            width=width,
            height=height,
            colors=colors,
            data_flags=data_flags,
            filter_types=0,
            x_block_count=x_block_count,
            y_block_count=y_block_count,
        )
