"""LZSSDecoderのテスト

TLG5形式で使用されるLZSS圧縮データの解凍機能をテストする。

TLG5 LZSSフォーマット:
- フラグビット0 = リテラルバイト
- フラグビット1 = マッチ（バックリファレンス）
- マッチ情報: 2バイト (position 12bit + length 4bit)
  - position = byte1 | ((byte2 & 0x0F) << 8)
  - length = (byte2 >> 4) + 3
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
                b"\x00" + b"ABCDEFGH",
                8,
                b"ABCDEFGH",
                id="正常系: 全リテラル-フラグ0x00-8バイト",
            ),
            pytest.param(
                b"\x00" + b"12345678",
                8,
                b"12345678",
                id="正常系: 全リテラル-フラグ0x00-数字8バイト",
            ),
            pytest.param(
                b"\x00" + b"ABC",
                3,
                b"ABC",
                id="正常系: 3バイトリテラル",
            ),
        ],
    )
    def test_decode_all_literals(
        self, decoder: LZSSDecoder, data: bytes, output_size: int, expected: bytes
    ) -> None:
        """全リテラルデータが正しく解凍されることを確認

        フラグ0x00 = 全てビット0 = 全てリテラル
        """
        result = decoder.decode(data, output_size)
        assert result == expected

    def test_decode_simple_match(self, decoder: LZSSDecoder) -> None:
        """単純なマッチ（繰り返しパターン）が正しく解凍されることを確認

        TLG5 LZSSフォーマット:
        - フラグビット0 = リテラル
        - フラグビット1 = マッチ

        フラグ=0x08: ビット0,1,2=0(リテラル), ビット3=1(マッチ)
        リテラル: "ABC" (3バイト)
        マッチ: position=0, length=0 (実際は3バイト)
        期待出力: "ABCABC" (リテラル3バイト + マッチ3バイト)
        """
        # position=0, length=0 -> byte1=0x00, byte2=0x00
        data = b"\x08" + b"ABC" + b"\x00\x00"
        result = decoder.decode(data, 6)
        assert result == b"ABCABC"

    def test_decode_match_with_longer_length(self, decoder: LZSSDecoder) -> None:
        """長いマッチパターンが正しく解凍されることを確認

        フラグ=0x02: ビット0=0(リテラル), ビット1=1(マッチ)
        リテラル: "A" (1バイト)
        マッチ: position=0, length=6 (実際は9バイト=6+3)

        期待出力: "AAAAAAAAAA" (1 + 9 = 10バイト)
        """
        # position=0, length=6 -> byte1=0x00, byte2=0x60
        data = b"\x02" + b"A" + b"\x00\x60"
        result = decoder.decode(data, 10)
        assert result == b"AAAAAAAAAA"

    def test_decode_mixed_literals_and_matches(self, decoder: LZSSDecoder) -> None:
        """リテラルとマッチの混合パターンが正しく解凍されることを確認

        フラグ=0x10: ビット0-3=0(リテラル4つ), ビット4=1(マッチ)
        リテラル: "ABCD" (4バイト)
        マッチ: position=0, length=0 (実際は3バイト)

        期待出力: "ABCDABC" (4 + 3 = 7バイト)
        """
        data = b"\x10" + b"ABCD" + b"\x00\x00"
        result = decoder.decode(data, 7)
        assert result == b"ABCDABC"

    def test_decode_multiple_flag_bytes(self, decoder: LZSSDecoder) -> None:
        """複数のフラグバイトを含むデータが正しく解凍されることを確認

        フラグ1=0x00 (8リテラル) + "12345678"
        フラグ2=0x00 (2リテラル) + "AB"
        期待出力: "12345678AB" (10バイト)
        """
        data = b"\x00" + b"12345678" + b"\x00" + b"AB"
        result = decoder.decode(data, 10)
        assert result == b"12345678AB"

    def test_decode_match_with_position(self, decoder: LZSSDecoder) -> None:
        """position指定のマッチが正しく解凍されることを確認

        フラグ=0x10: ビット0-3=0(リテラル4つ), ビット4=1(マッチ)
        リテラル: "ABCD"
        マッチ: position=2 (スライドバッファ位置2=C), length=0 (3バイト)
        期待出力: "ABCDCDC"
        """
        data = b"\x10" + b"ABCD" + b"\x02\x00"
        result = decoder.decode(data, 7)
        assert result == b"ABCDCDC"

    def test_decode_overlapping_match(self, decoder: LZSSDecoder) -> None:
        """重複するマッチが正しく解凍されることを確認

        フラグ=0x02: ビット0=0(リテラル), ビット1=1(マッチ)
        リテラル: "X"
        マッチ: position=0, length=2 (実際は5バイト=2+3)

        期待出力: "XXXXXX" (1 + 5 = 6バイト)
        """
        data = b"\x02" + b"X" + b"\x00\x20"
        result = decoder.decode(data, 6)
        assert result == b"XXXXXX"


class TestLZSSDecoderEdgeCases:
    """LZSSDecoderのエッジケーステスト"""

    @pytest.fixture
    def decoder(self) -> LZSSDecoder:
        """LZSSDecoderインスタンスを作成するフィクスチャ"""
        return LZSSDecoder()

    def test_decode_incomplete_literal_raises_error(self, decoder: LZSSDecoder) -> None:
        """不完全なリテラルデータで例外が発生することを確認

        フラグ0x00で8リテラル期待だが3バイトしかない場合
        """
        data = b"\x00" + b"ABC"
        with pytest.raises(ValueError, match="不完全|incomplete|data"):
            decoder.decode(data, 8)

    def test_decode_incomplete_match_raises_error(self, decoder: LZSSDecoder) -> None:
        """不完全なマッチ情報で例外が発生することを確認

        マッチには2バイト必要だが1バイトしかない場合
        """
        # フラグ=0x01 (ビット0がマッチ) だがマッチ情報が1バイトしかない
        data = b"\x01" + b"\x00"
        with pytest.raises(ValueError, match="不完全|incomplete|data"):
            decoder.decode(data, 3)

    def test_decode_output_size_matches(self, decoder: LZSSDecoder) -> None:
        """出力サイズが指定されたサイズと一致することを確認"""
        data = b"\x00" + b"12345678"
        result = decoder.decode(data, 8)
        assert len(result) == 8

    def test_decode_maximum_match_length(self, decoder: LZSSDecoder) -> None:
        """最大マッチ長（18バイト）が正しく処理されることを確認

        TLG5 LZSSでは、mlen==18の場合に追加バイトを読み取る仕様がある。
        length=15 -> mlen=15+3=18 -> 追加バイト(0)を読み -> 最終長=18
        """
        # フラグ=0x02: ビット0=0(リテラル), ビット1=1(マッチ)
        # リテラル: "A"
        # マッチ: position=0, length=15 (mlen=15+3=18)
        # byte2 = 0xF0 (length=15)
        # mlen==18のため追加バイト(0x00)が必要 -> 最終長=18+0=18
        data = b"\x02" + b"A" + b"\x00\xf0" + b"\x00"
        result = decoder.decode(data, 19)
        assert result == b"A" * 19
        assert len(result) == 19
