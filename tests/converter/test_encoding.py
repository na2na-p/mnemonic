"""EncodingDetectorのテスト"""

from pathlib import Path

import pytest

from mnemonic.converter.encoding import (
    SUPPORTED_ENCODINGS,
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
        sjis_bytes = "これはテストです".encode("shift_jis")
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
