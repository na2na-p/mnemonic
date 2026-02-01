"""SDL2 Java ソース取得機能のテスト

SDL2SourceFetcher、SDL2SourceCache クラスと関連する例外クラスのユニットテストを提供します。
"""

from datetime import datetime, timedelta
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

from mnemonic.builder.sdl2_sources import (
    SDL2SourceCache,
    SDL2SourceCacheError,
    SDL2SourceCacheInfo,
    SDL2SourceFetcher,
    SDL2SourceFetcherError,
    SDL2SourceFetchNetworkError,
    SDL2SourceFetchTimeoutError,
)


class TestSDL2SourceCacheInit:
    """SDL2SourceCache 初期化のテスト"""

    def test_init_sets_cache_dir(self, tmp_path: Path) -> None:
        """正常系: キャッシュディレクトリが正しく設定される"""
        cache = SDL2SourceCache(cache_dir=tmp_path)

        assert cache.cache_path == tmp_path / "sdl2_sources"

    @pytest.mark.parametrize(
        "dir_name",
        [
            pytest.param("cache", id="正常系: 通常のディレクトリ名"),
            pytest.param("my-cache", id="正常系: ハイフン付きディレクトリ名"),
            pytest.param("cache_123", id="正常系: 数字付きディレクトリ名"),
        ],
    )
    def test_init_with_various_cache_dirs(self, tmp_path: Path, dir_name: str) -> None:
        """正常系: 様々なディレクトリ名で初期化できる"""
        cache_dir = tmp_path / dir_name
        cache_dir.mkdir()

        cache = SDL2SourceCache(cache_dir=cache_dir)

        assert cache.cache_path == cache_dir / "sdl2_sources"


class TestSDL2SourceCacheIsValid:
    """SDL2SourceCache.is_valid のテスト"""

    def test_is_valid_returns_false_when_cache_not_exists(self, tmp_path: Path) -> None:
        """正常系: キャッシュが存在しない場合は False"""
        cache = SDL2SourceCache(cache_dir=tmp_path)

        assert cache.is_valid() is False

    def test_is_valid_returns_false_when_marker_not_exists(self, tmp_path: Path) -> None:
        """正常系: マーカーファイルがない場合は False"""
        cache = SDL2SourceCache(cache_dir=tmp_path)
        cache.cache_path.mkdir(parents=True)

        assert cache.is_valid() is False

    def test_is_valid_returns_true_when_cache_is_recent(self, tmp_path: Path) -> None:
        """正常系: キャッシュが有効期限内かつバージョンが一致する場合は True"""
        cache = SDL2SourceCache(cache_dir=tmp_path)
        cache.cache_path.mkdir(parents=True)

        marker = cache.cache_path / ".cached_at"
        marker.write_text(datetime.now().isoformat(), encoding="utf-8")

        version_file = cache.cache_path / ".version"
        version_file.write_text(cache.CURRENT_VERSION, encoding="utf-8")

        assert cache.is_valid() is True

    def test_is_valid_returns_false_when_cache_is_expired(self, tmp_path: Path) -> None:
        """正常系: キャッシュが有効期限切れの場合は False"""
        cache = SDL2SourceCache(cache_dir=tmp_path)
        cache.cache_path.mkdir(parents=True)

        marker = cache.cache_path / ".cached_at"
        expired_date = datetime.now() - timedelta(days=31)
        marker.write_text(expired_date.isoformat(), encoding="utf-8")

        assert cache.is_valid() is False

    def test_is_valid_returns_false_when_version_mismatch(self, tmp_path: Path) -> None:
        """正常系: バージョンが不一致の場合は False"""
        cache = SDL2SourceCache(cache_dir=tmp_path)
        cache.cache_path.mkdir(parents=True)

        marker = cache.cache_path / ".cached_at"
        marker.write_text(datetime.now().isoformat(), encoding="utf-8")

        version_file = cache.cache_path / ".version"
        version_file.write_text("old_version", encoding="utf-8")

        assert cache.is_valid() is False

    def test_is_valid_returns_false_when_version_file_missing(self, tmp_path: Path) -> None:
        """正常系: バージョンファイルがない場合は False"""
        cache = SDL2SourceCache(cache_dir=tmp_path)
        cache.cache_path.mkdir(parents=True)

        marker = cache.cache_path / ".cached_at"
        marker.write_text(datetime.now().isoformat(), encoding="utf-8")

        # バージョンファイルを作成しない

        assert cache.is_valid() is False

    def test_is_valid_returns_false_when_marker_is_invalid(self, tmp_path: Path) -> None:
        """正常系: マーカーファイルの内容が不正な場合は False"""
        cache = SDL2SourceCache(cache_dir=tmp_path)
        cache.cache_path.mkdir(parents=True)

        marker = cache.cache_path / ".cached_at"
        marker.write_text("invalid date format", encoding="utf-8")

        assert cache.is_valid() is False


