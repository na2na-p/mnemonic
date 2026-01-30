"""文字コード検出・変換モジュール

スクリプトファイルの文字コードを検出し、UTF-8に変換する機能を提供する。
"""

from dataclasses import dataclass
from pathlib import Path

SUPPORTED_ENCODINGS: tuple[str, ...] = (
    "shift_jis",
    "euc-jp",
    "utf-8",
    "gb2312",
    "big5",
    "cp949",
)

@dataclass(frozen=True)
class EncodingDetectionResult:
    """文字コード検出結果

    Attributes:
        encoding: 検出された文字コード（検出できなかった場合はNone）
        confidence: 検出の信頼度（0.0〜1.0）
        is_supported: サポートされている文字コードかどうか
    """

    encoding: str | None
    confidence: float
    is_supported: bool

class EncodingDetector:
    """文字コード検出クラス

    スクリプトファイルの文字コードを検出するためのクラス。
    chardetライブラリを使用して検出を行う。
    """

    def detect(self, file_path: Path) -> EncodingDetectionResult:
        """ファイルの文字コードを検出する

        Args:
            file_path: 検出対象のファイルパス

        Returns:
            検出結果を表すEncodingDetectionResultオブジェクト

        Raises:
            FileNotFoundError: ファイルが存在しない場合
        """
        raise NotImplementedError

    def detect_bytes(self, data: bytes) -> EncodingDetectionResult:
        """バイトデータの文字コードを検出する

        Args:
            data: 検出対象のバイトデータ

        Returns:
            検出結果を表すEncodingDetectionResultオブジェクト
        """
        raise NotImplementedError

    def is_text_file(self, file_path: Path) -> bool:
        """ファイルがテキストファイルかどうかを判定する

        バイナリファイルとテキストファイルを区別するために使用する。

        Args:
            file_path: 判定対象のファイルパス

        Returns:
            テキストファイルの場合True、バイナリファイルの場合False

        Raises:
            FileNotFoundError: ファイルが存在しない場合
        """
        raise NotImplementedError
