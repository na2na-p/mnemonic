"""Converter基底クラスのテスト"""

from pathlib import Path

import pytest

from mnemonic.converter import BaseConverter, ConversionResult, ConversionStatus


class MockConverter(BaseConverter):
    """テスト用の具象Converterクラス"""

    @property
    def supported_extensions(self) -> tuple[str, ...]:
        return (".txt", ".md")

    def can_convert(self, file_path: Path) -> bool:
        return file_path.suffix.lower() in self.supported_extensions

    def convert(self, source: Path, dest: Path) -> ConversionResult:
        return ConversionResult(
            source_path=source,
            dest_path=dest,
            status=ConversionStatus.SUCCESS,
            message="",
            bytes_before=100,
            bytes_after=80,
        )


class TestConversionStatus:
    """ConversionStatus列挙型のテスト"""

    @pytest.mark.parametrize(
        "status,expected_value",
        [
            pytest.param(ConversionStatus.SUCCESS, "success", id="正常系: SUCCESSステータス"),
            pytest.param(ConversionStatus.SKIPPED, "skipped", id="正常系: SKIPPEDステータス"),
            pytest.param(ConversionStatus.FAILED, "failed", id="正常系: FAILEDステータス"),
        ],
    )
    def test_status_values(self, status: ConversionStatus, expected_value: str) -> None:
        """各ステータスが正しい値を持つことをテスト"""
        assert status.value == expected_value


class TestConversionResult:
    """ConversionResultデータクラスのテスト"""

    def test_success_result_creation(self, tmp_path: Path) -> None:
        """成功結果の作成をテスト"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"
        result = ConversionResult(
            source_path=source,
            dest_path=dest,
            status=ConversionStatus.SUCCESS,
            bytes_before=100,
            bytes_after=80,
        )
        assert result.source_path == source
        assert result.dest_path == dest
        assert result.status == ConversionStatus.SUCCESS
        assert result.bytes_before == 100
        assert result.bytes_after == 80

    def test_failed_result_creation(self, tmp_path: Path) -> None:
        """失敗結果の作成をテスト"""
        source = tmp_path / "source.txt"
        result = ConversionResult(
            source_path=source,
            dest_path=None,
            status=ConversionStatus.FAILED,
            message="Conversion failed: file not found",
        )
        assert result.source_path == source
        assert result.dest_path is None
        assert result.status == ConversionStatus.FAILED
        assert "not found" in result.message

    def test_skipped_result_creation(self, tmp_path: Path) -> None:
        """スキップ結果の作成をテスト"""
        source = tmp_path / "source.txt"
        result = ConversionResult(
            source_path=source,
            dest_path=None,
            status=ConversionStatus.SKIPPED,
            message="Already converted",
        )
        assert result.status == ConversionStatus.SKIPPED

    def test_result_is_frozen(self, tmp_path: Path) -> None:
        """ConversionResultが不変であることをテスト"""
        source = tmp_path / "source.txt"
        result = ConversionResult(
            source_path=source,
            dest_path=None,
            status=ConversionStatus.SUCCESS,
        )
        with pytest.raises(AttributeError):
            result.status = ConversionStatus.FAILED  # type: ignore[misc]


class TestBaseConverter:
    """BaseConverter抽象クラスのテスト"""

    def test_cannot_instantiate_directly(self) -> None:
        """直接インスタンス化できないことをテスト"""
        with pytest.raises(TypeError):
            BaseConverter()  # type: ignore[abstract]

    def test_concrete_converter_can_be_instantiated(self) -> None:
        """具象クラスはインスタンス化できることをテスト"""
        converter = MockConverter()
        assert isinstance(converter, BaseConverter)

    def test_can_convert_returns_bool(self, tmp_path: Path) -> None:
        """can_convertがboolを返すことをテスト"""
        converter = MockConverter()
        supported_file = tmp_path / "test.txt"
        unsupported_file = tmp_path / "test.jpg"

        assert converter.can_convert(supported_file) is True
        assert converter.can_convert(unsupported_file) is False

    def test_convert_returns_result(self, tmp_path: Path) -> None:
        """convertがConversionResultを返すことをテスト"""
        converter = MockConverter()
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"

        result = converter.convert(source, dest)

        assert isinstance(result, ConversionResult)
        assert result.status == ConversionStatus.SUCCESS

    def test_supported_extensions_returns_tuple(self) -> None:
        """supported_extensionsがタプルを返すことをテスト"""
        converter = MockConverter()
        extensions = converter.supported_extensions

        assert isinstance(extensions, tuple)
        assert len(extensions) > 0
        assert all(ext.startswith(".") for ext in extensions)


class TestConversionResultProperties:
    """ConversionResultのユーティリティプロパティのテスト"""

    def test_compression_ratio_normal(self, tmp_path: Path) -> None:
        """正常系: 圧縮率の計算をテスト"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"
        result = ConversionResult(
            source_path=source,
            dest_path=dest,
            status=ConversionStatus.SUCCESS,
            bytes_before=100,
            bytes_after=80,
        )
        assert result.compression_ratio == 0.8

    def test_compression_ratio_zero_bytes_before(self, tmp_path: Path) -> None:
        """異常系: bytes_beforeが0の場合の圧縮率をテスト"""
        source = tmp_path / "source.txt"
        result = ConversionResult(
            source_path=source,
            dest_path=None,
            status=ConversionStatus.SUCCESS,
            bytes_before=0,
            bytes_after=50,
        )
        assert result.compression_ratio == 1.0

    def test_bytes_saved(self, tmp_path: Path) -> None:
        """正常系: 節約バイト数の計算をテスト"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"
        result = ConversionResult(
            source_path=source,
            dest_path=dest,
            status=ConversionStatus.SUCCESS,
            bytes_before=100,
            bytes_after=80,
        )
        assert result.bytes_saved == 20

    def test_bytes_saved_negative(self, tmp_path: Path) -> None:
        """正常系: サイズ増加時の節約バイト数をテスト"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"
        result = ConversionResult(
            source_path=source,
            dest_path=dest,
            status=ConversionStatus.SUCCESS,
            bytes_before=80,
            bytes_after=100,
        )
        assert result.bytes_saved == -20

    @pytest.mark.parametrize(
        "status,expected",
        [
            pytest.param(ConversionStatus.SUCCESS, True, id="正常系: SUCCESSの場合True"),
            pytest.param(ConversionStatus.SKIPPED, False, id="正常系: SKIPPEDの場合False"),
            pytest.param(ConversionStatus.FAILED, False, id="正常系: FAILEDの場合False"),
        ],
    )
    def test_is_success(self, tmp_path: Path, status: ConversionStatus, expected: bool) -> None:
        """is_successプロパティをテスト"""
        source = tmp_path / "source.txt"
        result = ConversionResult(
            source_path=source,
            dest_path=None,
            status=status,
        )
        assert result.is_success is expected


