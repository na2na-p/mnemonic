"""cache CLIサブコマンドのテスト"""

import pytest
from typer.testing import CliRunner

from mnemonic.cli import app

runner = CliRunner()

class TestCacheHelpCommand:
    """cache --help コマンドのテスト"""

    def test_cache_help_shows_subcommands(self) -> None:
        """cache --help でサブコマンド一覧が表示される"""
        result = runner.invoke(app, ["cache", "--help"])
        assert result.exit_code == 0
        assert "clean" in result.stdout
        assert "info" in result.stdout

    def test_cache_help_shows_description(self) -> None:
        """cache --help でキャッシュ管理の説明が表示される"""
        result = runner.invoke(app, ["cache", "--help"])
        assert result.exit_code == 0
        assert "キャッシュ" in result.stdout

class TestCacheCleanCommand:
    """cache clean コマンドのテスト"""

    def test_cache_clean_basic_execution(self) -> None:
        """cache clean の基本実行が正常終了する"""
        result = runner.invoke(app, ["cache", "clean"], input="y\n")
        assert result.exit_code == 0

    @pytest.mark.parametrize(
        "args",
        [
            pytest.param(["cache", "clean", "--force"], id="正常系: --force オプション"),
            pytest.param(["cache", "clean", "-f"], id="正常系: -f オプション（短縮形）"),
        ],
    )
    def test_cache_clean_force_option(self, args: list[str]) -> None:
        """cache clean --force / -f オプションが正常動作する"""
        result = runner.invoke(app, args)
        assert result.exit_code == 0

    def test_cache_clean_template_only_option(self) -> None:
        """cache clean --template-only オプションが正常動作する"""
        result = runner.invoke(app, ["cache", "clean", "--template-only"], input="y\n")
        assert result.exit_code == 0

    @pytest.mark.parametrize(
        "args",
        [
            pytest.param(
                ["cache", "clean", "--force", "--template-only"],
                id="正常系: --force と --template-only の組み合わせ",
            ),
            pytest.param(
                ["cache", "clean", "-f", "--template-only"],
                id="正常系: -f と --template-only の組み合わせ",
            ),
        ],
    )
    def test_cache_clean_combined_options(self, args: list[str]) -> None:
        """cache clean のオプション組み合わせが正常動作する"""
        result = runner.invoke(app, args)
        assert result.exit_code == 0

    def test_cache_clean_help_shows_options(self) -> None:
        """cache clean --help でオプション一覧が表示される"""
        result = runner.invoke(app, ["cache", "clean", "--help"])
        assert result.exit_code == 0
        assert "--force" in result.stdout or "-f" in result.stdout
        assert "--template-only" in result.stdout

    def test_cache_clean_cancelled_when_declined(self) -> None:
        """cache clean で確認を拒否した場合はキャンセルされる"""
        result = runner.invoke(app, ["cache", "clean"], input="n\n")
        assert result.exit_code == 0
        assert "キャンセル" in result.stdout

class TestCacheInfoCommand:
    """cache info コマンドのテスト"""

    def test_cache_info_basic_execution(self) -> None:
        """cache info の基本実行が正常終了する"""
        result = runner.invoke(app, ["cache", "info"])
        assert result.exit_code == 0

    def test_cache_info_help(self) -> None:
        """cache info --help が正常終了する"""
        result = runner.invoke(app, ["cache", "info", "--help"])
        assert result.exit_code == 0
        assert "キャッシュ情報" in result.stdout
