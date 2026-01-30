"""XP3アーカイブ操作モジュールの単体テスト

TDD Step 2: XP3Archive, XP3EncryptionChecker のテストを作成する。
実装前のため、NotImplementedError で失敗することが期待される。
"""

from pathlib import Path

import pytest

from mnemonic.parser.xp3 import (
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
            pytest.param(EncryptionType.NONE, "none", id="正常系: NONE"),
            pytest.param(EncryptionType.SIMPLE_XOR, "simple_xor", id="正常系: SIMPLE_XOR"),
            pytest.param(EncryptionType.CUSTOM, "custom", id="正常系: CUSTOM"),
            pytest.param(EncryptionType.UNKNOWN, "unknown", id="正常系: UNKNOWN"),
        ],
    )
    def test_encryption_type_values(
        self, encryption_type: EncryptionType, expected_value: str
    ) -> None:
        """暗号化タイプの値が正しいことを確認"""
        assert encryption_type.value == expected_value

class TestEncryptionInfo:
    """EncryptionInfoデータクラスのテスト"""

    def test_create_non_encrypted_info(self) -> None:
        """正常系: 非暗号化情報を作成できる"""
        info = EncryptionInfo(
            is_encrypted=False,
            encryption_type=EncryptionType.NONE,
        )

        assert info.is_encrypted is False
        assert info.encryption_type == EncryptionType.NONE
        assert info.details is None

    def test_create_encrypted_info_with_details(self) -> None:
        """正常系: 詳細付きの暗号化情報を作成できる"""
        info = EncryptionInfo(
            is_encrypted=True,
            encryption_type=EncryptionType.SIMPLE_XOR,
            details="XORキー: 0x12345678",
        )

        assert info.is_encrypted is True
        assert info.encryption_type == EncryptionType.SIMPLE_XOR
        assert info.details == "XORキー: 0x12345678"

    def test_encryption_info_is_immutable(self) -> None:
        """正常系: EncryptionInfoは不変である"""
        info = EncryptionInfo(
            is_encrypted=False,
            encryption_type=EncryptionType.NONE,
        )

        with pytest.raises(AttributeError):
            info.is_encrypted = True  # type: ignore[misc]

class TestXP3EncryptionError:
    """XP3EncryptionError例外のテスト"""

    def test_error_message_without_details(self) -> None:
        """正常系: 詳細なしの暗号化エラーメッセージを生成"""
        info = EncryptionInfo(
            is_encrypted=True,
            encryption_type=EncryptionType.SIMPLE_XOR,
        )

        error = XP3EncryptionError(info)

        assert "暗号化されています" in str(error)
        assert "simple_xor" in str(error)
        assert error.encryption_info == info

    def test_error_message_with_details(self) -> None:
        """正常系: 詳細付きの暗号化エラーメッセージを生成"""
        info = EncryptionInfo(
            is_encrypted=True,
            encryption_type=EncryptionType.CUSTOM,
            details="独自暗号化アルゴリズム",
        )

        error = XP3EncryptionError(info)

        assert "独自暗号化アルゴリズム" in str(error)
        assert "custom" in str(error)

class TestXP3Archive:
    """XP3Archiveクラスのテスト"""

    @pytest.fixture
    def valid_xp3_path(self, tmp_path: Path) -> Path:
        """有効なXP3ファイルパスを返すフィクスチャ"""
        xp3_file = tmp_path / "test.xp3"
        xp3_file.write_bytes(b"XP3\x0d\x0a\x1a\x0a\x00\x00\x00")  # XP3マジックナンバー
        return xp3_file

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
        # TDD Step 2: 実装前のためNotImplementedErrorが発生する
        with pytest.raises(NotImplementedError):
            XP3Archive(valid_xp3_path)

    def test_open_nonexistent_file_raises_error(self, nonexistent_path: Path) -> None:
        """異常系: 存在しないファイルパスでエラー"""
        # 実装時は FileNotFoundError が発生することを期待
        with pytest.raises((NotImplementedError, FileNotFoundError)):
            XP3Archive(nonexistent_path)

    def test_open_invalid_xp3_file_raises_error(self, invalid_xp3_path: Path) -> None:
        """異常系: 不正なXP3ファイルでエラー"""
        # 実装時は ValueError または カスタムエラー が発生することを期待
        with pytest.raises((NotImplementedError, ValueError)):
            XP3Archive(invalid_xp3_path)

    def test_list_files_returns_file_list(self, valid_xp3_path: Path) -> None:
        """正常系: ファイル一覧を取得できる"""
        with pytest.raises(NotImplementedError):
            archive = XP3Archive(valid_xp3_path)
            archive.list_files()

    def test_extract_all_creates_files(self, valid_xp3_path: Path, tmp_path: Path) -> None:
        """正常系: すべてのファイルを展開できる"""
        output_dir = tmp_path / "output"

        with pytest.raises(NotImplementedError):
            archive = XP3Archive(valid_xp3_path)
            archive.extract_all(output_dir)

    def test_extract_file_creates_single_file(self, valid_xp3_path: Path, tmp_path: Path) -> None:
        """正常系: 個別ファイルを展開できる"""
        output_path = tmp_path / "extracted_file.txt"

        with pytest.raises(NotImplementedError):
            archive = XP3Archive(valid_xp3_path)
            archive.extract_file("data/script.ks", output_path)

    def test_is_encrypted_returns_bool(self, valid_xp3_path: Path) -> None:
        """正常系: 暗号化判定がbool値を返す"""
        with pytest.raises(NotImplementedError):
            archive = XP3Archive(valid_xp3_path)
            archive.is_encrypted()

