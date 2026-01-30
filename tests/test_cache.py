"""キャッシュ管理のテスト"""

from pathlib import Path

import pytest

from mnemonic.cache import (
    CacheInfo,
    clear_cache,
    get_cache_dir,
    get_cache_info,
    get_template_cache_path,
    is_cache_valid,
)

class TestCacheInfo:
    """CacheInfo型のテスト"""

    def test_cache_info_creation(self) -> None:
        """CacheInfoが正しく作成される"""
        info = CacheInfo(
            directory=Path("/tmp/cache"),
            size_bytes=1024,
            template_version="1.0.0",
            template_expires_in_days=7,
        )
        assert info.directory == Path("/tmp/cache")
        assert info.size_bytes == 1024
        assert info.template_version == "1.0.0"
        assert info.template_expires_in_days == 7

    def test_cache_info_is_frozen(self) -> None:
        """CacheInfoは変更不可"""
        info = CacheInfo(
            directory=Path("/tmp"),
            size_bytes=0,
            template_version=None,
            template_expires_in_days=None,
        )
        with pytest.raises(AttributeError):
            info.size_bytes = 999  # type: ignore

class TestGetCacheDir:
    """get_cache_dir関数のテスト"""

    def test_get_cache_dir_not_implemented(self) -> None:
        """get_cache_dirは未実装でNotImplementedErrorを投げる"""
        with pytest.raises(NotImplementedError):
            get_cache_dir()

class TestGetTemplateCachePath:
    """get_template_cache_path関数のテスト"""

    def test_get_template_cache_path_not_implemented(self) -> None:
        """get_template_cache_pathは未実装でNotImplementedErrorを投げる"""
        with pytest.raises(NotImplementedError):
            get_template_cache_path("1.0.0")

class TestIsCacheValid:
    """is_cache_valid関数のテスト"""

    def test_is_cache_valid_not_implemented(self, tmp_path: Path) -> None:
        """is_cache_validは未実装でNotImplementedErrorを投げる"""
        with pytest.raises(NotImplementedError):
            is_cache_valid(tmp_path)

class TestClearCache:
    """clear_cache関数のテスト"""

    def test_clear_cache_not_implemented(self) -> None:
        """clear_cacheは未実装でNotImplementedErrorを投げる"""
        with pytest.raises(NotImplementedError):
            clear_cache()

    def test_clear_cache_template_only_not_implemented(self) -> None:
        """clear_cache(template_only=True)は未実装"""
        with pytest.raises(NotImplementedError):
            clear_cache(template_only=True)

class TestGetCacheInfo:
    """get_cache_info関数のテスト"""

    def test_get_cache_info_not_implemented(self) -> None:
        """get_cache_infoは未実装でNotImplementedErrorを投げる"""
        with pytest.raises(NotImplementedError):
            get_cache_info()