class TestSDL2SourceCacheGetCachedAt:
    """SDL2SourceCache.get_cached_at のテスト"""

    def test_get_cached_at_returns_none_when_not_exists(self, tmp_path: Path) -> None:
        """正常系: マーカーが存在しない場合は None"""
        cache = SDL2SourceCache(cache_dir=tmp_path)

        assert cache.get_cached_at() is None

    def test_get_cached_at_returns_datetime_when_valid(self, tmp_path: Path) -> None:
        """正常系: マーカーが存在する場合は datetime を返す"""
        cache = SDL2SourceCache(cache_dir=tmp_path)
        cache.cache_path.mkdir(parents=True)

        now = datetime.now()
        marker = cache.cache_path / ".cached_at"
        marker.write_text(now.isoformat(), encoding="utf-8")

        result = cache.get_cached_at()

        assert result is not None
        assert abs((result - now).total_seconds()) < 1


class TestSDL2SourceCacheGetCacheInfo:
    """SDL2SourceCache.get_cache_info のテスト"""

    def test_get_cache_info_returns_info_object(self, tmp_path: Path) -> None:
        """正常系: SDL2SourceCacheInfo オブジェクトを返す"""
        cache = SDL2SourceCache(cache_dir=tmp_path)
        cache.cache_path.mkdir(parents=True)

        now = datetime.now()
        marker = cache.cache_path / ".cached_at"
        marker.write_text(now.isoformat(), encoding="utf-8")

        version_file = cache.cache_path / ".version"
        version_file.write_text(cache.CURRENT_VERSION, encoding="utf-8")

        info = cache.get_cache_info()

        assert isinstance(info, SDL2SourceCacheInfo)
        assert info.is_valid is True
        assert info.cache_path == cache.cache_path
        assert info.cached_at is not None


class TestSDL2SourceCacheSave:
    """SDL2SourceCache.save のテスト"""

    def test_save_copies_org_directory(self, tmp_path: Path) -> None:
        """正常系: org ディレクトリをキャッシュにコピーする"""
        cache = SDL2SourceCache(cache_dir=tmp_path / "cache")

        # ソースディレクトリを作成
        source_dir = tmp_path / "source"
        org_dir = source_dir / "org" / "libsdl" / "app"
        org_dir.mkdir(parents=True)
        (org_dir / "SDLActivity.java").write_text("test content", encoding="utf-8")

        cache.save(source_dir)

        cached_file = cache.get_source_files_path() / "SDLActivity.java"
        assert cached_file.exists()
        assert cached_file.read_text(encoding="utf-8") == "test content"

    def test_save_creates_marker_file(self, tmp_path: Path) -> None:
        """正常系: キャッシュマーカーファイルを作成する"""
        cache = SDL2SourceCache(cache_dir=tmp_path / "cache")

        source_dir = tmp_path / "source"
        org_dir = source_dir / "org" / "libsdl" / "app"
        org_dir.mkdir(parents=True)
        (org_dir / "SDLActivity.java").write_text("test content", encoding="utf-8")

        cache.save(source_dir)

        marker = cache.cache_path / ".cached_at"
        assert marker.exists()

    def test_save_overwrites_existing_cache(self, tmp_path: Path) -> None:
        """正常系: 既存のキャッシュを上書きする"""
        cache = SDL2SourceCache(cache_dir=tmp_path / "cache")

        # 最初のキャッシュを作成
        source_dir1 = tmp_path / "source1"
        org_dir1 = source_dir1 / "org" / "libsdl" / "app"
        org_dir1.mkdir(parents=True)
        (org_dir1 / "SDLActivity.java").write_text("old content", encoding="utf-8")
        cache.save(source_dir1)

        # 新しいソースで上書き
        source_dir2 = tmp_path / "source2"
        org_dir2 = source_dir2 / "org" / "libsdl" / "app"
        org_dir2.mkdir(parents=True)
        (org_dir2 / "SDLActivity.java").write_text("new content", encoding="utf-8")
        cache.save(source_dir2)

        cached_file = cache.get_source_files_path() / "SDLActivity.java"
        assert cached_file.read_text(encoding="utf-8") == "new content"


