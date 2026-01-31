"""キャッシュ管理のテスト"""

import os
import time
from pathlib import Path
from unittest.mock import patch

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

    def test_get_cache_dir_returns_path(self) -> None:
        """get_cache_dirがPathを返す"""
        result = get_cache_dir()
        assert isinstance(result, Path)
        assert "mnemonic" in str(result)

    @pytest.mark.parametrize(
        "system, xdg_cache, expected_suffix",
        [
            pytest.param(
                "Linux",
                None,
                ".cache/mnemonic",
                id="正常系: Linuxでデフォルトキャッシュディレクトリ",
            ),
            pytest.param(
                "Linux",
                "/custom/cache",
                "mnemonic",
                id="正常系: LinuxでXDG_CACHE_HOME設定時",
            ),
            pytest.param(
                "Darwin",
                None,
                "Library/Caches/mnemonic",
                id="正常系: macOSのキャッシュディレクトリ",
            ),
        ],
    )
    def test_get_cache_dir_platform_specific(
        self, system: str, xdg_cache: str | None, expected_suffix: str
    ) -> None:
        """OSごとのキャッシュディレクトリが正しく決定される"""
        env_vars = {}
        if xdg_cache:
            env_vars["XDG_CACHE_HOME"] = xdg_cache
        else:
            env_vars.pop("XDG_CACHE_HOME", None)

        with (
            patch("mnemonic.cache.platform.system", return_value=system),
            patch.dict(os.environ, env_vars, clear=False),
        ):
            if xdg_cache is None:
                os.environ.pop("XDG_CACHE_HOME", None)
            result = get_cache_dir()
            assert expected_suffix in str(result)

    def test_get_cache_dir_windows(self) -> None:
        """Windowsのキャッシュディレクトリが正しく決定される"""
        with (
            patch("mnemonic.cache.platform.system", return_value="Windows"),
            patch.dict(os.environ, {"LOCALAPPDATA": "C:\\Users\\Test\\AppData\\Local"}),
        ):
            result = get_cache_dir()
            assert "mnemonic" in str(result)
            assert "cache" in str(result)


class TestGetTemplateCachePath:
    """get_template_cache_path関数のテスト"""

    def test_get_template_cache_path_returns_versioned_path(self) -> None:
        """テンプレートキャッシュパスにバージョンが含まれる"""
        result = get_template_cache_path("1.0.0")
        assert isinstance(result, Path)
        assert "templates" in str(result)
        assert "1.0.0" in str(result)

    def test_get_template_cache_path_different_versions(self) -> None:
        """異なるバージョンで異なるパスを返す"""
        v1 = get_template_cache_path("1.0.0")
        v2 = get_template_cache_path("2.0.0")
        assert v1 != v2
        assert "1.0.0" in str(v1)
        assert "2.0.0" in str(v2)


class TestIsCacheValid:
    """is_cache_valid関数のテスト"""

    def test_is_cache_valid_nonexistent_file(self, tmp_path: Path) -> None:
        """存在しないファイルは無効"""
        result = is_cache_valid(tmp_path / "nonexistent")
        assert result is False

    def test_is_cache_valid_fresh_file(self, tmp_path: Path) -> None:
        """新しいファイルは有効"""
        test_file = tmp_path / "test.txt"
        test_file.write_text("test")
        result = is_cache_valid(test_file)
        assert result is True

    def test_is_cache_valid_custom_max_age(self, tmp_path: Path) -> None:
        """カスタムmax_ageを指定できる"""
        test_file = tmp_path / "test.txt"
        test_file.write_text("test")
        result = is_cache_valid(test_file, max_age_days=1)
        assert result is True

    def test_is_cache_valid_expired_file(self, tmp_path: Path) -> None:
        """古いファイルは無効"""
        test_file = tmp_path / "test.txt"
        test_file.write_text("test")
        old_time = time.time() - (10 * 24 * 60 * 60)
        os.utime(test_file, (old_time, old_time))
        result = is_cache_valid(test_file, max_age_days=7)
        assert result is False


class TestClearCache:
    """clear_cache関数のテスト"""

    def test_clear_cache_all(self, tmp_path: Path) -> None:
        """全キャッシュをクリアする"""
        with patch("mnemonic.cache.get_cache_dir", return_value=tmp_path):
            cache_file = tmp_path / "test.txt"
            cache_file.write_text("test")
            template_dir = tmp_path / "templates" / "1.0.0"
            template_dir.mkdir(parents=True)
            (template_dir / "template.txt").write_text("template")

            clear_cache()

            assert not tmp_path.exists()

    def test_clear_cache_template_only(self, tmp_path: Path) -> None:
        """テンプレートのみクリアする"""
        with patch("mnemonic.cache.get_cache_dir", return_value=tmp_path):
            cache_file = tmp_path / "test.txt"
            cache_file.write_text("test")
            template_dir = tmp_path / "templates" / "1.0.0"
            template_dir.mkdir(parents=True)
            (template_dir / "template.txt").write_text("template")

            clear_cache(template_only=True)

            assert cache_file.exists()
            assert not (tmp_path / "templates").exists()

    def test_clear_cache_nonexistent_dir(self, tmp_path: Path) -> None:
        """存在しないディレクトリのクリアはエラーにならない"""
        nonexistent = tmp_path / "nonexistent"
        with patch("mnemonic.cache.get_cache_dir", return_value=nonexistent):
            clear_cache()


class TestGetCacheInfo:
    """get_cache_info関数のテスト"""

    def test_get_cache_info_nonexistent_dir(self, tmp_path: Path) -> None:
        """存在しないディレクトリの情報を取得"""
        nonexistent = tmp_path / "nonexistent"
        with patch("mnemonic.cache.get_cache_dir", return_value=nonexistent):
            result = get_cache_info()

            assert result.directory == nonexistent
            assert result.size_bytes == 0
            assert result.template_version is None
            assert result.template_expires_in_days is None

    def test_get_cache_info_with_files(self, tmp_path: Path) -> None:
        """ファイルが存在する場合のキャッシュ情報"""
        with patch("mnemonic.cache.get_cache_dir", return_value=tmp_path):
            test_file = tmp_path / "test.txt"
            test_file.write_text("test content")

            result = get_cache_info()

            assert result.directory == tmp_path
            assert result.size_bytes > 0
            assert result.template_version is None

    def test_get_cache_info_with_template(self, tmp_path: Path) -> None:
        """テンプレートが存在する場合のキャッシュ情報"""
        with patch("mnemonic.cache.get_cache_dir", return_value=tmp_path):
            template_dir = tmp_path / "templates" / "1.0.0"
            template_dir.mkdir(parents=True)
            (template_dir / "template.txt").write_text("template")

            result = get_cache_info()

            assert result.directory == tmp_path
            assert result.size_bytes > 0
            assert result.template_version == "1.0.0"
            assert result.template_expires_in_days is not None
            assert result.template_expires_in_days <= 7
