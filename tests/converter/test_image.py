"""TLGImageDecoderおよびImageConverterのテスト"""

import tempfile
from pathlib import Path

import pytest
from PIL import Image

from mnemonic.converter.base import ConversionStatus
from mnemonic.converter.image import (
    ImageConverter,
    OutputFormat,
    QualityPreset,
    TLGImageDecoder,
    TLGVersion,
)

# TLGマジックバイト定数
TLG5_MAGIC = b"TLG5.0\x00raw\x1a"
TLG6_MAGIC = b"TLG6.0\x00raw\x1a"


def create_tlg5_test_data(
    *,
    width: int = 2,
    height: int = 2,
    color_depth: int = 32,
    block_height: int = 2,
    r: int = 255,
    g: int = 128,
    b: int = 64,
    a: int = 255,
) -> bytes:
    """テスト用のTLG5データを生成する

    単色の画像データを生成する。

    TLG5フォーマット:
    1. ヘッダー (24バイト)
    2. ブロックサイズ配列 (block_count * 4バイト)
    3. ブロックデータ (各ブロック内に colors個の mark+size+data)

    Args:
        width: 画像の幅
        height: 画像の高さ
        color_depth: 色深度（24=RGB、32=RGBA）
        block_height: ブロックの高さ
        r, g, b, a: 画素値

    Returns:
        TLG5形式のバイト列
    """
    import struct

    # ヘッダー作成
    header = TLG5_MAGIC
    header += struct.pack("B", color_depth)
    header += struct.pack("<I", width)
    header += struct.pack("<I", height)
    header += struct.pack("<I", block_height)

    # チャンネルデータ作成（デルタエンコーディング済み）
    # 全ピクセル同じ色: 最初のピクセルのみ値、残りは0
    def create_channel_data(value: int, size: int) -> bytes:
        data = bytearray(size)
        data[0] = value
        return bytes(data)

    def create_channel_block(channel_data: bytes) -> bytes:
        # LZSS非圧縮形式（全リテラル）: フラグビット0=リテラル
        compressed = bytearray()
        pos = 0
        while pos < len(channel_data):
            chunk_size = min(8, len(channel_data) - pos)
            # 全ビット0 = 全てリテラル
            flag = 0
            compressed.append(flag)
            compressed.extend(channel_data[pos : pos + chunk_size])
            pos += chunk_size

        mark = 0
        block_size = len(compressed)
        return bytes([mark]) + struct.pack("<I", block_size) + bytes(compressed)

    block_count = (height + block_height - 1) // block_height
    channels = 4 if color_depth == 32 else 3
    channel_values = [b, g, r, a] if channels == 4 else [b, g, r]

    # 各ブロックのデータを先に生成
    block_data_list: list[bytes] = []
    for block_idx in range(block_count):
        block_rows = min(block_height, height - block_idx * block_height)
        pixel_count = width * block_rows

        block_bytes = bytearray()
        for _i, value in enumerate(channel_values):
            if block_idx == 0:
                channel = create_channel_data(value, pixel_count)
            else:
                # 2番目以降のブロックは差分0
                channel = bytes(pixel_count)
            block_bytes.extend(create_channel_block(channel))
        block_data_list.append(bytes(block_bytes))

    # ブロックサイズ配列を作成
    block_sizes = bytearray()
    for block_bytes in block_data_list:
        block_sizes.extend(struct.pack("<I", len(block_bytes)))

    # すべてを結合
    block_data = b"".join(block_data_list)
    return header + bytes(block_sizes) + block_data


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

    @pytest.fixture
    def tlg5_file(self) -> bytes:
        """TLG5テストデータを作成するフィクスチャ

        最小限のTLG5ヘッダーとダミーデータを含む。
        色深度: 24(RGB), サイズ: 4x2, ブロック高さ: 2
        """
        # TLG5ヘッダー構造:
        # マジック(11) + 色深度(1) + width(4) + height(4) + block_height(4)
        header = TLG5_MAGIC
        header += bytes([24])  # 色深度24=RGB
        header += (4).to_bytes(4, "little")  # width=4
        header += (2).to_bytes(4, "little")  # height=2
        header += (2).to_bytes(4, "little")  # block_height=2
        # ダミーブロックデータ（3チャンネル分）
        for _ in range(3):
            header += bytes([0])  # mark
            header += (8).to_bytes(4, "little")  # block_size=8
            header += bytes([0, 0, 0, 0, 0, 0, 0, 0])  # ダミーデータ
        return header

    @pytest.fixture
    def tlg6_file(self) -> bytes:
        """TLG6テストデータを作成するフィクスチャ"""
        # TLG6ヘッダー構造:
        # マジック(11) + 色深度(1) + フラグ(1) + width(4) + height(4) + x_block(4) + y_block(4)
        header = TLG6_MAGIC
        header += bytes([32])  # 色深度32=RGBA
        header += bytes([0])  # フラグ
        header += (8).to_bytes(4, "little")  # width=8
        header += (4).to_bytes(4, "little")  # height=4
        header += (1).to_bytes(4, "little")  # x_block_count=1
        header += (1).to_bytes(4, "little")  # y_block_count=1
        return header

    def test_get_info_tlg5(self, tlg5_file: bytes) -> None:
        """TLG5ファイルのメタ情報を取得できることを確認"""
        decoder = TLGImageDecoder()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(tlg5_file)
            temp_path = Path(f.name)

        try:
            info = decoder.get_info(temp_path)
            assert info.version == TLGVersion.TLG5
            assert info.width == 4
            assert info.height == 2
            assert info.has_alpha is False
        finally:
            temp_path.unlink()

    def test_get_info_tlg6(self, tlg6_file: bytes) -> None:
        """TLG6ファイルのメタ情報を取得できることを確認"""
        decoder = TLGImageDecoder()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(tlg6_file)
            temp_path = Path(f.name)

        try:
            info = decoder.get_info(temp_path)
            assert info.version == TLGVersion.TLG6
            assert info.width == 8
            assert info.height == 4
            assert info.has_alpha is True
        finally:
            temp_path.unlink()

    def test_get_info_file_not_found(self) -> None:
        """存在しないファイルに対してFileNotFoundErrorを発生させることを確認"""
        decoder = TLGImageDecoder()
        nonexistent_path = Path("/nonexistent/path/to/file.tlg")

        with pytest.raises(FileNotFoundError):
            decoder.get_info(nonexistent_path)

    def test_get_info_invalid_format(self) -> None:
        """TLG形式でないファイルに対してValueErrorを発生させることを確認"""
        decoder = TLGImageDecoder()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(b"NOT_TLG_FORMAT" + b"\x00" * 100)
            temp_path = Path(f.name)

        try:
            with pytest.raises(ValueError, match="TLG形式ではありません"):
                decoder.get_info(temp_path)
        finally:
            temp_path.unlink()


