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
from mnemonic.parser.xp3 import XP3_MAGIC, XP3FileEntry, XP3Segment


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


class TestXP3Segment:
    """XP3Segmentデータクラスのテスト"""

    def test_segment_creation(self) -> None:
        """正常系: XP3Segmentが正しく生成される"""
        segment = XP3Segment(
            offset=1000,
            size=500,
            original_size=800,
            is_compressed=True,
        )

        assert segment.offset == 1000
        assert segment.size == 500
        assert segment.original_size == 800
        assert segment.is_compressed is True

    def test_segment_is_immutable(self) -> None:
        """XP3Segmentはイミュータブル"""
        segment = XP3Segment(
            offset=1000,
            size=500,
            original_size=800,
            is_compressed=True,
        )

        with pytest.raises(AttributeError):
            segment.offset = 2000  # type: ignore[misc]


class TestXP3FileEntryWithSegments:
    """複数セグメント対応のXP3FileEntryのテスト"""

    def test_file_entry_with_single_segment(self) -> None:
        """正常系: 単一セグメントのXP3FileEntryが正しく生成される"""
        segment = XP3Segment(
            offset=1000,
            size=500,
            original_size=800,
            is_compressed=True,
        )
        entry = XP3FileEntry(
            name="test.ogg",
            segments=[segment],
            is_encrypted=False,
        )

        assert entry.name == "test.ogg"
        assert len(entry.segments) == 1
        assert entry.segments[0].offset == 1000
        assert entry.total_size == 800

    def test_file_entry_with_multiple_segments(self) -> None:
        """正常系: 複数セグメントのXP3FileEntryが正しく生成される"""
        segments = [
            XP3Segment(offset=100, size=50, original_size=101, is_compressed=True),
            XP3Segment(offset=200, size=100, original_size=192, is_compressed=True),
            XP3Segment(offset=300, size=5000, original_size=11905, is_compressed=True),
        ]
        entry = XP3FileEntry(
            name="水滴.ogg",
            segments=segments,
            is_encrypted=False,
        )

        assert entry.name == "水滴.ogg"
        assert len(entry.segments) == 3
        # total_sizeは全セグメントのoriginal_sizeの合計
        assert entry.total_size == 101 + 192 + 11905  # 12198

    def test_file_entry_total_size_property(self) -> None:
        """正常系: total_sizeプロパティが全セグメントの合計を返す"""
        segments = [
            XP3Segment(offset=0, size=100, original_size=100, is_compressed=False),
            XP3Segment(offset=100, size=200, original_size=200, is_compressed=False),
            XP3Segment(offset=300, size=300, original_size=300, is_compressed=False),
        ]
        entry = XP3FileEntry(
            name="test.bin",
            segments=segments,
            is_encrypted=False,
        )

        assert entry.total_size == 600

    def test_file_entry_is_immutable(self) -> None:
        """XP3FileEntryはイミュータブル"""
        segment = XP3Segment(
            offset=1000,
            size=500,
            original_size=800,
            is_compressed=True,
        )
        entry = XP3FileEntry(
            name="test.ogg",
            segments=[segment],
            is_encrypted=False,
        )

        with pytest.raises(AttributeError):
            entry.name = "other.ogg"  # type: ignore[misc]


