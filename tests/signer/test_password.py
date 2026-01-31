"""PasswordProviderのテスト

このモジュールはPasswordProviderインターフェースの実装に対するテストを提供します。
"""

from unittest.mock import patch

import pytest

from mnemonic.signer.apk import PasswordError


class TestPasswordErrorClass:
    """PasswordError例外クラスのテスト"""

    def test_password_error_inheritance(self) -> None:
        """正常系: PasswordErrorがExceptionを継承している"""
        error = PasswordError("password error")
        assert isinstance(error, Exception)
        assert str(error) == "password error"

    def test_password_error_with_message(self) -> None:
        """正常系: PasswordErrorにメッセージを設定できる"""
        error = PasswordError("Environment variable not set: MNEMONIC_KEYSTORE_PASS")
        assert "not set" in str(error)


class TestPasswordProviderProtocol:
    """PasswordProviderプロトコルのテスト

    注意: このテストはDefaultPasswordProvider実装に対して実行されます。
    実装がまだない場合、テストはインポートエラーで失敗します。
    """

    pass


class TestDefaultPasswordProviderGetPasswordFromEnv:
    """DefaultPasswordProvider.get_password_from_envメソッドのテスト"""

    @pytest.mark.parametrize(
        "env_var,env_value,expected",
        [
            pytest.param(
                "MNEMONIC_KEYSTORE_PASS",
                "my_secret_password",
                "my_secret_password",
                id="正常系: 環境変数からパスワードを取得",
            ),
            pytest.param(
                "CUSTOM_PASSWORD_VAR",
                "custom_password",
                "custom_password",
                id="正常系: カスタム環境変数名からパスワードを取得",
            ),
        ],
    )
    def test_get_password_from_env_success(
        self,
        env_var: str,
        env_value: str,
        expected: str,
    ) -> None:
        """環境変数が設定されている場合にパスワードを返す"""
        from mnemonic.signer.apk import DefaultPasswordProvider

        provider = DefaultPasswordProvider()

        with patch.dict("os.environ", {env_var: env_value}):
            result = provider.get_password_from_env(env_var)

            assert result == expected

    def test_get_password_from_env_not_set(self) -> None:
        """異常系: 環境変数が未設定の場合にNoneを返す"""
        from mnemonic.signer.apk import DefaultPasswordProvider

        provider = DefaultPasswordProvider()

        with patch.dict("os.environ", {}, clear=True):
            result = provider.get_password_from_env("MNEMONIC_KEYSTORE_PASS")

            assert result is None

    def test_get_password_from_env_empty_value(self) -> None:
        """異常系: 環境変数が空文字列の場合にNoneを返す"""
        from mnemonic.signer.apk import DefaultPasswordProvider

        provider = DefaultPasswordProvider()

        with patch.dict("os.environ", {"MNEMONIC_KEYSTORE_PASS": ""}):
            result = provider.get_password_from_env("MNEMONIC_KEYSTORE_PASS")

            assert result is None

    def test_get_password_from_env_default_env_var(self) -> None:
        """正常系: デフォルトの環境変数名を使用"""
        from mnemonic.signer.apk import DefaultPasswordProvider

        provider = DefaultPasswordProvider()

        with patch.dict("os.environ", {"MNEMONIC_KEYSTORE_PASS": "default_var_password"}):
            result = provider.get_password_from_env()

            assert result == "default_var_password"


