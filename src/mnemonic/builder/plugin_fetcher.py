"""krkrsdl2プラグイン取得機能

krkrsdl2向けのネイティブプラグイン(.so)をダウンロードし、キャッシュに保存する機能を提供する。

対応プラグイン:
- extrans: 拡張トランジション（ripple等）
- wuvorbis: Ogg Vorbis音声再生
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
    """単一プラグイン情報を表す不変オブジェクト

    Attributes:
        name: プラグイン名（例: extrans）
        paths: ABI名をキー、プラグインファイルパスを値とする辞書
    """

    name: str
    paths: dict[str, Path]


@dataclass(frozen=True)
class PluginsInfo:
    """複数プラグイン情報を表す不変オブジェクト

    Attributes:
        plugins: プラグイン名をキー、PluginInfoを値とする辞書
    """

    plugins: dict[str, PluginInfo]

    def get_all_paths_for_abi(self, abi: str) -> dict[str, Path]:
        """指定ABIの全プラグインパスを取得する

        Args:
            abi: ABI名（例: arm64-v8a）

        Returns:
            プラグイン名をキー、パスを値とする辞書
        """
        result: dict[str, Path] = {}
        for plugin_name, plugin_info in self.plugins.items():
            if abi in plugin_info.paths:
                result[plugin_name] = plugin_info.paths[abi]
        return result


# サポートするABI
SUPPORTED_ABIS: list[str] = ["arm64-v8a", "armeabi-v7a", "x86", "x86_64"]


# プラグイン設定
@dataclass(frozen=True)
class PluginConfig:
    """プラグインダウンロード設定

    Attributes:
        name: プラグイン名（例: extrans）
        source_filename: ZIP内のファイル名（例: extrans.so）
        output_filename: 出力ファイル名（例: libextrans.so）
        url_template: ダウンロードURLテンプレート（{abi}がABI名に置換される）
    """

    name: str
    source_filename: str
    output_filename: str
    url_template: str


# 各プラグインの設定
PLUGIN_CONFIGS: list[PluginConfig] = [
    PluginConfig(
        name="extrans",
        source_filename="extrans.so",
        output_filename="libextrans.so",
        url_template="https://github.com/krkrsdl2/SamplePlugin/releases/download/latest_krkrsdl2/SamplePlugin-android-{abi}.zip",
    ),
    PluginConfig(
        name="wuvorbis",
        source_filename="wuvorbis.so",
        output_filename="libwuvorbis.so",
        url_template="https://github.com/krkrsdl2/wuvorbis/releases/download/latest_krkrsdl2/wuvorbis-android-{abi}.zip",
    ),
]


def get_plugin_cache_dir() -> Path:
    """プラグインキャッシュディレクトリを取得する

    Returns:
        プラグインキャッシュディレクトリのパス
    """
    return get_cache_dir() / "plugins"


class PluginFetcher:
    """krkrsdl2プラグインを取得するクラス

    GitHubからkrkrsdl2用ネイティブプラグインをダウンロードし、
    ローカルキャッシュに保存する機能を提供する。

    Example:
        >>> fetcher = PluginFetcher()
        >>> plugins_info = await fetcher.get_plugins()
        >>> print(plugins_info.plugins["extrans"].paths["arm64-v8a"])
        /home/user/.cache/mnemonic/plugins/arm64-v8a/libextrans.so
    """

    DEFAULT_TIMEOUT = 60.0  # HTTPリクエストのタイムアウト秒数

    def __init__(
        self,
        cache_dir: Path | None = None,
        http_client: httpx.AsyncClient | None = None,
        plugin_configs: list[PluginConfig] | None = None,
    ) -> None:
        """PluginFetcherを初期化する

        Args:
            cache_dir: プラグインのキャッシュディレクトリ。
                       Noneの場合はデフォルトのキャッシュディレクトリを使用。
            http_client: HTTPクライアント（テスト用の依存性注入）。
                         Noneの場合は内部でクライアントを作成。
            plugin_configs: プラグイン設定リスト。
                            Noneの場合はデフォルト設定（extrans, wuvorbis）を使用。
        """
        self._cache_dir = cache_dir or get_plugin_cache_dir()
        self._http_client = http_client
        self._owns_client = http_client is None
        self._plugin_configs = plugin_configs if plugin_configs is not None else PLUGIN_CONFIGS

    async def get_plugins(self) -> PluginsInfo:
        """全プラグインを取得する

        キャッシュに有効なプラグインが存在する場合はそれを返し、
        存在しない場合はダウンロードする。

        Returns:
            プラグイン情報

        Raises:
            PluginDownloadError: ダウンロードに失敗した場合
        """
        if self.is_all_cache_valid():
            cached_plugins = self.get_all_cached_plugins()
            if cached_plugins is not None:
                return cached_plugins

        return await self.download_all_plugins()

    async def get_plugin(self) -> PluginInfo:
        """単一プラグインを取得する（後方互換性のため）

        最初のプラグイン（extrans）を返す。

        Returns:
            プラグイン情報

        Raises:
            PluginDownloadError: ダウンロードに失敗した場合
        """
        plugins_info = await self.get_plugins()
        first_plugin_name = self._plugin_configs[0].name
        return plugins_info.plugins[first_plugin_name]

    async def download_all_plugins(self) -> PluginsInfo:
        """全プラグインをダウンロードする

        GitHubから各プラグインの各ABI用.soをダウンロードしてキャッシュに保存する。

        Returns:
            プラグイン情報

        Raises:
            PluginDownloadError: ダウンロードに失敗した場合
        """
        self._cache_dir.mkdir(parents=True, exist_ok=True)

        plugins: dict[str, PluginInfo] = {}

        for config in self._plugin_configs:
            plugin_info = await self._download_single_plugin(config)
            plugins[config.name] = plugin_info

        return PluginsInfo(plugins=plugins)

    async def _download_single_plugin(self, config: PluginConfig) -> PluginInfo:
        """単一プラグインをダウンロードする

        Args:
            config: プラグイン設定

        Returns:
            プラグイン情報

        Raises:
            PluginDownloadError: ダウンロードに失敗した場合
        """
        paths: dict[str, Path] = {}

        for abi in SUPPORTED_ABIS:
            url = config.url_template.format(abi=abi)

            # ZIPをダウンロード
            zip_content = await self._download_zip(url, f"{config.name}/{abi}")

            # ZIPを解凍して.soを取得
            so_content = self._extract_so_from_zip(
                zip_content, config.source_filename, f"{config.name}/{abi}"
            )

            # ABI別ディレクトリに保存（lib プレフィックス付きファイル名）
            abi_dir = self._cache_dir / abi
            abi_dir.mkdir(parents=True, exist_ok=True)
            plugin_path = abi_dir / config.output_filename
            plugin_path.write_bytes(so_content)
            paths[abi] = plugin_path

        return PluginInfo(name=config.name, paths=paths)

    async def download_plugin(self) -> PluginInfo:
        """単一プラグインをダウンロードする（後方互換性のため）

        最初のプラグイン（extrans）をダウンロードして返す。

        Returns:
            プラグイン情報

        Raises:
            PluginDownloadError: ダウンロードに失敗した場合
        """
        plugins_info = await self.download_all_plugins()
        first_plugin_name = self._plugin_configs[0].name
        return plugins_info.plugins[first_plugin_name]

    async def _download_zip(self, url: str, context: str) -> bytes:
        """ZIPファイルをダウンロードする

        Args:
            url: ダウンロードURL
            context: コンテキスト情報（エラーメッセージ用）

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
            raise PluginDownloadError(f"ネットワークエラー ({context}): {e}") from e
        except httpx.HTTPStatusError as e:
            raise PluginDownloadError(
                f"HTTPエラー {e.response.status_code} ({context}): {e}"
            ) from e
        finally:
            if close_client and client is not None:
                await client.aclose()

    def _extract_so_from_zip(self, zip_content: bytes, target_filename: str, context: str) -> bytes:
        """ZIPファイルから.soファイルを抽出する

        Args:
            zip_content: ZIPファイルのバイナリデータ
            target_filename: 抽出するファイル名
            context: コンテキスト情報（エラーメッセージ用）

        Returns:
            .soファイルのバイナリデータ

        Raises:
            PluginDownloadError: ZIPの解凍に失敗した場合
        """
        try:
            with zipfile.ZipFile(io.BytesIO(zip_content), "r") as zf:
                # 対象ファイルを探す
                for name in zf.namelist():
                    if name.endswith(target_filename):
                        return zf.read(name)

                raise PluginDownloadError(
                    f"ZIPファイル内に{target_filename}が見つかりません ({context})"
                )
        except zipfile.BadZipFile as e:
            raise PluginDownloadError(f"無効なZIPファイルです ({context}): {e}") from e

    def is_all_cache_valid(self) -> bool:
        """全プラグインのキャッシュが有効かどうかを確認する

        全プラグインの全ABI用ファイルが存在する場合のみTrueを返す。

        Returns:
            キャッシュが存在し有効な場合はTrue
        """
        for config in self._plugin_configs:
            for abi in SUPPORTED_ABIS:
                plugin_path = self._cache_dir / abi / config.output_filename
                if not plugin_path.exists():
                    return False
        return True

    def is_cache_valid(self) -> bool:
        """最初のプラグインのキャッシュが有効かどうかを確認する（後方互換性のため）

        Returns:
            キャッシュが存在し有効な場合はTrue
        """
        if not self._plugin_configs:
            return False
        config = self._plugin_configs[0]
        for abi in SUPPORTED_ABIS:
            plugin_path = self._cache_dir / abi / config.output_filename
            if not plugin_path.exists():
                return False
        return True

    def get_all_cached_plugins(self) -> PluginsInfo | None:
        """全キャッシュされたプラグインを取得する

        Returns:
            プラグイン情報。キャッシュが無い場合はNone。
        """
        if not self.is_all_cache_valid():
            return None

        plugins: dict[str, PluginInfo] = {}
        for config in self._plugin_configs:
            paths: dict[str, Path] = {}
            for abi in SUPPORTED_ABIS:
                paths[abi] = self._cache_dir / abi / config.output_filename
            plugins[config.name] = PluginInfo(name=config.name, paths=paths)

        return PluginsInfo(plugins=plugins)

    def get_cached_plugin_paths(self) -> dict[str, Path] | None:
        """最初のプラグインのキャッシュパスを取得する（後方互換性のため）

        Returns:
            ABI名をキー、パスを値とする辞書。キャッシュが無い場合はNone。
        """
        if not self.is_cache_valid():
            return None

        if not self._plugin_configs:
            return None

        config = self._plugin_configs[0]
        paths: dict[str, Path] = {}
        for abi in SUPPORTED_ABIS:
            paths[abi] = self._cache_dir / abi / config.output_filename
        return paths
