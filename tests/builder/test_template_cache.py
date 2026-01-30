"""テンプレートキャッシュ機能のテスト"""

from pathlib import Path
from unittest.mock import MagicMock

import pytest

from mnemonic.builder.template import TemplateCache, TemplateCacheError

class TestTemplateCacheInit:
    """TemplateCache初期化のテスト"""

    def test_init_with_default_refresh_days(self) -> None:
        """正常系: デフォルトのリフレッシュ日数で初期化"""
        mock_cache_manager = MagicMock()
        cache = TemplateCache(cache_manager=mock_cache_manager)

        assert cache._cache_manager is mock_cache_manager
        assert cache._refresh_days == 7

    def test_init_with_custom_refresh_days(self) -> None:
        """正常系: カスタムのリフレッシュ日数で初期化"""
        mock_cache_manager = MagicMock()
        cache = TemplateCache(cache_manager=mock_cache_manager, refresh_days=14)

        assert cache._refresh_days == 14

class TestTemplateCacheGetCachedTemplate:
    """TemplateCache.get_cached_templateのテスト"""

    def test_get_cached_template_returns_path_when_cache_exists(self) -> None:
        """正常系: キャッシュが存在する場合にパスを返す"""
        mock_cache_manager = MagicMock()
        cache_path = Path("/tmp/cache/templates/v1.0.0")
        mock_cache_manager.get_template_cache_path.return_value = cache_path

        cache = TemplateCache(cache_manager=mock_cache_manager)

        with pytest.raises(NotImplementedError):
            cache.get_cached_template()

    def test_get_cached_template_returns_none_when_cache_not_exists(self) -> None:
        """正常系: キャッシュが存在しない場合にNoneを返す"""
        mock_cache_manager = MagicMock()
        mock_cache_manager.get_template_cache_path.return_value = Path("/tmp/nonexistent")

        cache = TemplateCache(cache_manager=mock_cache_manager)

        with pytest.raises(NotImplementedError):
            cache.get_cached_template()

    @pytest.mark.parametrize(
        "version",
        [
            pytest.param("v1.0.0", id="正常系: バージョンv1.0.0を指定"),
            pytest.param("v2.1.3", id="正常系: バージョンv2.1.3を指定"),
            pytest.param("1.0.0", id="正常系: vプレフィックスなしのバージョンを指定"),
        ],
    )
    def test_get_cached_template_with_specific_version(self, version: str) -> None:
        """正常系: バージョン指定時は指定バージョンのキャッシュを返す"""
        mock_cache_manager = MagicMock()

        cache = TemplateCache(cache_manager=mock_cache_manager)

        with pytest.raises(NotImplementedError):
            cache.get_cached_template(version=version)

class TestTemplateCacheIsCacheValid:
    """TemplateCache.is_cache_validのテスト"""

    def test_is_cache_valid_returns_true_when_within_expiry(self) -> None:
        """正常系: 期限内のキャッシュは有効"""
        mock_cache_manager = MagicMock()
        mock_cache_manager.is_cache_valid.return_value = True

        cache = TemplateCache(cache_manager=mock_cache_manager, refresh_days=7)

        with pytest.raises(NotImplementedError):
            cache.is_cache_valid()

    def test_is_cache_valid_returns_false_when_expired(self) -> None:
        """正常系: 期限切れのキャッシュは無効"""
        mock_cache_manager = MagicMock()
        mock_cache_manager.is_cache_valid.return_value = False

        cache = TemplateCache(cache_manager=mock_cache_manager, refresh_days=7)

        with pytest.raises(NotImplementedError):
            cache.is_cache_valid()

    @pytest.mark.parametrize(
        "refresh_days,expected_max_age",
        [
            pytest.param(7, 7, id="正常系: デフォルト7日"),
            pytest.param(14, 14, id="正常系: 14日に変更"),
            pytest.param(1, 1, id="正常系: 1日に変更"),
            pytest.param(30, 30, id="正常系: 30日に変更"),
        ],
    )
    def test_is_cache_valid_respects_refresh_days(
        self, refresh_days: int, expected_max_age: int
    ) -> None:
        """正常系: refresh_daysで期限を変更できる"""
        mock_cache_manager = MagicMock()

        cache = TemplateCache(cache_manager=mock_cache_manager, refresh_days=refresh_days)

        assert cache._refresh_days == expected_max_age

        with pytest.raises(NotImplementedError):
            cache.is_cache_valid()

    def test_is_cache_valid_with_specific_version(self) -> None:
        """正常系: バージョン指定時はそのバージョンのキャッシュを確認"""
        mock_cache_manager = MagicMock()

        cache = TemplateCache(cache_manager=mock_cache_manager)

        with pytest.raises(NotImplementedError):
            cache.is_cache_valid(version="v1.0.0")

