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

        TLG5形式のLZSS解凍を行う。4096バイトのスライドバッファを使用。

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
        # スライドバッファ（0で初期化）
        slide = bytearray(self.WINDOW_SIZE)
        slide_pos = 0

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
                    # TLG5: ビット1 = マッチ（バックリファレンス）
                    if input_pos + 2 > data_len:
                        raise ValueError("不完全な圧縮データ: マッチ情報が不足しています")

                    # 2バイトからマッチ位置と長さを抽出
                    low_byte = data[input_pos]
                    high_byte = data[input_pos + 1]
                    input_pos += 2

                    mpos = low_byte | ((high_byte & 0x0F) << 8)
                    mlen = ((high_byte >> 4) & 0x0F) + self.MATCH_MIN_LENGTH

                    # mlen == 18の場合は追加バイトを読む
                    if mlen == 18:
                        if input_pos >= data_len:
                            raise ValueError("不完全な圧縮データ: 追加長バイトが不足しています")
                        mlen += data[input_pos]
                        input_pos += 1

                    # スライドバッファからコピー
                    for _ in range(mlen):
                        if output_pos >= output_size:
                            break
                        byte_val = slide[mpos]
                        output[output_pos] = byte_val
                        slide[slide_pos] = byte_val
                        output_pos += 1
                        mpos = (mpos + 1) & (self.WINDOW_SIZE - 1)
                        slide_pos = (slide_pos + 1) & (self.WINDOW_SIZE - 1)
                else:
                    # TLG5: ビット0 = リテラルバイト
                    if input_pos >= data_len:
                        raise ValueError("不完全な圧縮データ: リテラルバイトが不足しています")
                    byte_val = data[input_pos]
                    input_pos += 1
                    output[output_pos] = byte_val
                    slide[slide_pos] = byte_val
                    output_pos += 1
                    slide_pos = (slide_pos + 1) & (self.WINDOW_SIZE - 1)

        return bytes(output)
