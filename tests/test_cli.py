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
        """doctorコマンドが実行される"""
        result = runner.invoke(app, ["doctor"])
        # exit_code は環境に依存（必須ツールが揃っていれば0、不足していれば1）
        assert result.exit_code in (0, 1)

    def test_doctor_shows_table(self) -> None:
        """doctorコマンドがテーブルを表示する"""
        result = runner.invoke(app, ["doctor"])
        assert "依存ツールチェック結果" in result.stdout

    def test_doctor_shows_python(self) -> None:
        """doctorコマンドがPythonチェック結果を表示する"""
        result = runner.invoke(app, ["doctor"])
        assert "Python" in result.stdout

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
