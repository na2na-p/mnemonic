"""テンプレートダウンロード機能"""

from __future__ import annotations

import fnmatch
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

    # 必須ファイルのリスト
    REQUIRED_FILES = [
        "app/build.gradle",
        "app/src/main/AndroidManifest.xml",
        "settings.gradle",
        "build.gradle",
    ]

    # Java/Kotlinの予約語リスト
    JAVA_RESERVED_WORDS = {
        "abstract",
        "assert",
        "boolean",
        "break",
        "byte",
        "case",
        "catch",
        "char",
        "class",
        "const",
        "continue",
        "default",
        "do",
        "double",
        "else",
        "enum",
        "extends",
        "final",
        "finally",
        "float",
        "for",
        "goto",
        "if",
        "implements",
        "import",
        "instanceof",
        "int",
        "interface",
        "long",
        "native",
        "new",
        "package",
        "private",
        "protected",
        "public",
        "return",
        "short",
        "static",
        "strictfp",
        "super",
        "switch",
        "synchronized",
        "this",
        "throw",
        "throws",
        "transient",
        "try",
        "void",
        "volatile",
        "while",
        "true",
        "false",
        "null",
    }

    # パッケージ名の各セグメントの検証パターン
    PACKAGE_SEGMENT_PATTERN = re.compile(r"^[a-z][a-z0-9_]*$")

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
        # テンプレートファイルの存在確認
        if not self._template_path.exists():
            raise ProjectGenerationError(f"Template file not found: {self._template_path}")

        # 出力ディレクトリの存在確認
        if not output_dir.exists():
            raise ProjectGenerationError(f"Output directory does not exist: {output_dir}")

        # パッケージ名の検証
        self._validate_package_name(config.package_name)

        # テンプレートの検証
        if not self.validate_template():
            raise InvalidTemplateError("Template validation failed")

        # テンプレートの展開
        self._extract_template(output_dir)

        # AndroidManifest.xml の更新
        self._update_android_manifest(output_dir, config)

        # build.gradle または build.gradle.kts の更新
        self._update_build_gradle(output_dir, config)

        return output_dir

    def validate_template(self) -> bool:
        """テンプレートの整合性を検証する

        Returns:
            テンプレートが有効な場合はTrue

        Raises:
            InvalidTemplateError: テンプレートが無効な場合
        """
        if not self._template_path.exists():
            raise InvalidTemplateError(f"Template file not found: {self._template_path}")

        try:
            import zipfile

            if not zipfile.is_zipfile(self._template_path):
                raise InvalidTemplateError(
                    f"Template is not a valid ZIP file: {self._template_path}"
                )

            with zipfile.ZipFile(self._template_path, "r") as zf:
                file_list = zf.namelist()
                missing_files = []

                for required_file in self.REQUIRED_FILES:
                    if required_file not in file_list:
                        missing_files.append(required_file)

                if missing_files:
                    raise InvalidTemplateError(
                        f"Missing required files in template: {', '.join(missing_files)}"
                    )

            return True
        except zipfile.BadZipFile as e:
            raise InvalidTemplateError(f"Invalid ZIP file: {e}") from e

    def _validate_package_name(self, package_name: str) -> None:
        """パッケージ名を検証する

        Args:
            package_name: 検証するパッケージ名

        Raises:
            ProjectGenerationError: パッケージ名が無効な場合
        """
        if not package_name:
            raise ProjectGenerationError("Package name cannot be empty")

        # ドットで分割してセグメントを検証
        segments = package_name.split(".")

        # 最低2つのセグメントが必要
        if len(segments) < 2:
            raise ProjectGenerationError(
                f"Package name must have at least two segments: {package_name}"
            )

        for segment in segments:
            # 空のセグメント（連続するドット）をチェック
            if not segment:
                raise ProjectGenerationError(f"Package name contains empty segment: {package_name}")

            # セグメントパターンの検証
            if not self.PACKAGE_SEGMENT_PATTERN.match(segment):
                raise ProjectGenerationError(
                    f"Invalid package name segment '{segment}' in: {package_name}"
                )

            # 予約語のチェック
            if segment in self.JAVA_RESERVED_WORDS:
                raise ProjectGenerationError(
                    f"Package name contains reserved word '{segment}': {package_name}"
                )

    def _extract_template(self, output_dir: Path) -> None:
        """テンプレートを展開する

        Args:
            output_dir: 展開先ディレクトリ

        Raises:
            ProjectGenerationError: 展開に失敗した場合
        """
        import zipfile

        try:
            with zipfile.ZipFile(self._template_path, "r") as zf:
                zf.extractall(output_dir)
        except zipfile.BadZipFile as e:
            raise ProjectGenerationError(f"Failed to extract template: {e}") from e
        except OSError as e:
            raise ProjectGenerationError(f"Failed to extract template: {e}") from e

    def _update_android_manifest(self, output_dir: Path, config: ProjectConfig) -> None:
        """AndroidManifest.xmlを更新する

        Args:
            output_dir: プロジェクトディレクトリ
            config: プロジェクト設定

        Raises:
            ProjectGenerationError: 更新に失敗した場合
        """
        manifest_path = output_dir / "app" / "src" / "main" / "AndroidManifest.xml"

        if not manifest_path.exists():
            raise ProjectGenerationError(f"AndroidManifest.xml not found: {manifest_path}")

        try:
            content = manifest_path.read_text(encoding="utf-8")

            # package属性の更新
            content = re.sub(
                r'package="[^"]*"',
                f'package="{config.package_name}"',
                content,
            )

            # android:label属性の更新
            content = re.sub(
                r'android:label="[^"]*"',
                f'android:label="{config.app_name}"',
                content,
            )

            manifest_path.write_text(content, encoding="utf-8")
        except OSError as e:
            raise ProjectGenerationError(f"Failed to update AndroidManifest.xml: {e}") from e

    def _update_build_gradle(self, output_dir: Path, config: ProjectConfig) -> None:
        """build.gradleまたはbuild.gradle.ktsを更新する

        Args:
            output_dir: プロジェクトディレクトリ
            config: プロジェクト設定

        Raises:
            ProjectGenerationError: 更新に失敗した場合
        """
        # Groovy DSL (build.gradle) を優先的に探す
        gradle_path = output_dir / "app" / "build.gradle"

        if not gradle_path.exists():
            # Kotlin DSL (build.gradle.kts) を試す
            gradle_path = output_dir / "app" / "build.gradle.kts"

        if not gradle_path.exists():
            raise ProjectGenerationError(
                f"build.gradle or build.gradle.kts not found in: {output_dir / 'app'}"
            )

        try:
            content = gradle_path.read_text(encoding="utf-8")

            # namespace の更新
            content = re.sub(
                r'namespace\s+["\']([^"\']*)["\']',
                f'namespace "{config.package_name}"',
                content,
            )

            # applicationId の更新
            content = re.sub(
                r'applicationId\s+["\']([^"\']*)["\']',
                f'applicationId "{config.package_name}"',
                content,
            )

            # versionCode の更新
            content = re.sub(
                r"versionCode\s+\d+",
                f"versionCode {config.version_code}",
                content,
            )

            # versionName の更新
            content = re.sub(
                r'versionName\s+["\']([^"\']*)["\']',
                f'versionName "{config.version_name}"',
                content,
            )

            gradle_path.write_text(content, encoding="utf-8")
        except OSError as e:
            raise ProjectGenerationError(f"Failed to update build.gradle: {e}") from e


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
GITHUB_API_BASE = "https://api.github.com/repos/krkrsdl2/krkrsdl2"
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

        /releases/latestを試み、404の場合は/releasesから最初のリリースを取得する。
        これにより、pre-releaseのみのリポジトリにも対応できる。

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

            if response.status_code == 404:
                # /releases/latestが404の場合（pre-releaseのみの場合）
                # /releasesから最初のリリースを取得
                response = await client.get(
                    GITHUB_RELEASES_URL,
                    headers={"Accept": "application/vnd.github+json"},
                )
                response.raise_for_status()
                releases = response.json()
                if not releases:
                    raise NetworkError("リリースが見つかりません")
                return str(releases[0]["tag_name"])

            response.raise_for_status()
            data = response.json()
            return str(data["tag_name"])
        except httpx.TimeoutException as e:
            raise NetworkError(f"Request timed out: {e}") from e
        except httpx.HTTPStatusError as e:
            raise NetworkError(f"HTTP error occurred: {e.response.status_code}") from e
        except httpx.RequestError as e:
            raise NetworkError(f"Network error occurred: {e}") from e
        except (KeyError, TypeError, IndexError) as e:
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


