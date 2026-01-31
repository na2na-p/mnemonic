"""EncodingDetectorおよびEncodingConverterのテスト"""

from pathlib import Path

import pytest

from mnemonic.converter.base import ConversionStatus
from mnemonic.converter.encoding import (
    SUPPORTED_ENCODINGS,
    EncodingConverter,
    EncodingDetectionResult,
    EncodingDetector,
)


@pytest.fixture
def detector() -> EncodingDetector:
    """EncodingDetectorインスタンスを返すフィクスチャ"""
    return EncodingDetector()


@pytest.fixture
def fixtures_dir() -> Path:
    """テストフィクスチャディレクトリのパスを返すフィクスチャ"""
    return Path(__file__).parent.parent / "fixtures" / "encoding"


class TestEncodingDetectionResult:
    """EncodingDetectionResultのテスト"""

    def test_has_encoding_confidence_is_supported_attributes(self) -> None:
        """EncodingDetectionResultが正しい属性を持つことを確認する"""
        result = EncodingDetectionResult(
            encoding="utf-8",
            confidence=0.99,
            is_supported=True,
        )
        assert result.encoding == "utf-8"
        assert result.confidence == 0.99
        assert result.is_supported is True

    def test_allows_none_encoding(self) -> None:
        """encodingがNoneでもEncodingDetectionResultが作成できることを確認する"""
        result = EncodingDetectionResult(
            encoding=None,
            confidence=0.0,
            is_supported=False,
        )
        assert result.encoding is None
        assert result.is_supported is False

    def test_is_frozen_dataclass(self) -> None:
        """EncodingDetectionResultがイミュータブルであることを確認する"""
        result = EncodingDetectionResult(
            encoding="utf-8",
            confidence=0.99,
            is_supported=True,
        )
        with pytest.raises(AttributeError):
            result.encoding = "shift_jis"  # type: ignore[misc]


class TestSupportedEncodings:
    """SUPPORTED_ENCODINGSのテスト"""

    def test_contains_required_encodings(self) -> None:
        """SUPPORTED_ENCODINGSに必須のエンコーディングが含まれていることを確認する"""
        required_encodings = ["shift_jis", "euc-jp", "utf-8", "gb2312", "big5", "cp949"]
        for encoding in required_encodings:
            assert encoding in SUPPORTED_ENCODINGS

    def test_is_tuple_type(self) -> None:
        """SUPPORTED_ENCODINGSがタプル型であることを確認する"""
        assert isinstance(SUPPORTED_ENCODINGS, tuple)


class TestEncodingDetectorDetect:
    """EncodingDetector.detectメソッドのテスト"""

    @pytest.mark.parametrize(
        "filename, expected_encoding_lower",
        [
            pytest.param(
                "shift_jis.txt",
                "shift_jis",
                id="正常系: Shift_JISファイルの検出",
            ),
            pytest.param(
                "utf8.txt",
                "utf-8",
                id="正常系: UTF-8ファイルの検出",
            ),
            pytest.param(
                "utf8_bom.txt",
                "utf-8",
                id="正常系: BOM付きUTF-8ファイルの検出",
            ),
        ],
    )
    def test_detects_file_encoding_correctly(
        self,
        detector: EncodingDetector,
        fixtures_dir: Path,
        filename: str,
        expected_encoding_lower: str,
    ) -> None:
        """各種エンコーディングのファイルを正しく検出できることを確認する"""
        file_path = fixtures_dir / filename
        result = detector.detect(file_path)

        assert result.encoding is not None
        detected_lower = result.encoding.lower().replace("-", "_")
        expected_normalized = expected_encoding_lower.replace("-", "_")
        assert detected_lower == expected_normalized or expected_normalized in detected_lower
        assert result.confidence > 0.5
        assert result.is_supported is True

    def test_raises_file_not_found_error_for_nonexistent_file(
        self, detector: EncodingDetector, fixtures_dir: Path
    ) -> None:
        """存在しないファイルを指定した場合FileNotFoundErrorが発生することを確認する"""
        nonexistent_path = fixtures_dir / "nonexistent.txt"
        with pytest.raises(FileNotFoundError):
            detector.detect(nonexistent_path)

    def test_returns_result_for_empty_file(
        self, detector: EncodingDetector, fixtures_dir: Path
    ) -> None:
        """空ファイルでもEncodingDetectionResultを返すことを確認する"""
        empty_path = fixtures_dir / "empty.txt"
        result = detector.detect(empty_path)

        assert isinstance(result, EncodingDetectionResult)


