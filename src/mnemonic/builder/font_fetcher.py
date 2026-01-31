"""Koruriフォント取得機能

KoruriフォントをGitHubからダウンロードし、キャッシュに保存する機能を提供する。
Koruriフォントは Apache License 2.0 でライセンスされている。
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING

import httpx

from mnemonic.cache import get_cache_dir

if TYPE_CHECKING:
    pass


class FontDownloadError(Exception):
    """フォントダウンロードに関する例外"""

    pass


@dataclass(frozen=True)
class FontInfo:
    """フォント情報を表す不変オブジェクト

    Attributes:
        name: フォント名（例: Koruri-Regular）
        path: フォントファイルのパス
        version: フォントのバージョン
    """

    name: str
    path: Path
    version: str


# Koruriフォントのダウンロード設定
# GitHubリリースにはZIPファイルが存在しないため、リポジトリから直接TTFをダウンロード
KORURI_VERSION = "master"
KORURI_FONT_NAME = "Koruri-Regular"
KORURI_TTF_FILENAME = f"{KORURI_FONT_NAME}.ttf"
KORURI_DOWNLOAD_URL = (
    f"https://raw.githubusercontent.com/Koruri/Koruri/master/{KORURI_TTF_FILENAME}"
)


def get_font_cache_dir() -> Path:
    """フォントキャッシュディレクトリを取得する

    Returns:
        フォントキャッシュディレクトリのパス
    """
    return get_cache_dir() / "fonts"


class FontFetcher:
    """Koruriフォントを取得するクラス

    GitHubからKoruriフォントをダウンロードし、
    ローカルキャッシュに保存する機能を提供する。

    Example:
        >>> fetcher = FontFetcher()
        >>> font_info = await fetcher.get_font()
        >>> print(font_info.path)
        /home/user/.cache/mnemonic/fonts/Koruri-Regular.ttf
    """

    DEFAULT_TIMEOUT = 60.0  # HTTPリクエストのタイムアウト秒数

    def __init__(
        self,
        cache_dir: Path | None = None,
        http_client: httpx.AsyncClient | None = None,
    ) -> None:
        """FontFetcherを初期化する

        Args:
            cache_dir: フォントのキャッシュディレクトリ。
                       Noneの場合はデフォルトのキャッシュディレクトリを使用。
            http_client: HTTPクライアント（テスト用の依存性注入）。
                         Noneの場合は内部でクライアントを作成。
        """
        self._cache_dir = cache_dir or get_font_cache_dir()
        self._http_client = http_client
        self._owns_client = http_client is None

    async def get_font(self) -> FontInfo:
        """フォントを取得する

        キャッシュに有効なフォントが存在する場合はそれを返し、
        存在しない場合はダウンロードする。

        Returns:
            フォント情報

        Raises:
            FontDownloadError: ダウンロードに失敗した場合
        """
        if self.is_cache_valid():
            cached_path = self.get_cached_font_path()
            if cached_path is not None:
                return FontInfo(
                    name=KORURI_FONT_NAME,
                    path=cached_path,
                    version=KORURI_VERSION,
                )

        return await self.download_font()

    async def download_font(self) -> FontInfo:
        """フォントをダウンロードする

        GitHubからKoruri-Regular.ttfを直接ダウンロードしてキャッシュに保存する。

        Returns:
            フォント情報

        Raises:
            FontDownloadError: ダウンロードに失敗した場合
        """
        self._cache_dir.mkdir(parents=True, exist_ok=True)

        # TTFを直接ダウンロード
        ttf_content = await self._download_ttf()

        # TTFファイルを保存
        font_path = self._cache_dir / KORURI_TTF_FILENAME
        font_path.write_bytes(ttf_content)

        return FontInfo(
            name=KORURI_FONT_NAME,
            path=font_path,
            version=KORURI_VERSION,
        )

    async def _download_ttf(self) -> bytes:
        """Koruri-Regular.ttfをダウンロードする

        Returns:
            TTFファイルのバイナリデータ

        Raises:
            FontDownloadError: ダウンロードに失敗した場合
        """
        client = self._http_client
        close_client = False

        if client is None:
            client = httpx.AsyncClient(timeout=self.DEFAULT_TIMEOUT)
            close_client = True

        try:
            response = await client.get(KORURI_DOWNLOAD_URL, follow_redirects=True)
            response.raise_for_status()
            return response.content
        except httpx.RequestError as e:
            raise FontDownloadError(f"ネットワークエラー: {e}") from e
        except httpx.HTTPStatusError as e:
            raise FontDownloadError(f"HTTPエラー {e.response.status_code}: {e}") from e
        finally:
            if close_client and client is not None:
                await client.aclose()

    def is_cache_valid(self) -> bool:
        """キャッシュが有効かどうかを確認する

        Returns:
            キャッシュが存在し有効な場合はTrue
        """
        font_path = self._cache_dir / KORURI_TTF_FILENAME
        return font_path.exists()

    def get_cached_font_path(self) -> Path | None:
        """キャッシュされたフォントのパスを取得する

        Returns:
            フォントファイルのパス。キャッシュが無い場合はNone。
        """
        font_path = self._cache_dir / KORURI_TTF_FILENAME
        if font_path.exists():
            return font_path
        return None