class TestDecode:
    """decodeメソッドのテスト"""

    def test_decode_tlg5(self) -> None:
        """TLG5ファイルをデコードできることを確認"""
        decoder = TLGImageDecoder()
        tlg5_data = create_tlg5_test_data(r=255, g=128, b=64, a=255)

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(tlg5_data)
            temp_path = Path(f.name)

        try:
            image = decoder.decode(temp_path)
            assert image.width == 2
            assert image.height == 2
            assert image.mode == "RGBA"

            # ピクセル値を検証
            for y in range(2):
                for x in range(2):
                    pixel = image.getpixel((x, y))
                    assert pixel == (255, 128, 64, 255)

            image.close()
        finally:
            temp_path.unlink()

    def test_decode_tlg6_raises_not_implemented(self) -> None:
        """TLG6ファイルのデコードがNotImplementedErrorを発生させることを確認"""
        decoder = TLGImageDecoder()

        # 最小限のTLG6ヘッダー
        header = TLG6_MAGIC
        header += bytes([32])  # 色深度
        header += bytes([0])  # フラグ
        header += (8).to_bytes(4, "little")  # width
        header += (4).to_bytes(4, "little")  # height
        header += (1).to_bytes(4, "little")  # x_block
        header += (1).to_bytes(4, "little")  # y_block

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(header)
            temp_path = Path(f.name)

        try:
            with pytest.raises(NotImplementedError, match="TLG6Decoder.decode"):
                decoder.decode(temp_path)
        finally:
            temp_path.unlink()

    def test_decode_file_not_found(self) -> None:
        """存在しないファイルに対してFileNotFoundErrorを発生させることを確認"""
        decoder = TLGImageDecoder()
        nonexistent_path = Path("/nonexistent/path/to/file.tlg")

        with pytest.raises(FileNotFoundError):
            decoder.decode(nonexistent_path)

    def test_decode_invalid_format(self) -> None:
        """TLG形式でないファイルに対してValueErrorを発生させることを確認"""
        decoder = TLGImageDecoder()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".tlg") as f:
            f.write(b"NOT_TLG_FORMAT" + b"\x00" * 100)
            temp_path = Path(f.name)

        try:
            with pytest.raises(ValueError, match="TLG形式ではありません"):
                decoder.decode(temp_path)
        finally:
            temp_path.unlink()