class TestBaseConverterUtilities:
    """BaseConverterのユーティリティメソッドのテスト"""

    def test_validate_source_not_found(self, tmp_path: Path) -> None:
        """異常系: 存在しないファイルの検証をテスト"""
        converter = MockConverter()
        non_existent = tmp_path / "non_existent.txt"

        with pytest.raises(FileNotFoundError) as exc_info:
            converter._validate_source(non_existent)

        assert "変換元ファイルが見つかりません" in str(exc_info.value)

    def test_validate_source_is_directory(self, tmp_path: Path) -> None:
        """異常系: ディレクトリの検証をテスト"""
        converter = MockConverter()

        with pytest.raises(ValueError) as exc_info:
            converter._validate_source(tmp_path)

        assert "変換元はファイルである必要があります" in str(exc_info.value)

    def test_validate_source_valid_file(self, tmp_path: Path) -> None:
        """正常系: 有効なファイルの検証をテスト"""
        converter = MockConverter()
        valid_file = tmp_path / "valid.txt"
        valid_file.write_text("test content")

        # 例外が発生しないことを確認
        converter._validate_source(valid_file)

    def test_get_file_size_existing_file(self, tmp_path: Path) -> None:
        """正常系: 存在するファイルのサイズ取得をテスト"""
        converter = MockConverter()
        test_file = tmp_path / "test.txt"
        test_content = "Hello, World!"
        test_file.write_text(test_content)

        size = converter._get_file_size(test_file)

        assert size == len(test_content)

    def test_get_file_size_non_existing_file(self, tmp_path: Path) -> None:
        """正常系: 存在しないファイルのサイズ取得をテスト（0を返す）"""
        converter = MockConverter()
        non_existent = tmp_path / "non_existent.txt"

        size = converter._get_file_size(non_existent)

        assert size == 0