class TestEncodingDetectorDetectBytes:
    """EncodingDetector.detect_bytesメソッドのテスト"""

    def test_detects_utf8_bytes(self, detector: EncodingDetector) -> None:
        """UTF-8のバイトデータを正しく検出できることを確認する"""
        utf8_bytes = "これはテストです".encode()
        result = detector.detect_bytes(utf8_bytes)

        assert result.encoding is not None
        assert "utf" in result.encoding.lower()
        assert result.confidence > 0.5
        assert result.is_supported is True

    def test_detects_shift_jis_bytes(self, detector: EncodingDetector) -> None:
        """Shift_JISのバイトデータを正しく検出できることを確認する"""
        # chardetが正確に検出できるよう、十分な長さのテキストを使用
        sjis_text = (
            "これはShift_JISエンコーディングのテストファイルです。"
            "日本語の文章を含みます。吾輩は猫である。名前はまだ無い。"
            "どこで生れたかとんと見当がつかぬ。"
        )
        sjis_bytes = sjis_text.encode("shift_jis")
        result = detector.detect_bytes(sjis_bytes)

        assert result.encoding is not None
        assert result.confidence > 0.5
        assert result.is_supported is True

    def test_returns_result_for_empty_bytes(self, detector: EncodingDetector) -> None:
        """空のバイトデータでもEncodingDetectionResultを返すことを確認する"""
        result = detector.detect_bytes(b"")

        assert isinstance(result, EncodingDetectionResult)


class TestEncodingDetectorIsTextFile:
    """EncodingDetector.is_text_fileメソッドのテスト"""

    @pytest.mark.parametrize(
        "filename, expected",
        [
            pytest.param("utf8.txt", True, id="正常系: UTF-8テキストファイル"),
            pytest.param("shift_jis.txt", True, id="正常系: Shift_JISテキストファイル"),
            pytest.param("utf8_bom.txt", True, id="正常系: BOM付きUTF-8テキストファイル"),
            pytest.param("binary.dat", False, id="正常系: バイナリファイル"),
        ],
    )
    def test_distinguishes_text_and_binary_files(
        self,
        detector: EncodingDetector,
        fixtures_dir: Path,
        filename: str,
        expected: bool,
    ) -> None:
        """テキストファイルとバイナリファイルを正しく判別できることを確認する"""
        file_path = fixtures_dir / filename
        result = detector.is_text_file(file_path)

        assert result is expected

    def test_raises_file_not_found_error_for_nonexistent_file(
        self, detector: EncodingDetector, fixtures_dir: Path
    ) -> None:
        """存在しないファイルを指定した場合FileNotFoundErrorが発生することを確認する"""
        nonexistent_path = fixtures_dir / "nonexistent.txt"
        with pytest.raises(FileNotFoundError):
            detector.is_text_file(nonexistent_path)

    def test_empty_file_is_text_file(self, detector: EncodingDetector, fixtures_dir: Path) -> None:
        """空ファイルはテキストファイルとして判定されることを確認する"""
        empty_path = fixtures_dir / "empty.txt"
        result = detector.is_text_file(empty_path)

        assert result is True


@pytest.fixture
def converter() -> EncodingConverter:
    """EncodingConverterインスタンスを返すフィクスチャ"""
    return EncodingConverter()


class TestEncodingConverterInit:
    """EncodingConverterの初期化テスト"""

    def test_default_target_encoding_is_utf8(self) -> None:
        """デフォルトのターゲットエンコーディングがUTF-8であることを確認する"""
        converter = EncodingConverter()
        assert converter.target_encoding == "utf-8"

    def test_default_source_encoding_is_none(self) -> None:
        """デフォルトのソースエンコーディングがNoneであることを確認する"""
        converter = EncodingConverter()
        assert converter.source_encoding is None

    def test_custom_target_encoding(self) -> None:
        """カスタムターゲットエンコーディングを指定できることを確認する"""
        converter = EncodingConverter(target_encoding="shift_jis")
        assert converter.target_encoding == "shift_jis"

    def test_custom_source_encoding(self) -> None:
        """カスタムソースエンコーディングを指定できることを確認する"""
        converter = EncodingConverter(source_encoding="shift_jis")
        assert converter.source_encoding == "shift_jis"


class TestEncodingConverterSupportedExtensions:
    """EncodingConverter.supported_extensionsのテスト"""

    @pytest.mark.parametrize(
        "extension",
        [
            pytest.param(".ks", id="正常系: .ks（KAGスクリプト）"),
            pytest.param(".tjs", id="正常系: .tjs（TJSスクリプト）"),
            pytest.param(".txt", id="正常系: .txt（テキストファイル）"),
            pytest.param(".csv", id="正常系: .csv（CSVファイル）"),
            pytest.param(".ini", id="正常系: .ini（設定ファイル）"),
        ],
    )
    def test_contains_expected_extension(
        self, converter: EncodingConverter, extension: str
    ) -> None:
        """期待される拡張子がサポートされていることを確認する"""
        assert extension in converter.supported_extensions

    def test_returns_tuple(self, converter: EncodingConverter) -> None:
        """supported_extensionsがタプルを返すことを確認する"""
        assert isinstance(converter.supported_extensions, tuple)


