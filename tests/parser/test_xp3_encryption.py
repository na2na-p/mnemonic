"""XP3暗号化チェック機能のテスト

XP3EncryptionChecker、EncryptionInfo、XP3EncryptionErrorの
単体テストを提供する。
"""

from pathlib import Path

import pytest

from mnemonic.parser import (
    EncryptionInfo,
    EncryptionType,
    XP3EncryptionChecker,
    XP3EncryptionError,
)

class TestEncryptionType:
    """EncryptionType列挙型のテスト"""

    @pytest.mark.parametrize(
        "encryption_type,expected_value",
        [
            pytest.param(
                EncryptionType.NONE,
                "none",
                id="正常系: 暗号化なしのvalue確認",
            ),
            pytest.param(
                EncryptionType.SIMPLE_XOR,
                "simple_xor",
                id="正常系: XOR暗号化のvalue確認",
            ),
            pytest.param(
                EncryptionType.CUSTOM,
                "custom",
                id="正常系: カスタム暗号化のvalue確認",
            ),
            pytest.param(
                EncryptionType.UNKNOWN,
                "unknown",
                id="正常系: 未知の暗号化のvalue確認",
            ),
        ],
    )
    def test_encryption_type_values(
        self,
        encryption_type: EncryptionType,
        expected_value: str,
    ) -> None:
        """EncryptionTypeの各値が正しいvalue文字列を持つことを確認"""
        assert encryption_type.value == expected_value

class TestEncryptionInfo:
    """EncryptionInfoデータクラスのテスト"""

    @pytest.mark.parametrize(
        "is_encrypted,encryption_type,details",
        [
            pytest.param(
                False,
                EncryptionType.NONE,
                None,
                id="正常系: 非暗号化・詳細なし",
            ),
            pytest.param(
                True,
                EncryptionType.SIMPLE_XOR,
                None,
                id="正常系: XOR暗号化・詳細なし",
            ),
            pytest.param(
                True,
                EncryptionType.CUSTOM,
                "ゲーム固有の暗号化方式",
                id="正常系: カスタム暗号化・詳細あり",
            ),
            pytest.param(
                True,
                EncryptionType.UNKNOWN,
                "不明なパターンを検出",
                id="正常系: 未知の暗号化・詳細あり",
            ),
        ],
    )
    def test_encryption_info_creation(
        self,
        is_encrypted: bool,
        encryption_type: EncryptionType,
        details: str | None,
    ) -> None:
        """EncryptionInfoが正しく生成されることを確認"""
        info = EncryptionInfo(
            is_encrypted=is_encrypted,
            encryption_type=encryption_type,
            details=details,
        )

        assert info.is_encrypted == is_encrypted
        assert info.encryption_type == encryption_type
        assert info.details == details

    def test_encryption_info_is_immutable(self) -> None:
        """EncryptionInfoはイミュータブル（frozen=True）であることを確認"""
        info = EncryptionInfo(
            is_encrypted=False,
            encryption_type=EncryptionType.NONE,
            details=None,
        )

        with pytest.raises(AttributeError):
            info.is_encrypted = True  # type: ignore[misc]

    def test_encryption_info_default_details_is_none(self) -> None:
        """EncryptionInfoのdetailsはデフォルトでNoneであることを確認"""
        info = EncryptionInfo(
            is_encrypted=False,
            encryption_type=EncryptionType.NONE,
        )

        assert info.details is None

