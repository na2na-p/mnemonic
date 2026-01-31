"""ZipalignRunnerのテスト

このモジュールはZipalignRunnerインターフェースの実装に対するテストを提供します。
"""

import subprocess
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from mnemonic.signer.apk import ZipalignError


class TestZipalignErrorClass:
    """ZipalignError例外クラスのテスト"""

    def test_zipalign_error_inheritance(self) -> None:
        """正常系: ZipalignErrorがExceptionを継承している"""
        error = ZipalignError("zipalign failed")
        assert isinstance(error, Exception)
        assert str(error) == "zipalign failed"

    def test_zipalign_error_with_message(self) -> None:
        """正常系: ZipalignErrorにメッセージを設定できる"""
        error = ZipalignError("Input file not found: /path/to/file.apk")
        assert "Input file not found" in str(error)


class TestZipalignRunnerProtocol:
    """ZipalignRunnerプロトコルのテスト

    注意: このテストはDefaultZipalignRunner実装に対して実行されます。
    実装がまだない場合、テストはインポートエラーで失敗します。
    """

    pass


class TestDefaultZipalignRunnerAlign:
    """DefaultZipalignRunner.alignメソッドのテスト"""

    @pytest.mark.parametrize(
        "input_exists,output_content,returncode,expected_success",
        [
            pytest.param(
                True,
                b"aligned apk content",
                0,
                True,
                id="正常系: アラインメント成功",
            ),
        ],
    )
    def test_align_success(
        self,
        tmp_path: Path,
        input_exists: bool,
        output_content: bytes,
        returncode: int,
        expected_success: bool,
    ) -> None:
        """alignが成功した場合に出力パスを返す"""
        from mnemonic.signer.apk import DefaultZipalignRunner

        input_apk = tmp_path / "input.apk"
        output_apk = tmp_path / "output.apk"

        if input_exists:
            input_apk.write_bytes(b"unaligned apk content")

        runner = DefaultZipalignRunner()

        with patch.object(runner, "find_zipalign") as mock_find:
            mock_find.return_value = Path("/android/sdk/build-tools/34.0.0/zipalign")

            with patch("subprocess.run") as mock_run:
                mock_result = MagicMock()
                mock_result.returncode = returncode
                mock_result.stdout = ""
                mock_result.stderr = ""
                mock_run.return_value = mock_result

                # モックの副作用として出力ファイルを作成
                def create_output(*args, **kwargs):
                    output_apk.write_bytes(output_content)
                    return mock_result

                mock_run.side_effect = create_output

                result = runner.align(input_apk, output_apk)

                assert result == output_apk
                mock_run.assert_called_once()
                call_args = mock_run.call_args[0][0]
                assert "zipalign" in str(call_args[0])
                assert "-f" in call_args
                assert "4" in call_args
                assert str(input_apk) in call_args
                assert str(output_apk) in call_args

    def test_align_input_file_not_found(self, tmp_path: Path) -> None:
        """異常系: 入力ファイルが存在しない場合にZipalignErrorが発生"""
        from mnemonic.signer.apk import DefaultZipalignRunner

        input_apk = tmp_path / "non_existent.apk"
        output_apk = tmp_path / "output.apk"

        runner = DefaultZipalignRunner()

        with pytest.raises(ZipalignError) as exc_info:
            runner.align(input_apk, output_apk)

        assert "not found" in str(exc_info.value).lower() or "存在しません" in str(exc_info.value)

    def test_align_zipalign_not_found(self, tmp_path: Path) -> None:
        """異常系: zipalignコマンドが見つからない場合にZipalignErrorが発生"""
        from mnemonic.signer.apk import DefaultZipalignRunner

        input_apk = tmp_path / "input.apk"
        input_apk.write_bytes(b"unaligned apk content")
        output_apk = tmp_path / "output.apk"

        runner = DefaultZipalignRunner()

        with patch.object(runner, "find_zipalign") as mock_find:
            mock_find.return_value = None

            with pytest.raises(ZipalignError) as exc_info:
                runner.align(input_apk, output_apk)

            assert "zipalign" in str(exc_info.value).lower()

    def test_align_command_failure(self, tmp_path: Path) -> None:
        """異常系: zipalignコマンドが失敗した場合にZipalignErrorが発生"""
        from mnemonic.signer.apk import DefaultZipalignRunner

        input_apk = tmp_path / "input.apk"
        input_apk.write_bytes(b"unaligned apk content")
        output_apk = tmp_path / "output.apk"

        runner = DefaultZipalignRunner()

        with patch.object(runner, "find_zipalign") as mock_find:
            mock_find.return_value = Path("/android/sdk/build-tools/34.0.0/zipalign")

            with patch("subprocess.run") as mock_run:
                mock_result = MagicMock()
                mock_result.returncode = 1
                mock_result.stdout = ""
                mock_result.stderr = "zipalign error: invalid input"
                mock_run.return_value = mock_result

                with pytest.raises(ZipalignError) as exc_info:
                    runner.align(input_apk, output_apk)

                assert "failed" in str(exc_info.value).lower() or "失敗" in str(exc_info.value)