class TestSDL2SourceCacheRestoreTo:
    """SDL2SourceCache.restore_to のテスト"""

    def test_restore_to_copies_to_destination(self, tmp_path: Path) -> None:
        """正常系: キャッシュからソースを復元する"""
        cache = SDL2SourceCache(cache_dir=tmp_path / "cache")

        # キャッシュを作成
        source_dir = tmp_path / "source"
        org_dir = source_dir / "org" / "libsdl" / "app"
        org_dir.mkdir(parents=True)
        (org_dir / "SDLActivity.java").write_text("test content", encoding="utf-8")
        cache.save(source_dir)

        # 復元
        dest_dir = tmp_path / "dest"
        dest_dir.mkdir()
        cache.restore_to(dest_dir)

        restored_file = dest_dir / "org" / "libsdl" / "app" / "SDLActivity.java"
        assert restored_file.exists()
        assert restored_file.read_text(encoding="utf-8") == "test content"

    def test_restore_to_raises_error_when_cache_invalid(self, tmp_path: Path) -> None:
        """異常系: キャッシュが無効な場合はエラー"""
        cache = SDL2SourceCache(cache_dir=tmp_path / "cache")
        dest_dir = tmp_path / "dest"
        dest_dir.mkdir()

        with pytest.raises(SDL2SourceCacheError) as exc_info:
            cache.restore_to(dest_dir)

        assert "有効なキャッシュがありません" in str(exc_info.value)


class TestSDL2SourceCacheClear:
    """SDL2SourceCache.clear のテスト"""

    def test_clear_removes_cache_directory(self, tmp_path: Path) -> None:
        """正常系: キャッシュディレクトリを削除する"""
        cache = SDL2SourceCache(cache_dir=tmp_path / "cache")

        # キャッシュを作成
        source_dir = tmp_path / "source"
        org_dir = source_dir / "org" / "libsdl" / "app"
        org_dir.mkdir(parents=True)
        (org_dir / "SDLActivity.java").write_text("test content", encoding="utf-8")
        cache.save(source_dir)

        assert cache.cache_path.exists()

        cache.clear()

        assert not cache.cache_path.exists()

    def test_clear_does_nothing_when_cache_not_exists(self, tmp_path: Path) -> None:
        """正常系: キャッシュが存在しなくてもエラーにならない"""
        cache = SDL2SourceCache(cache_dir=tmp_path / "cache")

        # エラーが発生しないことを確認
        cache.clear()


class TestSDL2SourceFetcherInit:
    """SDL2SourceFetcher 初期化のテスト"""

    def test_init_sets_default_timeout(self) -> None:
        """正常系: デフォルトタイムアウトが設定される"""
        fetcher = SDL2SourceFetcher()

        assert fetcher._timeout == 30.0

    def test_init_sets_custom_timeout(self) -> None:
        """正常系: カスタムタイムアウトを設定できる"""
        fetcher = SDL2SourceFetcher(timeout=60.0)

        assert fetcher._timeout == 60.0

    def test_init_sets_cache(self, tmp_path: Path) -> None:
        """正常系: キャッシュを設定できる"""
        cache = SDL2SourceCache(cache_dir=tmp_path)
        fetcher = SDL2SourceFetcher(cache=cache)

        assert fetcher._cache is cache


