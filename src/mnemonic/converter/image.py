"""画像変換モジュール

TLG画像形式のデコードおよび変換機能を提供する。
吉里吉里2エンジンで使用されるTLG5/TLG6形式の画像を
標準的な画像形式（PNG等）に変換する。
また、BMP/JPG/PNG形式からWebP/PNG形式への変換機能も提供する。
"""

from dataclasses import dataclass
from enum import Enum
from pathlib import Path

from PIL import Image

from mnemonic.converter.base import BaseConverter, ConversionResult, ConversionStatus
from mnemonic.converter.tlg import TLG5Decoder, TLG6Decoder


class TLGVersion(Enum):
    """TLG画像のバージョン

    吉里吉里2エンジンで使用されるTLG画像形式のバージョンを表す。
    TLG5とTLG6の2つのバージョンが存在する。
    """

    TLG5 = "TLG5"
    TLG6 = "TLG6"
    UNKNOWN = "UNKNOWN"


class QualityPreset(Enum):
    """WebP変換時の品質プリセット

    画像変換時のWebP品質値を定義する列挙型。
    HIGH/MEDIUM/LOWの3段階の品質レベルを提供する。
    """

    HIGH = 95
    MEDIUM = 85
    LOW = 70


class OutputFormat(Enum):
    """画像出力形式

    ImageConverterの出力形式を定義する列挙型。
    krkrsdl2がWebP未対応のため、PNG出力をデフォルトとする。
    """

    WEBP = "webp"
    PNG = "png"


@dataclass(frozen=True)
class TLGInfo:
    """TLG画像のメタ情報

    TLG画像ファイルから読み取ったメタ情報を保持する不変データクラス。

    Attributes:
        version: TLGバージョン（TLG5またはTLG6）
        width: 画像の幅（ピクセル）
        height: 画像の高さ（ピクセル）
        has_alpha: アルファチャンネルの有無
    """

    version: TLGVersion
    width: int
    height: int
    has_alpha: bool


class TLGImageDecoder:
    """TLG画像デコーダー

    TLG形式の画像ファイルを読み込み、PIL.Imageオブジェクトに変換する。
    TLG5およびTLG6形式に対応。
    """

    # TLGマジックバイト
    TLG5_MAGIC = b"TLG5.0\x00raw\x1a"
    TLG6_MAGIC = b"TLG6.0\x00raw\x1a"

    def __init__(self) -> None:
        """TLGImageDecoderを初期化する"""
        self._tlg5_decoder = TLG5Decoder()
        self._tlg6_decoder = TLG6Decoder()

    def is_tlg_file(self, file_path: Path) -> bool:
        """指定されたファイルがTLG形式かどうかを判定する

        ファイルのマジックバイトを読み取り、TLG5またはTLG6形式かを判定する。

        Args:
            file_path: 判定対象のファイルパス

        Returns:
            TLG形式の場合True、そうでない場合False
        """
        if not file_path.exists():
            return False

        try:
            with open(file_path, "rb") as f:
                header = f.read(len(self.TLG5_MAGIC))
                return header == self.TLG5_MAGIC or header == self.TLG6_MAGIC
        except OSError:
            return False

    def _detect_version(self, data: bytes) -> TLGVersion:
        """TLGデータのバージョンを判別する

        Args:
            data: TLG画像データ

        Returns:
            TLGバージョン
        """
        if data.startswith(self.TLG5_MAGIC):
            return TLGVersion.TLG5
        elif data.startswith(self.TLG6_MAGIC):
            return TLGVersion.TLG6
        else:
            return TLGVersion.UNKNOWN

    def get_info(self, file_path: Path) -> TLGInfo:
        """TLG画像のメタ情報を取得する

        Args:
            file_path: TLG画像ファイルのパス

        Returns:
            TLG画像のメタ情報

        Raises:
            ValueError: TLG形式でないファイルの場合
            FileNotFoundError: ファイルが存在しない場合
        """
        if not file_path.exists():
            raise FileNotFoundError(f"ファイルが見つかりません: {file_path}")

        data = file_path.read_bytes()
        version = self._detect_version(data)

        if version == TLGVersion.TLG5:
            tlg5_header = self._tlg5_decoder.parse_header(data)
            return TLGInfo(
                version=TLGVersion.TLG5,
                width=tlg5_header.width,
                height=tlg5_header.height,
                has_alpha=tlg5_header.colors == 4,
            )
        elif version == TLGVersion.TLG6:
            tlg6_header = self._tlg6_decoder.parse_header(data)
            return TLGInfo(
                version=TLGVersion.TLG6,
                width=tlg6_header.width,
                height=tlg6_header.height,
                has_alpha=tlg6_header.colors == 4,
            )
        else:
            raise ValueError(f"TLG形式ではありません: {file_path}")

    def decode(self, file_path: Path) -> Image.Image:
        """TLG画像をデコードしてPIL.Imageオブジェクトを返す

        Args:
            file_path: TLG画像ファイルのパス

        Returns:
            デコードされたPIL.Imageオブジェクト

        Raises:
            ValueError: TLG形式でないファイルの場合
            FileNotFoundError: ファイルが存在しない場合
            NotImplementedError: TLG6形式の場合（デコーダーが未実装）
        """
        if not file_path.exists():
            raise FileNotFoundError(f"ファイルが見つかりません: {file_path}")

        data = file_path.read_bytes()
        version = self._detect_version(data)

        if version == TLGVersion.TLG5:
            return self._tlg5_decoder.decode(data)
        elif version == TLGVersion.TLG6:
            # TLG6Decoder.decode()はNotImplementedErrorを投げる
            return self._tlg6_decoder.decode(data)
        else:
            raise ValueError(f"TLG形式ではありません: {file_path}")

    def decode_to_file(self, source: Path, dest: Path) -> None:
        """TLG画像をデコードしてファイルに保存する

        Args:
            source: TLG画像ファイルのパス
            dest: 出力先ファイルのパス（拡張子で出力形式を決定）

        Raises:
            ValueError: TLG形式でないファイルの場合
            FileNotFoundError: ファイルが存在しない場合
            NotImplementedError: TLG6形式の場合（デコーダーが未実装）
        """
        image = self.decode(source)
        try:
            dest.parent.mkdir(parents=True, exist_ok=True)
            image.save(dest)
        finally:
            image.close()


