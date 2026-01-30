"""XP3アーカイブ操作モジュール

吉里吉里/KAG/KiriKiri形式のXP3アーカイブファイルを読み込み、
ファイル一覧の取得や展開を行う機能を提供する。
"""

from pathlib import Path

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
