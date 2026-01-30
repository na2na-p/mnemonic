"""ゲーム構成検出モジュール

吉里吉里2などのゲームエンジンの構成を検出し、
ゲーム内のスクリプト、画像、音声などのリソースを特定する。
"""

from dataclasses import dataclass
from enum import Enum
from pathlib import Path

class EngineType(Enum):
    """検出可能なエンジン種別

    サポートされるゲームエンジンの種類を表す列挙型。
    """

    KIRIKIRI2 = "kirikiri2"
    KIRIKIRI2_KAG3 = "kirikiri2_kag3"
    UNKNOWN = "unknown"

@dataclass(frozen=True)
class GameStructure:
    """ゲーム構成情報

    ゲームディレクトリの解析結果を保持する不変データクラス。

    Attributes:
        engine: 検出されたゲームエンジンの種別
        scripts: 検出されたスクリプトファイルのパス一覧
        script_encoding: スクリプトファイルのエンコーディング（検出できない場合はNone）
        images: 検出された画像ファイルのパス一覧
        audio: 検出された音声ファイルのパス一覧
        video: 検出された動画ファイルのパス一覧
        plugins: 検出されたプラグインファイルのパス一覧
    """

    engine: EngineType
    scripts: list[str]
    script_encoding: str | None
    images: list[str]
    audio: list[str]
    video: list[str]
    plugins: list[str]

class GameDetector:
    """ゲーム構成を検出するクラス

    指定されたゲームディレクトリを解析し、
    使用されているエンジンの種類やリソースファイルを検出する。
    """

    def __init__(self, game_dir: Path) -> None:
        """ゲームディレクトリを指定して初期化

        Args:
            game_dir: 解析対象のゲームディレクトリのパス
        """
        raise NotImplementedError()

    def detect(self) -> GameStructure:
        """ゲーム構成を検出して返す

        ゲームディレクトリを走査し、エンジン種別の判定と
        各種リソースファイルの検出を行う。

        Returns:
            検出されたゲーム構成情報

        Raises:
            NotImplementedError: 未実装
        """
        raise NotImplementedError()

    def get_summary(self) -> str:
        """検出結果のサマリー文字列を返す

        CLI表示用に検出結果を人間が読みやすい形式で整形する。

        Returns:
            検出結果のサマリー文字列

        Raises:
            NotImplementedError: 未実装
        """
        raise NotImplementedError()
