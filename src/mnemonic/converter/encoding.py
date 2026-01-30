"""文字コード検出・変換モジュール

スクリプトファイルの文字コードを検出し、UTF-8に変換する機能を提供する。
"""

from dataclasses import dataclass
from pathlib import Path

import chardet

SUPPORTED_ENCODINGS: tuple[str, ...] = (
    "shift_jis",
    "euc-jp",
    "utf-8",
    "gb2312",
    "big5",
    "cp949",
)

# chardetが返すエンコーディング名とSUPPORTED_ENCODINGSの対応マッピング
_ENCODING_ALIASES: dict[str, str] = {
    "shift-jis": "shift_jis",
    "shiftjis": "shift_jis",
    "sjis": "shift_jis",
    "euc_jp": "euc-jp",
    "eucjp": "euc-jp",
    "utf8": "utf-8",
    "utf-8-sig": "utf-8",
    "ascii": "utf-8",  # ASCIIはUTF-8のサブセット
}

def _normalize_encoding(encoding: str | None) -> str | None:
    """エンコーディング名を正規化する

    Args:
        encoding: 検出されたエンコーディング名

    Returns:
        正規化されたエンコーディング名
    """
    if encoding is None:
        return None

    lower_encoding = encoding.lower().replace("_", "-")

    # エイリアスマッピングを確認
    if lower_encoding in _ENCODING_ALIASES:
        return _ENCODING_ALIASES[lower_encoding]

    # 元のエンコーディング名を返す（小文字に変換）
    return encoding.lower()

def _is_supported_encoding(encoding: str | None) -> bool:
    """エンコーディングがサポートされているか確認する

    Args:
        encoding: チェックするエンコーディング名

    Returns:
        サポートされている場合True
    """
    if encoding is None:
        return False

    normalized = _normalize_encoding(encoding)
    if normalized is None:
        return False

    # 正規化後のエンコーディング名でチェック
    normalized_lower = normalized.lower().replace("_", "-")
    for supported in SUPPORTED_ENCODINGS:
        supported_lower = supported.lower().replace("_", "-")
        if normalized_lower == supported_lower:
            return True

    return False

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
        if not file_path.exists():
            raise FileNotFoundError(f"ファイルが見つかりません: {file_path}")

        data = file_path.read_bytes()
        return self.detect_bytes(data)

    def detect_bytes(self, data: bytes) -> EncodingDetectionResult:
        """バイトデータの文字コードを検出する

        Args:
            data: 検出対象のバイトデータ

        Returns:
            検出結果を表すEncodingDetectionResultオブジェクト
        """
        if len(data) == 0:
            return EncodingDetectionResult(
                encoding=None,
                confidence=0.0,
                is_supported=False,
            )

        result = chardet.detect(data)
        encoding = result.get("encoding")
        confidence = result.get("confidence", 0.0) or 0.0

        # エンコーディング名を正規化
        normalized_encoding = _normalize_encoding(encoding)
        is_supported = _is_supported_encoding(encoding)

        return EncodingDetectionResult(
            encoding=normalized_encoding,
            confidence=confidence,
            is_supported=is_supported,
        )

    def is_text_file(self, file_path: Path) -> bool:
        """ファイルがテキストファイルかどうかを判定する

        バイナリファイルとテキストファイルを区別するために使用する。
        空ファイルはテキストファイルとして扱う。

        Args:
            file_path: 判定対象のファイルパス

        Returns:
            テキストファイルの場合True、バイナリファイルの場合False

        Raises:
            FileNotFoundError: ファイルが存在しない場合
        """
        if not file_path.exists():
            raise FileNotFoundError(f"ファイルが見つかりません: {file_path}")

        data = file_path.read_bytes()

        # 空ファイルはテキストファイルとして扱う
        if len(data) == 0:
            return True

        # NULバイトが含まれている場合はバイナリファイルと判定
        if b"\x00" in data:
            return False

        # chardetで検出を試みる
        result = self.detect_bytes(data)

        # エンコーディングが検出できなかった場合はバイナリファイルと判定
        return result.encoding is not None
