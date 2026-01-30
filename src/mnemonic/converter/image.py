"""画像変換モジュール

TLG画像形式のデコードおよび変換機能を提供する。
吉里吉里2エンジンで使用されるTLG5/TLG6形式の画像を
標準的な画像形式（PNG等）に変換する。
また、BMP/JPG/PNG形式からWebP形式への変換機能も提供する。
"""

from dataclasses import dataclass
from enum import Enum
from pathlib import Path

from PIL import Image

from mnemonic.converter.base import BaseConverter, ConversionResult

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
        raise NotImplementedError

    def decode(self, file_path: Path) -> Image.Image:
        """TLG画像をデコードしてPIL.Imageオブジェクトを返す

        Args:
            file_path: TLG画像ファイルのパス

        Returns:
            デコードされたPIL.Imageオブジェクト

        Raises:
            ValueError: TLG形式でないファイルの場合
            FileNotFoundError: ファイルが存在しない場合
        """
        raise NotImplementedError

    def decode_to_file(self, source: Path, dest: Path) -> None:
        """TLG画像をデコードしてファイルに保存する

        Args:
            source: TLG画像ファイルのパス
            dest: 出力先ファイルのパス（拡張子で出力形式を決定）

        Raises:
            ValueError: TLG形式でないファイルの場合
            FileNotFoundError: ファイルが存在しない場合
        """
        raise NotImplementedError

class ImageConverter(BaseConverter):
    """画像変換クラス

    BMP/JPG/PNG/TLG形式の画像をWebP形式に変換する。
    品質設定やアルファチャンネルの取り扱いをカスタマイズ可能。

    Attributes:
        quality: WebP出力時の品質値（0-100）
        lossless_alpha: アルファチャンネルをロスレスで保存するか
    """

    def __init__(
        self,
        quality: QualityPreset | int = QualityPreset.HIGH,
        lossless_alpha: bool = True,
    ) -> None:
        """ImageConverterを初期化する

        Args:
            quality: WebP品質（プリセットまたは0-100の整数）
            lossless_alpha: アルファチャンネルをロスレスで保存するか
        """
        if isinstance(quality, QualityPreset):
            self._quality = quality.value
        else:
            self._quality = quality
        self._lossless_alpha = lossless_alpha
        self._tlg_decoder = TLGImageDecoder()

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
            対応する拡張子のタプル（.tlg, .bmp, .jpg, .jpeg, .png）
        """
        return (".tlg", ".bmp", ".jpg", ".jpeg", ".png")

    def can_convert(self, file_path: Path) -> bool:
        """このConverterで変換可能なファイルかを判定する

        ファイル拡張子に基づいて判定を行う。

        Args:
            file_path: 判定対象のファイルパス

        Returns:
            変換可能な場合True、そうでない場合False
        """
        raise NotImplementedError

    def convert(self, source: Path, dest: Path) -> ConversionResult:
        """画像ファイルをWebP形式に変換する

        BMP/JPG/PNG/TLG形式の画像をWebP形式に変換し、
        指定されたパスに出力する。

        Args:
            source: 変換元ファイルのパス
            dest: 変換先ファイルのパス

        Returns:
            変換結果を表すConversionResultオブジェクト
        """
        raise NotImplementedError

    def convert_from_image(self, image: Image.Image, dest: Path) -> ConversionResult:
        """PIL.ImageオブジェクトをWebP形式で保存する

        既にメモリ上にある画像オブジェクトを直接WebP形式で保存する。
        TLGデコード後の画像変換等に使用。

        Args:
            image: 変換元のPIL.Imageオブジェクト
            dest: 変換先ファイルのパス

        Returns:
            変換結果を表すConversionResultオブジェクト
        """
        raise NotImplementedError
