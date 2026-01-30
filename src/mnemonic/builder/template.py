"""テンプレートダウンロード機能"""

from __future__ import annotations

import json
import re
import shutil
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import TYPE_CHECKING, Any

import httpx

if TYPE_CHECKING:
    from mnemonic.cache import CacheManager

class TemplateDownloadError(Exception):
    """テンプレートダウンロードに関する基本例外クラス"""

    pass

class TemplateCacheError(Exception):
    """テンプレートキャッシュに関する基本例外クラス"""

    pass

class TemplateNotFoundError(TemplateDownloadError):
    """指定されたバージョンのテンプレートが存在しない場合の例外"""

    pass

class NetworkError(TemplateDownloadError):
    """ネットワークエラー発生時の例外"""

    pass

class FileIntegrityError(TemplateDownloadError):
    """ファイル整合性チェックに失敗した場合の例外"""

    pass

@dataclass(frozen=True)
class TemplateInfo:
    """テンプレート情報を表す不変オブジェクト"""

    version: str
    download_url: str
    file_size: int
    file_name: str

@dataclass(frozen=True)
class ProjectConfig:
    """Androidプロジェクトの設定を表す不変オブジェクト

    Attributes:
        package_name: Androidパッケージ名（例: com.krkr.gamename）
        app_name: アプリの表示名
        version_code: バージョンコード（整数）
        version_name: バージョン名（例: 1.0.0）
    """

    package_name: str
    app_name: str
    version_code: int
    version_name: str

class ProjectGenerationError(Exception):
    """プロジェクト生成に関する基本例外クラス"""

    pass

class InvalidTemplateError(ProjectGenerationError):
    """テンプレートの整合性検証に失敗した場合の例外"""

    pass

class ProjectGenerator:
    """テンプレートからAndroidプロジェクトを生成するクラス

    krkrsdl2テンプレートを展開し、設定を適用して
    Androidプロジェクトを生成する機能を提供します。
    """

    def __init__(self, template_path: Path) -> None:
        """ProjectGeneratorを初期化する

        Args:
            template_path: テンプレートファイル（ZIP）のパス
        """
        self._template_path = template_path

    def generate(self, output_dir: Path, config: ProjectConfig) -> Path:
        """テンプレートからプロジェクトを生成する

        Args:
            output_dir: プロジェクトの出力先ディレクトリ
            config: プロジェクト設定

        Returns:
            生成されたプロジェクトのルートパス

        Raises:
            ProjectGenerationError: プロジェクト生成に失敗した場合
            InvalidTemplateError: テンプレートが無効な場合
        """
        raise NotImplementedError

    def validate_template(self) -> bool:
        """テンプレートの整合性を検証する

        Returns:
            テンプレートが有効な場合はTrue

        Raises:
            InvalidTemplateError: テンプレートが無効な場合
        """
        raise NotImplementedError

