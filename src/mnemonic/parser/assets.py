"""アセットファイル一覧機能モジュール"""

from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path

class AssetType(Enum):
    """アセットの種別

    ゲームアセットの種類を分類するための列挙型。
    スクリプト、画像、音声、動画、その他に分類される。
    """

    SCRIPT = "script"
    IMAGE = "image"
    AUDIO = "audio"
    VIDEO = "video"
    OTHER = "other"

class ConversionAction(Enum):
    """変換アクション

    アセットファイルに対して実行する変換処理の種類を定義する列挙型。
    """

    ENCODE_UTF8 = "encode_utf8"
    CONVERT_WEBP = "convert_webp"
    CONVERT_OGG = "convert_ogg"
    CONVERT_MP4 = "convert_mp4"
    COPY = "copy"
    SKIP = "skip"

@dataclass(frozen=True)
class AssetFile:
    """アセットファイル情報

    単一のアセットファイルに関するメタデータを保持する不変データクラス。

    Attributes:
        path: アセットファイルのパス
        asset_type: アセットの種別
        action: 実行する変換アクション
        source_format: 元ファイルのフォーマット（拡張子）
        target_format: 変換後のフォーマット（変換しない場合はNone）
    """

    path: Path
    asset_type: AssetType
    action: ConversionAction
    source_format: str
    target_format: str | None

@dataclass
class AssetManifest:
    """アセット一覧（マニフェスト）

    ゲームディレクトリ内の全アセットファイル情報を管理するデータクラス。

    Attributes:
        game_dir: ゲームディレクトリのパス
        files: アセットファイル情報のリスト
    """

    game_dir: Path
    files: list[AssetFile] = field(default_factory=list)

    def filter_by_type(self, asset_type: AssetType) -> list[AssetFile]:
        """指定種別のファイルのみ取得する

        Args:
            asset_type: フィルタリングするアセット種別

        Returns:
            指定された種別に一致するアセットファイルのリスト
        """
        raise NotImplementedError()

    def filter_by_action(self, action: ConversionAction) -> list[AssetFile]:
        """指定アクションのファイルのみ取得する

        Args:
            action: フィルタリングする変換アクション

        Returns:
            指定されたアクションに一致するアセットファイルのリスト
        """
        raise NotImplementedError()

    def get_summary(self) -> dict[AssetType, int]:
        """種別ごとのファイル数を取得する

        Returns:
            アセット種別をキー、ファイル数を値とする辞書
        """
        raise NotImplementedError()

class AssetScanner:
    """アセットをスキャンしてマニフェストを生成するクラス

    ゲームディレクトリ内のアセットファイルを走査し、
    各ファイルの種別と変換アクションを判定してマニフェストを生成する。
    """

    def __init__(self, game_dir: Path, config: dict | None = None) -> None:
        """ゲームディレクトリと設定を指定して初期化する

        Args:
            game_dir: スキャン対象のゲームディレクトリ
            config: スキャン設定（省略可）
        """
        raise NotImplementedError()

    def scan(self) -> AssetManifest:
        """アセットをスキャンしてマニフェストを返す

        ゲームディレクトリ内のすべてのアセットファイルを走査し、
        それぞれに対して種別と変換アクションを判定する。

        Returns:
            生成されたアセットマニフェスト
        """
        raise NotImplementedError()
