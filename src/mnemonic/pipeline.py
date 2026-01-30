"""ビルドパイプラインの統合インターフェース定義

このモジュールは、MnemonicのビルドパイプラインをオーケストレーションするためのINTERFACEを定義する。
Parser -> Converter -> Builder -> Signer の各コンポーネントを連携させ、
Windows EXE/XP3からAndroid APKを生成するパイプラインを構成する。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Any, Protocol

class PipelinePhase(Enum):
    """パイプラインフェーズ

    ビルドパイプラインの各段階を表す列挙型。
    パイプラインは以下の順序で実行される:
    1. ANALYZE: ゲーム構造解析
    2. EXTRACT: アセット抽出
    3. CONVERT: アセット変換
    4. BUILD: APKビルド
    5. SIGN: APK署名
    """

    ANALYZE = "analyze"
    EXTRACT = "extract"
    CONVERT = "convert"
    BUILD = "build"
    SIGN = "sign"

@dataclass(frozen=True)
class PipelineProgress:
    """パイプライン進捗情報

    パイプラインの実行進捗を表すデータクラス。
    進捗コールバックを通じて、各フェーズの進捗状況を通知するために使用される。

    Attributes:
        phase: 現在実行中のパイプラインフェーズ
        current: 現在の進捗（処理済みアイテム数）
        total: 総数（処理対象アイテム数）
        message: 追加の進捗メッセージ（オプション）
    """

    phase: PipelinePhase
    current: int
    total: int
    message: str = ""

class ProgressCallback(Protocol):
    """進捗コールバックのプロトコル

    パイプライン実行中の進捗通知を受け取るためのコールバックインターフェース。
    UIやログ出力などで進捗を表示する際に使用する。
    """

    def __call__(self, progress: PipelineProgress) -> None:
        """進捗情報を受け取るコールバック

        Args:
            progress: 現在の進捗情報
        """
        ...

@dataclass(frozen=True)
class PipelineConfig:
    """パイプライン設定

    ビルドパイプラインの実行に必要な設定を保持するデータクラス。
    CLIオプションやコンフィグファイルからの設定値をまとめて管理する。

    Attributes:
        input_path: 入力ファイルパス（EXEまたはXP3ファイル）
        output_path: 出力APKファイルパス
        package_name: Androidパッケージ名（空の場合は自動生成）
        app_name: アプリケーション表示名（空の場合は入力ファイル名から生成）
        keystore_path: APK署名用キーストアファイルパス（Noneの場合はデバッグキーを使用）
        skip_video: 動画変換をスキップするか
        quality: 変換品質（"low", "medium", "high"）
        clean_cache: キャッシュをクリアしてから実行するか
        verbose_level: 詳細出力レベル（0=通常, 1=詳細, 2=デバッグ）
        log_file: ログ出力ファイルパス（Noneの場合はファイル出力なし）
        ffmpeg_timeout: FFmpeg処理のタイムアウト秒数
        gradle_timeout: Gradleビルドのタイムアウト秒数
        template_version: 使用するAndroidテンプレートのバージョン（Noneの場合は最新）
        template_refresh_days: テンプレートキャッシュの有効期間（日数）
        template_offline: オフラインモードでテンプレートを使用するか
    """

    input_path: Path
    output_path: Path
    package_name: str = ""
    app_name: str = ""
    keystore_path: Path | None = None
    skip_video: bool = False
    quality: str = "high"
    clean_cache: bool = False
    verbose_level: int = 0
    log_file: Path | None = None
    ffmpeg_timeout: int = 300
    gradle_timeout: int = 1800
    template_version: str | None = None
    template_refresh_days: int = 7
    template_offline: bool = False

@dataclass
class PipelineResult:
    """パイプライン実行結果

    パイプライン実行の結果を保持するデータクラス。
    成功/失敗の状態、出力ファイルパス、統計情報などを含む。

    Attributes:
        success: パイプライン実行が成功したか
        output_path: 生成されたAPKファイルパス（失敗時はNone）
        error_message: エラーメッセージ（成功時は空文字列）
        phases_completed: 完了したフェーズのリスト
        statistics: 実行統計情報（処理時間、ファイル数など）
    """

    success: bool
    output_path: Path | None
    error_message: str = ""
    phases_completed: list[PipelinePhase] = field(default_factory=list)
    statistics: dict[str, Any] = field(default_factory=dict)

class BuildPipeline:
    """ビルドパイプラインオーケストレーター

    Parser -> Converter -> Builder -> Signer の各コンポーネントを連携させ、
    Windows EXE/XP3からAndroid APKを生成するパイプラインを管理する。

    使用例:
        >>> config = PipelineConfig(
        ...     input_path=Path("game.exe"),
        ...     output_path=Path("game.apk"),
        ... )
        >>> pipeline = BuildPipeline(config)
        >>> errors = pipeline.validate()
        >>> if not errors:
        ...     result = pipeline.run()
    """

    def __init__(self, config: PipelineConfig) -> None:
        """パイプラインを初期化する

        Args:
            config: パイプライン設定
        """
        self._config = config

    @property
    def config(self) -> PipelineConfig:
        """パイプライン設定を取得する

        Returns:
            現在のパイプライン設定
        """
        return self._config

    def run(self, progress_callback: ProgressCallback | None = None) -> PipelineResult:
        """パイプラインを実行する

        各フェーズ（解析、抽出、変換、ビルド、署名）を順次実行し、
        最終的にAPKファイルを生成する。

        Args:
            progress_callback: 進捗通知用コールバック（オプション）

        Returns:
            パイプライン実行結果

        Raises:
            ValueError: 設定が無効な場合
        """
        # スタブ実装: I-01-03で実装予定
        raise NotImplementedError("パイプライン実行はI-01-03で実装予定")

    def validate(self) -> list[str]:
        """設定を検証し、エラーメッセージのリストを返す

        入力ファイルの存在確認、出力パスの妥当性、
        オプション値の整合性などを検証する。

        Returns:
            エラーメッセージのリスト（エラーがない場合は空リスト）
        """
        # スタブ実装: I-01-03で実装予定
        raise NotImplementedError("設定検証はI-01-03で実装予定")
