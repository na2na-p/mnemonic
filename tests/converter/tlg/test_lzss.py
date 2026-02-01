"""LZSSDecoderのテスト

TLG5形式で使用されるLZSS圧縮データの解凍機能をテストする。
"""

import pytest

from mnemonic.converter.tlg.lzss import LZSSDecoder


class TestLZSSDecoderConstants:
    """LZSSDecoderの定数テスト"""

    def test_window_size(self) -> None:
        """スライディングウィンドウサイズが4096バイトであることを確認"""
        assert LZSSDecoder.WINDOW_SIZE == 4096

    def test_match_max_length(self) -> None:
        """最大マッチ長が18バイトであることを確認"""
        assert LZSSDecoder.MATCH_MAX_LENGTH == 18

    def test_match_min_length(self) -> None:
        """最小マッチ長が3バイトであることを確認"""
        assert LZSSDecoder.MATCH_MIN_LENGTH == 3


class TestLZSSDecoderDecode:
    """LZSSDecoder.decodeメソッドのテスト"""

    @pytest.fixture
    def decoder(self) -> LZSSDecoder:
        """LZSSDecoderインスタンスを作成するフィクスチャ"""
        return LZSSDecoder()

    def test_decode_empty_data(self, decoder: LZSSDecoder) -> None:
        """空データの解凍は空バイト列を返すことを確認"""
        result = decoder.decode(b"", 0)
        assert result == b""

    @pytest.mark.parametrize(
        "data, output_size, expected",
        [
            pytest.param(
                b"\xff" + b"ABCDEFGH",
                8,
                b"ABCDEFGH",
                id="正常系: 全リテラル-フラグ0xFF-8バイト",
            ),
            pytest.param(
                b"\xff" + b"12345678",
                8,
                b"12345678",
                id="正常系: 全リテラル-フラグ0xFF-数字8バイト",
            ),
            pytest.param(
                b"\x07" + b"ABC",
                3,
                b"ABC",
                id="正常系: 下位3ビットがリテラル-3バイト",
            ),
        ],
    )
    def test_decode_all_literals(
        self, decoder: LZSSDecoder, data: bytes, output_size: int, expected: bytes
    ) -> None:
        """全リテラルデータが正しく解凍されることを確認"""
        result = decoder.decode(data, output_size)
        assert result == expected

    def test_decode_simple_match(self, decoder: LZSSDecoder) -> None:
        """単純なマッチ（繰り返しパターン）が正しく解凍されることを確認

        入力: フラグ=0x07 (下位3ビットがリテラル)
              リテラル: "ABC" (3バイト)
              マッチ: オフセット=3, 長さ=0 (実際は3バイト)

        マッチ情報の2バイト構造:
        - offset[7:0] (下位バイト)
        - offset[11:8] | length[3:0] (上位バイト): 上位4ビット=オフセット上位、下位4ビット=長さ

        オフセット3, 長さ0 -> バイト: 0x03, 0x00
        期待出力: "ABCABC" (リテラル3バイト + マッチ3バイト)
        """
        # フラグ=0x07: ビット0,1,2=1(リテラル), ビット3=0(マッチ)
        # リテラル: "ABC"
        # マッチ: オフセット=3 (3バイト前から), 長さ=0 (3バイト=0+3)
        data = b"\x07" + b"ABC" + b"\x03\x00"
        result = decoder.decode(data, 6)
        assert result == b"ABCABC"

    def test_decode_match_with_longer_length(self, decoder: LZSSDecoder) -> None:
        """長いマッチパターンが正しく解凍されることを確認

        入力: フラグ=0x01 (ビット0のみリテラル)
              リテラル: "A" (1バイト)
              マッチ: オフセット=1, 長さ=6 (実際は9バイト)

        期待出力: "AAAAAAAAAA" (1 + 9 = 10バイト)
        """
        # フラグ=0x01: ビット0=1(リテラル), ビット1=0(マッチ)
        # リテラル: "A"
        # マッチ: オフセット=1, 長さ=6 (9バイト=6+3)
        data = b"\x01" + b"A" + b"\x01\x06"
        result = decoder.decode(data, 10)
        assert result == b"AAAAAAAAAA"

    def test_decode_mixed_literals_and_matches(self, decoder: LZSSDecoder) -> None:
        """リテラルとマッチの混合パターンが正しく解凍されることを確認

        入力: フラグ=0x0F (下位4ビットがリテラル、ビット4がマッチ)
              リテラル: "ABCD" (4バイト)
              マッチ: オフセット=4, 長さ=0 (実際は3バイト)

        期待出力: "ABCDABC" (4 + 3 = 7バイト)
        """
        data = b"\x0f" + b"ABCD" + b"\x04\x00"
        result = decoder.decode(data, 7)
        assert result == b"ABCDABC"

    def test_decode_multiple_flag_bytes(self, decoder: LZSSDecoder) -> None:
        """複数のフラグバイトを含むデータが正しく解凍されることを確認

        フラグ1=0xFF (8リテラル) + "12345678"
        フラグ2=0x03 (2リテラル) + "AB"
        期待出力: "12345678AB" (10バイト)
        """
        data = b"\xff" + b"12345678" + b"\x03" + b"AB"
        result = decoder.decode(data, 10)
        assert result == b"12345678AB"

    def test_decode_match_with_window_offset(self, decoder: LZSSDecoder) -> None:
        """ウィンドウオフセットを使用したマッチが正しく解凍されることを確認

        オフセットが現在位置より大きい場合（ウィンドウを参照）
        """
        # フラグ=0x0F (下位4ビットがリテラル)
        # リテラル: "ABCD"
        # マッチ: オフセット=2, 長さ=0 (3バイト="CDA"を参照)
        data = b"\x0f" + b"ABCD" + b"\x02\x00"
        result = decoder.decode(data, 7)
        assert result == b"ABCDCDC"

    def test_decode_overlapping_match(self, decoder: LZSSDecoder) -> None:
        """重複するマッチ（オフセット<長さ）が正しく解凍されることを確認

        入力: フラグ=0x01 (ビット0のみリテラル)
              リテラル: "X"
              マッチ: オフセット=1, 長さ=2 (実際は5バイト)

        期待出力: "XXXXXX" (1 + 5 = 6バイト)
        オフセット1で長さ5バイトをコピー = 同じバイトを繰り返しコピー
        """
        data = b"\x01" + b"X" + b"\x01\x02"
        result = decoder.decode(data, 6)
        assert result == b"XXXXXX"


