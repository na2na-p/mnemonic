"""テンプレートダウンロード機能のテスト"""

from collections.abc import AsyncIterator
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

from mnemonic.builder.template import (
    FileIntegrityError,
    NetworkError,
    TemplateDownloader,
    TemplateInfo,
    TemplateNotFoundError,
)


async def async_byte_iterator(data: bytes) -> AsyncIterator[bytes]:
    """バイトデータを非同期イテレータとして返すヘルパー関数"""
    yield data


class TestTemplateInfo:
    """TemplateInfoデータクラスのテスト"""

    def test_creation(self) -> None:
        """正常系: TemplateInfoの作成テスト"""
        info = TemplateInfo(
            version="v1.0.0",
            download_url="https://example.com/template.zip",
            file_size=1024,
            file_name="template.zip",
        )

        assert info.version == "v1.0.0"
        assert info.download_url == "https://example.com/template.zip"
        assert info.file_size == 1024
        assert info.file_name == "template.zip"

    def test_immutability(self) -> None:
        """正常系: TemplateInfoが不変であることのテスト"""
        info = TemplateInfo(
            version="v1.0.0",
            download_url="https://example.com/template.zip",
            file_size=1024,
            file_name="template.zip",
        )

        with pytest.raises(AttributeError):
            info.version = "v2.0.0"  # type: ignore[misc]


class TestTemplateDownloaderInit:
    """TemplateDownloader初期化のテスト"""

    def test_init_with_default_cache_dir(self) -> None:
        """正常系: デフォルトキャッシュディレクトリでの初期化"""
        downloader = TemplateDownloader()
        assert downloader._cache_dir is None

    def test_init_with_custom_cache_dir(self, tmp_path: Path) -> None:
        """正常系: カスタムキャッシュディレクトリでの初期化"""
        cache_dir = tmp_path / "cache"
        downloader = TemplateDownloader(cache_dir=cache_dir)
        assert downloader._cache_dir == cache_dir

    def test_init_with_http_client(self) -> None:
        """正常系: HTTPクライアント注入での初期化"""
        mock_client = MagicMock(spec=httpx.AsyncClient)
        downloader = TemplateDownloader(http_client=mock_client)
        assert downloader._http_client is mock_client
        assert downloader._owns_client is False