class TestXP3EncryptionError:
    """XP3EncryptionError例外クラスのテスト"""

    @pytest.mark.parametrize(
        "encryption_type,details,expected_message_contains",
        [
            pytest.param(
                EncryptionType.SIMPLE_XOR,
                None,
                "simple_xor",
                id="正常系: XOR暗号化のエラーメッセージに暗号化タイプが含まれる",
            ),
            pytest.param(
                EncryptionType.CUSTOM,
                "ゲーム固有の暗号化",
                "custom",
                id="正常系: カスタム暗号化のエラーメッセージに暗号化タイプが含まれる",
            ),
            pytest.param(
                EncryptionType.UNKNOWN,
                "不明なパターン",
                "unknown",
                id="正常系: 未知の暗号化のエラーメッセージに暗号化タイプが含まれる",
            ),
        ],
    )
    def test_error_message_contains_encryption_type(
        self,
        encryption_type: EncryptionType,
        details: str | None,
        expected_message_contains: str,
    ) -> None:
        """エラーメッセージに暗号化タイプが含まれることを確認"""
        info = EncryptionInfo(
            is_encrypted=True,
            encryption_type=encryption_type,
            details=details,
        )
        error = XP3EncryptionError(info)

        assert expected_message_contains in str(error)

    def test_error_message_contains_details_when_provided(self) -> None:
        """詳細が提供された場合、エラーメッセージに詳細が含まれることを確認"""
        details = "特定のゲームで使用される暗号化"
        info = EncryptionInfo(
            is_encrypted=True,
            encryption_type=EncryptionType.CUSTOM,
            details=details,
        )
        error = XP3EncryptionError(info)

        assert details in str(error)

    def test_error_has_encryption_info_attribute(self) -> None:
        """XP3EncryptionErrorがencryption_info属性を持つことを確認"""
        info = EncryptionInfo(
            is_encrypted=True,
            encryption_type=EncryptionType.SIMPLE_XOR,
            details=None,
        )
        error = XP3EncryptionError(info)

        assert error.encryption_info is info
        assert error.encryption_info.encryption_type == EncryptionType.SIMPLE_XOR

    def test_error_is_exception_subclass(self) -> None:
        """XP3EncryptionErrorがExceptionのサブクラスであることを確認"""
        info = EncryptionInfo(
            is_encrypted=True,
            encryption_type=EncryptionType.UNKNOWN,
            details=None,
        )
        error = XP3EncryptionError(info)

        assert isinstance(error, Exception)