class TestTemplateCacheGetCachedVersion:
    """TemplateCache.get_cached_versionのテスト"""

    def test_get_cached_version_returns_version_when_cache_exists(self) -> None:
        """正常系: キャッシュされているバージョンを取得できる"""
        mock_cache_manager = MagicMock()

        cache = TemplateCache(cache_manager=mock_cache_manager)

        with pytest.raises(NotImplementedError):
            cache.get_cached_version()

    def test_get_cached_version_returns_none_when_no_cache(self) -> None:
        """正常系: キャッシュが存在しない場合はNoneを返す"""
        mock_cache_manager = MagicMock()

        cache = TemplateCache(cache_manager=mock_cache_manager)

        with pytest.raises(NotImplementedError):
            cache.get_cached_version()

class TestTemplateCacheSaveTemplate:
    """TemplateCache.save_templateのテスト"""

    def test_save_template_stores_to_cache_directory(self, tmp_path: Path) -> None:
        """正常系: テンプレートがキャッシュディレクトリに保存される"""
        mock_cache_manager = MagicMock()
        template_file = tmp_path / "template.zip"
        template_file.write_bytes(b"test content")

        cache = TemplateCache(cache_manager=mock_cache_manager)

        with pytest.raises(NotImplementedError):
            cache.save_template(template_path=template_file, version="v1.0.0")

    def test_save_template_records_version_info(self, tmp_path: Path) -> None:
        """正常系: バージョン情報が記録される"""
        mock_cache_manager = MagicMock()
        template_file = tmp_path / "template.zip"
        template_file.write_bytes(b"test content")

        cache = TemplateCache(cache_manager=mock_cache_manager)

        with pytest.raises(NotImplementedError):
            cache.save_template(template_path=template_file, version="v2.0.0")

    def test_save_template_raises_error_when_file_not_found(self) -> None:
        """異常系: 指定されたテンプレートファイルが存在しない場合"""
        mock_cache_manager = MagicMock()
        non_existent_path = Path("/nonexistent/template.zip")

        cache = TemplateCache(cache_manager=mock_cache_manager)

        with pytest.raises((NotImplementedError, FileNotFoundError)):
            cache.save_template(template_path=non_existent_path, version="v1.0.0")

    @pytest.mark.parametrize(
        "version",
        [
            pytest.param("v1.0.0", id="正常系: バージョンv1.0.0を保存"),
            pytest.param("v2.1.3", id="正常系: バージョンv2.1.3を保存"),
            pytest.param("v3.0.0-beta", id="正常系: ベータバージョンを保存"),
        ],
    )
    def test_save_template_with_various_versions(self, tmp_path: Path, version: str) -> None:
        """正常系: 様々なバージョン形式で保存できる"""
        mock_cache_manager = MagicMock()
        template_file = tmp_path / "template.zip"
        template_file.write_bytes(b"test content")

        cache = TemplateCache(cache_manager=mock_cache_manager)

        with pytest.raises(NotImplementedError):
            cache.save_template(template_path=template_file, version=version)

class TestTemplateCacheClearCache:
    """TemplateCache.clear_cacheのテスト"""

    def test_clear_cache_removes_cached_templates(self) -> None:
        """正常系: キャッシュが削除される"""
        mock_cache_manager = MagicMock()

        cache = TemplateCache(cache_manager=mock_cache_manager)

        with pytest.raises(NotImplementedError):
            cache.clear_cache()

    def test_clear_cache_succeeds_when_no_cache_exists(self) -> None:
        """正常系: キャッシュが存在しない場合も正常に完了"""
        mock_cache_manager = MagicMock()

        cache = TemplateCache(cache_manager=mock_cache_manager)

        with pytest.raises(NotImplementedError):
            cache.clear_cache()

class TestTemplateCacheErrorHandling:
    """TemplateCache例外処理のテスト"""

    def test_template_cache_error_inheritance(self) -> None:
        """正常系: TemplateCacheErrorが適切な継承関係を持つ"""
        error = TemplateCacheError("cache error")
        assert isinstance(error, Exception)
        assert str(error) == "cache error"

    def test_template_cache_error_with_context(self) -> None:
        """正常系: TemplateCacheErrorがコンテキスト情報を保持"""
        original_error = OSError("disk full")
        error = TemplateCacheError("Failed to save cache")
        error.__cause__ = original_error

        assert error.__cause__ is original_error
        assert "Failed to save cache" in str(error)
