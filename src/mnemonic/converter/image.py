"""画像変換モジュール

TLG画像形式のデコードおよび変換機能を提供する。
吉里吉里2エンジンで使用されるTLG5/TLG6形式の画像を
標準的な画像形式（PNG等）に変換する。
"""

from dataclasses import dataclass
from enum import Enum
from pathlib import Path

from PIL import Image

class TLGVersion(Enum):
    """TLG画像のバージョン

    吉里吉里2エンジンで使用されるTLG画像形式のバージョンを表す。
    TLG5とTLG6の2つのバージョンが存在する。
    """

    TLG5 = "TLG5"
    TLG6 = "TLG6"
    UNKNOWN = "UNKNOWN"

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