class TemplateCache:
    """テンプレートキャッシュを管理するクラス

    CacheManagerを使用してkrkrsdl2テンプレートのキャッシュを管理します。
    キャッシュの有効期限管理、バージョン管理、保存・取得機能を提供します。
    """

    DEFAULT_REFRESH_DAYS = 7  # デフォルトのキャッシュ有効期間（日）
    METADATA_FILENAME = "metadata.json"

    def __init__(
        self,
        cache_manager: CacheManager,
        refresh_days: int = 7,
    ) -> None:
        """TemplateCacheを初期化する

        Args:
            cache_manager: キャッシュ管理インターフェース
            refresh_days: キャッシュの有効期間（日）。デフォルトは7日。
        """
        self._cache_manager = cache_manager
        self._refresh_days = refresh_days

    def _get_metadata_path(self, version: str) -> Path:
        """メタデータファイルのパスを取得する"""
        cache_path = self._cache_manager.get_template_cache_path(version)
        return cache_path / self.METADATA_FILENAME

    def _read_metadata(self, version: str) -> dict[str, Any] | None:
        """メタデータファイルを読み込む"""
        metadata_path = self._get_metadata_path(version)
        if not metadata_path.exists():
            return None
        try:
            with open(metadata_path, encoding="utf-8") as f:
                return json.load(f)
        except (json.JSONDecodeError, OSError):
            return None

    def _write_metadata(self, version: str, metadata: dict[str, Any]) -> None:
        """メタデータファイルを書き込む"""
        metadata_path = self._get_metadata_path(version)
        metadata_path.parent.mkdir(parents=True, exist_ok=True)
        with open(metadata_path, "w", encoding="utf-8") as f:
            json.dump(metadata, f, indent=2)

    def _find_template_file(self, cache_path: Path) -> Path | None:
        """キャッシュディレクトリからテンプレートファイルを検索する"""
        if not cache_path.exists():
            return None
        for file in cache_path.iterdir():
            if file.is_file() and file.suffix == ".zip":
                return file
        return None

    def _get_all_cached_versions(self) -> list[str]:
        """キャッシュされているすべてのバージョンを取得する"""
        cache_dir = self._cache_manager.get_cache_dir() / "templates"
        if not cache_dir.exists():
            return []

        versions = []
        for version_dir in cache_dir.iterdir():
            if version_dir.is_dir():
                metadata = self._read_metadata(version_dir.name)
                if metadata:
                    versions.append(version_dir.name)
        return versions

    def get_cached_template(self, version: str | None = None) -> Path | None:
        """キャッシュ済みテンプレートを取得する

        Args:
            version: 取得するテンプレートのバージョン。
                     Noneの場合は最新のキャッシュされたバージョンを取得。

        Returns:
            キャッシュされたテンプレートのパス。
            キャッシュが存在しない、または無効な場合はNone。

        Raises:
            TemplateCacheError: キャッシュ操作中にエラーが発生した場合
        """
        try:
            target_version = version
            if target_version is None:
                target_version = self.get_cached_version()
                if target_version is None:
                    return None

            if not self.is_cache_valid(target_version):
                return None

            cache_path = self._cache_manager.get_template_cache_path(target_version)
            return self._find_template_file(cache_path)
        except OSError as e:
            raise TemplateCacheError(f"Failed to get cached template: {e}") from e

    def is_cache_valid(self, version: str | None = None) -> bool:
        """キャッシュが有効かどうかを確認する

        Args:
            version: 確認するテンプレートのバージョン。
                     Noneの場合は最新のキャッシュされたバージョンを確認。

        Returns:
            キャッシュが存在し、有効期限内であればTrue。
            それ以外はFalse。
        """
        target_version = version
        if target_version is None:
            target_version = self.get_cached_version()
            if target_version is None:
                return False

        metadata = self._read_metadata(target_version)
        if metadata is None:
            return False

        expires_at_str = metadata.get("expires_at")
        if not expires_at_str:
            return False

        try:
            expires_at = datetime.fromisoformat(expires_at_str.replace("Z", "+00:00"))
            now = datetime.now(UTC)
            return now < expires_at
        except (ValueError, TypeError):
            return False

    def get_cached_version(self) -> str | None:
        """キャッシュされているバージョンを取得する

        Returns:
            キャッシュされているテンプレートのバージョン。
            キャッシュが存在しない場合はNone。
        """
        versions = self._get_all_cached_versions()
        if not versions:
            return None

        # 最新のダウンロード日時を持つバージョンを返す
        latest_version = None
        latest_time: datetime | None = None

        for version in versions:
            metadata = self._read_metadata(version)
            if metadata:
                downloaded_at_str = metadata.get("downloaded_at")
                if downloaded_at_str:
                    try:
                        downloaded_at = datetime.fromisoformat(
                            downloaded_at_str.replace("Z", "+00:00")
                        )
                        if latest_time is None or downloaded_at > latest_time:
                            latest_time = downloaded_at
                            latest_version = version
                    except (ValueError, TypeError):
                        continue

        return latest_version

    def save_template(self, template_path: Path, version: str) -> Path:
        """テンプレートをキャッシュに保存する

        Args:
            template_path: 保存するテンプレートファイルのパス
            version: テンプレートのバージョン

        Returns:
            キャッシュに保存されたテンプレートのパス

        Raises:
            TemplateCacheError: 保存中にエラーが発生した場合
            FileNotFoundError: 指定されたテンプレートファイルが存在しない場合
        """
        if not template_path.exists():
            raise FileNotFoundError(f"Template file not found: {template_path}")

        try:
            cache_path = self._cache_manager.get_template_cache_path(version)
            cache_path.mkdir(parents=True, exist_ok=True)

            destination = cache_path / template_path.name
            shutil.copy2(template_path, destination)

            now = datetime.now(UTC)
            expires_at = now + timedelta(days=self._refresh_days)

            metadata = {
                "version": version,
                "downloaded_at": now.strftime("%Y-%m-%dT%H:%M:%SZ"),
                "expires_at": expires_at.strftime("%Y-%m-%dT%H:%M:%SZ"),
            }
            self._write_metadata(version, metadata)

            return destination
        except OSError as e:
            raise TemplateCacheError(f"Failed to save template: {e}") from e

    def clear_cache(self) -> None:
        """テンプレートキャッシュをクリアする

        Raises:
            TemplateCacheError: クリア操作中にエラーが発生した場合
        """
        try:
            self._cache_manager.clear_cache(template_only=True)
        except OSError as e:
            raise TemplateCacheError(f"Failed to clear cache: {e}") from e