class TestXP3EncryptionChecker:
    """XP3EncryptionCheckerクラスのテスト"""

    def test_checker_initialization(self, tmp_path: Path) -> None:
        """XP3EncryptionCheckerが正しく初期化されることを確認"""
        # tmp_pathにダミーファイルを作成（初期化時はファイル存在チェックしない想定）
        archive_path = tmp_path / "test.xp3"
        archive_path.touch()

        checker = XP3EncryptionChecker(archive_path)

        # プライベート属性へのアクセスは最小限にするが、初期化確認のため
        assert checker._archive_path == archive_path

    # === 正常系テスト ===

    def test_check_returns_not_encrypted_for_unencrypted_xp3(
        self,
        tmp_path: Path,
    ) -> None:
        """非暗号化XP3を正しく検出できることを確認（is_encrypted=False）

        注意: 現在の実装はNotImplementedErrorを発生させる。
        実装完了後、このテストはEncryptionInfoを返すようになる。
        """
        # XP3マジックナンバー: "XP3\r\n\x1a\x8b\x67\x01" (11バイト)
        xp3_magic = b"XP3\r\n\x1a\x8b\x67\x01"
        # 非暗号化XP3のダミーデータ
        unencrypted_xp3_data = xp3_magic + b"\x00" * 100

        archive_path = tmp_path / "unencrypted.xp3"
        archive_path.write_bytes(unencrypted_xp3_data)

        checker = XP3EncryptionChecker(archive_path)

        # 実装前なのでNotImplementedErrorが発生することを確認
        with pytest.raises(NotImplementedError):
            checker.check()

    def test_check_returns_encrypted_for_simple_xor_xp3(
        self,
        tmp_path: Path,
    ) -> None:
        """XOR暗号化XP3を検出できることを確認（is_encrypted=True）

        注意: 現在の実装はNotImplementedErrorを発生させる。
        実装完了後、このテストはEncryptionInfoを返すようになる。
        """
        # XP3マジックナンバー + 暗号化フラグを示すダミーデータ
        xp3_magic = b"XP3\r\n\x1a\x8b\x67\x01"
        # 暗号化を示すパターン（実際の実装に合わせて調整が必要）
        encrypted_xp3_data = xp3_magic + b"\xff" * 100

        archive_path = tmp_path / "encrypted_xor.xp3"
        archive_path.write_bytes(encrypted_xp3_data)

        checker = XP3EncryptionChecker(archive_path)

        # 実装前なのでNotImplementedErrorが発生することを確認
        with pytest.raises(NotImplementedError):
            checker.check()

    def test_check_identifies_custom_encryption(
        self,
        tmp_path: Path,
    ) -> None:
        """カスタム暗号化を識別できることを確認

        注意: 現在の実装はNotImplementedErrorを発生させる。
        """
        xp3_magic = b"XP3\r\n\x1a\x8b\x67\x01"
        # カスタム暗号化を示すパターン（実際の実装に合わせて調整が必要）
        custom_encrypted_data = xp3_magic + b"\xab\xcd" * 50

        archive_path = tmp_path / "encrypted_custom.xp3"
        archive_path.write_bytes(custom_encrypted_data)

        checker = XP3EncryptionChecker(archive_path)

        # 実装前なのでNotImplementedErrorが発生することを確認
        with pytest.raises(NotImplementedError):
            checker.check()

    def test_raise_if_encrypted_does_not_raise_for_unencrypted(
        self,
        tmp_path: Path,
    ) -> None:
        """非暗号化XP3でraise_if_encrypted()が例外を発生させないことを確認

        注意: 現在の実装はNotImplementedErrorを発生させる。
        """
        xp3_magic = b"XP3\r\n\x1a\x8b\x67\x01"
        unencrypted_xp3_data = xp3_magic + b"\x00" * 100

        archive_path = tmp_path / "unencrypted.xp3"
        archive_path.write_bytes(unencrypted_xp3_data)

        checker = XP3EncryptionChecker(archive_path)

        # 実装前なのでNotImplementedErrorが発生することを確認
        with pytest.raises(NotImplementedError):
            checker.raise_if_encrypted()

    # === 異常系テスト ===

    def test_check_raises_file_not_found_for_nonexistent_file(
        self,
        tmp_path: Path,
    ) -> None:
        """存在しないファイルパスでFileNotFoundErrorが発生することを確認

        注意: 現在の実装はNotImplementedErrorを発生させる。
        実装完了後、このテストはFileNotFoundErrorを発生させるようになる。
        """
        nonexistent_path = tmp_path / "nonexistent.xp3"

        checker = XP3EncryptionChecker(nonexistent_path)

        # 実装前なのでNotImplementedErrorが発生することを確認
        # 実装後はFileNotFoundErrorに変更する
        with pytest.raises(NotImplementedError):
            checker.check()

    def test_check_raises_error_for_invalid_xp3_file(
        self,
        tmp_path: Path,
    ) -> None:
        """不正なXP3ファイルでエラーが発生することを確認

        注意: 現在の実装はNotImplementedErrorを発生させる。
        """
        # XP3マジックナンバーを含まない不正なデータ
        invalid_data = b"NOT_AN_XP3_FILE" + b"\x00" * 100

        archive_path = tmp_path / "invalid.xp3"
        archive_path.write_bytes(invalid_data)

        checker = XP3EncryptionChecker(archive_path)

        # 実装前なのでNotImplementedErrorが発生することを確認
        with pytest.raises(NotImplementedError):
            checker.check()

    def test_raise_if_encrypted_raises_error_for_encrypted_xp3(
        self,
        tmp_path: Path,
    ) -> None:
        """暗号化XP3でraise_if_encrypted()がXP3EncryptionErrorを発生させることを確認

        注意: 現在の実装はNotImplementedErrorを発生させる。
        実装完了後、このテストはXP3EncryptionErrorを発生させるようになる。
        """
        xp3_magic = b"XP3\r\n\x1a\x8b\x67\x01"
        encrypted_xp3_data = xp3_magic + b"\xff" * 100

        archive_path = tmp_path / "encrypted.xp3"
        archive_path.write_bytes(encrypted_xp3_data)

        checker = XP3EncryptionChecker(archive_path)

        # 実装前なのでNotImplementedErrorが発生することを確認
        # 実装後はXP3EncryptionErrorに変更する
        with pytest.raises(NotImplementedError):
            checker.raise_if_encrypted()

    def test_raise_if_encrypted_error_contains_encryption_type(
        self,
        tmp_path: Path,
    ) -> None:
        """XP3EncryptionErrorのメッセージに暗号化タイプが含まれることを確認

        このテストは実装完了後に有効化する。
        現在は実装前のためNotImplementedErrorが発生する。
        """
        xp3_magic = b"XP3\r\n\x1a\x8b\x67\x01"
        encrypted_xp3_data = xp3_magic + b"\xff" * 100

        archive_path = tmp_path / "encrypted_with_type.xp3"
        archive_path.write_bytes(encrypted_xp3_data)

        checker = XP3EncryptionChecker(archive_path)

        # 実装前なのでNotImplementedErrorが発生することを確認
        # 実装後は以下のようにXP3EncryptionErrorをチェックする:
        # with pytest.raises(XP3EncryptionError) as exc_info:
        #     checker.raise_if_encrypted()
        # assert exc_info.value.encryption_info.encryption_type in [
        #     EncryptionType.SIMPLE_XOR,
        #     EncryptionType.CUSTOM,
        #     EncryptionType.UNKNOWN,
        # ]
        with pytest.raises(NotImplementedError):
            checker.raise_if_encrypted()

    def test_raise_if_encrypted_raises_file_not_found_for_nonexistent_file(
        self,
        tmp_path: Path,
    ) -> None:
        """存在しないファイルでraise_if_encrypted()がFileNotFoundErrorを発生させることを確認

        注意: 現在の実装はNotImplementedErrorを発生させる。
        """
        nonexistent_path = tmp_path / "nonexistent.xp3"

        checker = XP3EncryptionChecker(nonexistent_path)

        # 実装前なのでNotImplementedErrorが発生することを確認
        # 実装後はFileNotFoundErrorに変更する
        with pytest.raises(NotImplementedError):
            checker.raise_if_encrypted()