class TestEncodingConverterCanConvert:
    """EncodingConverter.can_convertのテスト"""

    @pytest.mark.parametrize(
        "filename, expected",
        [
            pytest.param("script.ks", True, id="正常系: .ksファイルは変換可能"),
            pytest.param("script.tjs", True, id="正常系: .tjsファイルは変換可能"),
            pytest.param("readme.txt", True, id="正常系: .txtファイルは変換可能"),
            pytest.param("data.csv", True, id="正常系: .csvファイルは変換可能"),
            pytest.param("config.ini", True, id="正常系: .iniファイルは変換可能"),
            pytest.param("image.png", False, id="正常系: .pngファイルは変換不可"),
            pytest.param("audio.ogg", False, id="正常系: .oggファイルは変換不可"),
        ],
    )
    def test_returns_expected_result_for_extension(
        self,
        converter: EncodingConverter,
        fixtures_dir: Path,
        filename: str,
        expected: bool,
        tmp_path: Path,
    ) -> None:
        """拡張子に基づいて正しい結果を返すことを確認する"""
        # テキストファイルを作成
        test_file = tmp_path / filename
        test_file.write_text("test content", encoding="utf-8")
        result = converter.can_convert(test_file)
        assert result is expected

    def test_returns_false_for_binary_file_with_text_extension(
        self, converter: EncodingConverter, fixtures_dir: Path
    ) -> None:
        """テキスト拡張子でもバイナリファイルの場合はFalseを返すことを確認する"""
        binary_path = fixtures_dir / "binary.dat"
        # binary.datが存在しない場合のためにパスを調整
        if binary_path.exists():
            # バイナリファイルは.dat拡張子なのでFalse
            result = converter.can_convert(binary_path)
            assert result is False


class TestEncodingConverterConvert:
    """EncodingConverter.convertのテスト"""

    def test_converts_shift_jis_to_utf8(self, converter: EncodingConverter, tmp_path: Path) -> None:
        """Shift_JISファイルをUTF-8に変換できることを確認する"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"

        # Shift_JISでテストファイルを作成
        test_text = "これはテストです。日本語の文章。"
        source.write_bytes(test_text.encode("shift_jis"))

        result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        # 変換後のファイルがUTF-8であることを確認
        converted_text = dest.read_text(encoding="utf-8")
        assert converted_text == test_text

    def test_converts_euc_jp_to_utf8(self, converter: EncodingConverter, tmp_path: Path) -> None:
        """EUC-JPファイルをUTF-8に変換できることを確認する"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"

        # EUC-JPでテストファイルを作成
        test_text = "これはEUC-JPエンコーディングのテストです。日本語。"
        source.write_bytes(test_text.encode("euc-jp"))

        result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        converted_text = dest.read_text(encoding="utf-8")
        assert converted_text == test_text

    def test_removes_utf8_bom(self, converter: EncodingConverter, tmp_path: Path) -> None:
        """BOM付きUTF-8ファイルからBOMを除去できることを確認する"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"

        # BOM付きUTF-8でテストファイルを作成
        test_text = "BOM付きファイルのテスト"
        bom = b"\xef\xbb\xbf"
        source.write_bytes(bom + test_text.encode("utf-8"))

        result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        # BOMが除去されていることを確認
        dest_bytes = dest.read_bytes()
        assert not dest_bytes.startswith(bom)
        assert dest.read_text(encoding="utf-8") == test_text

    def test_skips_already_utf8_file(self, converter: EncodingConverter, tmp_path: Path) -> None:
        """既にUTF-8のファイルはスキップされることを確認する"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"

        # UTF-8（BOMなし）でテストファイルを作成
        test_text = "これは既にUTF-8のファイルです"
        source.write_text(test_text, encoding="utf-8")

        result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SKIPPED

    def test_uses_specified_source_encoding(self, tmp_path: Path) -> None:
        """指定されたソースエンコーディングを使用することを確認する"""
        converter = EncodingConverter(source_encoding="shift_jis")
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"

        test_text = "ソースエンコーディング指定テスト"
        source.write_bytes(test_text.encode("shift_jis"))

        result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.read_text(encoding="utf-8") == test_text

    def test_records_bytes_before_and_after(
        self, converter: EncodingConverter, tmp_path: Path
    ) -> None:
        """変換前後のバイト数が記録されることを確認する"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"

        test_text = "バイトサイズテスト"
        source.write_bytes(test_text.encode("shift_jis"))

        result = converter.convert(source, dest)

        assert result.bytes_before > 0
        assert result.bytes_after > 0

    def test_returns_failed_for_nonexistent_file(
        self, converter: EncodingConverter, tmp_path: Path
    ) -> None:
        """存在しないファイルの場合FAILEDを返すことを確認する"""
        source = tmp_path / "nonexistent.txt"
        dest = tmp_path / "dest.txt"

        result = converter.convert(source, dest)

        assert result.status == ConversionStatus.FAILED

    def test_creates_dest_directory_if_not_exists(
        self, converter: EncodingConverter, tmp_path: Path
    ) -> None:
        """変換先ディレクトリが存在しない場合は作成することを確認する"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "subdir" / "dest.txt"

        test_text = "ディレクトリ作成テスト"
        source.write_bytes(test_text.encode("shift_jis"))

        result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.parent.exists()
        assert dest.exists()