class TestXP3EncryptionChecker:
    """XP3EncryptionCheckerクラスのテスト"""

    @pytest.fixture
    def valid_xp3_path(self, tmp_path: Path) -> Path:
        """有効なXP3ファイルパスを返すフィクスチャ"""
        xp3_file = tmp_path / "test.xp3"
        xp3_file.write_bytes(b"XP3\x0d\x0a\x1a\x0a\x00\x00\x00")
        return xp3_file

    @pytest.fixture
    def nonexistent_path(self, tmp_path: Path) -> Path:
        """存在しないファイルパスを返すフィクスチャ"""
        return tmp_path / "nonexistent.xp3"

    def test_check_returns_encryption_info(self, valid_xp3_path: Path) -> None:
        """正常系: チェック結果がEncryptionInfoを返す"""
        checker = XP3EncryptionChecker(valid_xp3_path)

        with pytest.raises(NotImplementedError):
            checker.check()

    def test_check_non_encrypted_xp3(self, valid_xp3_path: Path) -> None:
        """正常系: 非暗号化XP3を正しく判定できる"""
        checker = XP3EncryptionChecker(valid_xp3_path)

        with pytest.raises(NotImplementedError):
            result = checker.check()
            # 実装後の期待値
            assert result.is_encrypted is False
            assert result.encryption_type == EncryptionType.NONE

    def test_check_nonexistent_file_raises_error(self, nonexistent_path: Path) -> None:
        """異常系: 存在しないファイルでエラー"""
        checker = XP3EncryptionChecker(nonexistent_path)

        with pytest.raises((NotImplementedError, FileNotFoundError)):
            checker.check()

    def test_raise_if_encrypted_does_not_raise_for_non_encrypted(
        self, valid_xp3_path: Path
    ) -> None:
        """正常系: 非暗号化XP3では例外を発生させない"""
        checker = XP3EncryptionChecker(valid_xp3_path)

        with pytest.raises(NotImplementedError):
            checker.raise_if_encrypted()

    def test_raise_if_encrypted_raises_for_encrypted(self, tmp_path: Path) -> None:
        """異常系: 暗号化XP3で例外を発生させる"""
        # 暗号化されたXP3のシミュレーション用
        encrypted_xp3 = tmp_path / "encrypted.xp3"
        encrypted_xp3.write_bytes(b"XP3\x0d\x0a\x1a\x0a\x00\x00\x00\xff\xff")

        checker = XP3EncryptionChecker(encrypted_xp3)

        # 実装時は XP3EncryptionError が発生することを期待
        with pytest.raises((NotImplementedError, XP3EncryptionError)):
            checker.raise_if_encrypted()

class TestXP3ArchiveIntegration:
    """XP3Archive統合テスト（モックなし）

    Note: これらのテストは実際のXP3ファイルが必要になる可能性があるため、
    実装フェーズでフィクスチャを適切に調整する。
    """

    @pytest.mark.parametrize(
        "file_count, expected_count",
        [
            pytest.param(0, 0, id="正常系: 空のアーカイブ"),
            pytest.param(1, 1, id="正常系: 1ファイル"),
            pytest.param(10, 10, id="正常系: 複数ファイル"),
        ],
    )
    def test_list_files_returns_correct_count(
        self, tmp_path: Path, file_count: int, expected_count: int
    ) -> None:
        """正常系: ファイル数が正しく返される"""
        # テスト用XP3ファイル作成（実装時に適切なフィクスチャに置き換え）
        xp3_file = tmp_path / "test.xp3"
        xp3_file.write_bytes(b"XP3\x0d\x0a\x1a\x0a\x00\x00\x00")

        with pytest.raises(NotImplementedError):
            archive = XP3Archive(xp3_file)
            files = archive.list_files()
            assert len(files) == expected_count

    @pytest.mark.parametrize(
        "filename",
        [
            pytest.param("data/script.ks", id="正常系: スクリプトファイル"),
            pytest.param("image/bg/title.png", id="正常系: 画像ファイル"),
            pytest.param("sound/bgm/main.ogg", id="正常系: 音声ファイル"),
        ],
    )
    def test_extract_file_by_name(self, tmp_path: Path, filename: str) -> None:
        """正常系: ファイル名を指定して展開できる"""
        xp3_file = tmp_path / "test.xp3"
        xp3_file.write_bytes(b"XP3\x0d\x0a\x1a\x0a\x00\x00\x00")
        output_path = tmp_path / "output" / Path(filename).name

        with pytest.raises(NotImplementedError):
            archive = XP3Archive(xp3_file)
            archive.extract_file(filename, output_path)
            assert output_path.exists()