class TestXP3MultipleSegmentParsing:
    """複数セグメントのパースと抽出のテスト"""

    def _create_xp3_with_multiple_segments(
        self, tmp_path: Path, segments_data: list[tuple[bytes, bool]]
    ) -> tuple[Path, bytes]:
        """複数セグメントを持つXP3ファイルを作成する

        Args:
            tmp_path: 一時ディレクトリ
            segments_data: (データ, 圧縮フラグ)のリスト

        Returns:
            (XP3ファイルパス, 期待される結合データ)
        """
        import struct
        import zlib

        xp3_file = tmp_path / "multi_segment.xp3"

        # ヘッダー: XP3 magic (11バイト) + info_offset (8バイト)
        header = bytearray(XP3_MAGIC)

        # ファイルデータを構築（各セグメントを連結）
        file_data_parts: list[bytes] = []
        segment_infos: list[tuple[int, int, int, bool]] = []  # (offset, size, orig_size, comp)

        current_offset = 100  # ファイルデータの開始位置（ヘッダー後）

        expected_data = bytearray()
        for data, is_compressed in segments_data:
            if is_compressed:
                compressed = zlib.compress(data)
                file_data_parts.append(compressed)
                segment_infos.append((current_offset, len(compressed), len(data), True))
                current_offset += len(compressed)
            else:
                file_data_parts.append(data)
                segment_infos.append((current_offset, len(data), len(data), False))
                current_offset += len(data)
            expected_data.extend(data)

        # ファイルエントリの構築
        entry_data = bytearray()

        # infoチャンク
        file_name = "test.bin"
        name_bytes = file_name.encode("utf-16-le")
        total_original_size = sum(s[2] for s in segment_infos)
        total_compressed_size = sum(s[1] for s in segment_infos)

        info_content = bytearray()
        info_content.extend(struct.pack("<I", 0))  # flags
        info_content.extend(struct.pack("<Q", total_original_size))
        info_content.extend(struct.pack("<Q", total_compressed_size))
        info_content.extend(struct.pack("<H", len(file_name)))
        info_content.extend(name_bytes)

        entry_data.extend(b"info")
        entry_data.extend(struct.pack("<Q", len(info_content)))
        entry_data.extend(info_content)

        # segmチャンク（複数セグメント情報を含む）
        segm_content = bytearray()
        for offset, size, orig_size, is_comp in segment_infos:
            flags = 0x07 if is_comp else 0x00
            segm_content.extend(struct.pack("<I", flags))
            segm_content.extend(struct.pack("<Q", offset))
            segm_content.extend(struct.pack("<Q", size))
            segm_content.extend(struct.pack("<Q", orig_size))

        entry_data.extend(b"segm")
        entry_data.extend(struct.pack("<Q", len(segm_content)))
        entry_data.extend(segm_content)

        # adlrチャンク
        entry_data.extend(b"adlr")
        entry_data.extend(struct.pack("<Q", 4))
        entry_data.extend(struct.pack("<I", 0))  # dummy checksum

        # Fileチャンク
        file_chunk = bytearray()
        file_chunk.extend(b"File")
        file_chunk.extend(struct.pack("<Q", len(entry_data)))
        file_chunk.extend(entry_data)

        # ファイルテーブルを圧縮
        compressed_table = zlib.compress(bytes(file_chunk))

        # info_offsetの位置（ファイルデータの後）
        info_offset = current_offset
        header.extend(struct.pack("<Q", info_offset))

        # パディング
        header_with_padding = bytes(header).ljust(100, b"\x00")

        # ファイルを構築
        with open(xp3_file, "wb") as f:
            f.write(header_with_padding)
            for part in file_data_parts:
                f.write(part)
            # info構造: flag (1) + compressed_size (8) + original_size (8) + zlib_data
            f.write(struct.pack("<B", 0x00))  # flag
            f.write(struct.pack("<Q", len(compressed_table)))
            f.write(struct.pack("<Q", len(file_chunk)))
            f.write(compressed_table)

        return xp3_file, bytes(expected_data)

    def test_parse_multiple_segments(self, tmp_path: Path) -> None:
        """正常系: 複数セグメントを持つエントリが正しくパースされる"""
        # 3つのセグメントを持つファイルを作成
        segments_data = [
            (b"A" * 101, True),  # セグメント1: 101バイト、圧縮
            (b"B" * 192, True),  # セグメント2: 192バイト、圧縮
            (b"C" * 11905, True),  # セグメント3: 11905バイト、圧縮
        ]
        xp3_file, expected_data = self._create_xp3_with_multiple_segments(tmp_path, segments_data)

        archive = XP3Archive(xp3_file)
        files = archive.list_files()

        assert "test.bin" in files

    def test_extract_multiple_segments_concatenated(self, tmp_path: Path) -> None:
        """正常系: 複数セグメントが連結されて抽出される"""
        segments_data = [
            (b"FIRST_SEGMENT_DATA_", False),
            (b"SECOND_SEGMENT_DATA_", False),
            (b"THIRD_SEGMENT_DATA", False),
        ]
        xp3_file, expected_data = self._create_xp3_with_multiple_segments(tmp_path, segments_data)

        archive = XP3Archive(xp3_file)
        output_path = tmp_path / "output" / "test.bin"
        archive.extract_file("test.bin", output_path)

        assert output_path.exists()
        actual_data = output_path.read_bytes()
        assert actual_data == expected_data

    def test_extract_multiple_compressed_segments(self, tmp_path: Path) -> None:
        """正常系: 複数の圧縮セグメントが正しく解凍・連結される"""
        segments_data = [
            (b"X" * 100, True),  # 圧縮セグメント1
            (b"Y" * 200, True),  # 圧縮セグメント2
            (b"Z" * 300, True),  # 圧縮セグメント3
        ]
        xp3_file, expected_data = self._create_xp3_with_multiple_segments(tmp_path, segments_data)

        archive = XP3Archive(xp3_file)
        output_path = tmp_path / "output" / "test.bin"
        archive.extract_file("test.bin", output_path)

        assert output_path.exists()
        actual_data = output_path.read_bytes()
        assert len(actual_data) == 600  # 100 + 200 + 300
        assert actual_data == expected_data

    def test_extract_mixed_compression_segments(self, tmp_path: Path) -> None:
        """正常系: 圧縮・非圧縮が混在するセグメントが正しく処理される"""
        segments_data = [
            (b"UNCOMPRESSED_1_", False),  # 非圧縮
            (b"C" * 100, True),  # 圧縮
            (b"UNCOMPRESSED_2", False),  # 非圧縮
        ]
        xp3_file, expected_data = self._create_xp3_with_multiple_segments(tmp_path, segments_data)

        archive = XP3Archive(xp3_file)
        output_path = tmp_path / "output" / "test.bin"
        archive.extract_file("test.bin", output_path)

        assert output_path.exists()
        actual_data = output_path.read_bytes()
        assert actual_data == expected_data
