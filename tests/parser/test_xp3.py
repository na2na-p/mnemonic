"""XP3アーカイブ操作のテスト

TDDのStep 2として、XP3アーカイブ展開機能の単体テストを記述する。
実装はまだ行われていないため、テストは NotImplementedError で失敗することが期待される。
"""

from pathlib import Path

import pytest

from mnemonic.parser import (
    EncryptionInfo,
    EncryptionType,
    XP3Archive,
    XP3EncryptionChecker,
    XP3EncryptionError,
)

class TestEncryptionType:
    """EncryptionType列挙型のテスト"""

    @pytest.mark.parametrize(
        "encryption_type, expected_value",
        [
            pytest.param(
                EncryptionType.NONE,
                "none",
                id="正常系: NONE型の値",
            ),
            pytest.param(
                EncryptionType.SIMPLE_XOR,
                "simple_xor",
                id="正常系: SIMPLE_XOR型の値",
            ),
            pytest.param(
                EncryptionType.CUSTOM,
                "custom",
                id="正常系: CUSTOM型の値",
            ),
            pytest.param(
                EncryptionType.UNKNOWN,
                "unknown",
                id="正常系: UNKNOWN型の値",
            ),
        ],
    )
    def test_encryption_type_values(
        self, encryption_type: EncryptionType, expected_value: str
    ) -> None:
        """EncryptionTypeの各値が正しい文字列値を持つ"""
        assert encryption_type.value == expected_value

class TestEncryptionInfo:
    """EncryptionInfoデータクラスのテスト"""

    @pytest.mark.parametrize(
        "is_encrypted, encryption_type, details",
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
                "XORキー: 0xFF",
                id="正常系: XOR暗号化・詳細あり",
            ),
            pytest.param(
                True,
                EncryptionType.CUSTOM,
                "カスタム暗号化検出",
                id="正常系: カスタム暗号化・詳細あり",
            ),
            pytest.param(
                True,
                EncryptionType.UNKNOWN,
                None,
                id="正常系: 未知の暗号化・詳細なし",
            ),
        ],
    )
    def test_encryption_info_creation(
        self,
        is_encrypted: bool,
        encryption_type: EncryptionType,
        details: str | None,
    ) -> None:
        """EncryptionInfoが正しく生成される"""
        info = EncryptionInfo(
            is_encrypted=is_encrypted,
            encryption_type=encryption_type,
            details=details,
        )

        assert info.is_encrypted == is_encrypted
        assert info.encryption_type == encryption_type
        assert info.details == details

    def test_encryption_info_default_details(self) -> None:
        """detailsのデフォルト値はNone"""
        info = EncryptionInfo(
            is_encrypted=False,
            encryption_type=EncryptionType.NONE,
        )

        assert info.details is None

    def test_encryption_info_is_immutable(self) -> None:
        """EncryptionInfoはイミュータブル"""
        info = EncryptionInfo(
            is_encrypted=False,
            encryption_type=EncryptionType.NONE,
        )

        with pytest.raises(AttributeError):
            info.is_encrypted = True  # type: ignore[misc]

class TestXP3EncryptionError:
    """XP3EncryptionError例外のテスト"""

    def test_xp3_encryption_error_creation(self) -> None:
        """XP3EncryptionErrorが正しく生成される"""
        encryption_info = EncryptionInfo(
            is_encrypted=True,
            encryption_type=EncryptionType.SIMPLE_XOR,
            details=None,
        )

        error = XP3EncryptionError(encryption_info)

        assert error.encryption_info == encryption_info
        assert "暗号化されています" in str(error)
        assert "simple_xor" in str(error)

    def test_xp3_encryption_error_with_details(self) -> None:
        """詳細情報を含むXP3EncryptionErrorが正しく生成される"""
        encryption_info = EncryptionInfo(
            is_encrypted=True,
            encryption_type=EncryptionType.CUSTOM,
            details="特殊な暗号化方式",
        )

        error = XP3EncryptionError(encryption_info)

        assert "特殊な暗号化方式" in str(error)