class TestDefaultPasswordProviderGetPassword:
    """DefaultPasswordProvider.get_passwordメソッドのテスト"""

    def test_get_password_success(self) -> None:
        """正常系: 対話的入力でパスワードを取得"""
        from mnemonic.signer.apk import DefaultPasswordProvider

        provider = DefaultPasswordProvider()

        with patch("getpass.getpass") as mock_getpass:
            mock_getpass.return_value = "interactive_password"

            result = provider.get_password()

            assert result == "interactive_password"
            mock_getpass.assert_called_once_with("Enter keystore password: ")

    def test_get_password_with_custom_prompt(self) -> None:
        """正常系: カスタムプロンプトで対話的入力"""
        from mnemonic.signer.apk import DefaultPasswordProvider

        provider = DefaultPasswordProvider()

        with patch("getpass.getpass") as mock_getpass:
            mock_getpass.return_value = "custom_prompt_password"

            result = provider.get_password("Custom prompt: ")

            assert result == "custom_prompt_password"
            mock_getpass.assert_called_once_with("Custom prompt: ")

    def test_get_password_empty_input(self) -> None:
        """異常系: 空入力の場合にPasswordErrorが発生"""
        from mnemonic.signer.apk import DefaultPasswordProvider

        provider = DefaultPasswordProvider()

        with patch("getpass.getpass") as mock_getpass:
            mock_getpass.return_value = ""

            with pytest.raises(PasswordError) as exc_info:
                provider.get_password()

            error_msg = str(exc_info.value).lower()
            assert "empty" in error_msg or "password" in error_msg

    def test_get_password_keyboard_interrupt(self) -> None:
        """異常系: ユーザーがキャンセルした場合にPasswordErrorが発生"""
        from mnemonic.signer.apk import DefaultPasswordProvider

        provider = DefaultPasswordProvider()

        with patch("getpass.getpass") as mock_getpass:
            mock_getpass.side_effect = KeyboardInterrupt()

            with pytest.raises(PasswordError) as exc_info:
                provider.get_password()

            error_msg = str(exc_info.value).lower()
            assert "cancel" in error_msg or "interrupt" in error_msg

    def test_get_password_eof_error(self) -> None:
        """異常系: EOFの場合にPasswordErrorが発生"""
        from mnemonic.signer.apk import DefaultPasswordProvider

        provider = DefaultPasswordProvider()

        with patch("getpass.getpass") as mock_getpass:
            mock_getpass.side_effect = EOFError()

            with pytest.raises(PasswordError) as exc_info:
                provider.get_password()

            error_msg = str(exc_info.value).lower()
            assert "eof" in error_msg or "input" in error_msg or "failed" in error_msg


class TestDefaultPasswordProviderPriority:
    """DefaultPasswordProviderの優先順位テスト

    環境変数が優先され、設定されていない場合は対話式にフォールバックする動作を検証します。
    """

    def test_env_var_takes_priority_when_set(self) -> None:
        """正常系: 環境変数が設定されている場合は環境変数を優先"""
        from mnemonic.signer.apk import DefaultPasswordProvider

        provider = DefaultPasswordProvider()

        with (
            patch.dict("os.environ", {"MNEMONIC_KEYSTORE_PASS": "env_password"}),
            patch("getpass.getpass") as mock_getpass,
        ):
            result = provider.get_password_from_env()

            assert result == "env_password"
            mock_getpass.assert_not_called()

    def test_fallback_to_interactive_when_env_not_set(self) -> None:
        """正常系: 環境変数が未設定の場合は対話式にフォールバック"""
        from mnemonic.signer.apk import DefaultPasswordProvider

        provider = DefaultPasswordProvider()

        with (
            patch.dict("os.environ", {}, clear=True),
            patch("getpass.getpass") as mock_getpass,
        ):
            mock_getpass.return_value = "interactive_fallback"

            env_result = provider.get_password_from_env()

            assert env_result is None

            interactive_result = provider.get_password()

            assert interactive_result == "interactive_fallback"
            mock_getpass.assert_called_once()

    def test_fallback_to_interactive_when_env_empty(self) -> None:
        """正常系: 環境変数が空の場合は対話式にフォールバック"""
        from mnemonic.signer.apk import DefaultPasswordProvider

        provider = DefaultPasswordProvider()

        with (
            patch.dict("os.environ", {"MNEMONIC_KEYSTORE_PASS": ""}),
            patch("getpass.getpass") as mock_getpass,
        ):
            mock_getpass.return_value = "fallback_password"

            env_result = provider.get_password_from_env()

            assert env_result is None

            interactive_result = provider.get_password()

            assert interactive_result == "fallback_password"