# GitHub APIとダウンロードURL構築に使用する定数
GITHUB_API_BASE = "https://api.github.com/repos/krkrz/krkrsdl2"
GITHUB_RELEASES_URL = f"{GITHUB_API_BASE}/releases"
GITHUB_LATEST_RELEASE_URL = f"{GITHUB_RELEASES_URL}/latest"

# テンプレートアセットのファイル名パターン
TEMPLATE_ASSET_PATTERN = re.compile(r"krkrsdl2_android.*\.zip$", re.IGNORECASE)

# バージョン文字列の検証パターン
VERSION_PATTERN = re.compile(r"^v?\d+\.\d+(\.\d+)?$")

class TemplateDownloader:
    """GitHub Releasesからkrkrsdl2テンプレートをダウンロードするクラス

    このクラスは指定されたバージョンのテンプレートをダウンロードし、
    ローカルファイルシステムに保存する機能を提供します。
    """

    DEFAULT_TIMEOUT = 30.0  # HTTP リクエストのタイムアウト秒数
    DOWNLOAD_TIMEOUT = 300.0  # ダウンロードのタイムアウト秒数

    def __init__(
        self,
        cache_dir: Path | None = None,
        http_client: httpx.AsyncClient | None = None,
    ) -> None:
        """TemplateDownloaderを初期化する

        Args:
            cache_dir: テンプレートのキャッシュディレクトリ。
                       Noneの場合はデフォルトのキャッシュディレクトリを使用。
            http_client: HTTPクライアント（テスト用の依存性注入）。
                         Noneの場合は内部でクライアントを作成。
        """
        self._cache_dir = cache_dir
        self._http_client = http_client
        self._owns_client = http_client is None

    async def _get_client(self) -> httpx.AsyncClient:
        """HTTPクライアントを取得する"""
        if self._http_client is not None:
            return self._http_client
        return httpx.AsyncClient(timeout=self.DEFAULT_TIMEOUT)

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
            FileIntegrityError: ダウンロードしたファイルの整合性チェックに失敗した場合
        """
        if version is None:
            version = await self.get_latest_version()

        # バージョン形式の検証
        self._validate_version(version)

        # リリース情報を取得してアセット情報を取得
        template_info = await self._get_release_info(version)

        # キャッシュディレクトリの設定
        cache_dir = self._cache_dir or self._get_default_cache_dir()
        cache_dir.mkdir(parents=True, exist_ok=True)

        # ダウンロード先のパスを構築
        download_path = cache_dir / version / template_info.file_name
        download_path.parent.mkdir(parents=True, exist_ok=True)

        # ファイルをダウンロード
        await self._download_file(template_info.download_url, download_path)

        # ファイルサイズの検証
        self._verify_file_integrity(download_path, template_info.file_size)

        return download_path

    async def get_latest_version(self) -> str:
        """最新バージョンを取得する

        Returns:
            最新のテンプレートバージョン文字列

        Raises:
            NetworkError: ネットワークエラーが発生した場合
        """
        client = await self._get_client()
        try:
            response = await client.get(
                GITHUB_LATEST_RELEASE_URL,
                headers={"Accept": "application/vnd.github+json"},
            )
            response.raise_for_status()
            data = response.json()
            return str(data["tag_name"])
        except httpx.TimeoutException as e:
            raise NetworkError(f"Request timed out: {e}") from e
        except httpx.HTTPStatusError as e:
            raise NetworkError(f"HTTP error occurred: {e.response.status_code}") from e
        except httpx.RequestError as e:
            raise NetworkError(f"Network error occurred: {e}") from e
        except (KeyError, TypeError) as e:
            raise NetworkError(f"Invalid API response format: {e}") from e
        finally:
            if self._owns_client:
                await client.aclose()

    def get_download_url(self, version: str) -> str:
        """指定バージョンのダウンロードURLを構築する

        Args:
            version: テンプレートのバージョン

        Returns:
            ダウンロードURL文字列

        Raises:
            ValueError: バージョン形式が不正な場合
        """
        self._validate_version(version)

        # バージョンがvプレフィックスで始まらない場合は追加
        normalized_version = version if version.startswith("v") else f"v{version}"

        return (
            f"https://github.com/krkrz/krkrsdl2/releases/download/"
            f"{normalized_version}/krkrsdl2_android_{normalized_version}.zip"
        )

    def _validate_version(self, version: str) -> None:
        """バージョン文字列を検証する

        Args:
            version: 検証するバージョン文字列

        Raises:
            ValueError: バージョン形式が不正な場合
        """
        if not version or not VERSION_PATTERN.match(version):
            raise ValueError(f"Invalid version format: '{version}'")

    def _get_default_cache_dir(self) -> Path:
        """デフォルトのキャッシュディレクトリを取得する"""
        return Path.home() / ".cache" / "mnemonic" / "templates"

    async def _get_release_info(self, version: str) -> TemplateInfo:
        """指定バージョンのリリース情報を取得する

        Args:
            version: テンプレートのバージョン

        Returns:
            テンプレート情報

        Raises:
            TemplateNotFoundError: 指定されたバージョンが存在しない場合
            NetworkError: ネットワークエラーが発生した場合
        """
        normalized_version = version if version.startswith("v") else f"v{version}"
        release_url = f"{GITHUB_RELEASES_URL}/tags/{normalized_version}"

        client = await self._get_client()
        try:
            response = await client.get(
                release_url,
                headers={"Accept": "application/vnd.github+json"},
            )

            if response.status_code == 404:
                raise TemplateNotFoundError(f"Version not found: {version}")

            response.raise_for_status()
            data = response.json()

            # アセットからテンプレートファイルを検索
            asset = self._find_template_asset(data.get("assets", []))
            if asset is None:
                raise TemplateNotFoundError(f"Template asset not found for version: {version}")

            return TemplateInfo(
                version=str(data["tag_name"]),
                download_url=str(asset["browser_download_url"]),
                file_size=int(asset["size"]),
                file_name=str(asset["name"]),
            )
        except httpx.TimeoutException as e:
            raise NetworkError(f"Request timed out: {e}") from e
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 404:
                raise TemplateNotFoundError(f"Version not found: {version}") from e
            raise NetworkError(f"HTTP error occurred: {e.response.status_code}") from e
        except httpx.RequestError as e:
            raise NetworkError(f"Network error occurred: {e}") from e
        finally:
            if self._owns_client:
                await client.aclose()

    def _find_template_asset(self, assets: list[dict[str, Any]]) -> dict[str, Any] | None:
        """アセットリストからテンプレートファイルを検索する

        Args:
            assets: GitHub Releasesのアセットリスト

        Returns:
            テンプレートアセット情報。見つからない場合はNone。
        """
        for asset in assets:
            name = asset.get("name", "")
            if TEMPLATE_ASSET_PATTERN.search(name):
                return asset
        return None

    async def _download_file(self, url: str, destination: Path) -> None:
        """ファイルをダウンロードする

        Args:
            url: ダウンロードURL
            destination: 保存先パス

        Raises:
            NetworkError: ネットワークエラーが発生した場合
        """
        client = await self._get_client()
        try:
            async with client.stream(
                "GET",
                url,
                timeout=self.DOWNLOAD_TIMEOUT,
                follow_redirects=True,
            ) as response:
                response.raise_for_status()
                with open(destination, "wb") as f:
                    async for chunk in response.aiter_bytes(chunk_size=8192):
                        f.write(chunk)
        except httpx.TimeoutException as e:
            raise NetworkError(f"Download timed out: {e}") from e
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 404:
                raise TemplateNotFoundError(f"File not found at: {url}") from e
            raise NetworkError(f"HTTP error during download: {e.response.status_code}") from e
        except httpx.RequestError as e:
            raise NetworkError(f"Network error during download: {e}") from e
        finally:
            if self._owns_client:
                await client.aclose()

    def _verify_file_integrity(self, file_path: Path, expected_size: int) -> None:
        """ダウンロードしたファイルの整合性を検証する

        Args:
            file_path: 検証するファイルのパス
            expected_size: 期待されるファイルサイズ

        Raises:
            FileIntegrityError: ファイルサイズが期待と異なる場合
        """
        actual_size = file_path.stat().st_size
        if actual_size != expected_size:
            raise FileIntegrityError(
                f"File size mismatch: expected {expected_size} bytes, got {actual_size} bytes"
            )
