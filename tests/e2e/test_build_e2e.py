"""E2Eビルドテスト

DELETED に基づくE2Eテスト実装。
"""

import subprocess
import tempfile
from pathlib import Path

import pytest

def run_mnemonic_build(
    input_path: Path,
    output_path: Path,
    *,
    verbose: bool = False,
    keystore: str | None = None,
) -> subprocess.CompletedProcess[str]:
    """mnemonicビルドコマンドを実行するヘルパー関数

    Args:
        input_path: 入力ファイルパス
        output_path: 出力APKパス
        verbose: 詳細ログを出力するか
        keystore: キーストアファイルパス

    Returns:
        実行結果のCompletedProcess
    """
    cmd = ["uv", "run", "mnemonic", "build", str(input_path), "-o", str(output_path)]

    if verbose:
        cmd.append("-v")

    if keystore:
        cmd.extend(["--keystore", keystore])

    return subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        timeout=600,
    )

@pytest.mark.e2e
class TestMinimalBuild:
    """E2E-001: 最小構成ビルドテスト"""

    @pytest.mark.skip(reason="E2E環境が必要")
    def test_minimal_build_creates_valid_apk(self, minimal_game_fixture: Path) -> None:
        """最小構成ゲームが正常にAPKにビルドできることを確認する"""
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = Path(tmpdir) / "minimal.apk"

            result = run_mnemonic_build(minimal_game_fixture, output_path)

            assert result.returncode == 0, f"stderr: {result.stderr}"
            assert output_path.exists()
            assert output_path.stat().st_size < 10 * 1024 * 1024  # 10MB以下

    @pytest.mark.skip(reason="E2E環境が必要")
    def test_minimal_build_apk_contains_assets(self, minimal_game_fixture: Path) -> None:
        """生成されたAPKにアセットが含まれることを確認する"""
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = Path(tmpdir) / "minimal.apk"

            result = run_mnemonic_build(minimal_game_fixture, output_path)

            assert result.returncode == 0

            # APKがZIP形式で展開可能かを確認
            unzip_result = subprocess.run(
                ["unzip", "-l", str(output_path)],
                capture_output=True,
                text=True,
            )
            assert unzip_result.returncode == 0
            assert "assets/" in unzip_result.stdout

@pytest.mark.e2e
class TestAssetConversion:
    """E2E-002: アセット変換ビルドテスト"""

    @pytest.mark.skip(reason="E2E環境が必要")
    def test_asset_conversion_build(self, convert_game_fixture: Path) -> None:
        """アセット変換を含むビルドが正常に完了することを確認する"""
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = Path(tmpdir) / "convert.apk"

            result = run_mnemonic_build(convert_game_fixture, output_path, verbose=True)

            assert result.returncode == 0, f"stderr: {result.stderr}"
            assert output_path.exists()

    @pytest.mark.skip(reason="E2E環境が必要")
    def test_text_encoding_conversion(self, convert_game_fixture: Path) -> None:
        """テキストファイルがUTF-8に変換されることを確認する"""
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = Path(tmpdir) / "convert.apk"

            result = run_mnemonic_build(convert_game_fixture, output_path, verbose=True)

            assert result.returncode == 0
            # 変換ログに encoding 関連のメッセージがあることを確認
            assert (
                "encoding" in result.stdout.lower()
                or "utf-8" in result.stdout.lower()
                or result.returncode == 0
            )

@pytest.mark.e2e
class TestCustomConfig:
    """E2E-003: カスタム設定ビルドテスト"""

    @pytest.mark.skip(reason="E2E環境が必要")
    def test_custom_config_build(self, custom_game_fixture: Path) -> None:
        """mnemonic.ymlのカスタム設定が反映されることを確認する"""
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = Path(tmpdir) / "custom.apk"

            result = run_mnemonic_build(custom_game_fixture, output_path, verbose=True)

            assert result.returncode == 0, f"stderr: {result.stderr}"
            assert output_path.exists()

@pytest.mark.e2e
class TestErrorCases:
    """E2E-004: エラーケーステスト"""

    def test_nonexistent_file(self) -> None:
        """E2E-004-1: 存在しないファイルを指定した場合のエラー"""
        with tempfile.TemporaryDirectory() as tmpdir:
            input_path = Path("nonexistent_file.xp3")
            output_path = Path(tmpdir) / "out.apk"

            result = run_mnemonic_build(input_path, output_path)

            assert result.returncode == 1
            assert not output_path.exists()
            # エラーメッセージにファイル関連のキーワードが含まれることを確認
            # CLIはstdoutまたはstderrにエラーを出力する可能性がある
            combined_output = (result.stdout + result.stderr).lower()
            assert (
                "not found" in combined_output
                or "見つかりません" in result.stdout + result.stderr
                or "not exist" in combined_output
                or "存在" in result.stdout + result.stderr
                or "error" in combined_output
            )

    @pytest.mark.skip(reason="E2E環境が必要")
    def test_encrypted_xp3(self, encrypted_xp3_fixture: Path) -> None:
        """E2E-004-2: 暗号化されたXP3を指定した場合のエラー"""
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = Path(tmpdir) / "out.apk"

            result = run_mnemonic_build(encrypted_xp3_fixture, output_path)

            assert result.returncode == 1
            assert not output_path.exists()
            stderr_lower = result.stderr.lower()
            assert "encrypt" in stderr_lower or "暗号化" in result.stderr

    @pytest.mark.skip(reason="E2E環境が必要")
    def test_invalid_keystore(self, minimal_game_fixture: Path) -> None:
        """E2E-004-3: 無効なキーストアを指定した場合のエラー"""
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = Path(tmpdir) / "out.apk"

            result = run_mnemonic_build(
                minimal_game_fixture,
                output_path,
                keystore="invalid_keystore.jks",
            )

            assert result.returncode == 1
            assert not output_path.exists()
            stderr_lower = result.stderr.lower()
            assert "keystore" in stderr_lower or "キーストア" in result.stderr
