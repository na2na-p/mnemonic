"""Converter基底クラスモジュール

アセット変換を行うすべてのConverterの基底クラスと共通データ型を定義する。
"""

from abc import ABC, abstractmethod
from dataclasses import dataclass
from enum import Enum
from pathlib import Path


class ConversionStatus(Enum):
    """変換ステータス

    ファイル変換処理の結果ステータスを表す列挙型。
    成功、スキップ、失敗の3状態を持つ。
    """

    SUCCESS = "success"
    SKIPPED = "skipped"
    FAILED = "failed"


@dataclass(frozen=True)
class ConversionResult:
    """変換結果を表すデータクラス

    単一ファイルの変換処理結果を保持する不変データクラス。

    Attributes:
        source_path: 変換元ファイルのパス
        dest_path: 変換先ファイルのパス（変換失敗・スキップ時はNone）
        status: 変換ステータス
        message: 追加メッセージ（エラー詳細等）
        bytes_before: 変換前のファイルサイズ（バイト）
        bytes_after: 変換後のファイルサイズ（バイト）
    """

    source_path: Path
    dest_path: Path | None
    status: ConversionStatus
    message: str = ""
    bytes_before: int = 0
    bytes_after: int = 0

    @property
    def compression_ratio(self) -> float:
        """圧縮率を計算する（bytes_after / bytes_before）

        Returns:
            圧縮率（0.0〜1.0+）。bytes_beforeが0の場合は1.0を返す
        """
        if self.bytes_before == 0:
            return 1.0
        return self.bytes_after / self.bytes_before

    @property
    def bytes_saved(self) -> int:
        """節約されたバイト数を返す

        Returns:
            節約バイト数（負の場合はサイズ増加）
        """
        return self.bytes_before - self.bytes_after

    @property
    def is_success(self) -> bool:
        """変換が成功したかどうかを返す"""
        return self.status == ConversionStatus.SUCCESS


class BaseConverter(ABC):
    """Converterの基底クラス

    すべてのアセット変換クラスが継承する抽象基底クラス。
    エンコーディング変換、画像変換、動画変換等の具象クラスは
    このクラスを継承して実装する。
    """

    @abstractmethod
    def can_convert(self, file_path: Path) -> bool:
        """このConverterで変換可能なファイルかを判定する

        Args:
            file_path: 判定対象のファイルパス

        Returns:
            変換可能な場合True、そうでない場合False
        """
        ...

    @abstractmethod
    def convert(self, source: Path, dest: Path) -> ConversionResult:
        """ファイルを変換する

        指定された変換元ファイルを変換し、変換先パスに出力する。

        Args:
            source: 変換元ファイルのパス
            dest: 変換先ファイルのパス

        Returns:
            変換結果を表すConversionResultオブジェクト
        """
        ...

    @property
    @abstractmethod
    def supported_extensions(self) -> tuple[str, ...]:
        """対応する拡張子のタプルを返す

        このConverterが変換可能なファイル拡張子の一覧を返す。
        拡張子はドット付き小文字形式（例: ".txt", ".bmp"）。

        Returns:
            対応する拡張子のタプル
        """
        ...

    def get_output_extension(self, source_path: Path) -> str | None:
        """変換後のファイル拡張子を返す

        入力ファイルに対する出力ファイルの拡張子を返す。
        拡張子を変更しない場合はNoneを返す。

        Args:
            source_path: 変換元ファイルのパス

        Returns:
            出力ファイルの拡張子（ドット付き小文字形式）、変更しない場合はNone
        """
        return None

    def _validate_source(self, source: Path) -> None:
        """変換元ファイルの検証を行う

        Args:
            source: 変換元ファイルのパス

        Raises:
            FileNotFoundError: ファイルが存在しない場合
            ValueError: ファイルではなくディレクトリの場合
        """
        if not source.exists():
            raise FileNotFoundError(f"変換元ファイルが見つかりません: {source}")
        if source.is_dir():
            raise ValueError(f"変換元はファイルである必要があります: {source}")

    def _get_file_size(self, path: Path) -> int:
        """ファイルサイズを取得する

        Args:
            path: ファイルパス

        Returns:
            ファイルサイズ（バイト）。ファイルが存在しない場合は0
        """
        if path.exists():
            return path.stat().st_size
        return 0