class TestDefaultZipalignRunnerFindZipalign:
    """DefaultZipalignRunner.find_zipalignメソッドのテスト"""

    def test_find_zipalign_from_android_home(self, tmp_path: Path) -> None:
        """正常系: ANDROID_HOME環境変数からzipalignを検出"""
        from mnemonic.signer.apk import DefaultZipalignRunner

        android_home = tmp_path / "android-sdk"
        build_tools = android_home / "build-tools" / "34.0.0"
        build_tools.mkdir(parents=True)
        zipalign_path = build_tools / "zipalign"
        zipalign_path.touch()

        runner = DefaultZipalignRunner()

        with (
            patch.dict("os.environ", {"ANDROID_HOME": str(android_home)}),
            patch("shutil.which") as mock_which,
        ):
            # PATHからは見つからない設定
            mock_which.return_value = None

            result = runner.find_zipalign()

            assert result is not None
            assert "zipalign" in str(result)

    def test_find_zipalign_from_path(self) -> None:
        """正常系: システムPATHからzipalignを検出"""
        from mnemonic.signer.apk import DefaultZipalignRunner

        runner = DefaultZipalignRunner()

        # ANDROID_HOMEを削除した環境変数を作成
        import os

        env_without_android_home = {k: v for k, v in os.environ.items() if k != "ANDROID_HOME"}

        with (
            patch.dict("os.environ", env_without_android_home, clear=True),
            patch("shutil.which") as mock_which,
        ):
            mock_which.return_value = "/usr/local/bin/zipalign"

            result = runner.find_zipalign()

            assert result == Path("/usr/local/bin/zipalign")
            mock_which.assert_called_with("zipalign")

    def test_find_zipalign_not_found(self) -> None:
        """正常系: zipalignが見つからない場合にNoneを返す"""
        from mnemonic.signer.apk import DefaultZipalignRunner

        runner = DefaultZipalignRunner()

        with (
            patch.dict("os.environ", {}, clear=True),
            patch("shutil.which") as mock_which,
        ):
            mock_which.return_value = None

            result = runner.find_zipalign()

            assert result is None

    @pytest.mark.parametrize(
        "build_tool_versions,expected_version",
        [
            pytest.param(
                ["30.0.0", "33.0.0", "34.0.0"],
                "34.0.0",
                id="正常系: 複数バージョンから最新を選択",
            ),
            pytest.param(
                ["31.0.0"],
                "31.0.0",
                id="正常系: 単一バージョン",
            ),
        ],
    )
    def test_find_zipalign_selects_latest_version(
        self,
        tmp_path: Path,
        build_tool_versions: list[str],
        expected_version: str,
    ) -> None:
        """正常系: 複数のbuild-toolsバージョンがある場合に最新を選択"""
        from mnemonic.signer.apk import DefaultZipalignRunner

        android_home = tmp_path / "android-sdk"
        for version in build_tool_versions:
            build_tools = android_home / "build-tools" / version
            build_tools.mkdir(parents=True)
            zipalign_path = build_tools / "zipalign"
            zipalign_path.touch()

        runner = DefaultZipalignRunner()

        with (
            patch.dict("os.environ", {"ANDROID_HOME": str(android_home)}),
            patch("shutil.which") as mock_which,
        ):
            mock_which.return_value = None

            result = runner.find_zipalign()

            assert result is not None
            assert expected_version in str(result)