class TestDecodeToFile:
    """decode_to_fileメソッドのテスト"""

    def test_decode_to_file_png(self) -> None:
        """decode_to_fileでPNGファイルに保存できることを確認"""
        decoder = TLGImageDecoder()
        tlg5_data = create_tlg5_test_data(r=255, g=128, b=64, a=255)

        with tempfile.TemporaryDirectory() as td:
            temp_dir = Path(td)
            source = temp_dir / "test.tlg"
            source.write_bytes(tlg5_data)

            dest = temp_dir / "output.png"
            decoder.decode_to_file(source, dest)

            assert dest.exists()
            with Image.open(dest) as img:
                assert img.format == "PNG"
                assert img.width == 2
                assert img.height == 2

    def test_decode_to_file_creates_parent_directories(self) -> None:
        """decode_to_fileで親ディレクトリが作成されることを確認"""
        decoder = TLGImageDecoder()
        tlg5_data = create_tlg5_test_data()

        with tempfile.TemporaryDirectory() as td:
            temp_dir = Path(td)
            source = temp_dir / "test.tlg"
            source.write_bytes(tlg5_data)

            dest = temp_dir / "subdir" / "nested" / "output.png"
            decoder.decode_to_file(source, dest)

            assert dest.exists()

    def test_decode_to_file_file_not_found(self) -> None:
        """存在しないソースファイルに対してFileNotFoundErrorを発生させることを確認"""
        decoder = TLGImageDecoder()

        with pytest.raises(FileNotFoundError):
            decoder.decode_to_file(
                Path("/nonexistent/source.tlg"),
                Path("/tmp/output.png"),
            )


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


