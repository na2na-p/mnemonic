"""extransプラグイン取得機能のテスト"""

from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import httpx
import pytest

from mnemonic.builder.plugin_fetcher import (
    PluginDownloadError,
    PluginFetcher,
    PluginInfo,
    get_plugin_cache_dir,
)

if TYPE_CHECKING:
    pass


class TestGetPluginCacheDir:
    """プラグインキャッシュディレクトリ取得のテスト"""

    def test_get_plugin_cache_dir_returns_path(self) -> None:
        """正常系: キャッシュディレクトリのパスを返す"""
        cache_dir = get_plugin_cache_dir()

        assert isinstance(cache_dir, Path)
        assert "mnemonic" in str(cache_dir)
        assert "plugins" in str(cache_dir)


class TestPluginInfo:
    """PluginInfoデータクラスのテスト"""

    def test_plugin_info_is_frozen(self) -> None:
        """正常系: PluginInfoは不変オブジェクト"""
        info = PluginInfo(
            name="extrans",
            paths={
                "arm64-v8a": Path("/cache/plugins/arm64-v8a/extrans.so"),
            },
        )

        with pytest.raises(AttributeError):
            info.name = "other"  # type: ignore[misc]


class TestPluginFetcher:
    """PluginFetcherクラスのテスト"""

    @pytest.fixture
    def temp_cache_dir(self, tmp_path: Path) -> Path:
        """一時キャッシュディレクトリ"""
        cache_dir = tmp_path / "plugins"
        cache_dir.mkdir(parents=True)
        return cache_dir

    @pytest.fixture
    def mock_plugin_zip(self) -> bytes:
        """モック用のプラグインZIPファイル"""
        import io
        import zipfile

        buffer = io.BytesIO()
        with zipfile.ZipFile(buffer, "w", zipfile.ZIP_DEFLATED) as zf:
            zf.writestr("extrans.so", b"fake so content")
        return buffer.getvalue()

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        "cached_plugin_exists, expected_download",
        [
            pytest.param(
                False,
                True,
                id="正常系: キャッシュが無い場合はダウンロードする",
            ),
            pytest.param(
                True,
                False,
                id="正常系: キャッシュが有効な場合はダウンロードしない",
            ),
        ],
    )
    async def test_get_plugin_caching_behavior(
        self,
        temp_cache_dir: Path,
        mock_plugin_zip: bytes,
        cached_plugin_exists: bool,
        expected_download: bool,
    ) -> None:
        """キャッシュ動作のテスト"""
        # キャッシュファイルを作成
        if cached_plugin_exists:
            for abi in ["arm64-v8a", "armeabi-v7a", "x86", "x86_64"]:
                abi_dir = temp_cache_dir / abi
                abi_dir.mkdir(parents=True, exist_ok=True)
                plugin_file = abi_dir / "extrans.so"
                plugin_file.write_bytes(b"cached so content")

        # モックHTTPクライアントを設定
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.content = mock_plugin_zip
        mock_response.raise_for_status = MagicMock()

        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.get = AsyncMock(return_value=mock_response)
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=None)

        fetcher = PluginFetcher(cache_dir=temp_cache_dir, http_client=mock_client)
        result = await fetcher.get_plugin()

        assert result is not None
        assert result.name == "extrans"
        assert len(result.paths) == 4  # 4つのABI

        for abi, path in result.paths.items():
            assert path.exists(), f"{abi} plugin should exist"

        if expected_download:
            # 4つのABIに対してダウンロード
            assert mock_client.get.call_count == 4
        else:
            mock_client.get.assert_not_called()

    @pytest.mark.asyncio
    async def test_download_plugin_success(
        self,
        temp_cache_dir: Path,
        mock_plugin_zip: bytes,
    ) -> None:
        """正常系: プラグインのダウンロードが成功する"""
        # モックHTTPレスポンスを設定
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.content = mock_plugin_zip
        mock_response.raise_for_status = MagicMock()

        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.get = AsyncMock(return_value=mock_response)
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=None)

        fetcher = PluginFetcher(cache_dir=temp_cache_dir, http_client=mock_client)
        result = await fetcher.download_plugin()

        assert result is not None
        assert result.name == "extrans"
        assert len(result.paths) == 4
        for _abi, path in result.paths.items():
            assert path.suffix == ".so"
            assert path.exists()

    @pytest.mark.asyncio
    async def test_download_plugin_network_error(
        self,
        temp_cache_dir: Path,
    ) -> None:
        """異常系: ネットワークエラー時に例外を送出する"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.get = AsyncMock(side_effect=httpx.RequestError("Connection failed"))
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=None)

        fetcher = PluginFetcher(cache_dir=temp_cache_dir, http_client=mock_client)

        with pytest.raises(PluginDownloadError) as exc_info:
            await fetcher.download_plugin()

        assert "Connection failed" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_download_plugin_http_error(
        self,
        temp_cache_dir: Path,
    ) -> None:
        """異常系: HTTPエラー時に例外を送出する"""
        mock_response = MagicMock()
        mock_response.status_code = 404
        mock_response.raise_for_status = MagicMock(
            side_effect=httpx.HTTPStatusError(
                "Not Found",
                request=MagicMock(),
                response=mock_response,
            )
        )

        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.get = AsyncMock(return_value=mock_response)
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=None)

        fetcher = PluginFetcher(cache_dir=temp_cache_dir, http_client=mock_client)

        with pytest.raises(PluginDownloadError) as exc_info:
            await fetcher.download_plugin()

        assert "HTTP" in str(exc_info.value) or "404" in str(exc_info.value)

    def test_is_cache_valid_returns_true_when_all_plugins_exist(
        self,
        temp_cache_dir: Path,
    ) -> None:
        """正常系: 全ABIのプラグインファイルが存在する場合はTrueを返す"""
        # 全ABIのキャッシュファイルを作成
        for abi in ["arm64-v8a", "armeabi-v7a", "x86", "x86_64"]:
            abi_dir = temp_cache_dir / abi
            abi_dir.mkdir(parents=True, exist_ok=True)
            plugin_file = abi_dir / "extrans.so"
            plugin_file.write_bytes(b"cached so content")

        fetcher = PluginFetcher(cache_dir=temp_cache_dir)
        assert fetcher.is_cache_valid() is True

    def test_is_cache_valid_returns_false_when_some_plugins_missing(
        self,
        temp_cache_dir: Path,
    ) -> None:
        """正常系: 一部ABIのプラグインファイルが欠けている場合はFalseを返す"""
        # 一部のABIのみキャッシュファイルを作成
        for abi in ["arm64-v8a", "armeabi-v7a"]:
            abi_dir = temp_cache_dir / abi
            abi_dir.mkdir(parents=True, exist_ok=True)
            plugin_file = abi_dir / "extrans.so"
            plugin_file.write_bytes(b"cached so content")

        fetcher = PluginFetcher(cache_dir=temp_cache_dir)
        assert fetcher.is_cache_valid() is False

    def test_is_cache_valid_returns_false_when_no_plugins_exist(
        self,
        temp_cache_dir: Path,
    ) -> None:
        """正常系: プラグインファイルが存在しない場合はFalseを返す"""
        fetcher = PluginFetcher(cache_dir=temp_cache_dir)
        assert fetcher.is_cache_valid() is False

    def test_get_cached_plugin_paths_returns_paths_when_exists(
        self,
        temp_cache_dir: Path,
    ) -> None:
        """正常系: キャッシュされたプラグインのパスを返す"""
        # 全ABIのキャッシュファイルを作成
        for abi in ["arm64-v8a", "armeabi-v7a", "x86", "x86_64"]:
            abi_dir = temp_cache_dir / abi
            abi_dir.mkdir(parents=True, exist_ok=True)
            plugin_file = abi_dir / "extrans.so"
            plugin_file.write_bytes(b"cached so content")

        fetcher = PluginFetcher(cache_dir=temp_cache_dir)
        result = fetcher.get_cached_plugin_paths()

        assert result is not None
        assert len(result) == 4
        for _abi, path in result.items():
            assert path.exists()

    def test_get_cached_plugin_paths_returns_none_when_not_exists(
        self,
        temp_cache_dir: Path,
    ) -> None:
        """正常系: キャッシュが無い場合はNoneを返す"""
        fetcher = PluginFetcher(cache_dir=temp_cache_dir)
        result = fetcher.get_cached_plugin_paths()

        assert result is None


class TestPluginFetcherDefaultCacheDir:
    """PluginFetcherのデフォルトキャッシュディレクトリのテスト"""

    def test_uses_default_cache_dir_when_not_specified(self) -> None:
        """正常系: cache_dirを指定しない場合はデフォルトを使用する"""
        fetcher = PluginFetcher()

        # キャッシュディレクトリがデフォルトの場所を指している
        assert "mnemonic" in str(fetcher._cache_dir)
        assert "plugins" in str(fetcher._cache_dir)
