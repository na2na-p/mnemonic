"""EXE埋め込みXP3抽出機能のテスト"""

from pathlib import Path

import pytest

from mnemonic.parser.exe import XP3_MAGIC, EmbeddedXP3Extractor


class TestEmbeddedXP3Extractor:
    """EmbeddedXP3Extractorのテスト"""

    def test_init_raises_for_nonexistent_exe(self, tmp_path: Path) -> None:
        """異常系: 存在しないEXEファイルでFileNotFoundError"""
        nonexistent = tmp_path / "nonexistent.exe"
        with pytest.raises(FileNotFoundError):
            EmbeddedXP3Extractor(nonexistent)

    def test_find_embedded_xp3_returns_empty_for_no_xp3(self, tmp_path: Path) -> None:
        """正常系: XP3が埋め込まれていないEXEでは空リスト"""
        exe_file = tmp_path / "no_xp3.exe"
        exe_file.write_bytes(b"MZ" + b"\x00" * 100)

        extractor = EmbeddedXP3Extractor(exe_file)
        result = extractor.find_embedded_xp3()

        assert result == []

    def test_find_embedded_xp3_returns_offsets(self, tmp_path: Path) -> None:
        """正常系: 埋め込みXP3のオフセットを検出"""
        exe_content = b"MZ" + b"\x00" * 100 + XP3_MAGIC + b"\x00" * 50
        exe_file = tmp_path / "with_xp3.exe"
        exe_file.write_bytes(exe_content)

        extractor = EmbeddedXP3Extractor(exe_file)
        result = extractor.find_embedded_xp3()

        assert len(result) == 1
        assert result[0].offset == 102
        assert result[0].estimated_size == 61

    def test_find_embedded_xp3_multiple(self, tmp_path: Path) -> None:
        """正常系: 複数のXP3が埋め込まれている場合"""
        exe_content = b"MZ" + b"\x00" * 100 + XP3_MAGIC + b"\x00" * 50 + XP3_MAGIC + b"\x00" * 30
        exe_file = tmp_path / "multi_xp3.exe"
        exe_file.write_bytes(exe_content)

        extractor = EmbeddedXP3Extractor(exe_file)
        result = extractor.find_embedded_xp3()

        assert len(result) == 2
        assert result[0].offset == 102
        assert result[1].offset == 163

    def test_extract_all_creates_files(self, tmp_path: Path) -> None:
        """正常系: XP3ファイルを抽出"""
        xp3_content = XP3_MAGIC + b"\x00" * 50
        exe_content = b"MZ" + b"\x00" * 100 + xp3_content
        exe_file = tmp_path / "test.exe"
        exe_file.write_bytes(exe_content)

        output_dir = tmp_path / "extracted"
        output_dir.mkdir()

        extractor = EmbeddedXP3Extractor(exe_file)
        result = extractor.extract_all(output_dir)

        assert len(result) == 1
        assert result[0].exists()
        assert result[0].suffix == ".xp3"
        assert result[0].read_bytes().startswith(XP3_MAGIC)

    def test_extract_all_creates_output_dir(self, tmp_path: Path) -> None:
        """正常系: 出力ディレクトリが存在しない場合は作成"""
        xp3_content = XP3_MAGIC + b"\x00" * 50
        exe_content = b"MZ" + b"\x00" * 100 + xp3_content
        exe_file = tmp_path / "test.exe"
        exe_file.write_bytes(exe_content)

        output_dir = tmp_path / "new_dir"
        assert not output_dir.exists()

        extractor = EmbeddedXP3Extractor(exe_file)
        result = extractor.extract_all(output_dir)

        assert output_dir.exists()
        assert len(result) == 1

    def test_extract_all_returns_empty_for_no_xp3(self, tmp_path: Path) -> None:
        """正常系: XP3が埋め込まれていない場合は空リスト"""
        exe_file = tmp_path / "no_xp3.exe"
        exe_file.write_bytes(b"MZ" + b"\x00" * 100)

        output_dir = tmp_path / "extracted"

        extractor = EmbeddedXP3Extractor(exe_file)
        result = extractor.extract_all(output_dir)

        assert result == []