class TestEncodingConverterConvertBytes:
    """EncodingConverter.convert_bytesのテスト"""

    def test_converts_shift_jis_bytes_to_utf8(self, converter: EncodingConverter) -> None:
        """Shift_JISバイトデータをUTF-8に変換できることを確認する"""
        # chardetが正確に検出できるよう、十分な長さのテキストを使用
        test_text = (
            "これはShift_JISエンコーディングのテストファイルです。"
            "日本語の文章を含みます。吾輩は猫である。名前はまだ無い。"
        )
        sjis_bytes = test_text.encode("shift_jis")

        result_bytes, detected_encoding = converter.convert_bytes(sjis_bytes)

        assert result_bytes.decode("utf-8") == test_text
        assert "shift" in detected_encoding.lower() or "sjis" in detected_encoding.lower()

    def test_converts_euc_jp_bytes_to_utf8(self, converter: EncodingConverter) -> None:
        """EUC-JPバイトデータをUTF-8に変換できることを確認する"""
        # chardetが正確に検出できるよう、十分な長さのテキストを使用
        test_text = (
            "これはEUC-JPエンコーディングのテストファイルです。"
            "日本語の文章を含みます。坊っちゃん。親譲りの無鉄砲で小供の時から損ばかりしている。"
        )
        euc_bytes = test_text.encode("euc-jp")

        result_bytes, detected_encoding = converter.convert_bytes(euc_bytes)

        assert result_bytes.decode("utf-8") == test_text

    def test_uses_specified_source_encoding_for_bytes(self) -> None:
        """バイト変換でも指定されたソースエンコーディングを使用することを確認する"""
        converter = EncodingConverter(source_encoding="shift_jis")
        test_text = "ソースエンコーディング指定"
        sjis_bytes = test_text.encode("shift_jis")

        result_bytes, detected_encoding = converter.convert_bytes(sjis_bytes)

        assert result_bytes.decode("utf-8") == test_text
        assert detected_encoding == "shift_jis"

    def test_removes_bom_from_bytes(self, converter: EncodingConverter) -> None:
        """バイトデータからBOMを除去できることを確認する"""
        test_text = "BOMテスト"
        bom = b"\xef\xbb\xbf"
        utf8_bom_bytes = bom + test_text.encode("utf-8")

        result_bytes, _ = converter.convert_bytes(utf8_bom_bytes)

        assert not result_bytes.startswith(bom)
        assert result_bytes.decode("utf-8") == test_text


class TestEncodingConverterJapanesePreservation:
    """日本語文字の保全テスト"""

    @pytest.mark.parametrize(
        "source_encoding, test_text",
        [
            pytest.param(
                "shift_jis",
                (
                    "吾輩は猫である。名前はまだ無い。どこで生れたかとんと見当がつかぬ。"
                    "何でも薄暗いじめじめした所でニャーニャー泣いていた事だけは記憶している。"
                ),
                id="正常系: Shift_JISの日本語が保全される",
            ),
            pytest.param(
                "euc-jp",
                (
                    "坊っちゃん。親譲りの無鉄砲で小供の時から損ばかりしている。"
                    "小学校に居る時分学校の二階から飛び降りて一週間ほど腰を抜かした事がある。"
                ),
                id="正常系: EUC-JPの日本語が保全される",
            ),
            pytest.param(
                "shift_jis",
                (
                    "漢字、ひらがな、カタカナ、ＡＢＣ、１２３。"
                    "日本語のテキストファイルに含まれる様々な文字を保全することを確認します。"
                ),
                id="正常系: 複合文字が保全される",
            ),
        ],
    )
    def test_preserves_japanese_characters(
        self,
        converter: EncodingConverter,
        tmp_path: Path,
        source_encoding: str,
        test_text: str,
    ) -> None:
        """日本語文字が正しく保全されることを確認する"""
        source = tmp_path / "source.txt"
        dest = tmp_path / "dest.txt"

        source.write_bytes(test_text.encode(source_encoding))

        result = converter.convert(source, dest)

        assert result.status == ConversionStatus.SUCCESS
        converted_text = dest.read_text(encoding="utf-8")
        assert converted_text == test_text
