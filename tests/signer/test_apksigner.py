"""ApkSignerRunnerのテスト

このモジュールはApkSignerRunnerインターフェースの実装に対するテストを提供します。
"""

import subprocess
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from mnemonic.signer.apk import ApkSignerError, KeystoreConfig


class TestApkSignerErrorClass:
    """ApkSignerError例外クラスのテスト"""

    def test_apksigner_error_inheritance(self) -> None:
        """正常系: ApkSignerErrorがExceptionを継承している"""
        error = ApkSignerError("apksigner failed")
        assert isinstance(error, Exception)
        assert str(error) == "apksigner failed"

    def test_apksigner_error_with_message(self) -> None:
        """正常系: ApkSignerErrorにメッセージを設定できる"""
        error = ApkSignerError("APK file not found: /path/to/file.apk")
        assert "APK file not found" in str(error)


class TestKeystoreConfigClass:
    """KeystoreConfig設定クラスのテスト"""

    def test_keystore_config_creation(self, tmp_path: Path) -> None:
        """正常系: KeystoreConfigが正しく作成される"""
        keystore_path = tmp_path / "keystore.jks"
        config = KeystoreConfig(
            keystore_path=keystore_path,
            key_alias="my_alias",
            keystore_password="keystore_pass",
            key_password="key_pass",
        )
        assert config.keystore_path == keystore_path
        assert config.key_alias == "my_alias"
        assert config.keystore_password == "keystore_pass"
        assert config.key_password == "key_pass"

    def test_keystore_config_key_password_defaults_to_none(self, tmp_path: Path) -> None:
        """正常系: key_passwordがデフォルトでNoneになる"""
        keystore_path = tmp_path / "keystore.jks"
        config = KeystoreConfig(
            keystore_path=keystore_path,
            key_alias="my_alias",
            keystore_password="keystore_pass",
        )
        assert config.key_password is None

    def test_keystore_config_is_frozen(self, tmp_path: Path) -> None:
        """正常系: KeystoreConfigが不変である"""
        keystore_path = tmp_path / "keystore.jks"
        config = KeystoreConfig(
            keystore_path=keystore_path,
            key_alias="my_alias",
            keystore_password="keystore_pass",
        )
        with pytest.raises(AttributeError):
            config.key_alias = "new_alias"  # type: ignore[misc]


class TestApkSignerRunnerProtocol:
    """ApkSignerRunnerプロトコルのテスト

    注意: このテストはDefaultApkSignerRunner実装に対して実行されます。
    実装がまだない場合、テストはインポートエラーで失敗します。
    """

    pass


