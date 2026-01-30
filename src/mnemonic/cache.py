"""Cache module for Mnemonic."""

from __future__ import annotations

from dataclasses import dataclass
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
    """OSごとのキャッシュディレクトリを取得する（未実装）"""
    raise NotImplementedError("F-04-03で実装予定")

def get_template_cache_path(version: str) -> Path:
    """テンプレートキャッシュパスを取得する（未実装）"""
    raise NotImplementedError("F-04-03で実装予定")

def is_cache_valid(path: Path, max_age_days: int = 7) -> bool:
    """キャッシュの有効性をチェックする（未実装）"""
    raise NotImplementedError("F-04-03で実装予定")

def clear_cache(template_only: bool = False) -> None:
    """キャッシュをクリアする（未実装）"""
    raise NotImplementedError("F-04-03で実装予定")

def get_cache_info() -> CacheInfo:
    """キャッシュ情報を取得する（未実装）"""
    raise NotImplementedError("F-04-03で実装予定")