class ImageConverter(BaseConverter):
    """画像変換クラス

    BMP/JPG/PNG/TLG形式の画像をWebP/PNG形式に変換する。
    品質設定やアルファチャンネルの取り扱いをカスタマイズ可能。

    Attributes:
        output_format: 出力形式（WebPまたはPNG）
        quality: WebP出力時の品質値（0-100）
        lossless_alpha: アルファチャンネルをロスレスで保存するか
    """

    def __init__(
        self,
        output_format: OutputFormat = OutputFormat.PNG,
        quality: QualityPreset | int = QualityPreset.HIGH,
        lossless_alpha: bool = True,
    ) -> None:
        """ImageConverterを初期化する

        Args:
            output_format: 出力形式（デフォルトはPNG、krkrsdl2互換のため）
            quality: WebP品質（プリセットまたは0-100の整数）
            lossless_alpha: アルファチャンネルをロスレスで保存するか（WebP時のみ使用）
        """
        self._output_format = output_format
        if isinstance(quality, QualityPreset):
            self._quality = quality.value
        else:
            self._quality = quality
        self._lossless_alpha = lossless_alpha
        self._tlg_decoder = TLGImageDecoder()

    @property
    def output_format(self) -> OutputFormat:
        """出力形式を返す"""
        return self._output_format

    @property
    def quality(self) -> int:
        """WebP品質値を返す"""
        return self._quality

    @property
    def lossless_alpha(self) -> bool:
        """ロスレスアルファ設定を返す"""
        return self._lossless_alpha

    @property
    def supported_extensions(self) -> tuple[str, ...]:
        """対応する拡張子のタプルを返す

        Returns:
            対応する拡張子のタプル（.tlgのみ）
            JPEG/PNG/BMPはkrkrsdl2でネイティブサポートのため変換不要
        """
        return (".tlg",)

    def can_convert(self, file_path: Path) -> bool:
        """このConverterで変換可能なファイルかを判定する

        ファイル拡張子に基づいて判定を行う。

        Args:
            file_path: 判定対象のファイルパス

        Returns:
            変換可能な場合True、そうでない場合False
        """
        ext = file_path.suffix.lower()
        return ext in self.supported_extensions

    def convert(self, source: Path, dest: Path) -> ConversionResult:
        """画像ファイルを指定された形式に変換する

        BMP/JPG/PNG/TLG形式の画像をWebP/PNG形式に変換し、
        指定されたパスに出力する。

        Args:
            source: 変換元ファイルのパス
            dest: 変換先ファイルのパス

        Returns:
            変換結果を表すConversionResultオブジェクト
        """
        self._validate_source(source)
        bytes_before = self._get_file_size(source)

        ext = source.suffix.lower()
        # TLGファイルはTLGImageDecoderでデコード
        img = self._tlg_decoder.decode(source) if ext == ".tlg" else Image.open(source)

        try:
            if self._output_format == OutputFormat.PNG:
                result = self._save_as_png(img, dest, source, bytes_before)
            else:
                result = self._save_as_webp(img, dest, source, bytes_before)
        finally:
            img.close()

        return result

    def convert_from_image(self, image: Image.Image, dest: Path) -> ConversionResult:
        """PIL.Imageオブジェクトを指定形式で保存する

        既にメモリ上にある画像オブジェクトを直接WebP/PNG形式で保存する。
        TLGデコード後の画像変換等に使用。

        Args:
            image: 変換元のPIL.Imageオブジェクト
            dest: 変換先ファイルのパス

        Returns:
            変換結果を表すConversionResultオブジェクト
        """
        # PIL.Imageからの変換はsource_pathが無いのでdestをダミーとして使用
        if self._output_format == OutputFormat.PNG:
            return self._save_as_png(image, dest, dest, bytes_before=0)
        else:
            return self._save_as_webp(image, dest, dest, bytes_before=0)

    def _save_as_webp(
        self,
        image: Image.Image,
        dest: Path,
        source: Path,
        bytes_before: int,
    ) -> ConversionResult:
        """画像をWebP形式で保存する内部メソッド

        Args:
            image: 保存するPIL.Imageオブジェクト
            dest: 保存先パス
            source: 変換元パス（結果記録用）
            bytes_before: 変換前のファイルサイズ

        Returns:
            変換結果
        """
        dest.parent.mkdir(parents=True, exist_ok=True)

        has_alpha = image.mode in ("RGBA", "LA", "PA")

        if has_alpha and self._lossless_alpha:
            # アルファチャンネル付きでロスレスアルファ
            image.save(dest, "WEBP", quality=self._quality, lossless=True)
        elif has_alpha:
            # アルファチャンネル付きだがロスレスでない
            image.save(dest, "WEBP", quality=self._quality)
        else:
            # アルファチャンネルなし→RGB変換
            if image.mode != "RGB":
                image = image.convert("RGB")
            image.save(dest, "WEBP", quality=self._quality)

        bytes_after = self._get_file_size(dest)

        return ConversionResult(
            source_path=source,
            dest_path=dest,
            status=ConversionStatus.SUCCESS,
            bytes_before=bytes_before,
            bytes_after=bytes_after,
        )

    def _save_as_png(
        self,
        image: Image.Image,
        dest: Path,
        source: Path,
        bytes_before: int,
    ) -> ConversionResult:
        """画像をPNG形式で保存する内部メソッド

        PNG形式はロスレス圧縮のため、品質設定は使用されない。

        Args:
            image: 保存するPIL.Imageオブジェクト
            dest: 保存先パス
            source: 変換元パス（結果記録用）
            bytes_before: 変換前のファイルサイズ

        Returns:
            変換結果
        """
        dest.parent.mkdir(parents=True, exist_ok=True)

        # PNG形式で保存（ロスレス）
        # アルファチャンネルがある場合はRGBA、ない場合はRGB
        if image.mode not in ("RGB", "RGBA", "L", "LA", "P", "PA"):
            # 他のモードはRGB/RGBAに変換
            if "A" in image.mode or image.mode == "PA":
                image = image.convert("RGBA")
            else:
                image = image.convert("RGB")

        image.save(dest, "PNG")

        bytes_after = self._get_file_size(dest)

        return ConversionResult(
            source_path=source,
            dest_path=dest,
            status=ConversionStatus.SUCCESS,
            bytes_before=bytes_before,
            bytes_after=bytes_after,
        )