class TestDefaultApkSignerRunnerSign:
    """DefaultApkSignerRunner.signメソッドのテスト"""

    @pytest.mark.parametrize(
        "returncode,expected_success",
        [
            pytest.param(
                0,
                True,
                id="正常系: 署名成功",
            ),
        ],
    )
    def test_sign_success(
        self,
        tmp_path: Path,
        returncode: int,
        expected_success: bool,
    ) -> None:
        """signが成功した場合に署名済みAPKのパスを返す"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        apk_path = tmp_path / "test.apk"
        apk_path.write_bytes(b"apk content")

        keystore_path = tmp_path / "keystore.jks"
        keystore_path.write_bytes(b"keystore content")

        keystore_config = KeystoreConfig(
            keystore_path=keystore_path,
            key_alias="my_alias",
            keystore_password="keystore_pass",
            key_password="key_pass",
        )

        runner = DefaultApkSignerRunner()

        with patch.object(runner, "find_apksigner") as mock_find:
            mock_find.return_value = Path("/android/sdk/build-tools/34.0.0/apksigner")

            with patch("subprocess.run") as mock_run:
                mock_result = MagicMock()
                mock_result.returncode = returncode
                mock_result.stdout = ""
                mock_result.stderr = ""
                mock_run.return_value = mock_result

                result = runner.sign(apk_path, keystore_config)

                assert result == apk_path
                mock_run.assert_called_once()
                call_args = mock_run.call_args[0][0]
                assert "apksigner" in str(call_args[0])
                assert "sign" in call_args
                assert "--ks" in call_args
                assert str(keystore_path) in call_args
                assert "--ks-key-alias" in call_args
                assert "my_alias" in call_args

    def test_sign_without_key_password_uses_keystore_password(
        self,
        tmp_path: Path,
    ) -> None:
        """正常系: key_passwordが未指定の場合はkeystore_passwordを使用"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        apk_path = tmp_path / "test.apk"
        apk_path.write_bytes(b"apk content")

        keystore_path = tmp_path / "keystore.jks"
        keystore_path.write_bytes(b"keystore content")

        keystore_config = KeystoreConfig(
            keystore_path=keystore_path,
            key_alias="my_alias",
            keystore_password="shared_password",
        )

        runner = DefaultApkSignerRunner()

        with patch.object(runner, "find_apksigner") as mock_find:
            mock_find.return_value = Path("/android/sdk/build-tools/34.0.0/apksigner")

            with patch("subprocess.run") as mock_run:
                mock_result = MagicMock()
                mock_result.returncode = 0
                mock_result.stdout = ""
                mock_result.stderr = ""
                mock_run.return_value = mock_result

                runner.sign(apk_path, keystore_config)

                mock_run.assert_called_once()
                call_args = mock_run.call_args[0][0]
                assert "--key-pass" in call_args
                key_pass_idx = call_args.index("--key-pass")
                assert call_args[key_pass_idx + 1] == "pass:shared_password"

    def test_sign_apk_not_found(self, tmp_path: Path) -> None:
        """異常系: APKファイルが存在しない場合にApkSignerErrorが発生"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        apk_path = tmp_path / "non_existent.apk"

        keystore_path = tmp_path / "keystore.jks"
        keystore_path.write_bytes(b"keystore content")

        keystore_config = KeystoreConfig(
            keystore_path=keystore_path,
            key_alias="my_alias",
            keystore_password="keystore_pass",
        )

        runner = DefaultApkSignerRunner()

        with pytest.raises(ApkSignerError) as exc_info:
            runner.sign(apk_path, keystore_config)

        assert "not found" in str(exc_info.value).lower() or "APK" in str(exc_info.value)

    def test_sign_keystore_not_found(self, tmp_path: Path) -> None:
        """異常系: キーストアファイルが存在しない場合にApkSignerErrorが発生"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        apk_path = tmp_path / "test.apk"
        apk_path.write_bytes(b"apk content")

        keystore_path = tmp_path / "non_existent.jks"

        keystore_config = KeystoreConfig(
            keystore_path=keystore_path,
            key_alias="my_alias",
            keystore_password="keystore_pass",
        )

        runner = DefaultApkSignerRunner()

        with pytest.raises(ApkSignerError) as exc_info:
            runner.sign(apk_path, keystore_config)

        error_msg = str(exc_info.value).lower()
        assert "not found" in error_msg or "keystore" in error_msg

    def test_sign_invalid_password(self, tmp_path: Path) -> None:
        """異常系: パスワードが不正な場合にApkSignerErrorが発生"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        apk_path = tmp_path / "test.apk"
        apk_path.write_bytes(b"apk content")

        keystore_path = tmp_path / "keystore.jks"
        keystore_path.write_bytes(b"keystore content")

        keystore_config = KeystoreConfig(
            keystore_path=keystore_path,
            key_alias="my_alias",
            keystore_password="wrong_password",
        )

        runner = DefaultApkSignerRunner()

        with patch.object(runner, "find_apksigner") as mock_find:
            mock_find.return_value = Path("/android/sdk/build-tools/34.0.0/apksigner")

            with patch("subprocess.run") as mock_run:
                mock_result = MagicMock()
                mock_result.returncode = 1
                mock_result.stdout = ""
                mock_result.stderr = "Failed to load signer: keystore password was incorrect"
                mock_run.return_value = mock_result

                with pytest.raises(ApkSignerError) as exc_info:
                    runner.sign(apk_path, keystore_config)

                error_msg = str(exc_info.value).lower()
                assert "failed" in error_msg or "password" in error_msg

    def test_sign_apksigner_not_found(self, tmp_path: Path) -> None:
        """異常系: apksignerコマンドが見つからない場合にApkSignerErrorが発生"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        apk_path = tmp_path / "test.apk"
        apk_path.write_bytes(b"apk content")

        keystore_path = tmp_path / "keystore.jks"
        keystore_path.write_bytes(b"keystore content")

        keystore_config = KeystoreConfig(
            keystore_path=keystore_path,
            key_alias="my_alias",
            keystore_password="keystore_pass",
        )

        runner = DefaultApkSignerRunner()

        with patch.object(runner, "find_apksigner") as mock_find:
            mock_find.return_value = None

            with pytest.raises(ApkSignerError) as exc_info:
                runner.sign(apk_path, keystore_config)

            assert "apksigner" in str(exc_info.value).lower()

    def test_sign_command_failure(self, tmp_path: Path) -> None:
        """異常系: apksignerコマンドが失敗した場合にApkSignerErrorが発生"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        apk_path = tmp_path / "test.apk"
        apk_path.write_bytes(b"apk content")

        keystore_path = tmp_path / "keystore.jks"
        keystore_path.write_bytes(b"keystore content")

        keystore_config = KeystoreConfig(
            keystore_path=keystore_path,
            key_alias="my_alias",
            keystore_password="keystore_pass",
        )

        runner = DefaultApkSignerRunner()

        with patch.object(runner, "find_apksigner") as mock_find:
            mock_find.return_value = Path("/android/sdk/build-tools/34.0.0/apksigner")

            with patch("subprocess.run") as mock_run:
                mock_result = MagicMock()
                mock_result.returncode = 1
                mock_result.stdout = ""
                mock_result.stderr = "apksigner error: unknown error"
                mock_run.return_value = mock_result

                with pytest.raises(ApkSignerError) as exc_info:
                    runner.sign(apk_path, keystore_config)

                assert "failed" in str(exc_info.value).lower()


class TestDefaultApkSignerRunnerVerify:
    """DefaultApkSignerRunner.verifyメソッドのテスト"""

    @pytest.mark.parametrize(
        "returncode,expected_valid",
        [
            pytest.param(0, True, id="正常系: 署名検証成功"),
            pytest.param(1, False, id="正常系: 署名検証失敗（無効な署名）"),
        ],
    )
    def test_verify(
        self,
        tmp_path: Path,
        returncode: int,
        expected_valid: bool,
    ) -> None:
        """verifyが署名の有効性を正しく判定"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        apk_path = tmp_path / "test.apk"
        apk_path.write_bytes(b"apk content")

        runner = DefaultApkSignerRunner()

        with patch.object(runner, "find_apksigner") as mock_find:
            mock_find.return_value = Path("/android/sdk/build-tools/34.0.0/apksigner")

            with patch("subprocess.run") as mock_run:
                mock_result = MagicMock()
                mock_result.returncode = returncode
                mock_result.stdout = "Verifies" if returncode == 0 else ""
                mock_result.stderr = "" if returncode == 0 else "DOES NOT VERIFY"
                mock_run.return_value = mock_result

                result = runner.verify(apk_path)

                assert result is expected_valid
                mock_run.assert_called_once()
                call_args = mock_run.call_args[0][0]
                assert "apksigner" in str(call_args[0])
                assert "verify" in call_args
                assert str(apk_path) in call_args

    def test_verify_apk_not_found(self, tmp_path: Path) -> None:
        """異常系: APKファイルが存在しない場合にApkSignerErrorが発生"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        apk_path = tmp_path / "non_existent.apk"

        runner = DefaultApkSignerRunner()

        with pytest.raises(ApkSignerError) as exc_info:
            runner.verify(apk_path)

        assert "not found" in str(exc_info.value).lower() or "APK" in str(exc_info.value)

    def test_verify_apksigner_not_found(self, tmp_path: Path) -> None:
        """異常系: apksignerコマンドが見つからない場合にApkSignerErrorが発生"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        apk_path = tmp_path / "test.apk"
        apk_path.write_bytes(b"apk content")

        runner = DefaultApkSignerRunner()

        with patch.object(runner, "find_apksigner") as mock_find:
            mock_find.return_value = None

            with pytest.raises(ApkSignerError) as exc_info:
                runner.verify(apk_path)

            assert "apksigner" in str(exc_info.value).lower()

    def test_verify_command_error(self, tmp_path: Path) -> None:
        """異常系: apksignerコマンド実行中にエラーが発生した場合にApkSignerErrorが発生"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        apk_path = tmp_path / "test.apk"
        apk_path.write_bytes(b"apk content")

        runner = DefaultApkSignerRunner()

        with patch.object(runner, "find_apksigner") as mock_find:
            mock_find.return_value = Path("/android/sdk/build-tools/34.0.0/apksigner")

            with patch("subprocess.run") as mock_run:
                mock_run.side_effect = subprocess.SubprocessError("command failed")

                with pytest.raises(ApkSignerError) as exc_info:
                    runner.verify(apk_path)

                assert "failed" in str(exc_info.value).lower()


class TestDefaultApkSignerRunnerFindApksigner:
    """DefaultApkSignerRunner.find_apksignerメソッドのテスト"""

    def test_find_apksigner_from_android_home(self, tmp_path: Path) -> None:
        """正常系: ANDROID_HOME環境変数からapksignerを検出"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        android_home = tmp_path / "android-sdk"
        build_tools = android_home / "build-tools" / "34.0.0"
        build_tools.mkdir(parents=True)
        apksigner_path = build_tools / "apksigner"
        apksigner_path.touch()

        runner = DefaultApkSignerRunner()

        with (
            patch.dict("os.environ", {"ANDROID_HOME": str(android_home)}),
            patch("shutil.which") as mock_which,
        ):
            mock_which.return_value = None

            result = runner.find_apksigner()

            assert result is not None
            assert "apksigner" in str(result)

    def test_find_apksigner_from_path(self) -> None:
        """正常系: システムPATHからapksignerを検出"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        runner = DefaultApkSignerRunner()

        import os

        env_without_android_home = {k: v for k, v in os.environ.items() if k != "ANDROID_HOME"}

        with (
            patch.dict("os.environ", env_without_android_home, clear=True),
            patch("shutil.which") as mock_which,
        ):
            mock_which.return_value = "/usr/local/bin/apksigner"

            result = runner.find_apksigner()

            assert result == Path("/usr/local/bin/apksigner")
            mock_which.assert_called_with("apksigner")

    def test_find_apksigner_not_found(self) -> None:
        """正常系: apksignerが見つからない場合にNoneを返す"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        runner = DefaultApkSignerRunner()

        with (
            patch.dict("os.environ", {}, clear=True),
            patch("shutil.which") as mock_which,
        ):
            mock_which.return_value = None

            result = runner.find_apksigner()

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
    def test_find_apksigner_selects_latest_version(
        self,
        tmp_path: Path,
        build_tool_versions: list[str],
        expected_version: str,
    ) -> None:
        """正常系: 複数のbuild-toolsバージョンがある場合に最新を選択"""
        from mnemonic.signer.apk import DefaultApkSignerRunner

        android_home = tmp_path / "android-sdk"
        for version in build_tool_versions:
            build_tools = android_home / "build-tools" / version
            build_tools.mkdir(parents=True)
            apksigner_path = build_tools / "apksigner"
            apksigner_path.touch()

        runner = DefaultApkSignerRunner()

        with (
            patch.dict("os.environ", {"ANDROID_HOME": str(android_home)}),
            patch("shutil.which") as mock_which,
        ):
            mock_which.return_value = None

            result = runner.find_apksigner()

            assert result is not None
            assert expected_version in str(result)