class TestTemplateDownloaderGetLatestVersion:
    """TemplateDownloader.get_latest_versionのテスト"""

    @pytest.mark.asyncio
    async def test_get_latest_version_success(self) -> None:
        """正常系: GitHub APIから最新バージョンを取得"""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"tag_name": "template-2026.01.31"}
        mock_response.raise_for_status = MagicMock()

        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.get.return_value = mock_response

        downloader = TemplateDownloader(http_client=mock_client)
        version = await downloader.get_latest_version()

        assert version == "template-2026.01.31"
        mock_client.get.assert_called_once()

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        "api_response,expected_version",
        [
            pytest.param(
                {"tag_name": "template-2026.01.31"},
                "template-2026.01.31",
                id="正常系: template-2026.01.31形式のバージョン",
            ),
            pytest.param(
                {"tag_name": "template-2026.03.20"},
                "template-2026.03.20",
                id="正常系: template-2026.03.20形式のバージョン",
            ),
        ],
    )
    async def test_get_latest_version_parses_api_response(
        self, api_response: dict, expected_version: str
    ) -> None:
        """正常系: APIレスポンスからバージョンを正しくパース"""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = api_response
        mock_response.raise_for_status = MagicMock()

        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.get.return_value = mock_response

        downloader = TemplateDownloader(http_client=mock_client)
        version = await downloader.get_latest_version()

        assert version == expected_version

    @pytest.mark.asyncio
    async def test_get_latest_version_network_error(self) -> None:
        """異常系: ネットワークエラー時にNetworkErrorが発生"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.get.side_effect = httpx.RequestError("Connection failed")

        downloader = TemplateDownloader(http_client=mock_client)

        with pytest.raises(NetworkError) as exc_info:
            await downloader.get_latest_version()

        assert "Network error occurred" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_get_latest_version_timeout_error(self) -> None:
        """異常系: タイムアウト時にNetworkErrorが発生"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.get.side_effect = httpx.TimeoutException("Request timed out")

        downloader = TemplateDownloader(http_client=mock_client)

        with pytest.raises(NetworkError) as exc_info:
            await downloader.get_latest_version()

        assert "timed out" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_get_latest_version_fallback_to_releases_on_404(self) -> None:
        """正常系: /releases/latestが404の場合、/releasesにフォールバック"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)

        # 最初の呼び出し: /releases/latest が404を返す
        latest_response = MagicMock()
        latest_response.status_code = 404

        # 2回目の呼び出し: /releases がリリース一覧を返す
        releases_response = MagicMock()
        releases_response.status_code = 200
        releases_response.json.return_value = [
            {"tag_name": "template-2026.01.15"},
            {"tag_name": "template-2026.01.10"},
            {"tag_name": "template-2026.01.31"},
        ]
        releases_response.raise_for_status = MagicMock()

        mock_client.get.side_effect = [latest_response, releases_response]

        downloader = TemplateDownloader(http_client=mock_client)
        version = await downloader.get_latest_version()

        assert version == "template-2026.01.15"
        assert mock_client.get.call_count == 2

    @pytest.mark.asyncio
    async def test_get_latest_version_fallback_with_empty_releases(self) -> None:
        """異常系: フォールバック先でリリースが空の場合、NetworkErrorが発生"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)

        # /releases/latest が404を返す
        latest_response = MagicMock()
        latest_response.status_code = 404

        # /releases が空のリストを返す
        releases_response = MagicMock()
        releases_response.status_code = 200
        releases_response.json.return_value = []
        releases_response.raise_for_status = MagicMock()

        mock_client.get.side_effect = [latest_response, releases_response]

        downloader = TemplateDownloader(http_client=mock_client)

        with pytest.raises(NetworkError) as exc_info:
            await downloader.get_latest_version()

        assert "リリースが見つかりません" in str(exc_info.value)


class TestTemplateDownloaderGetDownloadUrl:
    """TemplateDownloader.get_download_urlのテスト"""

    @pytest.mark.parametrize(
        "version,expected_url_contains",
        [
            pytest.param(
                "template-2026.01.31",
                "template-2026.01.31",
                id="正常系: template-2026.01.31でURL生成",
            ),
            pytest.param(
                "template-2026.03.20",
                "template-2026.03.20",
                id="正常系: template-2026.03.20でURL生成",
            ),
        ],
    )
    def test_get_download_url_success(self, version: str, expected_url_contains: str) -> None:
        """正常系: ダウンロードURLが正しく構築される"""
        downloader = TemplateDownloader()

        url = downloader.get_download_url(version)

        assert expected_url_contains in url
        assert "mnemonic" in url
        assert "github.com" in url

    @pytest.mark.parametrize(
        "invalid_version",
        [
            pytest.param("", id="異常系: 空文字のバージョン"),
            pytest.param("invalid", id="異常系: 不正なバージョン形式"),
            pytest.param("v", id="異常系: vのみのバージョン"),
        ],
    )
    def test_get_download_url_invalid_version(self, invalid_version: str) -> None:
        """異常系: 不正なバージョン形式でValueError"""
        downloader = TemplateDownloader()

        with pytest.raises(ValueError) as exc_info:
            downloader.get_download_url(invalid_version)

        assert "Invalid version format" in str(exc_info.value)


def create_mock_stream_response(data: bytes) -> MagicMock:
    """ダウンロードストリームレスポンスのモックを作成する"""
    mock_stream_response = MagicMock()
    mock_stream_response.status_code = 200
    mock_stream_response.raise_for_status = MagicMock()
    mock_stream_response.aiter_bytes = lambda chunk_size=8192: async_byte_iterator(data)
    return mock_stream_response