class TestSDL2SourceFetcherFetch:
    """SDL2SourceFetcher.fetch のテスト"""

    @pytest.mark.asyncio
    async def test_fetch_restores_from_valid_cache(self, tmp_path: Path) -> None:
        """正常系: 有効なキャッシュがある場合は復元する"""
        cache = SDL2SourceCache(cache_dir=tmp_path / "cache")

        # キャッシュを作成
        source_dir = tmp_path / "source"
        org_dir = source_dir / "org" / "libsdl" / "app"
        org_dir.mkdir(parents=True)
        (org_dir / "SDLActivity.java").write_text("cached content", encoding="utf-8")
        cache.save(source_dir)

        fetcher = SDL2SourceFetcher(cache=cache)
        dest_dir = tmp_path / "dest"
        dest_dir.mkdir()

        await fetcher.fetch(dest_dir)

        restored_file = dest_dir / "org" / "libsdl" / "app" / "SDLActivity.java"
        assert restored_file.exists()
        assert restored_file.read_text(encoding="utf-8") == "cached content"

    @pytest.mark.asyncio
    async def test_fetch_downloads_when_cache_invalid(self, tmp_path: Path) -> None:
        """正常系: キャッシュが無効な場合はダウンロードする"""
        cache = SDL2SourceCache(cache_dir=tmp_path / "cache")
        fetcher = SDL2SourceFetcher(cache=cache)
        dest_dir = tmp_path / "dest"
        dest_dir.mkdir()

        # モックでHTTP応答を返す
        mock_response = MagicMock()
        mock_response.text = "downloaded content"
        mock_response.raise_for_status = MagicMock()

        with patch("mnemonic.builder.sdl2_sources.httpx.AsyncClient") as mock_client:
            mock_instance = AsyncMock()
            mock_instance.get.return_value = mock_response
            mock_instance.__aenter__.return_value = mock_instance
            mock_instance.__aexit__.return_value = None
            mock_client.return_value = mock_instance

            await fetcher.fetch(dest_dir)

        # 8ファイルすべてが作成されている (krkrsdl2 互換コミット)
        sdl_app_dir = dest_dir / "org" / "libsdl" / "app"
        assert sdl_app_dir.exists()
        assert (sdl_app_dir / "SDLActivity.java").exists()
        assert mock_instance.get.call_count == 8

    @pytest.mark.asyncio
    async def test_fetch_saves_to_cache_after_download(self, tmp_path: Path) -> None:
        """正常系: ダウンロード後にキャッシュに保存する"""
        cache = SDL2SourceCache(cache_dir=tmp_path / "cache")
        fetcher = SDL2SourceFetcher(cache=cache)
        dest_dir = tmp_path / "dest"
        dest_dir.mkdir()

        mock_response = MagicMock()
        mock_response.text = "downloaded content"
        mock_response.raise_for_status = MagicMock()

        with patch("mnemonic.builder.sdl2_sources.httpx.AsyncClient") as mock_client:
            mock_instance = AsyncMock()
            mock_instance.get.return_value = mock_response
            mock_instance.__aenter__.return_value = mock_instance
            mock_instance.__aexit__.return_value = None
            mock_client.return_value = mock_instance

            await fetcher.fetch(dest_dir)

        # キャッシュが有効になっている
        assert cache.is_valid()

    @pytest.mark.asyncio
    async def test_fetch_raises_network_error_on_http_error(self, tmp_path: Path) -> None:
        """異常系: HTTP エラーの場合は SDL2SourceFetchNetworkError"""
        fetcher = SDL2SourceFetcher()
        dest_dir = tmp_path / "dest"
        dest_dir.mkdir()

        mock_request = MagicMock()
        mock_response = MagicMock()
        mock_response.status_code = 404

        with patch("mnemonic.builder.sdl2_sources.httpx.AsyncClient") as mock_client:
            mock_instance = AsyncMock()
            mock_instance.get.side_effect = httpx.HTTPStatusError(
                "Not Found", request=mock_request, response=mock_response
            )
            mock_instance.__aenter__.return_value = mock_instance
            mock_instance.__aexit__.return_value = None
            mock_client.return_value = mock_instance

            with pytest.raises(SDL2SourceFetchNetworkError) as exc_info:
                await fetcher.fetch(dest_dir)

            assert "HTTP 404" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_fetch_raises_timeout_error(self, tmp_path: Path) -> None:
        """異常系: タイムアウトの場合は SDL2SourceFetchTimeoutError"""
        fetcher = SDL2SourceFetcher()
        dest_dir = tmp_path / "dest"
        dest_dir.mkdir()

        with patch("mnemonic.builder.sdl2_sources.httpx.AsyncClient") as mock_client:
            mock_instance = AsyncMock()
            mock_instance.get.side_effect = httpx.TimeoutException("Timeout")
            mock_instance.__aenter__.return_value = mock_instance
            mock_instance.__aexit__.return_value = None
            mock_client.return_value = mock_instance

            with pytest.raises(SDL2SourceFetchTimeoutError) as exc_info:
                await fetcher.fetch(dest_dir)

            assert "タイムアウト" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_fetch_raises_network_error_on_request_error(self, tmp_path: Path) -> None:
        """異常系: 接続エラーの場合は SDL2SourceFetchNetworkError"""
        fetcher = SDL2SourceFetcher()
        dest_dir = tmp_path / "dest"
        dest_dir.mkdir()

        with patch("mnemonic.builder.sdl2_sources.httpx.AsyncClient") as mock_client:
            mock_instance = AsyncMock()
            mock_instance.get.side_effect = httpx.RequestError("Connection refused")
            mock_instance.__aenter__.return_value = mock_instance
            mock_instance.__aexit__.return_value = None
            mock_client.return_value = mock_instance

            with pytest.raises(SDL2SourceFetchNetworkError) as exc_info:
                await fetcher.fetch(dest_dir)

            assert "ダウンロードに失敗" in str(exc_info.value)


