"""TLGImageDecoderおよびImageConverterのテスト"""

import tempfile
from pathlib import Path

import pytest
from PIL import Image

from mnemonic.converter.base import ConversionStatus
from mnemonic.converter.image import (
    ImageConverter,
    QualityPreset,
    TLGImageDecoder,
    TLGVersion,
)

# TLGマジックバイト定数
TLG5_MAGIC = b"TLG5.0\x00raw\x1a"
TLG6_MAGIC = b"TLG6.0\x00raw\x1a"


class TestIsTlgFile:
    """is_tlg_fileメソッドのテスト"""

    @pytest.mark.parametrize(
        "file_content, expected",
        [
            pytest.param(
                TLG5_MAGIC + b"\x00" * 100,
                True,
                id="正常系: TLG5形式のファイルはTrueを返す",
            ),
            pytest.param(
                TLG6_MAGIC + b"\x00" * 100,
                True,
                id="正常系: TLG6形式のファイルはTrueを返す",
            ),
            pytest.param(
                b"PNG\x00\x00\x00" + b"\x00" * 100,
                False,
                id="異常系: PNG形式のファイルはFalseを返す",
            ),
            pytest.param(
                b"JPEG\x00\x00\x00" + b"\x00" * 100,
                False,
                id="異常系: JPEG形式のファイルはFalseを返す",
            ),
            pytest.param(
                b"TLG5",
                False,
                id="異常系: マジックバイトが不完全なファイルはFalseを返す",
            ),
            pytest.param(
                b"",
                False,
                id="異常系: 空ファイルはFalseを返す",
            ),
        ],
    )
    def test_is_tlg_file(self, file_content: bytes, expected: bool) -> None:
        """ファイルがTLG形式かどうかを正しく判定できることを確認"""
        decoder = TLGImageDecoder()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(file_content)
            temp_path = Path(f.name)

        try:
            result = decoder.is_tlg_file(temp_path)
            assert result == expected
        finally:
            temp_path.unlink()

    def test_is_tlg_file_nonexistent_file(self) -> None:
        """存在しないファイルに対してFalseを返すことを確認"""
        decoder = TLGImageDecoder()
        nonexistent_path = Path("/nonexistent/path/to/file.tlg")

        result = decoder.is_tlg_file(nonexistent_path)
        assert result is False


class TestGetInfo:
    """get_infoメソッドのテスト"""

    def test_get_info_raises_not_implemented(self) -> None:
        """get_infoがNotImplementedErrorを発生させることを確認"""
        decoder = TLGImageDecoder()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(TLG5_MAGIC + b"\x00" * 100)
            temp_path = Path(f.name)

        try:
            with pytest.raises(NotImplementedError):
                decoder.get_info(temp_path)
        finally:
            temp_path.unlink()


class TestDecode:
    """decodeメソッドのテスト"""

    def test_decode_raises_not_implemented(self) -> None:
        """decodeがNotImplementedErrorを発生させることを確認"""
        decoder = TLGImageDecoder()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(TLG5_MAGIC + b"\x00" * 100)
            temp_path = Path(f.name)

        try:
            with pytest.raises(NotImplementedError):
                decoder.decode(temp_path)
        finally:
            temp_path.unlink()


class TestDecodeToFile:
    """decode_to_fileメソッドのテスト"""

    def test_decode_to_file_raises_not_implemented(self) -> None:
        """decode_to_fileがNotImplementedErrorを発生させることを確認"""
        decoder = TLGImageDecoder()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(TLG5_MAGIC + b"\x00" * 100)
            temp_path = Path(f.name)

        try:
            dest_path = Path("/tmp/output.png")
            with pytest.raises(NotImplementedError):
                decoder.decode_to_file(temp_path, dest_path)
        finally:
            temp_path.unlink()


class TestTLGVersion:
    """TLGVersionのテスト"""

    def test_tlg_version_values(self) -> None:
        """TLGVersionの値が正しいことを確認"""
        assert TLGVersion.TLG5.value == "TLG5"
        assert TLGVersion.TLG6.value == "TLG6"
        assert TLGVersion.UNKNOWN.value == "UNKNOWN"


class TestQualityPreset:
    """QualityPreset列挙型のテスト"""

    @pytest.mark.parametrize(
        "preset, expected_value",
        [
            pytest.param(QualityPreset.HIGH, 95, id="正常系: HIGHプリセットは95"),
            pytest.param(QualityPreset.MEDIUM, 85, id="正常系: MEDIUMプリセットは85"),
            pytest.param(QualityPreset.LOW, 70, id="正常系: LOWプリセットは70"),
        ],
    )
    def test_quality_preset_values(self, preset: QualityPreset, expected_value: int) -> None:
        """QualityPresetの値が正しいことを確認"""
        assert preset.value == expected_value