class TestXP3Archive:
    """XP3アーカイブ操作クラスのテスト"""

    def test_xp3archive_init_raises_not_implemented(self, tmp_path: Path) -> None:
        """XP3Archiveの初期化は未実装でNotImplementedErrorを発生"""
        archive_path = tmp_path / "test.xp3"
        archive_path.touch()

        with pytest.raises(NotImplementedError):
            XP3Archive(archive_path)

    @pytest.mark.parametrize(
        "archive_name, expected_files",
        [
            pytest.param(
                "valid.xp3",
                ["data/script.ks", "image.png"],
                id="正常系: 有効なXP3ファイルのファイル一覧取得",
            ),
            pytest.param(
                "empty.xp3",
                [],
                id="正常系: 空のXP3ファイルのファイル一覧取得",
            ),
            pytest.param(
                "nested.xp3",
                ["data/scripts/main.ks", "data/images/bg/title.png", "sound.ogg"],
                id="正常系: ネストされたディレクトリ構造",
            ),
        ],
    )
    def test_list_files(self, archive_name: str, expected_files: list[str], tmp_path: Path) -> None:
        """ファイル一覧取得テスト（実装後に動作する）

        現時点ではNotImplementedErrorが発生する。
        実装完了後、このテストは期待されるファイル一覧を返すことを検証する。
        """
        archive_path = tmp_path / archive_name
        archive_path.touch()

        with pytest.raises(NotImplementedError):
            archive = XP3Archive(archive_path)
            archive.list_files()

    def test_extract_all(self, tmp_path: Path) -> None:
        """すべてのファイル展開テスト

        XP3アーカイブ内の全ファイルを指定ディレクトリに展開する。
        """
        archive_path = tmp_path / "test.xp3"
        archive_path.touch()
        output_dir = tmp_path / "output"

        with pytest.raises(NotImplementedError):
            archive = XP3Archive(archive_path)
            archive.extract_all(output_dir)

    def test_extract_file(self, tmp_path: Path) -> None:
        """個別ファイル展開テスト

        XP3アーカイブ内の特定ファイルを指定パスに展開する。
        """
        archive_path = tmp_path / "test.xp3"
        archive_path.touch()
        output_path = tmp_path / "extracted" / "script.ks"

        with pytest.raises(NotImplementedError):
            archive = XP3Archive(archive_path)
            archive.extract_file("data/script.ks", output_path)

    def test_is_encrypted(self, tmp_path: Path) -> None:
        """暗号化判定テスト

        XP3アーカイブが暗号化されているかを判定する。
        """
        archive_path = tmp_path / "test.xp3"
        archive_path.touch()

        with pytest.raises(NotImplementedError):
            archive = XP3Archive(archive_path)
            archive.is_encrypted()

class TestXP3ArchiveFileNotFound:
    """XP3アーカイブ - ファイル不在時の異常系テスト"""

    def test_nonexistent_file_raises_error(self) -> None:
        """存在しないファイルパスでエラーが発生する

        実装時、FileNotFoundErrorまたはカスタム例外が発生することを期待。
        現時点では初期化自体がNotImplementedErrorで失敗する。
        """
        nonexistent_path = Path("/nonexistent/path/to/archive.xp3")

        with pytest.raises(NotImplementedError):
            XP3Archive(nonexistent_path)

class TestXP3ArchiveInvalidFormat:
    """XP3アーカイブ - 不正フォーマット時の異常系テスト"""

    def test_invalid_xp3_raises_error(self, tmp_path: Path) -> None:
        """不正なXP3ファイルでエラーが発生する

        XP3シグネチャを持たないファイルを読み込んだ場合、
        適切な例外が発生することを期待。
        現時点では初期化自体がNotImplementedErrorで失敗する。
        """
        invalid_file = tmp_path / "invalid.xp3"
        invalid_file.write_bytes(b"This is not a valid XP3 archive")

        with pytest.raises(NotImplementedError):
            XP3Archive(invalid_file)

    def test_truncated_xp3_raises_error(self, tmp_path: Path) -> None:
        """切り詰められたXP3ファイルでエラーが発生する

        不完全なXP3ファイルを読み込んだ場合、
        適切な例外が発生することを期待。
        """
        truncated_file = tmp_path / "truncated.xp3"
        # XP3シグネチャ部分だけ書き込む（不完全なファイル）
        truncated_file.write_bytes(b"XP3\r\n")

        with pytest.raises(NotImplementedError):
            XP3Archive(truncated_file)