class TestSDL2SourceFetcherRequiredFiles:
    """SDL2SourceFetcher.REQUIRED_FILES のテスト"""

    def test_required_files_contains_all_expected_files(self) -> None:
        """正常系: すべての必要なファイルが含まれている (krkrsdl2 互換コミット)"""
        expected_files = [
            "SDLActivity.java",
            "SDL.java",
            "SDLAudioManager.java",
            "SDLControllerManager.java",
            "HIDDevice.java",
            "HIDDeviceManager.java",
            "HIDDeviceUSB.java",
            "HIDDeviceBLESteamController.java",
        ]

        assert expected_files == SDL2SourceFetcher.REQUIRED_FILES


class TestExceptionClasses:
    """例外クラスのテスト"""

    def test_sdl2_source_fetcher_error_is_exception(self) -> None:
        """正常系: SDL2SourceFetcherError は Exception を継承"""
        error = SDL2SourceFetcherError("test error")
        assert isinstance(error, Exception)
        assert str(error) == "test error"

    def test_sdl2_source_fetch_network_error_is_fetcher_error(self) -> None:
        """正常系: SDL2SourceFetchNetworkError は SDL2SourceFetcherError を継承"""
        error = SDL2SourceFetchNetworkError("network error")
        assert isinstance(error, SDL2SourceFetcherError)
        assert str(error) == "network error"

    def test_sdl2_source_fetch_timeout_error_is_fetcher_error(self) -> None:
        """正常系: SDL2SourceFetchTimeoutError は SDL2SourceFetcherError を継承"""
        error = SDL2SourceFetchTimeoutError("timeout error")
        assert isinstance(error, SDL2SourceFetcherError)
        assert str(error) == "timeout error"

    def test_sdl2_source_cache_error_is_fetcher_error(self) -> None:
        """正常系: SDL2SourceCacheError は SDL2SourceFetcherError を継承"""
        error = SDL2SourceCacheError("cache error")
        assert isinstance(error, SDL2SourceFetcherError)
        assert str(error) == "cache error"


class TestSDL2SourceCacheInfo:
    """SDL2SourceCacheInfo データクラスのテスト"""

    def test_cache_info_is_frozen(self, tmp_path: Path) -> None:
        """正常系: SDL2SourceCacheInfo は frozen dataclass"""
        info = SDL2SourceCacheInfo(
            cached_at=datetime.now(),
            is_valid=True,
            cache_path=tmp_path,
        )

        with pytest.raises(AttributeError):
            info.is_valid = False  # type: ignore[misc]

    def test_cache_info_stores_all_fields(self, tmp_path: Path) -> None:
        """正常系: すべてのフィールドが保存される"""
        now = datetime.now()
        info = SDL2SourceCacheInfo(
            cached_at=now,
            is_valid=True,
            cache_path=tmp_path,
        )

        assert info.cached_at == now
        assert info.is_valid is True
        assert info.cache_path == tmp_path