class TestOutputFormat:
    """OutputFormat列挙型のテスト"""

    def test_output_format_values(self) -> None:
        """OutputFormatの値が正しいことを確認"""
        assert OutputFormat.WEBP.value == "webp"
        assert OutputFormat.PNG.value == "png"


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

    def test_default_output_format_is_png(self) -> None:
        """デフォルトの出力形式がPNGであることを確認"""
        converter = ImageConverter()
        assert converter.output_format == OutputFormat.PNG

    def test_output_format_webp(self) -> None:
        """WebP出力形式を指定できることを確認"""
        converter = ImageConverter(output_format=OutputFormat.WEBP)
        assert converter.output_format == OutputFormat.WEBP

    @pytest.mark.parametrize(
        "output_format, expected_ext",
        [
            pytest.param(OutputFormat.PNG, ".png", id="正常系: PNG出力形式は.pngを返す"),
            pytest.param(OutputFormat.WEBP, ".webp", id="正常系: WebP出力形式は.webpを返す"),
        ],
    )
    def test_get_output_extension(self, output_format: OutputFormat, expected_ext: str) -> None:
        """出力形式に応じた拡張子を返すことを確認"""
        converter = ImageConverter(output_format=output_format)
        result = converter.get_output_extension(Path("test.tlg"))
        assert result == expected_ext

    def test_convert_bmp_to_png(self, bmp_image: Path, temp_dir: Path) -> None:
        """BMPファイルをPNGに変換できることを確認（デフォルト）"""
        converter = ImageConverter()
        dest = temp_dir / "output.png"

        result = converter.convert(bmp_image, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        with Image.open(dest) as img:
            assert img.format == "PNG"

    def test_convert_bmp_to_webp(self, bmp_image: Path, temp_dir: Path) -> None:
        """BMPファイルをWebPに変換できることを確認"""
        converter = ImageConverter(output_format=OutputFormat.WEBP)
        dest = temp_dir / "output.webp"

        result = converter.convert(bmp_image, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        with Image.open(dest) as img:
            assert img.format == "WEBP"

    def test_convert_jpg_to_png(self, jpg_image: Path, temp_dir: Path) -> None:
        """JPGファイルをPNGに変換できることを確認"""
        converter = ImageConverter()
        dest = temp_dir / "output.png"

        result = converter.convert(jpg_image, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        with Image.open(dest) as img:
            assert img.format == "PNG"

    def test_convert_jpg_to_webp(self, jpg_image: Path, temp_dir: Path) -> None:
        """JPGファイルをWebPに変換できることを確認"""
        converter = ImageConverter(output_format=OutputFormat.WEBP)
        dest = temp_dir / "output.webp"

        result = converter.convert(jpg_image, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        with Image.open(dest) as img:
            assert img.format == "WEBP"

    def test_convert_png_to_webp(self, png_image: Path, temp_dir: Path) -> None:
        """PNGファイルをWebPに変換できることを確認"""
        converter = ImageConverter(output_format=OutputFormat.WEBP)
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
        converter = ImageConverter(output_format=OutputFormat.WEBP, quality=preset)

        assert converter.quality == expected_quality

        dest = temp_dir / "output.webp"
        result = converter.convert(bmp_image, dest)
        assert result.status == ConversionStatus.SUCCESS

    def test_custom_quality_value(self, bmp_image: Path, temp_dir: Path) -> None:
        """カスタム品質値（整数）が正しく適用されることを確認"""
        converter = ImageConverter(output_format=OutputFormat.WEBP, quality=50)

        assert converter.quality == 50

        dest = temp_dir / "output.webp"
        result = converter.convert(bmp_image, dest)
        assert result.status == ConversionStatus.SUCCESS

    def test_alpha_channel_preservation_png(self, png_with_alpha: Path, temp_dir: Path) -> None:
        """PNG出力でアルファチャンネルが保持されることを確認"""
        converter = ImageConverter()
        dest = temp_dir / "output.png"

        result = converter.convert(png_with_alpha, dest)

        assert result.status == ConversionStatus.SUCCESS
        with Image.open(dest) as img:
            assert img.mode == "RGBA"

    def test_alpha_channel_preservation_webp(self, png_with_alpha: Path, temp_dir: Path) -> None:
        """WebP出力でアルファチャンネルが保持されることを確認"""
        converter = ImageConverter(output_format=OutputFormat.WEBP)
        dest = temp_dir / "output.webp"

        result = converter.convert(png_with_alpha, dest)

        assert result.status == ConversionStatus.SUCCESS
        with Image.open(dest) as img:
            assert img.mode in ("RGBA", "LA")

    def test_lossless_alpha_option_true(self, png_with_alpha: Path, temp_dir: Path) -> None:
        """lossless_alpha=Trueでロスレスアルファが適用されることを確認"""
        converter = ImageConverter(output_format=OutputFormat.WEBP, lossless_alpha=True)
        dest = temp_dir / "output.webp"

        assert converter.lossless_alpha is True

        result = converter.convert(png_with_alpha, dest)
        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()

    def test_lossless_alpha_option_false(self, png_with_alpha: Path, temp_dir: Path) -> None:
        """lossless_alpha=Falseで非ロスレスアルファが適用されることを確認"""
        converter = ImageConverter(output_format=OutputFormat.WEBP, lossless_alpha=False)
        dest = temp_dir / "output.webp"

        assert converter.lossless_alpha is False

        result = converter.convert(png_with_alpha, dest)
        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()

    def test_supported_extensions(self) -> None:
        """supported_extensionsが正しい拡張子を返すことを確認"""
        converter = ImageConverter()

        # TLGのみ変換対象（JPEG/PNG/BMPはkrkrsdl2でネイティブサポート）
        expected = (".tlg",)
        assert converter.supported_extensions == expected

    @pytest.mark.parametrize(
        "file_path, expected",
        [
            pytest.param(Path("test.tlg"), True, id="正常系: TLGファイルは変換可能"),
            pytest.param(Path("TEST.TLG"), True, id="正常系: 大文字拡張子TLGも変換可能"),
            # JPEG/PNG/BMPはkrkrsdl2でネイティブサポートのため変換不可（変換不要）
            pytest.param(Path("test.bmp"), False, id="正常系: BMPファイルは変換不可"),
            pytest.param(Path("test.jpg"), False, id="正常系: JPGファイルは変換不可"),
            pytest.param(Path("test.jpeg"), False, id="正常系: JPEGファイルは変換不可"),
            pytest.param(Path("test.png"), False, id="正常系: PNGファイルは変換不可"),
            pytest.param(Path("test.gif"), False, id="異常系: GIFファイルは変換不可"),
            pytest.param(Path("test.webp"), False, id="異常系: WebPファイルは変換不可"),
            pytest.param(Path("test.txt"), False, id="異常系: TXTファイルは変換不可"),
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
        dest = temp_dir / "output.png"

        result = converter.convert(bmp_image, dest)

        assert result.is_success is True
        assert result.status == ConversionStatus.SUCCESS
        assert result.source_path == bmp_image
        assert result.dest_path == dest

    def test_convert_from_image_png(self, temp_dir: Path) -> None:
        """convert_from_imageでPIL.ImageをPNGに変換できることを確認"""
        converter = ImageConverter()
        img = Image.new("RGB", (100, 100), color=(255, 128, 0))
        dest = temp_dir / "from_image.png"

        result = converter.convert_from_image(img, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        with Image.open(dest) as saved_img:
            assert saved_img.format == "PNG"

    def test_convert_from_image_webp(self, temp_dir: Path) -> None:
        """convert_from_imageでPIL.ImageをWebPに変換できることを確認"""
        converter = ImageConverter(output_format=OutputFormat.WEBP)
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
        dest = temp_dir / "output.png"
        bytes_before = bmp_image.stat().st_size

        result = converter.convert(bmp_image, dest)

        assert result.bytes_before == bytes_before
        assert result.bytes_after > 0
        assert result.bytes_after == dest.stat().st_size

    def test_creates_parent_directories(self, bmp_image: Path, temp_dir: Path) -> None:
        """変換先の親ディレクトリが存在しない場合に作成されることを確認"""
        converter = ImageConverter()
        dest = temp_dir / "subdir" / "nested" / "output.png"

        result = converter.convert(bmp_image, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()

    def test_convert_tlg_to_png(self, temp_dir: Path) -> None:
        """TLGファイルをPNGに変換できることを確認"""
        tlg5_data = create_tlg5_test_data(r=255, g=128, b=64, a=255)
        tlg_path = temp_dir / "test.tlg"
        tlg_path.write_bytes(tlg5_data)

        converter = ImageConverter()
        dest = temp_dir / "output.png"

        result = converter.convert(tlg_path, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        with Image.open(dest) as img:
            assert img.format == "PNG"
            assert img.mode == "RGBA"

    def test_convert_tlg_to_webp(self, temp_dir: Path) -> None:
        """TLGファイルをWebPに変換できることを確認"""
        tlg5_data = create_tlg5_test_data(r=255, g=128, b=64, a=255)
        tlg_path = temp_dir / "test.tlg"
        tlg_path.write_bytes(tlg5_data)

        converter = ImageConverter(output_format=OutputFormat.WEBP)
        dest = temp_dir / "output.webp"

        result = converter.convert(tlg_path, dest)

        assert result.status == ConversionStatus.SUCCESS
        assert dest.exists()
        with Image.open(dest) as img:
            assert img.format == "WEBP"
