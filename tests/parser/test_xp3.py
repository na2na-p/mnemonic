"""XP3アーカイブ操作のテスト

TDD Step 3: XP3Archive, XP3EncryptionChecker の実装テスト。
実装後のため、実際の動作を検証する。
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

    @pytest.fixture
    def valid_xp3_path(self, tmp_path: Path) -> Path:
        """有効なXP3ファイルパスを返すフィクスチャ"""
        valid_file = tmp_path / "valid.xp3"
        valid_file.write_bytes(b"XP3\x0d\x0a\x1a\x0a\x00\x00\x00")
        return valid_file

    @pytest.fixture
    def invalid_xp3_path(self, tmp_path: Path) -> Path:
        """不正なXP3ファイルパスを返すフィクスチャ"""
        invalid_file = tmp_path / "invalid.xp3"
        invalid_file.write_bytes(b"NOT_XP3_FILE")
        return invalid_file

    @pytest.fixture
    def nonexistent_path(self, tmp_path: Path) -> Path:
        """存在しないファイルパスを返すフィクスチャ"""
        return tmp_path / "nonexistent.xp3"

    def test_open_valid_xp3_file(self, valid_xp3_path: Path) -> None:
        """正常系: 有効なXP3ファイルを開ける"""
        archive = XP3Archive(valid_xp3_path)
        assert archive is not None

    def test_open_nonexistent_file_raises_error(self, nonexistent_path: Path) -> None:
        """異常系: 存在しないファイルパスでエラー"""
        with pytest.raises(FileNotFoundError):
            XP3Archive(nonexistent_path)

    def test_open_invalid_xp3_file_raises_error(self, invalid_xp3_path: Path) -> None:
        """異常系: 不正なXP3ファイルでエラー"""
        with pytest.raises(ValueError):
            XP3Archive(invalid_xp3_path)

    def test_list_files_returns_file_list(self, valid_xp3_path: Path) -> None:
        """正常系: ファイル一覧を取得できる"""
        archive = XP3Archive(valid_xp3_path)
        files = archive.list_files()
        assert isinstance(files, list)

    def test_extract_all_creates_output_dir(self, valid_xp3_path: Path, tmp_path: Path) -> None:
        """正常系: すべてのファイルを展開できる（空のアーカイブの場合）"""
        output_dir = tmp_path / "output"

        archive = XP3Archive(valid_xp3_path)
        archive.extract_all(output_dir)

        assert output_dir.exists()

    def test_extract_file_raises_error_for_nonexistent_file(
        self, valid_xp3_path: Path, tmp_path: Path
    ) -> None:
        """異常系: 存在しないファイルの展開でエラー"""
        output_path = tmp_path / "extracted_file.txt"

        archive = XP3Archive(valid_xp3_path)
        with pytest.raises(FileNotFoundError):
            archive.extract_file("data/script.ks", output_path)

    def test_is_encrypted_returns_bool(self, valid_xp3_path: Path) -> None:
        """正常系: 暗号化判定がbool値を返す"""
        archive = XP3Archive(valid_xp3_path)
        result = archive.is_encrypted()
        assert isinstance(result, bool)
        assert result is False  # テスト用ファイルは暗号化されていない

class TestXP3EncryptionChecker:
    """XP3暗号化チェッカーのテスト"""

    @pytest.fixture
    def valid_xp3_path(self, tmp_path: Path) -> Path:
        """有効なXP3ファイルパスを返すフィクスチャ"""
        valid_file = tmp_path / "valid.xp3"
        valid_file.write_bytes(b"XP3\x0d\x0a\x1a\x0a\x00\x00\x00")
        return valid_file

    @pytest.fixture
    def nonexistent_path(self, tmp_path: Path) -> Path:
        """存在しないファイルパスを返すフィクスチャ"""
        return tmp_path / "nonexistent.xp3"

    def test_checker_initialization(self, tmp_path: Path) -> None:
        """XP3EncryptionCheckerが正しく初期化される"""
        archive_path = tmp_path / "test.xp3"
        archive_path.touch()

        checker = XP3EncryptionChecker(archive_path)
        assert checker is not None

    def test_check_returns_encryption_info(self, valid_xp3_path: Path) -> None:
        """正常系: チェック結果がEncryptionInfoを返す"""
        checker = XP3EncryptionChecker(valid_xp3_path)
        result = checker.check()
        assert isinstance(result, EncryptionInfo)

    def test_check_non_encrypted_xp3(self, valid_xp3_path: Path) -> None:
        """正常系: 非暗号化XP3を正しく判定できる"""
        checker = XP3EncryptionChecker(valid_xp3_path)
        result = checker.check()

        assert result.is_encrypted is False
        assert result.encryption_type == EncryptionType.NONE

    def test_check_nonexistent_file_raises_error(self, nonexistent_path: Path) -> None:
        """異常系: 存在しないファイルでエラー"""
        checker = XP3EncryptionChecker(nonexistent_path)

        with pytest.raises(FileNotFoundError):
            checker.check()

    def test_raise_if_encrypted_does_not_raise_for_non_encrypted(
        self, valid_xp3_path: Path
    ) -> None:
        """正常系: 非暗号化XP3では例外を発生させない"""
        checker = XP3EncryptionChecker(valid_xp3_path)
        # 例外が発生しないことを確認
        checker.raise_if_encrypted()

    def test_raise_if_encrypted_raises_for_encrypted(self, tmp_path: Path) -> None:
        """異常系: 暗号化XP3で例外を発生させる"""
        # 暗号化されたXP3のシミュレーション用
        # 実際の暗号化XP3ファイルがないため、このテストはスキップ
        # 将来的に実際の暗号化XP3ファイルでテストする
        pytest.skip("暗号化されたXP3ファイルのテストは統合テストで実施")

class TestXP3ArchiveIntegration:
    """XP3アーカイブの統合テスト

    実際のXP3ファイル構造をシミュレートしてテストする。
    """

    def test_list_files_returns_empty_for_minimal_xp3(self, tmp_path: Path) -> None:
        """正常系: 最小限のXP3ファイルで空のファイル一覧を返す"""
        xp3_file = tmp_path / "test.xp3"
        xp3_file.write_bytes(b"XP3\x0d\x0a\x1a\x0a\x00\x00\x00")

        archive = XP3Archive(xp3_file)
        files = archive.list_files()
        assert len(files) == 0

    def test_extract_file_raises_for_nonexistent_entry(self, tmp_path: Path) -> None:
        """異常系: 存在しないエントリの展開でエラー"""
        xp3_file = tmp_path / "test.xp3"
        xp3_file.write_bytes(b"XP3\x0d\x0a\x1a\x0a\x00\x00\x00")
        output_path = tmp_path / "output" / "script.ks"

        archive = XP3Archive(xp3_file)
        with pytest.raises(FileNotFoundError):
            archive.extract_file("data/script.ks", output_path)

    @pytest.mark.parametrize(
        "filename",
        [
            pytest.param("data/script.ks", id="正常系: スクリプトファイル"),
            pytest.param("image/bg/title.png", id="正常系: 画像ファイル"),
            pytest.param("sound/bgm/main.ogg", id="正常系: 音声ファイル"),
        ],
    )
    def test_extract_file_raises_for_missing_file(self, tmp_path: Path, filename: str) -> None:
        """異常系: 存在しないファイルの展開でエラー"""
        xp3_file = tmp_path / "test.xp3"
        xp3_file.write_bytes(b"XP3\x0d\x0a\x1a\x0a\x00\x00\x00")
        output_path = tmp_path / "output" / Path(filename).name

        archive = XP3Archive(xp3_file)
        with pytest.raises(FileNotFoundError):
            archive.extract_file(filename, output_path)
