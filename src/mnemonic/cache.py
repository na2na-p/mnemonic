"""Cache module for Mnemonic."""

from __future__ import annotations

import os
import platform
import shutil
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path
from typing import Protocol


@dataclass(frozen=True)
class CacheInfo:
    """キャッシュ情報"""

    directory: Path
    size_bytes: int
    template_version: str | None
    template_expires_in_days: int | None


class CacheManager(Protocol):
    """キャッシュ管理インターフェース"""

    def get_cache_dir(self) -> Path:
        """OSごとのキャッシュディレクトリを取得する"""
        ...

    def get_template_cache_path(self, version: str) -> Path:
        """テンプレートキャッシュパスを取得する"""
        ...

    def is_cache_valid(self, path: Path, max_age_days: int) -> bool:
        """キャッシュの有効性をチェックする"""
        ...

    def clear_cache(self, template_only: bool = False) -> None:
        """キャッシュをクリアする"""
        ...

    def get_cache_info(self) -> CacheInfo:
        """キャッシュ情報を取得する"""
        ...


def get_cache_dir() -> Path:
    """OSごとのキャッシュディレクトリを取得する"""
    system = platform.system()

    if system == "Linux":
        xdg_cache = os.environ.get("XDG_CACHE_HOME")
        base = Path(xdg_cache) if xdg_cache else Path.home() / ".cache"
    elif system == "Darwin":
        base = Path.home() / "Library" / "Caches"
    elif system == "Windows":
        local_app_data = os.environ.get("LOCALAPPDATA")
        if local_app_data:
            base = Path(local_app_data) / "mnemonic"
        else:
            base = Path.home() / "AppData" / "Local" / "mnemonic"
        return base / "cache"
    else:
        base = Path.home() / ".cache"

    return base / "mnemonic"


def get_template_cache_path(version: str) -> Path:
    """テンプレートキャッシュパスを取得する"""
    return get_cache_dir() / "templates" / version


def is_cache_valid(path: Path, max_age_days: int = 7) -> bool:
    """キャッシュの有効性をチェックする"""
    if not path.exists():
        return False

    mtime = datetime.fromtimestamp(path.stat().st_mtime)
    age = datetime.now() - mtime
    return age < timedelta(days=max_age_days)


def clear_cache(template_only: bool = False) -> None:
    """キャッシュをクリアする"""
    cache_dir = get_cache_dir()

    if template_only:
        template_dir = cache_dir / "templates"
        if template_dir.exists():
            shutil.rmtree(template_dir)
    else:
        if cache_dir.exists():
            shutil.rmtree(cache_dir)


def get_cache_info() -> CacheInfo:
    """キャッシュ情報を取得する"""
    cache_dir = get_cache_dir()

    if not cache_dir.exists():
        return CacheInfo(
            directory=cache_dir,
            size_bytes=0,
            template_version=None,
            template_expires_in_days=None,
        )

    total_size = sum(f.stat().st_size for f in cache_dir.rglob("*") if f.is_file())

    template_dir = cache_dir / "templates"
    template_version = None
    template_expires = None

    if template_dir.exists():
        versions = list(template_dir.iterdir())
        if versions:
            latest = max(versions, key=lambda p: p.stat().st_mtime)
            template_version = latest.name
            mtime = datetime.fromtimestamp(latest.stat().st_mtime)
            expires = 7 - (datetime.now() - mtime).days
            template_expires = max(0, expires)

    return CacheInfo(
        directory=cache_dir,
        size_bytes=total_size,
        template_version=template_version,
        template_expires_in_days=template_expires,
    )
