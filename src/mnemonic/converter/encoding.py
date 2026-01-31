"""文字コード検出・変換モジュール

スクリプトファイルの文字コードを検出し、UTF-8に変換する機能を提供する。
"""

from dataclasses import dataclass
from pathlib import Path

import chardet

from .base import BaseConverter, ConversionResult, ConversionStatus

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


class EncodingConverter(BaseConverter):
    """文字コード変換Converter

    テキストファイルの文字コードを変換するためのConverterクラス。
    ソースエンコーディングが指定されていない場合は自動検出を行う。

    Attributes:
        target_encoding: 変換先の文字コード（デフォルト: utf-8）
        source_encoding: 変換元の文字コード（Noneの場合は自動検出）
    """

    def __init__(
        self,
        target_encoding: str = "utf-8",
        source_encoding: str | None = None,
    ) -> None:
        """EncodingConverterを初期化する

        Args:
            target_encoding: 変換先の文字コード（デフォルト: utf-8）
            source_encoding: 変換元の文字コード（Noneの場合は自動検出）
        """
        self._target_encoding = target_encoding
        self._source_encoding = source_encoding
        self._detector = EncodingDetector()

    @property
    def target_encoding(self) -> str:
        """変換先の文字コードを返す"""
        return self._target_encoding

    @property
    def source_encoding(self) -> str | None:
        """変換元の文字コードを返す（Noneの場合は自動検出）"""
        return self._source_encoding

    @property
    def supported_extensions(self) -> tuple[str, ...]:
        """対応する拡張子のタプルを返す

        Returns:
            対応するテキストファイル拡張子のタプル
        """
        return (".ks", ".tjs", ".txt", ".csv", ".ini")

    def can_convert(self, file_path: Path) -> bool:
        """このConverterで変換可能なファイルかを判定する

        拡張子がサポート対象であり、かつテキストファイルである場合にTrueを返す。

        Args:
            file_path: 判定対象のファイルパス

        Returns:
            変換可能な場合True、そうでない場合False
        """
        if file_path.suffix.lower() not in self.supported_extensions:
            return False
        if not file_path.exists():
            return False
        return self._detector.is_text_file(file_path)

    def convert(self, source: Path, dest: Path) -> ConversionResult:
        """ファイルの文字コードを変換する

        指定された変換元ファイルを変換先文字コードに変換し、
        変換先パスに出力する。

        Args:
            source: 変換元ファイルのパス
            dest: 変換先ファイルのパス

        Returns:
            変換結果を表すConversionResultオブジェクト
        """
        if not source.exists():
            return ConversionResult(
                source_path=source,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message=f"変換元ファイルが見つかりません: {source}",
            )

        bytes_before = self._get_file_size(source)
        data = source.read_bytes()

        # ソースエンコーディングの決定
        if self._source_encoding is not None:
            source_encoding = self._source_encoding
        else:
            detection_result = self._detector.detect_bytes(data)
            source_encoding = detection_result.encoding or "utf-8"

        # 既にターゲットエンコーディングの場合はスキップ（BOMなしのUTF-8）
        target_normalized = self._target_encoding.lower().replace("-", "_")
        source_normalized = source_encoding.lower().replace("-", "_")
        utf8_bom = b"\xef\xbb\xbf"
        has_bom = data.startswith(utf8_bom)

        if source_normalized == target_normalized and not has_bom:
            return ConversionResult(
                source_path=source,
                dest_path=dest,
                status=ConversionStatus.SKIPPED,
                message="既にターゲットエンコーディングです",
                bytes_before=bytes_before,
                bytes_after=bytes_before,
            )

        # BOM除去
        if has_bom:
            data = data[len(utf8_bom) :]

        # 変換実行
        try:
            text = data.decode(source_encoding)
            result_bytes = text.encode(self._target_encoding)
        except (UnicodeDecodeError, UnicodeEncodeError) as e:
            return ConversionResult(
                source_path=source,
                dest_path=None,
                status=ConversionStatus.FAILED,
                message=f"エンコーディング変換に失敗しました: {e}",
                bytes_before=bytes_before,
            )

        # 出力先ディレクトリを作成
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_bytes(result_bytes)
        bytes_after = self._get_file_size(dest)

        return ConversionResult(
            source_path=source,
            dest_path=dest,
            status=ConversionStatus.SUCCESS,
            bytes_before=bytes_before,
            bytes_after=bytes_after,
        )

    def convert_bytes(self, data: bytes) -> tuple[bytes, str]:
        """バイトデータの文字コードを変換する

        Args:
            data: 変換対象のバイトデータ

        Returns:
            (変換後のバイトデータ, 検出されたソースエンコーディング)のタプル
        """
        # ソースエンコーディングの決定
        if self._source_encoding is not None:
            source_encoding = self._source_encoding
        else:
            detection_result = self._detector.detect_bytes(data)
            source_encoding = detection_result.encoding or "utf-8"

        # BOM除去
        utf8_bom = b"\xef\xbb\xbf"
        if data.startswith(utf8_bom):
            data = data[len(utf8_bom) :]

        # デコードしてターゲットエンコーディングでエンコード
        text = data.decode(source_encoding)
        result_bytes = text.encode(self._target_encoding)

        return result_bytes, source_encoding