class TestXP3EncryptionChecker:
    """XP3暗号化チェッカーのテスト"""

    def test_checker_initialization(self, tmp_path: Path) -> None:
        """XP3EncryptionCheckerが正しく初期化される"""
        archive_path = tmp_path / "test.xp3"
        archive_path.touch()

        checker = XP3EncryptionChecker(archive_path)

        # _archive_pathがprotectedメンバーとして保持される
        assert checker._archive_path == archive_path

    def test_check_returns_encryption_info(self, tmp_path: Path) -> None:
        """checkメソッドがEncryptionInfoを返す

        現時点では未実装のためNotImplementedErrorが発生する。
        """
        archive_path = tmp_path / "test.xp3"
        archive_path.touch()

        checker = XP3EncryptionChecker(archive_path)

        with pytest.raises(NotImplementedError):
            checker.check()

    @pytest.mark.parametrize(
        "file_content, expected_encrypted",
        [
            pytest.param(
                b"unencrypted_xp3_content",
                False,
                id="正常系: 非暗号化XP3を正しく判定",
            ),
        ],
    )
    def test_check_non_encrypted_xp3(
        self, file_content: bytes, expected_encrypted: bool, tmp_path: Path
    ) -> None:
        """非暗号化XP3を正しく判定できる

        実装後、非暗号化XP3ファイルに対してis_encrypted=Falseを返すことを検証。
        """
        archive_path = tmp_path / "test.xp3"
        archive_path.write_bytes(file_content)

        checker = XP3EncryptionChecker(archive_path)

        with pytest.raises(NotImplementedError):
            checker.check()
            # 実装後に検証: assert result.is_encrypted == expected_encrypted

    def test_raise_if_encrypted_not_implemented(self, tmp_path: Path) -> None:
        """raise_if_encryptedは未実装でNotImplementedErrorを発生"""
        archive_path = tmp_path / "test.xp3"
        archive_path.touch()

        checker = XP3EncryptionChecker(archive_path)

        with pytest.raises(NotImplementedError):
            checker.raise_if_encrypted()

    def test_check_nonexistent_file(self) -> None:
        """存在しないファイルのチェックで例外が発生する

        実装時、FileNotFoundErrorが発生することを期待。
        現時点ではNotImplementedErrorが発生する。
        """
        nonexistent_path = Path("/nonexistent/path/to/archive.xp3")

        checker = XP3EncryptionChecker(nonexistent_path)

        with pytest.raises(NotImplementedError):
            checker.check()

class TestXP3EncryptionCheckerRaiseIfEncrypted:
    """XP3暗号化チェッカー - raise_if_encryptedのテスト"""

    def test_raise_if_encrypted_does_not_raise_for_unencrypted(self, tmp_path: Path) -> None:
        """非暗号化XP3では例外が発生しない

        実装後、非暗号化XP3ファイルに対しては例外なく完了することを検証。
        現時点ではNotImplementedErrorが発生する。
        """
        archive_path = tmp_path / "unencrypted.xp3"
        archive_path.touch()

        checker = XP3EncryptionChecker(archive_path)

        with pytest.raises(NotImplementedError):
            checker.raise_if_encrypted()

    def test_raise_if_encrypted_raises_for_encrypted(self, tmp_path: Path) -> None:
        """暗号化XP3ではXP3EncryptionErrorが発生する

        実装後、暗号化XP3ファイルに対してXP3EncryptionErrorが発生することを検証。
        現時点ではNotImplementedErrorが発生する。
        """
        archive_path = tmp_path / "encrypted.xp3"
        archive_path.touch()

        checker = XP3EncryptionChecker(archive_path)

        with pytest.raises(NotImplementedError):
            checker.raise_if_encrypted()

class TestXP3OutputDirectory:
    """XP3アーカイブ展開 - 出力ディレクトリの挙動テスト"""

    def test_extract_creates_output_directory(self, tmp_path: Path) -> None:
        """存在しない出力ディレクトリが自動作成される

        実装後、extract_allが出力ディレクトリを自動作成することを検証。
        現時点ではNotImplementedErrorが発生する。
        """
        archive_path = tmp_path / "test.xp3"
        archive_path.touch()
        output_dir = tmp_path / "nonexistent" / "nested" / "output"

        with pytest.raises(NotImplementedError):
            archive = XP3Archive(archive_path)
            archive.extract_all(output_dir)

    def test_extract_file_creates_parent_directory(self, tmp_path: Path) -> None:
        """個別ファイル展開時に親ディレクトリが自動作成される

        実装後、extract_fileが出力先の親ディレクトリを自動作成することを検証。
        現時点ではNotImplementedErrorが発生する。
        """
        archive_path = tmp_path / "test.xp3"
        archive_path.touch()
        output_path = tmp_path / "nonexistent" / "dir" / "script.ks"

        with pytest.raises(NotImplementedError):
            archive = XP3Archive(archive_path)
            archive.extract_file("script.ks", output_path)
