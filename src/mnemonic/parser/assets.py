"""アセットファイル一覧機能モジュール"""

import fnmatch
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Any


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
    CONVERT_PNG = "convert_png"
    CONVERT_WEBP = "convert_webp"
    CONVERT_OGG = "convert_ogg"
    CONVERT_MP4 = "convert_mp4"
    COPY = "copy"
    SKIP = "skip"


# 拡張子からアセット種別へのマッピング
_EXTENSION_TO_TYPE: dict[str, AssetType] = {
    # スクリプト
    ".ks": AssetType.SCRIPT,
    ".tjs": AssetType.SCRIPT,
    # 画像
    ".tlg": AssetType.IMAGE,
    ".bmp": AssetType.IMAGE,
    ".jpg": AssetType.IMAGE,
    ".jpeg": AssetType.IMAGE,
    ".png": AssetType.IMAGE,
    # 音声
    ".wav": AssetType.AUDIO,
    ".ogg": AssetType.AUDIO,
    ".mp3": AssetType.AUDIO,
    # 動画
    ".mpg": AssetType.VIDEO,
    ".mpeg": AssetType.VIDEO,
    ".wmv": AssetType.VIDEO,
    ".avi": AssetType.VIDEO,
}

# 拡張子から変換アクションへのマッピング
_EXTENSION_TO_ACTION: dict[str, ConversionAction] = {
    # スクリプト -> UTF-8エンコード
    ".ks": ConversionAction.ENCODE_UTF8,
    ".tjs": ConversionAction.ENCODE_UTF8,
    # 画像
    # TLGのみ変換（krkrsdl2がTLG未対応のため）
    # JPEG/PNG/BMPはkrkrsdl2でネイティブサポート
    ".tlg": ConversionAction.CONVERT_PNG,
    ".bmp": ConversionAction.COPY,
    ".jpg": ConversionAction.COPY,
    ".jpeg": ConversionAction.COPY,
    ".png": ConversionAction.COPY,
    # 音声
    ".wav": ConversionAction.CONVERT_OGG,
    ".ogg": ConversionAction.COPY,
    ".mp3": ConversionAction.COPY,
    # 動画 -> MP4変換
    ".mpg": ConversionAction.CONVERT_MP4,
    ".mpeg": ConversionAction.CONVERT_MP4,
    ".wmv": ConversionAction.CONVERT_MP4,
    ".avi": ConversionAction.CONVERT_MP4,
}

# 拡張子から変換後フォーマットへのマッピング
_EXTENSION_TO_TARGET: dict[str, str | None] = {
    # スクリプトはフォーマット変換なし
    ".ks": None,
    ".tjs": None,
    # 画像
    # TLGのみPNGに変換（krkrsdl2がTLG未対応のため）
    # JPEG/PNG/BMPは変換不要（krkrsdl2でネイティブサポート）
    ".tlg": ".png",
    ".bmp": None,
    ".jpg": None,
    ".jpeg": None,
    ".png": None,
    # 音声
    ".wav": ".ogg",
    ".ogg": None,
    ".mp3": None,
    # 動画 -> MP4
    ".mpg": ".mp4",
    ".mpeg": ".mp4",
    ".wmv": ".mp4",
    ".avi": ".mp4",
}

# コンバータ名からConversionActionへのマッピング
_CONVERTER_TO_ACTION: dict[str, ConversionAction] = {
    "encode_utf8": ConversionAction.ENCODE_UTF8,
    "convert_png": ConversionAction.CONVERT_PNG,
    "convert_webp": ConversionAction.CONVERT_WEBP,
    "convert_ogg": ConversionAction.CONVERT_OGG,
    "convert_mp4": ConversionAction.CONVERT_MP4,
    "copy": ConversionAction.COPY,
    "skip": ConversionAction.SKIP,
}


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
        return [f for f in self.files if f.asset_type == asset_type]

    def filter_by_action(self, action: ConversionAction) -> list[AssetFile]:
        """指定アクションのファイルのみ取得する

        Args:
            action: フィルタリングする変換アクション

        Returns:
            指定されたアクションに一致するアセットファイルのリスト
        """
        return [f for f in self.files if f.action == action]

    def get_summary(self) -> dict[AssetType, int]:
        """種別ごとのファイル数を取得する

        Returns:
            アセット種別をキー、ファイル数を値とする辞書
        """
        summary: dict[AssetType, int] = {}
        for asset_type in AssetType:
            count = len([f for f in self.files if f.asset_type == asset_type])
            if count > 0:
                summary[asset_type] = count
        return summary


