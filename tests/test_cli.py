"""CLIエントリポイントのテスト"""

import pytest
from typer.testing import CliRunner

from mnemonic.cli import app

runner = CliRunner()

class TestMainCommand:
    """メインコマンドのテスト"""

    @pytest.mark.parametrize(
        "args,expected_in_output",
        [
            pytest.param(["--help"], "吉里吉里ゲーム", id="正常系: ヘルプ表示"),
            pytest.param(["--version"], "0.1.0", id="正常系: バージョン表示"),
        ],
    )
    def test_main_options(self, args: list[str], expected_in_output: str) -> None:
        result = runner.invoke(app, args)
        assert result.exit_code == 0
        assert expected_in_output in result.stdout

class TestBuildCommand:
    """buildコマンドのテスト"""

    def test_build_help(self) -> None:
        """buildコマンドのヘルプが表示される"""
        result = runner.invoke(app, ["build", "--help"])
        assert result.exit_code == 0
        assert "ビルド" in result.stdout or "build" in result.stdout.lower()

class TestDoctorCommand:
    """doctorコマンドのテスト"""

    def test_doctor_runs(self) -> None:
        """doctorコマンドが正常終了する"""
        result = runner.invoke(app, ["doctor"])
        assert result.exit_code == 0

class TestInfoCommand:
    """infoコマンドのテスト"""

    def test_info_help(self) -> None:
        """infoコマンドのヘルプが表示される"""
        result = runner.invoke(app, ["info", "--help"])
        assert result.exit_code == 0

class TestCacheCommand:
    """cacheコマンドのテスト"""

    def test_cache_help(self) -> None:
        """cacheコマンドのヘルプが表示される"""
        result = runner.invoke(app, ["cache", "--help"])
        assert result.exit_code == 0
        assert "clean" in result.stdout
        assert "info" in result.stdout

    def test_cache_clean_help(self) -> None:
        """cache cleanコマンドのヘルプが表示される"""
        result = runner.invoke(app, ["cache", "clean", "--help"])
        assert result.exit_code == 0
        assert "--force" in result.stdout or "-f" in result.stdout

    def test_cache_info_runs(self) -> None:
        """cache infoコマンドが正常終了する"""
        result = runner.invoke(app, ["cache", "info"])
        assert result.exit_code == 0
