"""LZSS解凍アルゴリズムモジュール

TLG5形式で使用されるLZSS圧縮データの解凍機能を提供する。
"""

from typing import Protocol


class LZSSDecoderProtocol(Protocol):
    """LZSS解凍インターフェース

    LZSS圧縮されたバイト列を解凍するためのプロトコル定義。
    """

    def decode(self, data: bytes, output_size: int) -> bytes:
        """LZSS圧縮データを解凍する

        Args:
            data: LZSS圧縮されたバイト列
            output_size: 解凍後の期待サイズ（バイト）

        Returns:
            解凍されたバイト列

        Raises:
            ValueError: 不正な圧縮データの場合
        """
        ...


class LZSSDecoder:
    """LZSS解凍クラス

    TLG5形式で使用されるLZSS圧縮データを解凍する。
    スライディングウィンドウサイズは4096バイト。
    """

    WINDOW_SIZE: int = 4096
    """スライディングウィンドウのサイズ（バイト）"""

    MATCH_MAX_LENGTH: int = 18
    """最大マッチ長"""

    MATCH_MIN_LENGTH: int = 3
    """最小マッチ長"""

    def decode(self, data: bytes, output_size: int) -> bytes:
        """LZSS圧縮データを解凍する

        Args:
            data: LZSS圧縮されたバイト列
            output_size: 解凍後の期待サイズ（バイト）

        Returns:
            解凍されたバイト列

        Raises:
            ValueError: 不完全または不正な圧縮データの場合
        """
        if output_size == 0:
            return b""

        output = bytearray(output_size)
        output_pos = 0
        input_pos = 0
        data_len = len(data)

        while output_pos < output_size:
            if input_pos >= data_len:
                raise ValueError("不完全な圧縮データ: フラグバイトが不足しています")

            flags = data[input_pos]
            input_pos += 1

            for bit in range(8):
                if output_pos >= output_size:
                    break

                if flags & (1 << bit):
                    # リテラルバイト
                    if input_pos >= data_len:
                        raise ValueError("不完全な圧縮データ: リテラルバイトが不足しています")
                    output[output_pos] = data[input_pos]
                    output_pos += 1
                    input_pos += 1
                else:
                    # マッチ（2バイト）
                    if input_pos + 2 > data_len:
                        raise ValueError("不完全な圧縮データ: マッチ情報が不足しています")

                    # 2バイトからオフセットと長さを抽出
                    # offset[7:0] = 下位バイト
                    # offset[11:8] | length[3:0] = 上位バイト
                    low_byte = data[input_pos]
                    high_byte = data[input_pos + 1]
                    input_pos += 2

                    offset = low_byte | ((high_byte & 0xF0) << 4)
                    length = (high_byte & 0x0F) + self.MATCH_MIN_LENGTH

                    # スライディングウィンドウからコピー
                    for _ in range(length):
                        if output_pos >= output_size:
                            break
                        # オフセットは現在位置からの相対位置
                        src_pos = output_pos - offset
                        if src_pos < 0:
                            # ウィンドウ初期化領域（ゼロで埋められている想定）
                            output[output_pos] = 0
                        else:
                            output[output_pos] = output[src_pos]
                        output_pos += 1

        return bytes(output)