class TestImageConverter:
    """ImageConverterクラスのテスト"""

    @pytest.fixture
    def temp_dir(self) -> Path:
        """一時ディレクトリを作成するフィクスチャ"""
        with tempfile.TemporaryDirectory() as td:
            yield Path(td)

    @pytest.fixture
    def bmp_image(self, temp_dir: Path) -> Path:
        """テスト用BMPファイルを作成するフィクスチャ"""
        img = Image.new("RGB", (100, 100), color=(255, 0, 0))
        path = temp_dir / "test.bmp"
        img.save(path, "BMP")
        return path

    @pytest.fixture
    def jpg_image(self, temp_dir: Path) -> Path:
        """テスト用JPGファイルを作成するフィクスチャ"""
        img = Image.new("RGB", (100, 100), color=(0, 255, 0))
        path = temp_dir / "test.jpg"
        img.save(path, "JPEG")
        return path

    @pytest.fixture
    def png_image(self, temp_dir: Path) -> Path:
        """テスト用PNGファイルを作成するフィクスチャ"""
        img = Image.new("RGB", (100, 100), color=(0, 0, 255))
        path = temp_dir / "test.png"
        img.save(path, "PNG")
        return path

    @pytest.fixture
    def png_with_alpha(self, temp_dir: Path) -> Path:
        """テスト用アルファチャンネル付きPNGファイルを作成するフィクスチャ"""
        img = Image.new("RGBA", (100, 100), color=(255, 0, 0, 128))
        path = temp_dir / "test_alpha.png"
        img.save(path, "PNG")
        return path

    def test_convert_bmp_to_webp(self, bmp_image: Path, temp_dir: Path) -> None:
        """BMPファイルをWebPに変換できることを確認"""
        converter = ImageConverter()
        dest = temp_dir / "output.webp"

        result = converter.convert(bmp_image, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        with Image.open(dest) as img:
            assert img.format == "WEBP"

    def test_convert_jpg_to_webp(self, jpg_image: Path, temp_dir: Path) -> None:
        """JPGファイルをWebPに変換できることを確認"""
        converter = ImageConverter()
        dest = temp_dir / "output.webp"

        result = converter.convert(jpg_image, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        with Image.open(dest) as img:
            assert img.format == "WEBP"

    def test_convert_png_to_webp(self, png_image: Path, temp_dir: Path) -> None:
        """PNGファイルをWebPに変換できることを確認"""
        converter = ImageConverter()
        dest = temp_dir / "output.webp"

        result = converter.convert(png_image, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        with Image.open(dest) as img:
            assert img.format == "WEBP"

    @pytest.mark.parametrize(
        "preset, expected_quality",
        [
            pytest.param(QualityPreset.HIGH, 95, id="正常系: HIGHプリセットで品質95が適用される"),
            pytest.param(
                QualityPreset.MEDIUM,
                85,
                id="正常系: MEDIUMプリセットで品質85が適用される",
            ),
            pytest.param(QualityPreset.LOW, 70, id="正常系: LOWプリセットで品質70が適用される"),
        ],
    )
    def test_quality_preset_application(
        self, bmp_image: Path, temp_dir: Path, preset: QualityPreset, expected_quality: int
    ) -> None:
        """品質プリセットが正しく適用されることを確認"""
        converter = ImageConverter(quality=preset)

        assert converter.quality == expected_quality

        dest = temp_dir / "output.webp"
        result = converter.convert(bmp_image, dest)
        assert result.status == ConversionStatus.SUCCESS

    def test_custom_quality_value(self, bmp_image: Path, temp_dir: Path) -> None:
        """カスタム品質値（整数）が正しく適用されることを確認"""
        converter = ImageConverter(quality=50)

        assert converter.quality == 50

        dest = temp_dir / "output.webp"
        result = converter.convert(bmp_image, dest)
        assert result.status == ConversionStatus.SUCCESS

    def test_alpha_channel_preservation(self, png_with_alpha: Path, temp_dir: Path) -> None:
        """アルファチャンネルが保持されることを確認"""
        converter = ImageConverter()
        dest = temp_dir / "output.webp"

        result = converter.convert(png_with_alpha, dest)

        assert result.status == ConversionStatus.SUCCESS
        with Image.open(dest) as img:
            assert img.mode in ("RGBA", "LA")

    def test_lossless_alpha_option_true(self, png_with_alpha: Path, temp_dir: Path) -> None:
        """lossless_alpha=Trueでロスレスアルファが適用されることを確認"""
        converter = ImageConverter(lossless_alpha=True)
        dest = temp_dir / "output.webp"

        assert converter.lossless_alpha is True

        result = converter.convert(png_with_alpha, dest)
        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()

    def test_lossless_alpha_option_false(self, png_with_alpha: Path, temp_dir: Path) -> None:
        """lossless_alpha=Falseで非ロスレスアルファが適用されることを確認"""
        converter = ImageConverter(lossless_alpha=False)
        dest = temp_dir / "output.webp"

        assert converter.lossless_alpha is False

        result = converter.convert(png_with_alpha, dest)
        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()

    def test_supported_extensions(self) -> None:
        """supported_extensionsが正しい拡張子を返すことを確認"""
        converter = ImageConverter()

        expected = (".tlg", ".bmp", ".jpg", ".jpeg", ".png")
        assert converter.supported_extensions == expected

    @pytest.mark.parametrize(
        "file_path, expected",
        [
            pytest.param(Path("test.bmp"), True, id="正常系: BMPファイルは変換可能"),
            pytest.param(Path("test.jpg"), True, id="正常系: JPGファイルは変換可能"),
            pytest.param(Path("test.jpeg"), True, id="正常系: JPEGファイルは変換可能"),
            pytest.param(Path("test.png"), True, id="正常系: PNGファイルは変換可能"),
            pytest.param(Path("test.tlg"), True, id="正常系: TLGファイルは変換可能"),
            pytest.param(Path("test.gif"), False, id="異常系: GIFファイルは変換不可"),
            pytest.param(Path("test.webp"), False, id="異常系: WebPファイルは変換不可"),
            pytest.param(Path("test.txt"), False, id="異常系: TXTファイルは変換不可"),
            pytest.param(Path("TEST.BMP"), True, id="正常系: 大文字拡張子BMPも変換可能"),
            pytest.param(Path("TEST.PNG"), True, id="正常系: 大文字拡張子PNGも変換可能"),
        ],
    )
    def test_can_convert(self, file_path: Path, expected: bool) -> None:
        """can_convertが正しく判定することを確認"""
        converter = ImageConverter()
        result = converter.can_convert(file_path)
        assert result == expected

    def test_conversion_result_success_state(self, bmp_image: Path, temp_dir: Path) -> None:
        """ConversionResultが成功状態を正しく返すことを確認"""
        converter = ImageConverter()
        dest = temp_dir / "output.webp"

        result = converter.convert(bmp_image, dest)

        assert result.is_success is True
        assert result.status == ConversionStatus.SUCCESS
        assert result.source_path == bmp_image
        assert result.dest_path == dest

    def test_convert_from_image(self, temp_dir: Path) -> None:
        """convert_from_imageでPIL.ImageをWebPに変換できることを確認"""
        converter = ImageConverter()
        img = Image.new("RGB", (100, 100), color=(255, 128, 0))
        dest = temp_dir / "from_image.webp"

        result = converter.convert_from_image(img, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        with Image.open(dest) as saved_img:
            assert saved_img.format == "WEBP"

    def test_file_size_recorded_in_result(self, bmp_image: Path, temp_dir: Path) -> None:
        """変換結果にファイルサイズが記録されることを確認"""
        converter = ImageConverter()
        dest = temp_dir / "output.webp"
        bytes_before = bmp_image.stat().st_size

        result = converter.convert(bmp_image, dest)

        assert result.bytes_before == bytes_before
        assert result.bytes_after > 0
        assert result.bytes_after == dest.stat().st_size

    def test_creates_parent_directories(self, bmp_image: Path, temp_dir: Path) -> None:
        """変換先の親ディレクトリが存在しない場合に作成されることを確認"""
        converter = ImageConverter()
        dest = temp_dir / "subdir" / "nested" / "output.webp"

        result = converter.convert(bmp_image, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()

    @pytest.mark.xfail(reason="TLGImageDecoder.decodeが未実装")
    def test_convert_tlg_to_webp(self, temp_dir: Path) -> None:
        """TLGファイルをWebPに変換できることを確認（未実装のため失敗予定）"""
        tlg_path = temp_dir / "test.tlg"
        tlg_path.write_bytes(TLG5_MAGIC + b"\x00" * 100)

        converter = ImageConverter()
        dest = temp_dir / "output.webp"

        result = converter.convert(tlg_path, dest)
        assert result.status == ConversionStatus.SUCCESS
