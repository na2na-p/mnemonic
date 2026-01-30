"""XP3アーカイブ操作モジュール

吉里吉里/KAG/KiriKiri形式のXP3アーカイブファイルを読み込み、
ファイル一覧の取得や展開を行う機能を提供する。
また、XP3アーカイブの暗号化チェック機能も提供する。
"""

from dataclasses import dataclass
from enum import Enum
from pathlib import Path

class EncryptionType(Enum):
    """検出可能な暗号化タイプ

    XP3アーカイブで使用される暗号化方式を表す列挙型。
    """

    NONE = "none"
    """暗号化なし"""

    SIMPLE_XOR = "simple_xor"
    """単純なXOR暗号化"""

    CUSTOM = "custom"
    """カスタム暗号化（ゲーム固有の実装）"""

    UNKNOWN = "unknown"
    """未知の暗号化方式"""

@dataclass(frozen=True)
class EncryptionInfo:
    """暗号化情報を保持するデータクラス

    XP3アーカイブの暗号化状態に関する情報を格納する。

    Attributes:
        is_encrypted: 暗号化されているかどうか
        encryption_type: 検出された暗号化タイプ
        details: 暗号化に関する追加情報（オプション）
    """

    is_encrypted: bool
    encryption_type: EncryptionType
    details: str | None = None

class XP3EncryptionError(Exception):
    """XP3が暗号化されている場合に発生する例外

    暗号化されたXP3アーカイブを処理しようとした際に発生する。
    暗号化情報を保持し、エラーメッセージとして詳細を提供する。

    Attributes:
        encryption_info: 検出された暗号化情報
    """

    def __init__(self, encryption_info: EncryptionInfo) -> None:
        """暗号化情報を指定して初期化する

        Args:
            encryption_info: 検出された暗号化情報
        """
        self.encryption_info = encryption_info
        message = self._build_message(encryption_info)
        super().__init__(message)

    def _build_message(self, encryption_info: EncryptionInfo) -> str:
        """エラーメッセージを構築する

        Args:
            encryption_info: 暗号化情報

        Returns:
            エラーメッセージ文字列
        """
        base_message = (
            f"XP3アーカイブは暗号化されています (タイプ: {encryption_info.encryption_type.value})"
        )
        if encryption_info.details:
            return f"{base_message}: {encryption_info.details}"
        return base_message

class XP3Archive:
    """XP3アーカイブを操作するクラス

    吉里吉里/KAG形式のXP3アーカイブファイルを開き、
    内包されているファイルの一覧取得や展開を行う。
    """

    def __init__(self, archive_path: Path) -> None:
        """アーカイブファイルを開く

        Args:
            archive_path: XP3アーカイブファイルのパス
        """
        raise NotImplementedError()

    def list_files(self) -> list[str]:
        """アーカイブ内のファイル一覧を取得する

        Returns:
            アーカイブに含まれるファイルパスのリスト
        """
        raise NotImplementedError()

    def extract_all(self, output_dir: Path) -> None:
        """すべてのファイルを指定ディレクトリに展開する

        Args:
            output_dir: 展開先ディレクトリのパス
        """
        raise NotImplementedError()

    def extract_file(self, filename: str, output_path: Path) -> None:
        """指定ファイルを展開する

        Args:
            filename: アーカイブ内のファイル名
            output_path: 展開先のファイルパス
        """
        raise NotImplementedError()

    def is_encrypted(self) -> bool:
        """暗号化されているかを判定する

        Returns:
            暗号化されている場合はTrue、そうでない場合はFalse
        """
        raise NotImplementedError()

class XP3EncryptionChecker:
    """XP3ファイルの暗号化をチェックするクラス

    XP3アーカイブファイルを解析し、暗号化されているかどうかを判定する。
    暗号化されている場合は、その暗号化タイプも検出する。
    """

    def __init__(self, archive_path: Path) -> None:
        """アーカイブファイルを指定して初期化する

        Args:
            archive_path: チェック対象のXP3アーカイブファイルパス
        """
        self._archive_path = archive_path

    def check(self) -> EncryptionInfo:
        """暗号化状態をチェックして返す

        アーカイブファイルを解析し、暗号化されているかどうかを判定する。

        Returns:
            暗号化情報を含むEncryptionInfoオブジェクト

        Raises:
            FileNotFoundError: アーカイブファイルが存在しない場合
            IOError: ファイルの読み取りに失敗した場合
        """
        raise NotImplementedError()

    def raise_if_encrypted(self) -> None:
        """暗号化されている場合は例外を発生させる

        アーカイブが暗号化されていないことを確認し、
        暗号化されている場合はXP3EncryptionErrorを発生させる。

        Raises:
            XP3EncryptionError: アーカイブが暗号化されている場合
            FileNotFoundError: アーカイブファイルが存在しない場合
            IOError: ファイルの読み取りに失敗した場合
        """
        raise NotImplementedError()
