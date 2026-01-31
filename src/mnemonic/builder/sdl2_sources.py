"""SDL2 Java ソース取得機能

このモジュールは SDL2 の Java ソースコードをダウンロードし、
キャッシュを管理する機能を提供します。
"""

from __future__ import annotations

import shutil
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path
from typing import Final, Protocol

import httpx


class SDL2SourceFetcherError(Exception):
    """SDL2 ソース取得に関する基本例外クラス"""

    pass


class SDL2SourceFetchNetworkError(SDL2SourceFetcherError):
    """ネットワークエラーが発生した場合の例外"""

    pass


class SDL2SourceFetchTimeoutError(SDL2SourceFetcherError):
    """タイムアウトが発生した場合の例外"""

    pass


class SDL2SourceCacheError(SDL2SourceFetcherError):
    """キャッシュ操作に関する例外"""

    pass


class SDL2SourceFetcherProtocol(Protocol):
    """SDL2 Java ソース取得のインターフェース"""

    async def fetch(self, dest_dir: Path) -> None:
        """SDL2 Java ソースをダウンロードして配置する

        Args:
            dest_dir: 配置先ディレクトリ（org/libsdl/app が作成される）

        Raises:
            SDL2SourceFetchNetworkError: ネットワークエラーが発生した場合
            SDL2SourceFetchTimeoutError: タイムアウトが発生した場合
        """
        ...


@dataclass(frozen=True)
class SDL2SourceCacheInfo:
    """キャッシュ情報を表すデータクラス"""

    cached_at: datetime | None
    is_valid: bool
    cache_path: Path


class SDL2SourceCache:
    """SDL2 Java ソースのキャッシュ管理クラス"""

    CACHE_VALIDITY_DAYS: Final[int] = 30
    CACHE_MARKER_FILE: Final[str] = ".cached_at"
    CACHE_VERSION_FILE: Final[str] = ".version"

    # 現在のキャッシュバージョン（SDL コミット SHA の短縮形）
    # SDL2SourceFetcher.SDL_COMMIT と同期している必要がある
    CURRENT_VERSION: Final[str] = "53dea983"

    def __init__(self, cache_dir: Path) -> None:
        """SDL2SourceCache を初期化する

        Args:
            cache_dir: キャッシュのベースディレクトリ
        """
        self._cache_dir = cache_dir / "sdl2_sources"

    @property
    def cache_path(self) -> Path:
        """キャッシュディレクトリのパスを返す"""
        return self._cache_dir

    def is_valid(self) -> bool:
        """キャッシュが有効か確認する

        Returns:
            キャッシュが存在し、有効期限内かつバージョンが一致すれば True
        """
        marker = self._cache_dir / self.CACHE_MARKER_FILE
        version_file = self._cache_dir / self.CACHE_VERSION_FILE

        if not marker.exists():
            return False

        # バージョンチェック
        if not version_file.exists():
            return False

        try:
            cached_version = version_file.read_text(encoding="utf-8").strip()
            if cached_version != self.CURRENT_VERSION:
                return False
        except OSError:
            return False

        try:
            cached_at_str = marker.read_text(encoding="utf-8").strip()
            cached_at = datetime.fromisoformat(cached_at_str)
            age = datetime.now() - cached_at
            return age < timedelta(days=self.CACHE_VALIDITY_DAYS)
        except (ValueError, OSError):
            return False

    def get_cached_at(self) -> datetime | None:
        """キャッシュ作成日時を取得する

        Returns:
            キャッシュ作成日時、存在しない場合は None
        """
        marker = self._cache_dir / self.CACHE_MARKER_FILE
        if not marker.exists():
            return None

        try:
            cached_at_str = marker.read_text(encoding="utf-8").strip()
            return datetime.fromisoformat(cached_at_str)
        except (ValueError, OSError):
            return None

    def get_cache_info(self) -> SDL2SourceCacheInfo:
        """キャッシュ情報を取得する

        Returns:
            キャッシュ情報
        """
        return SDL2SourceCacheInfo(
            cached_at=self.get_cached_at(),
            is_valid=self.is_valid(),
            cache_path=self._cache_dir,
        )

    def get_source_files_path(self) -> Path:
        """キャッシュされたソースファイルのパスを返す

        Returns:
            org/libsdl/app ディレクトリのパス
        """
        return self._cache_dir / "org" / "libsdl" / "app"

    def save(self, sources_dir: Path) -> None:
        """ソースをキャッシュに保存する

        Args:
            sources_dir: コピー元のディレクトリ（org ディレクトリを含む）

        Raises:
            SDL2SourceCacheError: キャッシュ保存に失敗した場合
        """
        try:
            # 既存のキャッシュを削除
            if self._cache_dir.exists():
                shutil.rmtree(self._cache_dir)

            self._cache_dir.mkdir(parents=True, exist_ok=True)

            # org ディレクトリをコピー
            src_org_dir = sources_dir / "org"
            if src_org_dir.exists():
                shutil.copytree(src_org_dir, self._cache_dir / "org")

            # キャッシュマーカーを作成
            marker = self._cache_dir / self.CACHE_MARKER_FILE
            marker.write_text(datetime.now().isoformat(), encoding="utf-8")

            # バージョンファイルを作成
            version_file = self._cache_dir / self.CACHE_VERSION_FILE
            version_file.write_text(self.CURRENT_VERSION, encoding="utf-8")
        except OSError as e:
            raise SDL2SourceCacheError(f"キャッシュ保存に失敗しました: {e}") from e

    def restore_to(self, dest_dir: Path) -> None:
        """キャッシュからソースを復元する

        Args:
            dest_dir: 復元先のディレクトリ

        Raises:
            SDL2SourceCacheError: キャッシュ復元に失敗した場合
        """
        if not self.is_valid():
            raise SDL2SourceCacheError("有効なキャッシュがありません")

        try:
            src_org_dir = self._cache_dir / "org"
            dest_org_dir = dest_dir / "org"

            if dest_org_dir.exists():
                shutil.rmtree(dest_org_dir)

            shutil.copytree(src_org_dir, dest_org_dir)
        except OSError as e:
            raise SDL2SourceCacheError(f"キャッシュ復元に失敗しました: {e}") from e

    def clear(self) -> None:
        """キャッシュを削除する"""
        if self._cache_dir.exists():
            shutil.rmtree(self._cache_dir)