class TestXP3EncryptionCheckerIntegration:
    """XP3EncryptionCheckerの統合テスト

    実際のXP3ファイル構造をシミュレートしたテスト。
    実装完了後に有効化する。
    """

    @pytest.mark.parametrize(
        "encryption_type",
        [
            pytest.param(EncryptionType.NONE, id="正常系: 非暗号化XP3"),
            pytest.param(EncryptionType.SIMPLE_XOR, id="正常系: XOR暗号化XP3"),
            pytest.param(EncryptionType.CUSTOM, id="正常系: カスタム暗号化XP3"),
            pytest.param(EncryptionType.UNKNOWN, id="正常系: 未知の暗号化XP3"),
        ],
    )
    def test_encryption_type_detection_workflow(
        self,
        encryption_type: EncryptionType,
        tmp_path: Path,
    ) -> None:
        """暗号化タイプの検出ワークフロー全体のテスト

        注意: このテストは実装完了後に具体的なアサーションを追加する。
        現在は実装前のためNotImplementedErrorが発生することを確認。
        """
        xp3_magic = b"XP3\r\n\x1a\x8b\x67\x01"
        test_data = xp3_magic + b"\x00" * 100

        archive_path = tmp_path / f"test_{encryption_type.value}.xp3"
        archive_path.write_bytes(test_data)

        checker = XP3EncryptionChecker(archive_path)

        # 実装前なのでNotImplementedErrorが発生することを確認
        with pytest.raises(NotImplementedError):
            checker.check()
