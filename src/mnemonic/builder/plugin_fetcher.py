"""extransプラグイン取得機能

krkrsdl2/SamplePluginからextrans.soをダウンロードし、キャッシュに保存する機能を提供する。
extransはextended transitionプラグインで、ripple等のトランジション効果を提供する。
"""

from __future__ import annotations

import io
import zipfile
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING

import httpx

from mnemonic.cache import get_cache_dir

if TYPE_CHECKING:
    pass


class PluginDownloadError(Exception):
    """プラグインダウンロードに関する例外"""

    pass


@dataclass(frozen=True)
class PluginInfo:
    """プラグイン情報を表す不変オブジェクト

    Attributes:
        name: プラグイン名（例: extrans）
        paths: ABI名をキー、プラグインファイルパスを値とする辞書
    """

    name: str
    paths: dict[str, Path]


# extransプラグインのダウンロード設定
PLUGIN_NAME = "extrans"
PLUGIN_FILENAME = "extrans.so"

# 各ABI用のダウンロードURL
PLUGIN_DOWNLOAD_URLS: dict[str, str] = {
    "arm64-v8a": "https://github.com/krkrsdl2/SamplePlugin/releases/download/latest_krkrsdl2/SamplePlugin-android-arm64-v8a.zip",
    "armeabi-v7a": "https://github.com/krkrsdl2/SamplePlugin/releases/download/latest_krkrsdl2/SamplePlugin-android-armeabi-v7a.zip",
    "x86": "https://github.com/krkrsdl2/SamplePlugin/releases/download/latest_krkrsdl2/SamplePlugin-android-x86.zip",
    "x86_64": "https://github.com/krkrsdl2/SamplePlugin/releases/download/latest_krkrsdl2/SamplePlugin-android-x86_64.zip",
}


def get_plugin_cache_dir() -> Path:
    """プラグインキャッシュディレクトリを取得する

    Returns:
        プラグインキャッシュディレクトリのパス
    """
    return get_cache_dir() / "plugins"


class PluginFetcher:
    """extransプラグインを取得するクラス

    GitHubからextransプラグインをダウンロードし、
    ローカルキャッシュに保存する機能を提供する。

    Example:
        >>> fetcher = PluginFetcher()
        >>> plugin_info = await fetcher.get_plugin()
        >>> print(plugin_info.paths["arm64-v8a"])
        /home/user/.cache/mnemonic/plugins/arm64-v8a/extrans.so
    """

    DEFAULT_TIMEOUT = 60.0  # HTTPリクエストのタイムアウト秒数

    def __init__(
        self,
        cache_dir: Path | None = None,
        http_client: httpx.AsyncClient | None = None,
    ) -> None:
        """PluginFetcherを初期化する

        Args:
            cache_dir: プラグインのキャッシュディレクトリ。
                       Noneの場合はデフォルトのキャッシュディレクトリを使用。
            http_client: HTTPクライアント（テスト用の依存性注入）。
                         Noneの場合は内部でクライアントを作成。
        """
        self._cache_dir = cache_dir or get_plugin_cache_dir()
        self._http_client = http_client
        self._owns_client = http_client is None

    async def get_plugin(self) -> PluginInfo:
        """プラグインを取得する

        キャッシュに有効なプラグインが存在する場合はそれを返し、
        存在しない場合はダウンロードする。

        Returns:
            プラグイン情報

        Raises:
            PluginDownloadError: ダウンロードに失敗した場合
        """
        if self.is_cache_valid():
            cached_paths = self.get_cached_plugin_paths()
            if cached_paths is not None:
                return PluginInfo(
                    name=PLUGIN_NAME,
                    paths=cached_paths,
                )

        return await self.download_plugin()

    async def download_plugin(self) -> PluginInfo:
        """プラグインをダウンロードする

        GitHubから各ABI用のextrans.soをダウンロードしてキャッシュに保存する。

        Returns:
            プラグイン情報

        Raises:
            PluginDownloadError: ダウンロードに失敗した場合
        """
        self._cache_dir.mkdir(parents=True, exist_ok=True)

        paths: dict[str, Path] = {}

        for abi, url in PLUGIN_DOWNLOAD_URLS.items():
            # ZIPをダウンロード
            zip_content = await self._download_zip(url, abi)

            # ZIPを解凍してextrans.soを取得
            so_content = self._extract_so_from_zip(zip_content, abi)

            # ABI別ディレクトリに保存
            abi_dir = self._cache_dir / abi
            abi_dir.mkdir(parents=True, exist_ok=True)
            plugin_path = abi_dir / PLUGIN_FILENAME
            plugin_path.write_bytes(so_content)
            paths[abi] = plugin_path

        return PluginInfo(
            name=PLUGIN_NAME,
            paths=paths,
        )

    async def _download_zip(self, url: str, abi: str) -> bytes:
        """ZIPファイルをダウンロードする

        Args:
            url: ダウンロードURL
            abi: ABI名（エラーメッセージ用）

        Returns:
            ZIPファイルのバイナリデータ

        Raises:
            PluginDownloadError: ダウンロードに失敗した場合
        """
        client = self._http_client
        close_client = False

        if client is None:
            client = httpx.AsyncClient(timeout=self.DEFAULT_TIMEOUT)
            close_client = True

        try:
            response = await client.get(url, follow_redirects=True)
            response.raise_for_status()
            return response.content
        except httpx.RequestError as e:
            raise PluginDownloadError(f"ネットワークエラー ({abi}): {e}") from e
        except httpx.HTTPStatusError as e:
            raise PluginDownloadError(f"HTTPエラー {e.response.status_code} ({abi}): {e}") from e
        finally:
            if close_client and client is not None:
                await client.aclose()

    def _extract_so_from_zip(self, zip_content: bytes, abi: str) -> bytes:
        """ZIPファイルからextrans.soを抽出する

        Args:
            zip_content: ZIPファイルのバイナリデータ
            abi: ABI名（エラーメッセージ用）

        Returns:
            extrans.soのバイナリデータ

        Raises:
            PluginDownloadError: ZIPの解凍に失敗した場合
        """
        try:
            with zipfile.ZipFile(io.BytesIO(zip_content), "r") as zf:
                # extrans.soを探す
                for name in zf.namelist():
                    if name.endswith(PLUGIN_FILENAME):
                        return zf.read(name)

                raise PluginDownloadError(
                    f"ZIPファイル内に{PLUGIN_FILENAME}が見つかりません ({abi})"
                )
        except zipfile.BadZipFile as e:
            raise PluginDownloadError(f"無効なZIPファイルです ({abi}): {e}") from e

    def is_cache_valid(self) -> bool:
        """キャッシュが有効かどうかを確認する

        全てのABI用のプラグインファイルが存在する場合のみTrueを返す。

        Returns:
            キャッシュが存在し有効な場合はTrue
        """
        for abi in PLUGIN_DOWNLOAD_URLS:
            plugin_path = self._cache_dir / abi / PLUGIN_FILENAME
            if not plugin_path.exists():
                return False
        return True

    def get_cached_plugin_paths(self) -> dict[str, Path] | None:
        """キャッシュされたプラグインのパスを取得する

        Returns:
            ABI名をキー、パスを値とする辞書。キャッシュが無い場合はNone。
        """
        if not self.is_cache_valid():
            return None

        paths: dict[str, Path] = {}
        for abi in PLUGIN_DOWNLOAD_URLS:
            paths[abi] = self._cache_dir / abi / PLUGIN_FILENAME
        return paths