class SDL2SourceFetcher:
    """SDL2 Java ソースを取得するクラス"""

    # GitHub raw URL ベース
    # krkrsdl2 が使用している SDL コミット (53dea9830964eee8b5c2a7ee0a65d6e268dc78a1) を使用
    # このコミットの Java ソースは krkrsdl2 の native ライブラリ (libSDL2.so) と互換性がある
    SDL_COMMIT: Final[str] = "53dea9830964eee8b5c2a7ee0a65d6e268dc78a1"
    BASE_URL: Final[str] = (
        f"https://raw.githubusercontent.com/libsdl-org/SDL/{SDL_COMMIT}/"
        "android-project/app/src/main/java/org/libsdl/app"
    )

    # 必要な Java ファイル (krkrsdl2 互換コミット)
    # SDLSurface と SDLInputConnection は SDLActivity.java の内部クラスとして定義されている
    REQUIRED_FILES: Final[list[str]] = [
        "SDLActivity.java",
        "SDL.java",
        "SDLAudioManager.java",
        "SDLControllerManager.java",
        "HIDDevice.java",
        "HIDDeviceManager.java",
        "HIDDeviceUSB.java",
        "HIDDeviceBLESteamController.java",
    ]

    # デフォルトタイムアウト（秒）
    DEFAULT_TIMEOUT: Final[float] = 30.0

    def __init__(
        self,
        timeout: float | None = None,
        cache: SDL2SourceCache | None = None,
    ) -> None:
        """SDL2SourceFetcher を初期化する

        Args:
            timeout: HTTP リクエストのタイムアウト（秒）
            cache: キャッシュマネージャー（オプション）
        """
        self._timeout = timeout if timeout is not None else self.DEFAULT_TIMEOUT
        self._cache = cache

    async def fetch(self, dest_dir: Path) -> None:
        """SDL2 Java ソースをダウンロードして配置する

        キャッシュが有効な場合はキャッシュから復元し、
        そうでない場合は GitHub からダウンロードします。

        Args:
            dest_dir: 配置先ディレクトリ（org/libsdl/app が作成される）

        Raises:
            SDL2SourceFetchNetworkError: ネットワークエラーが発生した場合
            SDL2SourceFetchTimeoutError: タイムアウトが発生した場合
        """
        # キャッシュが有効な場合は復元
        if self._cache is not None and self._cache.is_valid():
            self._cache.restore_to(dest_dir)
            return

        # ダウンロード先ディレクトリを作成
        sdl_app_dir = dest_dir / "org" / "libsdl" / "app"
        sdl_app_dir.mkdir(parents=True, exist_ok=True)

        try:
            async with httpx.AsyncClient(timeout=self._timeout) as client:
                for filename in self.REQUIRED_FILES:
                    url = f"{self.BASE_URL}/{filename}"
                    response = await client.get(url)
                    response.raise_for_status()
                    (sdl_app_dir / filename).write_text(response.text, encoding="utf-8")
        except httpx.TimeoutException as e:
            raise SDL2SourceFetchTimeoutError(
                f"SDL2 ソースのダウンロードがタイムアウトしました: {e}"
            ) from e
        except httpx.HTTPStatusError as e:
            raise SDL2SourceFetchNetworkError(
                f"SDL2 ソースのダウンロードに失敗しました: HTTP {e.response.status_code}"
            ) from e
        except httpx.RequestError as e:
            raise SDL2SourceFetchNetworkError(
                f"SDL2 ソースのダウンロードに失敗しました: {e}"
            ) from e

        # キャッシュに保存
        if self._cache is not None:
            self._cache.save(dest_dir)
