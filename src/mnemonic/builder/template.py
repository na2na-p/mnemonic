"""テンプレートダウンロード機能"""

from dataclasses import dataclass
from pathlib import Path

class TemplateDownloadError(Exception):
    """テンプレートダウンロードに関する基本例外クラス"""

    pass

class TemplateNotFoundError(TemplateDownloadError):
    """指定されたバージョンのテンプレートが存在しない場合の例外"""

    pass

class NetworkError(TemplateDownloadError):
    """ネットワークエラー発生時の例外"""

    pass

@dataclass(frozen=True)
class TemplateInfo:
    """テンプレート情報を表す不変オブジェクト"""

    version: str
    download_url: str
    file_name: str

class TemplateDownloader:
    """GitHub Releasesからkrkrsdl2テンプレートをダウンロードするクラス

    このクラスは指定されたバージョンのテンプレートをダウンロードし、
    ローカルファイルシステムに保存する機能を提供します。
    """

    def __init__(self, cache_dir: Path | None = None) -> None:
        """TemplateDownloaderを初期化する

        Args:
            cache_dir: テンプレートのキャッシュディレクトリ。
                       Noneの場合はデフォルトのキャッシュディレクトリを使用。
        """
        self._cache_dir = cache_dir

    async def download(self, version: str | None = None) -> Path:
        """指定バージョンのテンプレートをダウンロードする

        Args:
            version: ダウンロードするテンプレートのバージョン。
                     Noneの場合は最新バージョンをダウンロード。

        Returns:
            ダウンロードしたテンプレートのパス

        Raises:
            TemplateNotFoundError: 指定されたバージョンが存在しない場合
            NetworkError: ネットワークエラーが発生した場合
        """
        raise NotImplementedError

    async def get_latest_version(self) -> str:
        """最新バージョンを取得する

        Returns:
            最新のテンプレートバージョン文字列

        Raises:
            NetworkError: ネットワークエラーが発生した場合
        """
        raise NotImplementedError

    def get_download_url(self, version: str) -> str:
        """指定バージョンのダウンロードURLを構築する

        Args:
            version: テンプレートのバージョン

        Returns:
            ダウンロードURL文字列
        """
        raise NotImplementedError