class AssetScanner:
    """アセットをスキャンしてマニフェストを生成するクラス

    ゲームディレクトリ内のアセットファイルを走査し、
    各ファイルの種別と変換アクションを判定してマニフェストを生成する。
    """

    def __init__(self, game_dir: Path, config: dict[str, Any] | None = None) -> None:
        """ゲームディレクトリと設定を指定して初期化する

        Args:
            game_dir: スキャン対象のゲームディレクトリ
            config: スキャン設定（省略可）

        Raises:
            FileNotFoundError: ゲームディレクトリが存在しない場合
        """
        if not game_dir.exists():
            raise FileNotFoundError(f"ゲームディレクトリが見つかりません: {game_dir}")

        self._game_dir = game_dir
        self._config = config or {}
        self._exclude_patterns: list[str] = self._config.get("exclude", [])
        self._conversion_rules: list[dict[str, str]] = self._config.get("conversion_rules", [])

    def scan(self) -> AssetManifest:
        """アセットをスキャンしてマニフェストを返す

        ゲームディレクトリ内のすべてのアセットファイルを走査し、
        それぞれに対して種別と変換アクションを判定する。

        Returns:
            生成されたアセットマニフェスト
        """
        files: list[AssetFile] = []

        for file_path in self._game_dir.rglob("*"):
            if not file_path.is_file():
                continue

            # 隠しファイルは除外
            if file_path.name.startswith("."):
                continue

            # 相対パス取得
            relative_path = file_path.relative_to(self._game_dir)

            # exclude設定でスキップ判定
            if self._should_exclude(relative_path):
                continue

            # ファイル情報を作成
            asset_file = self._classify_file(relative_path)
            files.append(asset_file)

        return AssetManifest(game_dir=self._game_dir, files=files)

    def _should_exclude(self, relative_path: Path) -> bool:
        """ファイルを除外すべきかどうか判定する

        Args:
            relative_path: ゲームディレクトリからの相対パス

        Returns:
            除外する場合True
        """
        path_str = str(relative_path)
        for pattern in self._exclude_patterns:
            if fnmatch.fnmatch(path_str, pattern):
                return True
            if fnmatch.fnmatch(relative_path.name, pattern):
                return True
        return False

    def _get_conversion_rule_override(self, relative_path: Path) -> ConversionAction | None:
        """conversion_rulesによる変換ルールの上書きを取得する

        Args:
            relative_path: ゲームディレクトリからの相対パス

        Returns:
            上書きされた変換アクション、上書きなしの場合None
        """
        path_str = str(relative_path)
        for rule in self._conversion_rules:
            pattern = rule.get("pattern", "")
            converter = rule.get("converter", "")
            if fnmatch.fnmatch(path_str, pattern):
                return _CONVERTER_TO_ACTION.get(converter)
        return None

    def _classify_file(self, relative_path: Path) -> AssetFile:
        """ファイルを分類してAssetFileを生成する

        Args:
            relative_path: ゲームディレクトリからの相対パス

        Returns:
            分類されたAssetFile
        """
        extension = relative_path.suffix.lower()

        # デフォルトの分類
        asset_type = _EXTENSION_TO_TYPE.get(extension, AssetType.OTHER)
        action = _EXTENSION_TO_ACTION.get(extension, ConversionAction.COPY)
        target_format = _EXTENSION_TO_TARGET.get(extension)

        # conversion_rulesによる上書き
        override_action = self._get_conversion_rule_override(relative_path)
        if override_action is not None:
            action = override_action
            # SKIPの場合はtarget_formatをNoneに
            if action == ConversionAction.SKIP:
                target_format = None

        return AssetFile(
            path=relative_path,
            asset_type=asset_type,
            action=action,
            source_format=extension,
            target_format=target_format,
        )