class TestLZSSDecoderEdgeCases:
    """LZSSDecoderのエッジケーステスト"""

    @pytest.fixture
    def decoder(self) -> LZSSDecoder:
        """LZSSDecoderインスタンスを作成するフィクスチャ"""
        return LZSSDecoder()

    def test_decode_incomplete_data_raises_error(self, decoder: LZSSDecoder) -> None:
        """不完全なデータで例外が発生することを確認

        フラグは8ビットあるが、リテラルが足りない場合
        """
        # フラグ=0xFF (8リテラル期待) だが3バイトしかない
        data = b"\xff" + b"ABC"
        with pytest.raises(ValueError, match="不完全|incomplete|data"):
            decoder.decode(data, 8)

    def test_decode_incomplete_match_raises_error(self, decoder: LZSSDecoder) -> None:
        """不完全なマッチ情報で例外が発生することを確認

        マッチには2バイト必要だが1バイトしかない場合
        """
        # フラグ=0x00 (ビット0がマッチ) だがマッチ情報が1バイトしかない
        data = b"\x00" + b"\x01"
        with pytest.raises(ValueError, match="不完全|incomplete|data"):
            decoder.decode(data, 3)

    def test_decode_output_size_matches(self, decoder: LZSSDecoder) -> None:
        """出力サイズが指定されたサイズと一致することを確認"""
        data = b"\xff" + b"12345678"
        result = decoder.decode(data, 8)
        assert len(result) == 8

    def test_decode_maximum_match_length(self, decoder: LZSSDecoder) -> None:
        """最大マッチ長（18バイト）が正しく処理されることを確認

        長さ=15 -> 実際の長さ=15+3=18バイト
        """
        # フラグ=0x01 (ビット0のみリテラル)
        # リテラル: "A"
        # マッチ: オフセット=1, 長さ=15 (18バイト=15+3)
        data = b"\x01" + b"A" + b"\x01\x0f"
        result = decoder.decode(data, 19)
        assert result == b"A" * 19
        assert len(result) == 19