class TestDefaultZipalignRunnerIsAligned:
    """DefaultZipalignRunner.is_alignedメソッドのテスト"""

    @pytest.mark.parametrize(
        "returncode,expected_aligned",
        [
            pytest.param(0, True, id="正常系: アラインメント済み"),
            pytest.param(1, False, id="正常系: アラインメント未済"),
        ],
    )
    def test_is_aligned(
        self,
        tmp_path: Path,
        returncode: int,
        expected_aligned: bool,
    ) -> None:
        """is_alignedがアラインメント状態を正しく判定"""
        from mnemonic.signer.apk import DefaultZipalignRunner

        apk_path = tmp_path / "test.apk"
        apk_path.write_bytes(b"apk content")

        runner = DefaultZipalignRunner()

        with patch.object(runner, "find_zipalign") as mock_find:
            mock_find.return_value = Path("/android/sdk/build-tools/34.0.0/zipalign")

            with patch("subprocess.run") as mock_run:
                mock_result = MagicMock()
                mock_result.returncode = returncode
                mock_result.stdout = "Verification successful" if returncode == 0 else ""
                mock_result.stderr = "" if returncode == 0 else "Verification failed"
                mock_run.return_value = mock_result

                result = runner.is_aligned(apk_path)

                assert result is expected_aligned
                mock_run.assert_called_once()
                call_args = mock_run.call_args[0][0]
                assert "-c" in call_args
                assert "4" in call_args
                assert str(apk_path) in call_args

    def test_is_aligned_file_not_found(self, tmp_path: Path) -> None:
        """異常系: ファイルが存在しない場合にZipalignErrorが発生"""
        from mnemonic.signer.apk import DefaultZipalignRunner

        apk_path = tmp_path / "non_existent.apk"

        runner = DefaultZipalignRunner()

        with pytest.raises(ZipalignError) as exc_info:
            runner.is_aligned(apk_path)

        assert "not found" in str(exc_info.value).lower() or "存在しません" in str(exc_info.value)

    def test_is_aligned_zipalign_not_found(self, tmp_path: Path) -> None:
        """異常系: zipalignコマンドが見つからない場合にZipalignErrorが発生"""
        from mnemonic.signer.apk import DefaultZipalignRunner

        apk_path = tmp_path / "test.apk"
        apk_path.write_bytes(b"apk content")

        runner = DefaultZipalignRunner()

        with patch.object(runner, "find_zipalign") as mock_find:
            mock_find.return_value = None

            with pytest.raises(ZipalignError) as exc_info:
                runner.is_aligned(apk_path)

            assert "zipalign" in str(exc_info.value).lower()

    def test_is_aligned_command_error(self, tmp_path: Path) -> None:
        """異常系: zipalignコマンド実行中にエラーが発生した場合にZipalignErrorが発生"""
        from mnemonic.signer.apk import DefaultZipalignRunner

        apk_path = tmp_path / "test.apk"
        apk_path.write_bytes(b"apk content")

        runner = DefaultZipalignRunner()

        with patch.object(runner, "find_zipalign") as mock_find:
            mock_find.return_value = Path("/android/sdk/build-tools/34.0.0/zipalign")

            with patch("subprocess.run") as mock_run:
                mock_run.side_effect = subprocess.SubprocessError("command failed")

                with pytest.raises(ZipalignError) as exc_info:
                    runner.is_aligned(apk_path)

                assert "failed" in str(exc_info.value).lower() or "失敗" in str(exc_info.value)