class AssetPlacementError(Exception):
    """アセット配置に関する基本例外クラス"""

    pass


@dataclass(frozen=True)
class AssetConfig:
    """アセット配置の設定を表す不変オブジェクト

    Attributes:
        no_compress_extensions: 圧縮しない拡張子のリスト（例: [".ogg", ".mp3"]）
        exclude_patterns: 除外パターンのリスト（例: ["*.bak", "thumbs.db"]）
    """

    no_compress_extensions: list[str]
    exclude_patterns: list[str]


@dataclass(frozen=True)
class AssetPlacementResult:
    """アセット配置結果を表す不変オブジェクト

    Attributes:
        total_files: 配置されたファイルの総数
        total_size: 配置されたファイルの合計サイズ（バイト）
        placed_files: 配置されたファイルのパスリスト
    """

    total_files: int
    total_size: int
    placed_files: list[Path]


class AssetPlacer:
    """変換済みアセットをAndroidプロジェクトに配置するクラス

    このクラスは変換済みのゲームアセットをAndroidプロジェクトの
    適切な場所に配置し、build.gradleの設定を更新する機能を提供します。
    """

    # アセットディレクトリの相対パス
    ASSETS_DIR_PATH = Path("app") / "src" / "main" / "assets"

    def __init__(
        self,
        project_path: Path,
        exclude_patterns: list[str] | None = None,
    ) -> None:
        """AssetPlacerを初期化する

        Args:
            project_path: Androidプロジェクトのルートパス
            exclude_patterns: 除外パターンのリスト（オプション）
        """
        self._project_path = project_path
        self._exclude_patterns = exclude_patterns or []

    def _get_assets_dir(self) -> Path:
        """アセットディレクトリのパスを取得する"""
        return self._project_path / self.ASSETS_DIR_PATH

    def _should_exclude(self, file_path: Path) -> bool:
        """ファイルが除外パターンに一致するかを判定する

        Args:
            file_path: 判定するファイルのパス

        Returns:
            除外すべき場合はTrue
        """
        file_name = file_path.name
        return any(fnmatch.fnmatch(file_name, pattern) for pattern in self._exclude_patterns)

    def _get_build_gradle_path(self) -> Path | None:
        """build.gradleまたはbuild.gradle.ktsのパスを取得する

        Returns:
            build.gradleファイルのパス。存在しない場合はNone。
        """
        # Groovy DSL (build.gradle) を優先
        gradle_path = self._project_path / "app" / "build.gradle"
        if gradle_path.exists():
            return gradle_path

        # Kotlin DSL (build.gradle.kts)
        gradle_kts_path = self._project_path / "app" / "build.gradle.kts"
        if gradle_kts_path.exists():
            return gradle_kts_path

        return None

    def _is_kotlin_dsl(self, gradle_path: Path) -> bool:
        """Kotlin DSLかどうかを判定する"""
        return gradle_path.suffix == ".kts"

    def place_assets(
        self,
        source_dir: Path,
        exclude_patterns: list[str] | None = None,
    ) -> AssetPlacementResult:
        """アセットをAndroidプロジェクトに配置する

        Args:
            source_dir: 配置するアセットが格納されたディレクトリ
            exclude_patterns: 除外パターンのリスト（オプション、インスタンスの設定を上書き）

        Returns:
            配置結果を表すAssetPlacementResult

        Raises:
            AssetPlacementError: アセット配置に失敗した場合
        """
        # ソースディレクトリの存在確認
        if not source_dir.exists():
            raise AssetPlacementError(f"Source directory does not exist: {source_dir}")

        # プロジェクトパスの存在確認
        if not self._project_path.exists():
            raise AssetPlacementError(f"Project path does not exist: {self._project_path}")

        # アセットディレクトリの存在確認
        assets_dir = self._get_assets_dir()
        if not assets_dir.exists():
            raise AssetPlacementError(f"Assets directory does not exist: {assets_dir}")

        # 除外パターンの決定
        patterns = exclude_patterns if exclude_patterns is not None else self._exclude_patterns

        placed_files: list[Path] = []
        total_size = 0

        try:
            # ソースディレクトリ内のすべてのファイルを走査
            for source_file in source_dir.rglob("*"):
                if source_file.is_dir():
                    continue

                # 除外パターンのチェック
                if self._should_exclude_with_patterns(source_file, patterns):
                    continue

                # 相対パスを維持して配置先を決定
                relative_path = source_file.relative_to(source_dir)
                dest_file = assets_dir / relative_path

                # 配置先ディレクトリの作成
                dest_file.parent.mkdir(parents=True, exist_ok=True)

                # ファイルのコピー
                shutil.copy2(source_file, dest_file)

                placed_files.append(relative_path)
                total_size += source_file.stat().st_size

        except OSError as e:
            raise AssetPlacementError(f"Failed to place assets: {e}") from e

        return AssetPlacementResult(
            total_files=len(placed_files),
            total_size=total_size,
            placed_files=placed_files,
        )

    def _should_exclude_with_patterns(
        self,
        file_path: Path,
        patterns: list[str],
    ) -> bool:
        """指定されたパターンでファイルが除外対象かを判定する"""
        file_name = file_path.name
        return any(fnmatch.fnmatch(file_name, pattern) for pattern in patterns)

    def configure_build_gradle(self, asset_config: AssetConfig) -> None:
        """build.gradleにアセット設定を追加する

        noCompress拡張子やexclude設定をbuild.gradleに反映します。

        Args:
            asset_config: アセット設定

        Raises:
            AssetPlacementError: build.gradleの設定に失敗した場合
        """
        gradle_path = self._get_build_gradle_path()
        if gradle_path is None:
            raise AssetPlacementError(
                f"build.gradle or build.gradle.kts not found in: {self._project_path / 'app'}"
            )

        try:
            content = gradle_path.read_text(encoding="utf-8")
            is_kotlin = self._is_kotlin_dsl(gradle_path)

            # noCompress設定の追加
            if asset_config.no_compress_extensions:
                content = self._add_no_compress_config(
                    content,
                    asset_config.no_compress_extensions,
                    is_kotlin,
                )

            gradle_path.write_text(content, encoding="utf-8")
        except OSError as e:
            raise AssetPlacementError(f"Failed to configure build.gradle: {e}") from e

    def _add_no_compress_config(
        self,
        content: str,
        extensions: list[str],
        is_kotlin: bool,
    ) -> str:
        """noCompress設定をbuild.gradleに追加する

        Args:
            content: build.gradleの内容
            extensions: 圧縮しない拡張子のリスト
            is_kotlin: Kotlin DSLかどうか

        Returns:
            更新されたbuild.gradleの内容
        """
        # 既にaaptOptionsが存在するかチェック
        if "aaptOptions" in content:
            # 既存のaaptOptions内にnoCompressを追加/更新
            return self._update_existing_aapt_options(content, extensions, is_kotlin)

        # aaptOptionsが存在しない場合、androidブロック内に追加
        return self._add_new_aapt_options(content, extensions, is_kotlin)

    def _update_existing_aapt_options(
        self,
        content: str,
        extensions: list[str],
        is_kotlin: bool,
    ) -> str:
        """既存のaaptOptionsにnoCompress設定を追加/更新する"""
        if is_kotlin:
            # Kotlin DSL形式
            extensions_str = ", ".join(f'"{ext}"' for ext in extensions)
            no_compress_line = f"        noCompress += listOf({extensions_str})"
        else:
            # Groovy形式
            extensions_str = ", ".join(f"'{ext}'" for ext in extensions)
            no_compress_line = f"        noCompress {extensions_str}"

        # aaptOptionsブロック内にnoCompressがあれば置換、なければ追加
        if "noCompress" in content:
            # 既存のnoCompress行を置換
            content = re.sub(
                r"(\s*)noCompress.*",
                f"\\1{no_compress_line.strip()}",
                content,
            )
        else:
            # aaptOptionsブロック内にnoCompressを追加
            content = re.sub(
                r"(aaptOptions\s*\{)",
                f"\\1\n{no_compress_line}",
                content,
            )

        return content

    def _add_new_aapt_options(
        self,
        content: str,
        extensions: list[str],
        is_kotlin: bool,
    ) -> str:
        """新しいaaptOptionsブロックをandroidブロック内に追加する"""
        if is_kotlin:
            # Kotlin DSL形式
            extensions_str = ", ".join(f'"{ext}"' for ext in extensions)
            aapt_block = f"""    aaptOptions {{
        noCompress += listOf({extensions_str})
    }}"""
        else:
            # Groovy形式
            extensions_str = ", ".join(f"'{ext}'" for ext in extensions)
            aapt_block = f"""    aaptOptions {{
        noCompress {extensions_str}
    }}"""

        # androidブロックの終了直前に挿入
        # androidブロックの最後の } を見つけて、その前に挿入
        pattern = r"(android\s*\{)(.*?)(^\})"
        match = re.search(pattern, content, re.MULTILINE | re.DOTALL)

        if match:
            android_start = match.group(1)
            android_body = match.group(2)
            android_end = match.group(3)
            content = f"{android_start}{android_body}{aapt_block}\n{android_end}"

        return content

    def validate_placement(self) -> bool:
        """アセット配置が正しく行われたかを検証する

        Returns:
            配置が正しい場合はTrue、問題がある場合はFalse

        Raises:
            AssetPlacementError: 検証中にエラーが発生した場合
        """
        try:
            assets_dir = self._get_assets_dir()

            # アセットディレクトリの存在確認
            if not assets_dir.exists():
                return False

            # アセットディレクトリ内にファイルが存在するか確認
            has_files = False
            for item in assets_dir.rglob("*"):
                if item.is_file():
                    has_files = True
                    break

            return has_files
        except OSError as e:
            raise AssetPlacementError(f"Failed to validate placement: {e}") from e
