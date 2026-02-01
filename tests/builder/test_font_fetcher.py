"""Koruriフォント取得機能のテスト"""

from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import httpx
import pytest

from mnemonic.builder.font_fetcher import (
    FontDownloadError,
    FontFetcher,
    FontInfo,
    get_font_cache_dir,
)

if TYPE_CHECKING:
    pass


class TestGetFontCacheDir:
    """フォントキャッシュディレクトリ取得のテスト"""

    def test_get_font_cache_dir_returns_path(self) -> None:
        """正常系: キャッシュディレクトリのパスを返す"""
        cache_dir = get_font_cache_dir()

        assert isinstance(cache_dir, Path)
        assert "mnemonic" in str(cache_dir)
        assert "fonts" in str(cache_dir)


class TestFontInfo:
    """FontInfoデータクラスのテスト"""

    def test_font_info_is_frozen(self) -> None:
        """正常系: FontInfoは不変オブジェクト"""
        info = FontInfo(
            name="Koruri-Regular",
            path=Path("/cache/fonts/Koruri-Regular.ttf"),
            version="20210720",
        )

        with pytest.raises(AttributeError):
            info.name = "other"  # type: ignore[misc]


class TestFontFetcher:
    """FontFetcherクラスのテスト"""

    @pytest.fixture
    def temp_cache_dir(self, tmp_path: Path) -> Path:
        """一時キャッシュディレクトリ"""
        cache_dir = tmp_path / "fonts"
        cache_dir.mkdir(parents=True)
        return cache_dir

    @pytest.fixture
    def mock_koruri_ttf(self) -> bytes:
        """モック用のKoruri TTFファイル"""
        return b"fake ttf content"

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        "cached_font_exists, expected_download",
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
    async def test_get_font_caching_behavior(
        self,
        temp_cache_dir: Path,
        mock_koruri_ttf: bytes,
        cached_font_exists: bool,
        expected_download: bool,
    ) -> None:
        """キャッシュ動作のテスト"""
        # キャッシュファイルを作成
        if cached_font_exists:
            font_file = temp_cache_dir / "Koruri-Regular.ttf"
            font_file.write_bytes(b"cached ttf content")

        # モックHTTPクライアントを設定
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.content = mock_koruri_ttf
        mock_response.raise_for_status = MagicMock()

        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.get = AsyncMock(return_value=mock_response)
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=None)

        fetcher = FontFetcher(cache_dir=temp_cache_dir, http_client=mock_client)
        result = await fetcher.get_font()

        assert result is not None
        assert result.name == "Koruri-Regular"
        assert result.path.exists()

        if expected_download:
            mock_client.get.assert_called_once()
        else:
            mock_client.get.assert_not_called()

    @pytest.mark.asyncio
    async def test_download_font_success(
        self,
        temp_cache_dir: Path,
        mock_koruri_ttf: bytes,
    ) -> None:
        """正常系: フォントのダウンロードが成功する"""
        # モックHTTPレスポンスを設定
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.content = mock_koruri_ttf
        mock_response.raise_for_status = MagicMock()

        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.get = AsyncMock(return_value=mock_response)
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=None)

        fetcher = FontFetcher(cache_dir=temp_cache_dir, http_client=mock_client)
        result = await fetcher.download_font()

        assert result is not None
        assert result.name == "Koruri-Regular"
        assert result.path.suffix == ".ttf"
        assert result.path.exists()
        assert result.version == "master"

    @pytest.mark.asyncio
    async def test_download_font_network_error(
        self,
        temp_cache_dir: Path,
    ) -> None:
        """異常系: ネットワークエラー時に例外を送出する"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.get = AsyncMock(side_effect=httpx.RequestError("Connection failed"))
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=None)

        fetcher = FontFetcher(cache_dir=temp_cache_dir, http_client=mock_client)

        with pytest.raises(FontDownloadError) as exc_info:
            await fetcher.download_font()

        assert "Connection failed" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_download_font_http_error(
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

        fetcher = FontFetcher(cache_dir=temp_cache_dir, http_client=mock_client)

        with pytest.raises(FontDownloadError) as exc_info:
            await fetcher.download_font()

        assert "HTTP" in str(exc_info.value) or "404" in str(exc_info.value)

    def test_is_cache_valid_returns_true_when_font_exists(
        self,
        temp_cache_dir: Path,
    ) -> None:
        """正常系: フォントファイルが存在する場合はTrueを返す"""
        # キャッシュファイルを作成
        font_file = temp_cache_dir / "Koruri-Regular.ttf"
        font_file.write_bytes(b"cached ttf content")

        fetcher = FontFetcher(cache_dir=temp_cache_dir)
        assert fetcher.is_cache_valid() is True

    def test_is_cache_valid_returns_false_when_font_not_exists(
        self,
        temp_cache_dir: Path,
    ) -> None:
        """正常系: フォントファイルが存在しない場合はFalseを返す"""
        fetcher = FontFetcher(cache_dir=temp_cache_dir)
        assert fetcher.is_cache_valid() is False

    def test_get_cached_font_path_returns_path_when_exists(
        self,
        temp_cache_dir: Path,
    ) -> None:
        """正常系: キャッシュされたフォントのパスを返す"""
        # キャッシュファイルを作成
        font_file = temp_cache_dir / "Koruri-Regular.ttf"
        font_file.write_bytes(b"cached ttf content")

        fetcher = FontFetcher(cache_dir=temp_cache_dir)
        result = fetcher.get_cached_font_path()

        assert result is not None
        assert result == font_file

    def test_get_cached_font_path_returns_none_when_not_exists(
        self,
        temp_cache_dir: Path,
    ) -> None:
        """正常系: キャッシュが無い場合はNoneを返す"""
        fetcher = FontFetcher(cache_dir=temp_cache_dir)
        result = fetcher.get_cached_font_path()

        assert result is None


class TestFontFetcherDefaultCacheDir:
    """FontFetcherのデフォルトキャッシュディレクトリのテスト"""

    def test_uses_default_cache_dir_when_not_specified(self) -> None:
        """正常系: cache_dirを指定しない場合はデフォルトを使用する"""
        fetcher = FontFetcher()

        # キャッシュディレクトリがデフォルトの場所を指している
        assert "mnemonic" in str(fetcher._cache_dir)
        assert "fonts" in str(fetcher._cache_dir)