class TestTemplateDownloaderDownload:
    """TemplateDownloader.downloadのテスト"""

    @pytest.mark.asyncio
    async def test_download_with_specific_version(self, tmp_path: Path) -> None:
        """正常系: 指定バージョンのダウンロードが成功"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)

        # リリース情報取得のモック
        release_response = MagicMock()
        release_response.status_code = 200
        release_response.json.return_value = {
            "tag_name": "template-2026.01.31",
            "assets": [
                {
                    "name": "android-template.zip",
                    "browser_download_url": "https://example.com/template.zip",
                    "size": 100,
                }
            ],
        }
        release_response.raise_for_status = MagicMock()

        mock_client.get.return_value = release_response

        # ダウンロードストリームのモック
        mock_stream_response = create_mock_stream_response(b"test content")

        mock_stream_context = AsyncMock()
        mock_stream_context.__aenter__.return_value = mock_stream_response
        mock_stream_context.__aexit__.return_value = None
        mock_client.stream.return_value = mock_stream_context

        downloader = TemplateDownloader(cache_dir=tmp_path, http_client=mock_client)

        # ファイルサイズを一致させるためにモック
        with patch.object(downloader, "_verify_file_integrity"):
            result = await downloader.download(version="template-2026.01.31")

        assert result.parent.parent == tmp_path

    @pytest.mark.asyncio
    async def test_download_latest_version_when_none_specified(self, tmp_path: Path) -> None:
        """正常系: バージョン未指定時は最新版をダウンロード"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)

        # 最新バージョン取得のモック
        latest_response = MagicMock()
        latest_response.status_code = 200
        latest_response.json.return_value = {"tag_name": "template-2026.02.15"}
        latest_response.raise_for_status = MagicMock()

        # リリース情報取得のモック
        release_response = MagicMock()
        release_response.status_code = 200
        release_response.json.return_value = {
            "tag_name": "template-2026.02.15",
            "assets": [
                {
                    "name": "android-template.zip",
                    "browser_download_url": "https://example.com/template.zip",
                    "size": 100,
                }
            ],
        }
        release_response.raise_for_status = MagicMock()

        mock_client.get.side_effect = [latest_response, release_response]

        # ダウンロードストリームのモック
        mock_stream_response = create_mock_stream_response(b"test content")

        mock_stream_context = AsyncMock()
        mock_stream_context.__aenter__.return_value = mock_stream_response
        mock_stream_context.__aexit__.return_value = None
        mock_client.stream.return_value = mock_stream_context

        downloader = TemplateDownloader(cache_dir=tmp_path, http_client=mock_client)

        with patch.object(downloader, "_verify_file_integrity"):
            result = await downloader.download()

        assert "template-2026.02.15" in str(result)

    @pytest.mark.asyncio
    async def test_download_returns_path_to_template(self, tmp_path: Path) -> None:
        """正常系: ダウンロード成功時にテンプレートのパスを返す"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)

        release_response = MagicMock()
        release_response.status_code = 200
        release_response.json.return_value = {
            "tag_name": "template-2026.01.31",
            "assets": [
                {
                    "name": "android-template.zip",
                    "browser_download_url": "https://example.com/template.zip",
                    "size": 100,
                }
            ],
        }
        release_response.raise_for_status = MagicMock()
        mock_client.get.return_value = release_response

        mock_stream_response = create_mock_stream_response(b"test content")

        mock_stream_context = AsyncMock()
        mock_stream_context.__aenter__.return_value = mock_stream_response
        mock_stream_context.__aexit__.return_value = None
        mock_client.stream.return_value = mock_stream_context

        downloader = TemplateDownloader(cache_dir=tmp_path, http_client=mock_client)

        with patch.object(downloader, "_verify_file_integrity"):
            result = await downloader.download(version="template-2026.01.31")

        assert isinstance(result, Path)
        assert result.name == "android-template.zip"

    @pytest.mark.asyncio
    async def test_download_template_not_found_error(self, tmp_path: Path) -> None:
        """異常系: 存在しないバージョン指定でTemplateNotFoundError"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)

        mock_response = MagicMock()
        mock_response.status_code = 404
        mock_client.get.return_value = mock_response

        downloader = TemplateDownloader(cache_dir=tmp_path, http_client=mock_client)

        with pytest.raises(TemplateNotFoundError) as exc_info:
            await downloader.download(version="template-9999.99.99")

        assert "not found" in str(exc_info.value).lower()

    @pytest.mark.asyncio
    async def test_download_network_error(self, tmp_path: Path) -> None:
        """異常系: ネットワークエラー時にNetworkError"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        mock_client.get.side_effect = httpx.RequestError("Connection failed")

        downloader = TemplateDownloader(cache_dir=tmp_path, http_client=mock_client)

        with pytest.raises(NetworkError) as exc_info:
            await downloader.download(version="template-2026.01.31")

        assert "Network error" in str(exc_info.value)


class TestTemplateDownloaderIntegrityCheck:
    """ダウンロードファイルの整合性チェックのテスト"""

    @pytest.mark.asyncio
    async def test_download_verifies_file_integrity(self, tmp_path: Path) -> None:
        """正常系: ダウンロードファイルの整合性チェックが成功"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        test_content = b"test content"

        release_response = MagicMock()
        release_response.status_code = 200
        release_response.json.return_value = {
            "tag_name": "template-2026.01.31",
            "assets": [
                {
                    "name": "android-template.zip",
                    "browser_download_url": "https://example.com/template.zip",
                    "size": len(test_content),  # 実際のコンテンツサイズ
                }
            ],
        }
        release_response.raise_for_status = MagicMock()
        mock_client.get.return_value = release_response

        mock_stream_response = create_mock_stream_response(test_content)

        mock_stream_context = AsyncMock()
        mock_stream_context.__aenter__.return_value = mock_stream_response
        mock_stream_context.__aexit__.return_value = None
        mock_client.stream.return_value = mock_stream_context

        downloader = TemplateDownloader(cache_dir=tmp_path, http_client=mock_client)

        result = await downloader.download(version="template-2026.01.31")

        assert result.exists()
        assert result.stat().st_size == len(test_content)

    @pytest.mark.asyncio
    async def test_download_fails_on_corrupted_file(self, tmp_path: Path) -> None:
        """異常系: 破損ファイルのダウンロード時にFileIntegrityError"""
        mock_client = AsyncMock(spec=httpx.AsyncClient)
        test_content = b"test content"

        release_response = MagicMock()
        release_response.status_code = 200
        release_response.json.return_value = {
            "tag_name": "template-2026.01.31",
            "assets": [
                {
                    "name": "android-template.zip",
                    "browser_download_url": "https://example.com/template.zip",
                    "size": 1000,  # ファイルサイズ不一致を起こす
                }
            ],
        }
        release_response.raise_for_status = MagicMock()
        mock_client.get.return_value = release_response

        mock_stream_response = create_mock_stream_response(test_content)

        mock_stream_context = AsyncMock()
        mock_stream_context.__aenter__.return_value = mock_stream_response
        mock_stream_context.__aexit__.return_value = None
        mock_client.stream.return_value = mock_stream_context

        downloader = TemplateDownloader(cache_dir=tmp_path, http_client=mock_client)

        with pytest.raises(FileIntegrityError) as exc_info:
            await downloader.download(version="template-2026.01.31")

        assert "mismatch" in str(exc_info.value).lower()


class TestExceptionClasses:
    """例外クラスのテスト"""

    def test_template_not_found_error_inheritance(self) -> None:
        """正常系: TemplateNotFoundErrorが適切な継承関係を持つ"""
        error = TemplateNotFoundError("version not found")
        assert isinstance(error, Exception)
        assert str(error) == "version not found"

    def test_network_error_inheritance(self) -> None:
        """正常系: NetworkErrorが適切な継承関係を持つ"""
        error = NetworkError("connection failed")
        assert isinstance(error, Exception)
        assert str(error) == "connection failed"

    def test_file_integrity_error_inheritance(self) -> None:
        """正常系: FileIntegrityErrorが適切な継承関係を持つ"""
        error = FileIntegrityError("file corrupted")
        assert isinstance(error, Exception)
        assert str(error) == "file corrupted"
